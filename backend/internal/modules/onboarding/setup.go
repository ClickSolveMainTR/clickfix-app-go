package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	aimodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
)

// UndoWindow is how long "geri al" stays available after an apply.
const UndoWindow = 24 * time.Hour

// MaxAIDiscoveriesPerDay caps AI discoveries (and website fetches) of one
// organization; the onboarding allowance covers two or three of them.
const MaxAIDiscoveriesPerDay = 5

// EventSetupProgress is the Centrifugo event on the organization channel.
const EventSetupProgress = "onboarding.setup.progress"

// Errors of the set-up runs.
var (
	ErrRunNotFound   = errors.New("setup run not found")
	ErrRunNotDraft   = errors.New("setup run was already applied")
	ErrRunActive     = errors.New("another setup run is being applied")
	ErrUndoExpired   = errors.New("setup run can no longer be undone")
	ErrDiscoverLimit = errors.New("too many discoveries today")
	ErrEmptySelect   = errors.New("select at least one item")
)

// SetupService runs CF-140: discovery, template gallery, preview, apply
// and undo.
type SetupService struct {
	Q         *db.Queries
	Applier   *Applier
	Templates []Template
	Fetcher   Fetcher
	AI        AIRunner
	Pub       realtime.Publisher
	Log       *slog.Logger
	Now       func() time.Time
}

func (s *SetupService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *SetupService) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

// RunItem is one row of a run: preview status for drafts, ledger status
// once applied.
type RunItem struct {
	Item
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
	Selected bool   `json:"selected"`
	Undone   bool   `json:"undone"`
}

// Discovery is what the run remembers of the discovery step.
type Discovery struct {
	Mode    string       `json:"mode"`
	Notice  string       `json:"notice,omitempty"`
	Answers *Answers     `json:"answers,omitempty"`
	Site    *SiteSummary `json:"site,omitempty"`
}

// RunView is the API shape of a run.
type RunView struct {
	UUID            uuid.UUID  `json:"uuid"`
	Source          string     `json:"source"`
	TemplateKey     string     `json:"template_key"`
	TemplateVersion int        `json:"template_version"`
	Status          string     `json:"status"`
	Total           int        `json:"total"`
	Done            int        `json:"done"`
	Failed          int        `json:"failed"`
	Error           string     `json:"error"`
	Discovery       Discovery  `json:"discovery"`
	Plan            SetupPlan  `json:"plan"`
	Items           []RunItem  `json:"items"`
	CreatedAt       time.Time  `json:"created_at"`
	StartedAt       *time.Time `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at"`
	UndoneAt        *time.Time `json:"undone_at"`
	UndoUntil       *time.Time `json:"undo_until"`
	CanUndo         bool       `json:"can_undo"`
}

// TemplateList is the gallery.
func (s *SetupService) TemplateList() []TemplateSummary {
	out := make([]TemplateSummary, 0, len(s.Templates))
	for _, t := range s.Templates {
		out = append(out, t.Summary())
	}
	return out
}

// FromTemplate opens a draft run from a gallery template.
func (s *SetupService) FromTemplate(ctx context.Context, org, actor int64, key, companyName string) (RunView, error) {
	idx := slices.IndexFunc(s.Templates, func(t Template) bool { return t.Key == key })
	if idx < 0 {
		return RunView{}, ErrUnknownTemplate
	}
	tpl := s.Templates[idx]
	plan := clonePlan(tpl.Plan)
	// Template content is written in the template's language (Turkish);
	// the plan keeps that locale so articles and procedures are filed
	// under the language they are written in.
	personalize(&plan, "", Answers{CompanyName: companyName}, nil)
	if err := plan.Normalize(); err != nil {
		return RunView{}, err
	}
	return s.createRun(ctx, org, actor, "template", tpl.Key, tpl.Version, Discovery{Mode: "template"}, plan, uuid.Nil)
}

