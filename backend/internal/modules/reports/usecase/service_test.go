package usecase

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/reports/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDefinitionAndCalendar(t *testing.T) {
	d := model.Definition{Metrics: []string{"volume"}, Dimensions: []string{"agent"}, ChartType: "table", Timezone: "UTC"}
	if Validate(d) != nil {
		t.Fatal("valid definition")
	}
	d.Metrics = []string{"count(*)"}
	if Validate(d) == nil {
		t.Fatal("SQL accepted")
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ at, frequency, want string }{{"2026-10-31T12:00:00-04:00", "daily", "2026-11-01T00:00:00-04:00"}, {"2026-11-01T12:00:00-05:00", "daily", "2026-11-02T00:00:00-05:00"}, {"2026-11-02T01:00:00-05:00", "weekly", "2026-11-09T00:00:00-05:00"}, {"2026-01-31T12:00:00-05:00", "monthly", "2026-02-01T00:00:00-05:00"}} {
		at, _ := time.Parse(time.RFC3339, tc.at)
		want, _ := time.Parse(time.RFC3339, tc.want)
		if got := Next(at, tc.frequency, loc); !got.Equal(want) {
			t.Fatalf("%s %v != %v", tc.frequency, got, want)
		}
	}
}

type resolver struct{ scope orgctx.Scope }

func (r resolver) Resolve(context.Context, string, int64) (orgctx.Scope, error) { return r.scope, nil }

func TestReportEvaluationAndScheduledArtifact(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	p, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	q := db.New(p)
	var org, other, user, contact, conv int64
	var userUUID uuid.UUID
	for _, dst := range []*int64{&org, &other} {
		if err = p.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Report') RETURNING id", uuid.NewString()).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		_, _ = p.Exec(ctx, "DELETE FROM outbox_events WHERE organization_id=ANY($1::bigint[])", []int64{org, other})
		_, _ = p.Exec(ctx, "DELETE FROM organizations WHERE id=ANY($1::bigint[])", []int64{org, other})
		_, _ = p.Exec(ctx, "DELETE FROM users WHERE id=$1", user)
	}()
	if err = p.QueryRow(ctx, "INSERT INTO users(email,name,surname,password_hash,status) VALUES($1,'Agent','Test','test','active') RETURNING id,uuid", uuid.NewString()+"@example.test").Scan(&user, &userUUID); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(ctx, "INSERT INTO organization_members(organization_id,user_id,role_id) SELECT $1,$2,id FROM roles WHERE scope='tenant' AND organization_id IS NULL AND slug='owner'", org, user); err != nil {
		t.Fatal(err)
	}
	if err = p.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Contact') RETURNING id", org).Scan(&contact); err != nil {
		t.Fatal(err)
	}
	if err = p.QueryRow(ctx, "INSERT INTO conversations(organization_id,number,channel_type,contact_id,assignee_user_id,status,created_at,first_response_at,resolved_at) VALUES($1,1,'email',$2,$3,'resolved',now()-interval '3 hours',now()-interval '2 hours',now()-interval '1 hour') RETURNING id", org, contact, user).Scan(&conv); err != nil {
		t.Fatal(err)
	}
	if err = q.ProjectAnalyticsConversation(ctx, db.ProjectAnalyticsConversationParams{OrganizationID: org, ConversationID: conv}); err != nil {
		t.Fatal(err)
	}
	d := model.Definition{Dimensions: []string{"agent"}, Metrics: model.Metrics, ChartType: "table", Timezone: "UTC"}
	w := model.Evaluation{From: time.Now().Add(-24 * time.Hour), To: time.Now()}
	ds, err := Evaluate(ctx, q, org, d, w)
	if err != nil || len(ds.Rows) != 1 || ds.Rows[0]["volume"] != float64(1) || ds.Rows[0]["first_response_seconds"] != float64(3600) {
		t.Fatalf("metrics: %v %v", ds, err)
	}
	foreign, err := Evaluate(ctx, q, other, d, w)
	if err != nil || len(foreign.Rows) != 0 {
		t.Fatal("foreign data", foreign, err)
	}
	d.Filters.Channel = "whatsapp"
	ds, err = Evaluate(ctx, q, org, d, w)
	if err != nil || len(ds.Rows) != 0 {
		t.Fatal("channel filter", ds, err)
	}
	d.Filters.Channel = ""
	s := &Service{Repo: repository.New(p, q), Resolver: resolver{orgctx.Scope{Permissions: []string{rbac.PermTenantConversationsRead, rbac.PermTenantConversationsReadAll}}}, Secret: strings.Repeat("x", 32), BaseURL: "https://example.test"}
	rep, err := s.Save(ctx, org, user, uuid.Nil, model.Input{Name: "Agents", Definition: d})
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := s.Schedule(ctx, org, user, model.ScheduleInput{ReportUUID: rep.Uuid, Frequency: "daily", Timezone: "UTC", Format: "xlsx", Recipients: []uuid.UUID{userUUID}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(ctx, "UPDATE report_schedules SET next_run_at=now() WHERE organization_id=$1 AND id=$2", org, schedule.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.run(ctx, org, schedule.ID); err != nil {
		t.Fatal(err)
	}
	var delivery uuid.UUID
	if err = p.QueryRow(ctx, "SELECT uuid FROM report_deliveries WHERE organization_id=$1", org).Scan(&delivery); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour).Unix()
	out, err := s.Download(ctx, org, delivery, userUUID, expires, s.signature(org, delivery, userUUID, expires))
	if err != nil || len(out.Content) < 2 || string(out.Content[:2]) != "PK" {
		t.Fatal("xlsx artifact", err)
	}
	if _, err = s.Download(ctx, other, delivery, userUUID, expires, s.signature(org, delivery, userUUID, expires)); err == nil {
		t.Fatal("cross org signed download")
	}
	s.Resolver = resolver{}
	if _, err = s.Download(ctx, org, delivery, userUUID, expires, s.signature(org, delivery, userUUID, expires)); err == nil {
		t.Fatal("revoked recipient download")
	}
	var events int
	if err = p.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE organization_id=$1 AND event_name='reports.ready'", org).Scan(&events); err != nil || events != 1 {
		t.Fatal(events, err)
	}
	if err = s.run(ctx, org, schedule.ID); err == nil {
		t.Fatal("schedule executed before next due")
	}
	for _, format := range []ioengine.ExportFormat{ioengine.ExportCSV, ioengine.ExportXLSX} {
		b, err := ioengine.EncodeExport(format, ds, "en", nil, "Agents")
		if err != nil || len(b) == 0 {
			t.Fatal(format, err)
		}
	}
	_, _ = json.Marshal(ds)
}
