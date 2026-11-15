package usecase

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	aimodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/history/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/google/uuid"
)

// gatedEmbedder answers `allowed` calls, then fails with err as the AI core
// does when the batch share of the day is used up.
type gatedEmbedder struct {
	fakeEmbedder
	allowed atomic.Int32
	err     error
	live    atomic.Int32 // calls that were not marked as batch work
}

func (e *gatedEmbedder) Embed(ctx context.Context, in []string) (ai.Embeddings, error) {
	if ai.LaneOf(ctx) != ai.LaneBatch {
		e.live.Add(1)
	}
	if e.allowed.Add(-1) < 0 {
		return ai.Embeddings{}, e.err
	}
	return e.fakeEmbedder.Embed(ctx, in)
}

// gatedDrafter fails the first draft with err, then drafts.
type gatedDrafter struct {
	fakeDrafter
	fail atomic.Bool
	err  error
}

func (d *gatedDrafter) Draft(ctx context.Context, org int64, locale, input string) (Draft, error) {
	if d.fail.CompareAndSwap(true, false) {
		return Draft{}, d.err
	}
	return d.fakeDrafter.Draft(ctx, org, locale, input)
}

func (f *fixture) get(t *testing.T, org int64, id uuid.UUID) db.HistoryImport {
	t.Helper()
	out, err := f.svc.Q.HistoryImportGet(f.ctx, db.HistoryImportGetParams{OrganizationID: org, Uuid: id})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// due makes a paused run due now (the limit "reset").
func (f *fixture) due(t *testing.T, id uuid.UUID) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, "UPDATE history_imports SET resume_at=now()-interval '1 second' WHERE uuid=$1", id); err != nil {
		t.Fatal(err)
	}
}

// CF-228: the daily limit pauses the run instead of failing it; when the
// limit resets the sweep continues from the batch that hit it.
func TestDailyLimitPausesAndResumes(t *testing.T) {
	f := setup(t)
	org := f.org(t)
	emb := &gatedEmbedder{fakeEmbedder: fakeEmbedder{rec: f.rec}, err: aimodel.ErrDaily}
	emb.allowed.Store(1)
	f.svc.Embedder = emb
	f.svc.BatchSize = 2
	run, err := f.svc.Upload(f.ctx, org, f.user, model.UploadInput{ConsentAnalysis: true}, "export.json", bundle(t, "z", "Fatura", 5))
	if err != nil {
		t.Fatal(err)
	}
	// The pause is computed from this clock (raw retention used the real one).
	now := time.Date(2026, 11, 15, 20, 30, 0, 0, time.UTC) // 23:30 in Istanbul
	f.svc.Now = func() time.Time { return now }
	if err := f.svc.Process(f.ctx, org, run.Uuid); err != nil {
		t.Fatal(err)
	}
	paused := f.get(t, org, run.Uuid)
	want := time.Date(2026, 11, 15, 21, 1, 0, 0, time.UTC) // 00:01 in Istanbul
	if paused.Status != "paused" || paused.ErrorCode != CodeDailyLimit || paused.ResumeOffset != 2 || paused.ProcessedRecords != 2 ||
		!paused.ResumeAt.Valid || !paused.ResumeAt.Time.Equal(want) || paused.Error == "" {
		t.Fatalf("not paused at the second batch: %+v", paused)
	}
	// The raw file stays for the resume; a paused run still counts as active.
	if ok, err := f.store.Exists(f.ctx, paused.RawKey); err != nil || !ok {
		t.Fatalf("raw file gone while paused: %v %v", ok, err)
	}
	if n, err := f.svc.Q.HistoryImportActive(f.ctx, org); err != nil || n != 1 {
		t.Fatalf("active %d %v", n, err)
	}
	// Before the reset the sweep leaves it alone.
	if err := f.svc.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.get(t, org, run.Uuid); got.Status != "paused" {
		t.Fatalf("resumed before the reset: %s", got.Status)
	}
	emb.allowed.Store(100)
	f.due(t, run.Uuid)
	if err := f.svc.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	done := f.get(t, org, run.Uuid)
	if done.Status != "completed" || done.ProcessedRecords != 5 || done.SkippedRecords != 0 || done.ErrorCode != "" || done.Error != "" || done.ResumeAt.Valid {
		t.Fatalf("resume: %+v", done)
	}
	var cases int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM history_cases WHERE organization_id=$1 AND import_id=$2", org, done.ID).Scan(&cases); err != nil || cases != 5 {
		t.Fatalf("archived %d cases (%v)", cases, err)
	}
	if done.SuggestionsCreated != 1 || len(f.pending(t, org)) != 1 {
		t.Fatalf("suggestion after resume: %+v", done)
	}
	if emb.live.Load() != 0 {
		t.Fatalf("%d embedding calls were not marked as batch work", emb.live.Load())
	}
}