// DiscoverInput is the discovery request.
type DiscoverInput struct {
	Website string
	Answers Answers
	Locale  string
}

// Discover reads the website (optional), asks the AI for a plan and opens
// a draft run. Without AI (not configured, not in the plan, out of
// credits) the closest template personalised with the answers is used and
// the run says why (discovery.notice).
func (s *SetupService) Discover(ctx context.Context, org, actor int64, in DiscoverInput) (RunView, error) {
	locale := oneOf(in.Locale, planLocales, "tr")
	in.Answers.normalize()
	count, err := s.Q.OnboardingCountAIRunsSince(ctx, db.OnboardingCountAIRunsSinceParams{
		OrganizationID: org, Since: pgtype.Timestamptz{Time: s.now().Add(-24 * time.Hour), Valid: true},
	})
	if err != nil {
		return RunView{}, err
	}
	if count >= MaxAIDiscoveriesPerDay {
		return RunView{}, ErrDiscoverLimit
	}
	disc := Discovery{Mode: "ai", Answers: &in.Answers}
	if in.Website != "" {
		if s.Fetcher == nil {
			return RunView{}, ErrWebsite
		}
		site, err := s.Fetcher.Summarize(ctx, in.Website)
		if err != nil {
			// The site is optional: carry on with the answers only.
			disc.Notice = "website_unreachable"
		} else {
			disc.Site = &site
		}
	}
	if err := s.Q.OnboardingEnsureAIPolicy(ctx, org); err != nil {
		return RunView{}, err
	}
	plan, aiRun, aerr := aiPlan(ctx, s.AI, org, actor, locale, in.Answers, disc.Site)
	if aerr == nil {
		personalize(&plan, locale, Answers{CompanyName: in.Answers.CompanyName, Languages: in.Answers.Languages}, nil)
		if disc.Site != nil {
			plan.Summary.Website = disc.Site.URL
		}
		aerr = plan.Normalize()
	}
	if aerr != nil {
		s.log().Info("onboarding_ai_fallback", "organization_id", org, "error", aerr)
		var tpl Template
		plan, tpl = fallbackPlan(s.Templates, in.Answers, disc.Site)
		if err := plan.Normalize(); err != nil {
			return RunView{}, err
		}
		disc.Mode = "fallback"
		disc.Notice = aiNotice(aerr)
		return s.createRun(ctx, org, actor, "ai", tpl.Key, tpl.Version, disc, plan, aiRun)
	}
	return s.createRun(ctx, org, actor, "ai", "", 0, disc, plan, aiRun)
}

func (s *SetupService) createRun(ctx context.Context, org, actor int64, source, key string, version int, disc Discovery, plan SetupPlan, aiRun uuid.UUID) (RunView, error) {
	ai := pgtype.UUID{Bytes: aiRun, Valid: aiRun != uuid.Nil}
	row, err := s.Q.OnboardingCreateRun(ctx, db.OnboardingCreateRunParams{
		OrganizationID: org, Source: source, TemplateKey: key, TemplateVersion: safeconv.ToInt32(version),
		Discovery: mustJSON(disc), Plan: mustJSON(plan), AiRunUuid: ai, CreatedBy: i8(actor),
	})
	if err != nil {
		return RunView{}, err
	}
	return s.view(ctx, row)
}

