package supportoperations_test

import (
	"encoding/json"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/coaching"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/quality"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/workforce"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/i18n"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine"
	"github.com/google/uuid"
	"math"
	"testing"
	"time"
)

func TestCalibrationBlindSnapshotVarianceAndCoachingIsolation(t *testing.T) {
	f := setup(t)
	s := &coaching.Service{Repo: f.repo}
	q := &quality.Service{Repo: f.repo}
	t.Cleanup(func() {
		f.exec(t, "DELETE FROM qa_calibrations WHERE organization_id=$1", f.org)
		f.exec(t, "DELETE FROM coaching_items WHERE organization_id=$1", f.org)
	})
	card, err := q.Card(f.ctx, f.org, quality.CardInput{Name: "QA", Threshold: 70, Criteria: []quality.Criterion{{Key: "accuracy", Label: "Accuracy", Weight: 100}}})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Session(f.ctx, f.org, f.actor, coaching.SessionInput{Name: "Calibrate", Conversation: f.conv, Scorecard: card.Uuid})
	if err != nil {
		t.Fatal(err)
	}
	id := v.(map[string]any)["uuid"].(uuid.UUID)
	if err = s.Close(f.ctx, f.org, f.actor, id); err == nil {
		t.Fatal("closed without two reviewers")
	}
	if _, err = s.Report(f.ctx, f.other, f.actor, id); err == nil {
		t.Fatal("cross org session")
	}
	score := []quality.Score{{Key: "accuracy", Value: 80, Rationale: "Good"}}
	if _, err = s.Score(f.ctx, f.org, f.actor, id, score); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Score(f.ctx, f.org, f.actor, id, score); err == nil {
		t.Fatal("duplicate reviewer")
	}
	v, err = s.Report(f.ctx, f.org, f.actor, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := v.(map[string]any)["reviews"]; ok {
		t.Fatal("blind scores leaked")
	}
	// Editing the parent scorecard cannot change the session's rubric.
	f.exec(t, `UPDATE qa_scorecards SET criteria='[{"key":"other","label":"Other","weight":100}]' WHERE organization_id=$1 AND id=$2`, f.org, card.ID)
	score[0].Value = 100
	if _, err = s.Score(f.ctx, f.org, f.peer, id, score); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(f.ctx, f.org, f.actor, id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Score(f.ctx, f.org, f.outsider, id, score); err == nil {
		t.Fatal("closed score accepted")
	}
	v, err = s.Report(f.ctx, f.org, f.actor, id)
	if err != nil {
		t.Fatal(err)
	}
	summary := v.(map[string]any)["summary"].(coaching.Variance)
	if summary.Mean != 90 || summary.Variance != 100 {
		t.Fatal(summary)
	}
	goal, err := s.Item(f.ctx, f.org, f.actor, coaching.ItemInput{Agent: f.user, Kind: "goal", Body: "Improve clarity"})
	if err != nil {
		t.Fatal(err)
	}
	gid := goal.(db.CoachingCreateRow).Uuid
	due := time.Now().Add(24 * time.Hour)
	if _, err = s.Item(f.ctx, f.org, f.actor, coaching.ItemInput{Agent: f.peerUUID, Kind: "task", Body: "Follow up", Parent: &gid, DueAt: &due}); err == nil {
		t.Fatal("cross agent goal")
	}
	if _, err = s.Item(f.ctx, f.org, f.actor, coaching.ItemInput{Agent: f.user, Kind: "task", Body: "Follow up", Parent: &gid, DueAt: &due}); err != nil {
		t.Fatal(err)
	}
}
func TestWorkforceActivityCoverageExportAndOptOut(t *testing.T) {
	f := setup(t)
	s := &workforce.Service{Repo: f.repo}
	c := &coaching.Service{Repo: f.repo}
	t.Cleanup(func() { f.exec(t, "DELETE FROM workforce_events WHERE organization_id=$1", f.org) })
	client := uuid.New()
	in := workforce.ActivityInput{ClientUUID: client, State: "handling", Conversation: &f.conv}
	first, err := s.Activity(f.ctx, f.org, f.actor, in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Activity(f.ctx, f.org, f.actor, in)
	if err != nil || first.Uuid != again.Uuid {
		t.Fatalf("replay %v", err)
	}
	var eventCount int
	if err = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM outbox_events WHERE organization_id=$1 AND event_name='workforce.activity'", f.org).Scan(&eventCount); err != nil || eventCount != 1 {
		t.Fatalf("outbox replay count=%d: %v", eventCount, err)
	}
	if _, err = s.Activity(f.ctx, f.other, f.actor, in); err == nil {
		t.Fatal("cross org conversation")
	}
	now := time.Now().UTC().Truncate(time.Second)
	since := now.Add(-time.Hour)
	f.exec(t, "UPDATE workforce_events SET occurred_at=$2 WHERE organization_id=$1 AND uuid=$3", f.org, since, first.Uuid)
	rows, err := s.Timesheets(f.ctx, f.org, since, now, nil)
	if err != nil || len(rows) != 1 || math.Abs(rows[0].Seconds-300) > 0.01 {
		t.Fatalf("heartbeat cap %+v %v", rows, err)
	}
	other, err := s.Timesheets(f.ctx, f.other, since, now, nil)
	if err != nil || len(other) != 0 {
		t.Fatal("timesheet isolation", err)
	}
	ds, err := (&workforce.TimesheetAdapter{Service: s}).Export(f.ctx, ioengine.ExportQuery{"since": since.Format(time.RFC3339), "until": now.Format(time.RFC3339)}, i18n.LocaleEN)
	if err != nil || len(ds.Rows) != 1 {
		t.Fatal(err)
	}
	data, err := ioengine.EncodeExport(ioengine.ExportCSV, ds, "en", nil, "Timesheets")
	if err != nil || len(data) == 0 {
		t.Fatal(err)
	}
	start := now.Add(time.Hour).Truncate(time.Hour)
	end := start.Add(time.Hour)
	f.exec(t, "INSERT INTO agent_shifts(organization_id,user_id,starts_at,ends_at) VALUES($1,$2,$3,$4)", f.org, f.actor, start, end)
	cov, err := f.repo.Q.WorkforceCoverage(f.ctx, db.WorkforceCoverageParams{OrganizationID: f.org, Since: repository.TS(start), Until: repository.TS(end)})
	if err != nil || len(cov) != 1 || cov[0].Agents != 1 {
		t.Fatal(cov, err)
	}
	f.exec(t, "INSERT INTO agent_time_off(organization_id,user_id,starts_at,ends_at,status) VALUES($1,$2,$3,$4,'approved')", f.org, f.actor, start.Add(20*time.Minute), end)
	cov, err = f.repo.Q.WorkforceCoverage(f.ctx, db.WorkforceCoverageParams{OrganizationID: f.org, Since: repository.TS(start), Until: repository.TS(end)})
	if err != nil || cov[0].Agents != 0 {
		t.Fatal(cov, err)
	}
	v, err := s.Forecast(f.ctx, f.org, workforce.ForecastInput{Since: start, Until: end})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = json.Marshal(v); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Rule(f.ctx, f.org, coaching.RuleInput{Name: "Resolved", Metric: "resolutions", Threshold: 1, Points: 10, MinSamples: 1, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	f.exec(t, "UPDATE conversations SET resolved_at=$2 WHERE organization_id=$1 AND uuid=$3", f.org, now.Add(-time.Minute), f.conv)
	board, err := c.Leaderboard(f.ctx, f.org, since, now, nil)
	if err != nil || len(board) != 3 || board[0].Agent != f.user || board[0].Points != 10 {
		t.Fatal(board, err)
	}
	if err = f.repo.Q.GameOptOut(f.ctx, db.GameOptOutParams{OrganizationID: f.org, UserID: f.actor, OptedOut: true}); err != nil {
		t.Fatal(err)
	}
	board, err = c.Leaderboard(f.ctx, f.org, since, now, nil)
	if err != nil || len(board) != 2 {
		t.Fatal(board, err)
	}
	for _, r := range board {
		if r.Agent == f.user {
			t.Fatal("opt-out leaked")
		}
	}
}