// A limit hit while drafting suggestions resumes at that cluster.
func TestBudgetPauseWhileSuggesting(t *testing.T) {
	f := setup(t)
	org := f.org(t)
	d := &gatedDrafter{fakeDrafter: fakeDrafter{rec: f.rec}, err: aimodel.ErrBudget}
	d.fail.Store(true)
	f.svc.Drafter = d
	run, err := f.svc.Upload(f.ctx, org, f.user, model.UploadInput{ConsentAnalysis: true}, "export.json", bundle(t, "s", "Fatura", 3))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Process(f.ctx, org, run.Uuid); err != nil {
		t.Fatal(err)
	}
	paused := f.get(t, org, run.Uuid)
	if paused.Status != "paused" || paused.ErrorCode != CodeBudget || paused.ResumeOffset != 3 || paused.ProcessedRecords != 3 || paused.SuggestionsCreated != 0 {
		t.Fatalf("not paused while suggesting: %+v", paused)
	}
	f.due(t, run.Uuid)
	if err := f.svc.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	done := f.get(t, org, run.Uuid)
	if done.Status != "completed" || done.ProcessedRecords != 3 || done.SuggestionsCreated != 1 || d.calls != 1 {
		t.Fatalf("resume while suggesting: %+v drafts=%d", done, d.calls)
	}
}

// A paused run can be cancelled at once, and a failure keeps its reason.
func TestPausedRunCancelsAndFailureHasCode(t *testing.T) {
	f := setup(t)
	org := f.org(t)
	emb := &gatedEmbedder{fakeEmbedder: fakeEmbedder{rec: f.rec}, err: aimodel.ErrDaily}
	f.svc.Embedder = emb
	run, err := f.svc.Upload(f.ctx, org, f.user, model.UploadInput{ConsentAnalysis: true}, "export.json", bundle(t, "c", "Fatura", 2))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Process(f.ctx, org, run.Uuid); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.Cancel(f.ctx, org, run.Uuid)
	if err != nil || got.Status != "cancelled" || got.ResumeAt.Valid {
		t.Fatalf("cancel paused: %+v %v", got, err)
	}

	emb.err = aimodel.ErrCredits
	run, err = f.svc.Upload(f.ctx, org, f.user, model.UploadInput{ConsentAnalysis: true}, "export.json", bundle(t, "d", "Fatura", 2))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Process(f.ctx, org, run.Uuid); err != nil {
		t.Fatal(err)
	}
	failed := f.get(t, org, run.Uuid)
	if failed.Status != "failed" || failed.ErrorCode != CodeCredits || failed.Error == "" {
		t.Fatalf("failure reason: %+v", failed)
	}
}

func TestFailureCodesAndReset(t *testing.T) {
	cases := map[error]string{
		aimodel.ErrDaily: CodeDailyLimit, aimodel.ErrBudget: CodeBudget, aimodel.ErrConcurrency: CodeConcurrency,
		aimodel.ErrCredits: CodeCredits, aimodel.ErrDisabled: CodeAIDisabled, ai.ErrNotConfigured: CodeNotConfigured,
		ai.ErrUnavailable: CodeProvider, ai.ErrInjection: CodeUnsafe, model.ErrInvalid: CodeInvalid, errors.New("x"): CodeUnknown,
	}
	for err, want := range cases {
		if got := failureCode(errors.Join(errors.New("wrapped"), err)); got != want {
			t.Errorf("%v: %s, want %s", err, got, want)
		}
	}
	// Midnight in Istanbul is 21:00 UTC.
	for in, want := range map[string]string{
		"2026-11-15T20:59:00Z": "2026-11-15T21:00:00Z",
		"2026-11-15T21:00:00Z": "2026-11-16T21:00:00Z",
		"2026-12-31T22:00:00Z": "2027-01-01T21:00:00Z",
	} {
		at, _ := time.Parse(time.RFC3339, in)
		if got := NextLimitReset(at).Format(time.RFC3339); got != want {
			t.Errorf("reset after %s: %s, want %s", in, got, want)
		}
	}
}

func TestEmbedChunksAndDays(t *testing.T) {
	// 200 small texts fit one call; the byte bound splits big ones.
	small := make([]int, 200)
	for i := range small {
		small[i] = 1000
	}
	if got := embedChunks(small, model.MaxEmbedInputs, model.MaxEmbedBytes); len(got) != 1 || got[0] != 200 {
		t.Fatalf("small: %v", got)
	}
	big := []int{150000, 150000, 10, 300000}
	if got := embedChunks(big, model.MaxEmbedInputs, model.MaxEmbedBytes); len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 1 {
		t.Fatalf("big: %v", got)
	}
	n := func(v int64) *int64 { return &v }
	if days(56, nil, nil) != 1 || days(40, n(50), n(50)) != 1 || days(56, n(50), n(50)) != 2 || days(56, n(50), n(0)) != 3 || days(5, n(0), n(0)) != 0 {
		t.Fatal("days")
	}
}