// Run returns one run (drafts carry a fresh preview).
func (s *SetupService) Run(ctx context.Context, org int64, id uuid.UUID) (RunView, error) {
	_ = s.Q.OnboardingFailStaleRuns(ctx, org)
	row, err := s.Q.OnboardingGetRun(ctx, db.OnboardingGetRunParams{OrganizationID: org, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return RunView{}, ErrRunNotFound
	}
	if err != nil {
		return RunView{}, err
	}
	return s.view(ctx, row)
}

// Latest returns the newest run of the organization, or nil.
func (s *SetupService) Latest(ctx context.Context, org int64) (*RunView, error) {
	_ = s.Q.OnboardingFailStaleRuns(ctx, org)
	row, err := s.Q.OnboardingLatestRun(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v, err := s.view(ctx, row)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *SetupService) view(ctx context.Context, row db.OnboardingSetupRun) (RunView, error) {
	var plan SetupPlan
	if err := json.Unmarshal(row.Plan, &plan); err != nil {
		return RunView{}, fmt.Errorf("onboarding: stored plan: %w", err)
	}
	var disc Discovery
	_ = json.Unmarshal(row.Discovery, &disc)
	v := RunView{
		UUID: row.Uuid, Source: row.Source, TemplateKey: row.TemplateKey, TemplateVersion: int(row.TemplateVersion),
		Status: row.Status, Total: int(row.Total), Done: int(row.Done), Failed: int(row.Failed), Error: row.Error,
		Discovery: disc, Plan: plan, Items: []RunItem{}, CreatedAt: row.CreatedAt.Time,
		StartedAt: tsPtr(row.StartedAt), FinishedAt: tsPtr(row.FinishedAt), UndoneAt: tsPtr(row.UndoneAt),
	}
	if v.FinishedAt != nil && row.Status != "undone" {
		until := v.FinishedAt.Add(UndoWindow)
		v.UndoUntil = &until
		v.CanUndo = s.now().Before(until) && slices.Contains([]string{"succeeded", "partial", "failed"}, row.Status)
	}
	if row.Status == "draft" {
		preview, err := s.Applier.Preview(ctx, row.OrganizationID, &plan)
		if err != nil {
			return RunView{}, err
		}
		for _, p := range preview {
			v.Items = append(v.Items, RunItem{Item: p.Item, Status: p.Status, Reason: p.Reason, Selected: p.Status == PreviewNew})
		}
		return v, nil
	}
	ledger, err := s.Q.OnboardingRunItems(ctx, db.OnboardingRunItemsParams{OrganizationID: row.OrganizationID, RunID: row.ID})
	if err != nil {
		return RunView{}, err
	}
	byKey := map[string]db.OnboardingSetupItem{}
	for _, l := range ledger {
		byKey[l.ItemKey] = l
	}
	for _, it := range plan.Items() {
		ri := RunItem{Item: it, Status: "pending", Selected: slices.Contains(row.Selection, it.Key)}
		if l, ok := byKey[it.Key]; ok {
			ri.Status, ri.Reason, ri.Undone = l.Status, l.Error, l.UndoneAt.Valid
		} else if !ri.Selected {
			ri.Status = "excluded"
		}
		v.Items = append(v.Items, ri)
	}
	return v, nil
}

// Apply queues the run with the owner's selection.
func (s *SetupService) Apply(ctx context.Context, org int64, id uuid.UUID, selection []string) (RunView, error) {
	row, err := s.Q.OnboardingGetRun(ctx, db.OnboardingGetRunParams{OrganizationID: org, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return RunView{}, ErrRunNotFound
	}
	if err != nil {
		return RunView{}, err
	}
	if row.Status != "draft" {
		return RunView{}, ErrRunNotDraft
	}
	var plan SetupPlan
	if err := json.Unmarshal(row.Plan, &plan); err != nil {
		return RunView{}, err
	}
	keys := map[string]bool{}
	for _, it := range plan.Items() {
		keys[it.Key] = true
	}
	picked := []string{}
	for _, k := range selection {
		if keys[k] && !slices.Contains(picked, k) {
			picked = append(picked, k)
		}
	}
	if len(picked) == 0 {
		return RunView{}, ErrEmptySelect
	}
	_ = s.Q.OnboardingFailStaleRuns(ctx, org)
	queued, err := s.Q.OnboardingQueueRun(ctx, db.OnboardingQueueRunParams{
		OrganizationID: org, ID: row.ID, Selection: picked, Total: safeconv.ToInt32(len(picked)),
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return RunView{}, ErrRunActive
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return RunView{}, ErrRunNotDraft
	}
	if err != nil {
		return RunView{}, err
	}
	s.dispatch(ctx, org, queued.ID)
	return s.view(ctx, queued)
}

// dispatch runs the apply job detached from the request. It takes seconds
// (a few dozen inserts); its state lives on the run row, so a restart
// mid-way is closed as "interrupted" by OnboardingFailStaleRuns and the
// owner re-applies (items already created are skipped as existing).
func (s *SetupService) dispatch(ctx context.Context, org, runID int64) {
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Minute)
		defer cancel()
		if err := s.Execute(ctx, org, runID); err != nil {
			s.log().Error("onboarding_apply_failed", "organization_id", org, "run_id", runID, "error", err)
		}
	}()
}

// Execute is the apply job: every selected item in plan order, each
// recorded in the ledger and announced on the organization channel.
func (s *SetupService) Execute(ctx context.Context, org, runID int64) error {
	n, err := s.Q.OnboardingStartRun(ctx, db.OnboardingStartRunParams{OrganizationID: org, ID: runID})
	if err != nil {
		return err
	}
	if n == 0 {
		return nil // already started (retry) or not queued
	}
	row, err := s.Q.OnboardingGetRunByID(ctx, db.OnboardingGetRunByIDParams{OrganizationID: org, ID: runID})
	if err != nil {
		return err
	}
	orgRow, err := s.Q.GetOrganizationByID(ctx, org)
	if err != nil {
		return err
	}
	var plan SetupPlan
	if err := json.Unmarshal(row.Plan, &plan); err != nil {
		return s.finish(ctx, org, row, orgRow.Uuid, "failed", "invalid plan")
	}
	actor := row.CreatedBy.Int64
	st := &applyState{org: org, actor: actor, plan: &plan, teams: map[string]int64{}, fields: map[string]uuid.UUID{},
		fieldTypes: map[string]string{}, cols: map[string]uuid.UUID{}}
	selected := map[string]bool{}
	for _, k := range row.Selection {
		selected[k] = true
	}
	done, failed := 0, 0
	s.publish(ctx, orgRow.Uuid, row, "running", nil, done, failed)
	for _, it := range plan.Items() {
		if !selected[it.Key] {
			// Unticked items that already exist still resolve references
			// (a routing rule to a team the organization had before).
			s.Applier.remember(ctx, st, it)
			continue
		}
		res, aerr := s.Applier.apply(ctx, st, it)
		if aerr != nil {
			res = result{status: ItemFailed, reason: clip(aerr.Error(), 300)}
			s.log().Warn("onboarding_item_failed", "organization_id", org, "item", it.Key, "error", aerr)
		}
		if res.status == ItemFailed {
			failed++
		}
		done++
		params := db.OnboardingUpsertItemParams{
			OrganizationID: org, RunID: row.ID, ItemKey: it.Key, Kind: it.Kind, Label: clip(it.Label, 200),
			Status: res.status, Error: res.reason, Previous: res.previous,
		}
		if res.id != 0 {
			params.EntityID = i8(res.id)
		}
		if res.uuid != uuid.Nil {
			params.EntityUuid = pgtype.UUID{Bytes: res.uuid, Valid: true}
		}
		if _, err := s.Q.OnboardingUpsertItem(ctx, params); err != nil {
			return err
		}
		if err := s.Q.OnboardingProgressRun(ctx, db.OnboardingProgressRunParams{
			OrganizationID: org, ID: row.ID, Done: safeconv.ToInt32(done), Failed: safeconv.ToInt32(failed),
		}); err != nil {
			return err
		}
		s.publish(ctx, orgRow.Uuid, row, "running", &RunItem{Item: it, Status: res.status, Reason: res.reason, Selected: true}, done, failed)
	}
	status := "succeeded"
	switch {
	case failed > 0 && failed == done:
		status = "failed"
	case failed > 0:
		status = "partial"
	}
	return s.finish(ctx, org, row, orgRow.Uuid, status, "")
}

func (s *SetupService) finish(ctx context.Context, org int64, row db.OnboardingSetupRun, orgUUID uuid.UUID, status, msg string) error {
	if err := s.Q.OnboardingFinishRun(ctx, db.OnboardingFinishRunParams{OrganizationID: org, ID: row.ID, Status: status, Error: msg}); err != nil {
		return err
	}
	fresh, err := s.Q.OnboardingGetRunByID(ctx, db.OnboardingGetRunByIDParams{OrganizationID: org, ID: row.ID})
	if err != nil {
		return err
	}
	s.publish(ctx, orgUUID, fresh, status, nil, int(fresh.Done), int(fresh.Failed))
	return nil
}

func (s *SetupService) publish(ctx context.Context, orgUUID uuid.UUID, row db.OnboardingSetupRun, status string, item *RunItem, done, failed int) {
	if s.Pub == nil {
		return
	}
	data := map[string]any{
		"run_uuid": row.Uuid.String(), "status": status, "done": done, "failed": failed, "total": int(row.Total),
	}
	if item != nil {
		data["item"] = item
	}
	if err := s.Pub.Publish(ctx, realtime.OrgChannel(orgUUID), realtime.Message{Type: EventSetupProgress, Data: data}); err != nil {
		s.log().Warn("onboarding_publish_failed", "error", err)
	}
}

// Undo removes what the run created (reverse order) and restores the
// settings it changed, within UndoWindow.
func (s *SetupService) Undo(ctx context.Context, org, actor int64, id uuid.UUID) (RunView, error) {
	row, err := s.Q.OnboardingGetRun(ctx, db.OnboardingGetRunParams{OrganizationID: org, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return RunView{}, ErrRunNotFound
	}
	if err != nil {
		return RunView{}, err
	}
	if !row.FinishedAt.Valid || row.Status == "undone" || s.now().After(row.FinishedAt.Time.Add(UndoWindow)) {
		return RunView{}, ErrUndoExpired
	}
	items, err := s.Q.OnboardingRunItems(ctx, db.OnboardingRunItemsParams{OrganizationID: org, RunID: row.ID})
	if err != nil {
		return RunView{}, err
	}
	for i := len(items) - 1; i >= 0; i-- {
		it := items[i]
		if it.UndoneAt.Valid || (it.Status != ItemCreated && it.Status != ItemUpdated) {
			continue
		}
		msg := ""
		if err := s.Applier.undo(ctx, org, actor, it); err != nil {
			msg = clip(err.Error(), 300)
			s.log().Info("onboarding_undo_kept", "organization_id", org, "item", it.ItemKey, "error", err)
		}
		if err := s.Q.OnboardingMarkItemUndone(ctx, db.OnboardingMarkItemUndoneParams{OrganizationID: org, ID: it.ID, Error: msg}); err != nil {
			return RunView{}, err
		}
	}
	if _, err := s.Q.OnboardingMarkRunUndone(ctx, db.OnboardingMarkRunUndoneParams{OrganizationID: org, ID: row.ID}); err != nil {
		return RunView{}, err
	}
	return s.Run(ctx, org, id)
}

// aiNotice tells the owner why the plan came from a template instead.
func aiNotice(err error) string {
	switch {
	case errors.Is(err, aimodel.ErrCredits):
		return "ai_credits"
	case errors.Is(err, aimodel.ErrDisabled), errors.Is(err, entitlement.ErrDenied):
		return "ai_not_allowed"
	case errors.Is(err, aimodel.ErrBudget), errors.Is(err, aimodel.ErrDaily), errors.Is(err, aimodel.ErrConcurrency):
		return "ai_limit"
	}
	return "ai_unavailable"
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
