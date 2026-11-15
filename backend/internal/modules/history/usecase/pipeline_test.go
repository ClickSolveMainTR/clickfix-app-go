package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"unicode"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/history/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// recorder captures everything that would be sent to a provider.
type recorder struct {
	mu   sync.Mutex
	sent []string
}

func (r *recorder) add(s ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, s...)
}

func (r *recorder) all() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.sent, "\n")
}

// fakeEmbedder is a deterministic bag-of-words embedding: texts that share
// most words are close, unrelated texts are far.
type fakeEmbedder struct{ rec *recorder }

func (e fakeEmbedder) Model() string { return "fake-bow-64" }
func (e fakeEmbedder) Embed(_ context.Context, in []string) (ai.Embeddings, error) {
	e.rec.add(in...)
	out := ai.Embeddings{InputTokens: int64(len(in))}
	for _, s := range in {
		v := make([]float32, 64)
		for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) }) {
			if len([]rune(w)) < 3 {
				continue
			}
			h := fnv.New32a()
			_, _ = h.Write([]byte(w))
			v[h.Sum32()%64]++
		}
		var n float64
		for _, x := range v {
			n += float64(x * x)
		}
		if n == 0 {
			v[0], n = 1, 1
		}
		for i := range v {
			v[i] /= float32(math.Sqrt(n))
		}
		out.Vectors = append(out.Vectors, v)
	}
	return out, nil
}

// fakeDrafter always proposes the same article, so a second cluster with
// the same idea is close to a remembered rejection.
type fakeDrafter struct {
	rec   *recorder
	calls int
}

func (d *fakeDrafter) Draft(_ context.Context, _ int64, _ string, input string) (Draft, error) {
	d.rec.add(input)
	d.calls++
	return Draft{Title: "Fatura PDF indirilemiyorsa", Body: "Tarayıcı önbelleğini temizleyin ve faturayı Faturalar > İndir menüsünden yeniden indirin."}, nil
}

type fixture struct {
	ctx   context.Context
	pool  *pgxpool.Pool
	svc   *Service
	rec   *recorder
	draft *fakeDrafter
	store *storage.Memory
	user  int64
}

func setup(t *testing.T) *fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f := &fixture{ctx: ctx, pool: pool, rec: &recorder{}, store: storage.NewMemory()}
	f.draft = &fakeDrafter{rec: f.rec}
	f.svc = New(pool, db.New(pool), f.store, fakeEmbedder{rec: f.rec}, f.draft)
	if err := pool.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname) VALUES($1,'fake','Geçmiş','Test') RETURNING id", uuid.NewString()+"@example.test").Scan(&f.user); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) org(t *testing.T) int64 {
	t.Helper()
	var id int64
	if err := f.pool.QueryRow(f.ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Geçmiş test') RETURNING id", uuid.NewString()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SaveSettings(f.ctx, id, f.user, model.SettingsInput{EvidenceThreshold: 3, ClusterSimilarity: 0.6, ResolutionSimilarity: 0.6, RejectionSimilarity: 0.85, RawRetentionHours: 24}); err != nil {
		t.Fatal(err)
	}
	return id
}

// bundle builds an export of invoice-download tickets (all data invented).
func bundle(t *testing.T, prefix, category string, n int) []byte {
	t.Helper()
	b := model.Bundle{Schema: model.SchemaVersion, Locale: "tr"}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%s-%d", prefix, i)
		contact := fmt.Sprintf("c-%s", id)
		name := fmt.Sprintf("Sahtekişi%s Uydurmaoğlu", strings.Repeat("a", i+1))
		b.Contacts = append(b.Contacts, model.Contact{ID: contact, Name: name, Email: fmt.Sprintf("kisi%d@example.test", i), Phone: fmt.Sprintf("+90 555 010 %02d %02d", i, i),
			Company: "Uydurma Ticaret Ltd. Şti.", TaxNumber: fmt.Sprintf("99999999%02d", i), Address: "Hayal Mah. Masal Sok. No: 4"})
		b.Tickets = append(b.Tickets, model.Ticket{ID: id, ContactID: contact, Subject: "Fatura PDF indirilemiyor", Status: "solved", Category: category})
		b.Messages = append(b.Messages,
			model.Message{TicketID: id, Author: "customer", Body: fmt.Sprintf("Merhaba ben %s, Uydurma Ticaret Ltd. Şti. faturası PDF indirilemiyor. Telefonum 0555 010 %02d %02d.", name, i, i)},
			model.Message{TicketID: id, Author: "agent", Body: "Tarayıcı önbelleğini temizleyip faturayı Faturalar İndir menüsünden yeniden indirin."})
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func (f *fixture) run(t *testing.T, org int64, data []byte) db.HistoryImport {
	t.Helper()
	run, err := f.svc.Upload(f.ctx, org, f.user, model.UploadInput{ConsentAnalysis: true}, "export.json", data)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Process(f.ctx, org, run.Uuid); err != nil {
		t.Fatal(err)
	}
	out, err := f.svc.Q.HistoryImportGet(f.ctx, db.HistoryImportGetParams{OrganizationID: org, Uuid: run.Uuid})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "completed" {
		t.Fatalf("run %s: %s", out.Status, out.Error)
	}
	// The raw file is gone as soon as the run ends.
	if ok, err := f.store.Exists(f.ctx, run.RawKey); err != nil || ok || out.RawKey != "" || !out.RawDeletedAt.Valid {
		t.Fatalf("raw file kept: exists=%v err=%v run=%+v", ok, err, out)
	}
	return out
}

func (f *fixture) pending(t *testing.T, org int64) []db.HistorySuggestionListRow {
	t.Helper()
	rows, err := f.svc.Q.HistorySuggestionList(f.ctx, db.HistorySuggestionListParams{OrganizationID: org, Limit: 50, Status: pgtype.Text{String: "pending", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestEvidenceThresholdAccumulatesAcrossRuns(t *testing.T) {
	f := setup(t)
	org := f.org(t)
	first := f.run(t, org, bundle(t, "a", "Fatura", 2))
	if first.ProcessedRecords != 2 || first.SuggestionsCreated != 0 || len(f.pending(t, org)) != 0 {
		t.Fatalf("below threshold produced a suggestion: %+v", first)
	}
	// A re-upload of the same tickets is deduplicated, not double evidence.
	again := f.run(t, org, bundle(t, "a", "Fatura", 2))
	if again.ProcessedRecords != 0 || again.SkippedRecords != 2 {
		t.Fatalf("re-upload counted again: %+v", again)
	}
	second := f.run(t, org, bundle(t, "b", "Fatura", 1))
	if second.SuggestionsCreated != 1 {
		t.Fatalf("threshold reached without a suggestion: %+v", second)
	}
	got := f.pending(t, org)
	if len(got) != 1 || got[0].EvidenceCount != 3 || got[0].Source != "history" || got[0].ConversationID.Valid {
		t.Fatalf("suggestion %+v", got)
	}
}

func TestProviderNeverSeesPersonalData(t *testing.T) {
	f := setup(t)
	org := f.org(t)
	f.run(t, org, bundle(t, "p", "Fatura", 3))
	sent := f.rec.all()
	if sent == "" || f.draft.calls != 1 {
		t.Fatalf("nothing reached the fake provider (drafts=%d)", f.draft.calls)
	}
	for _, leak := range []string{"Sahtekişi", "Uydurmaoğlu", "example.test", "555 010", "0555", "Uydurma Ticaret", "99999999", "Hayal Mah", "Masal Sok"} {
		if strings.Contains(sent, leak) {
			t.Fatalf("provider input leaks %q:\n%s", leak, sent)
		}
	}
	// The archive holds the same anonymized text.
	var stored string
	if err := f.pool.QueryRow(f.ctx, "SELECT string_agg(problem||resolution||subject, ' ') FROM history_cases WHERE organization_id=$1", org).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "Uydurmaoğlu") || strings.Contains(stored, "example.test") || !strings.Contains(stored, "[PII_") {
		t.Fatalf("archive not anonymized: %s", stored)
	}
}

func TestRejectionIsRememberedAcrossClusters(t *testing.T) {
	f := setup(t)
	org := f.org(t)
	f.run(t, org, bundle(t, "r", "Fatura", 3))
	s := f.pending(t, org)
	if len(s) != 1 {
		t.Fatalf("pending %d", len(s))
	}
	if _, err := f.svc.Decide(f.ctx, org, f.user, s[0].Uuid, model.DecisionInput{Decision: model.DecisionReject, Reason: "Kısa"}); !errors.Is(err, model.ErrInvalid) {
		t.Fatalf("missing locale or short reason accepted: %v", err)
	}
	res, err := f.svc.Decide(f.ctx, org, f.user, s[0].Uuid, model.DecisionInput{Decision: model.DecisionReject, Reason: "Bu sorun sürüm 4.2 ile kalktı", Locale: "tr"})
	if err != nil || res.Decision.Decision != "rejected" || len(res.Decision.Embedding) == 0 {
		t.Fatalf("%+v %v", res.Decision, err)
	}
	// More cases in the rejected cluster do not bring it back.
	more := f.run(t, org, bundle(t, "r2", "Fatura", 2))
	// A different category is a different cluster; its draft is the same
	// idea, so the decision memory suppresses it.
	other := f.run(t, org, bundle(t, "r3", "Faturalama", 3))
	if more.SuggestionsCreated != 0 || other.SuggestionsCreated != 0 || more.SuggestionsSuppressed+other.SuggestionsSuppressed < 2 {
		t.Fatalf("rejected idea came back: more=%+v other=%+v", more, other)
	}
	if len(f.pending(t, org)) != 0 {
		t.Fatal("pending suggestion after rejection")
	}
}

func TestApproveAndCorrectCreateDraftsAndRememberReason(t *testing.T) {
	f := setup(t)
	org := f.org(t)
	f.run(t, org, bundle(t, "c", "Fatura", 3))
	s := f.pending(t, org)[0]
	res, err := f.svc.Decide(f.ctx, org, f.user, s.Uuid, model.DecisionInput{Decision: model.DecisionCorrect, Reason: "Menü adı değişti", Title: "Fatura indirme", Body: "Ayarlar > Faturalar > İndir.", Locale: "tr"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Suggestion.Status != "approved" || !res.Suggestion.ArticleID.Valid || res.Suggestion.Body != "Ayarlar > Faturalar > İndir." || res.Decision.Decision != "corrected" || res.Decision.Correction == "" {
		t.Fatalf("%+v %+v", res.Suggestion, res.Decision)
	}
	var visibility string
	if err := f.pool.QueryRow(f.ctx, "SELECT visibility FROM knowledge_articles WHERE organization_id=$1 AND id=$2 AND published_version IS NULL", org, res.Suggestion.ArticleID.Int64).Scan(&visibility); err != nil || visibility != "internal" {
		t.Fatalf("draft article %q %v", visibility, err)
	}
	if _, err := f.svc.Decide(f.ctx, org, f.user, s.Uuid, model.DecisionInput{Decision: model.DecisionApprove, Reason: "ikinci kez", Locale: "tr"}); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("second decision: %v", err)
	}
}

func TestTenantIsolation(t *testing.T) {
	f := setup(t)
	a, b := f.org(t), f.org(t)
	f.run(t, a, bundle(t, "t", "Fatura", 3))
	s := f.pending(t, a)[0]
	if _, err := f.svc.Decide(f.ctx, b, f.user, s.Uuid, model.DecisionInput{Decision: model.DecisionReject, Reason: "başka kiracı", Locale: "tr"}); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("org B decided on org A's suggestion: %v", err)
	}
	if _, err := f.svc.Decide(f.ctx, a, f.user, s.Uuid, model.DecisionInput{Decision: model.DecisionReject, Reason: "bizde geçerli değil", Locale: "tr"}); err != nil {
		t.Fatal(err)
	}
	// The same export in org B: its own archive, clusters and suggestion; org
	// A's rejection does not apply there.
	runB := f.run(t, b, bundle(t, "t", "Fatura", 3))
	if runB.ProcessedRecords != 3 || runB.SuggestionsCreated != 1 {
		t.Fatalf("org B run %+v", runB)
	}
	for org, want := range map[int64]int64{a: 3, b: 3} {
		n, err := f.svc.Q.HistoryCaseCount(f.ctx, db.HistoryCaseCountParams{OrganizationID: org})
		if err != nil || n != want {
			t.Fatalf("org %d cases %d %v", org, n, err)
		}
	}
	if _, err := f.svc.Rollback(f.ctx, b, runB.Uuid); err != nil {
		t.Fatal(err)
	}
	if n, _ := f.svc.Q.HistoryCaseCount(f.ctx, db.HistoryCaseCountParams{OrganizationID: a}); n != 3 {
		t.Fatalf("rollback in org B touched org A: %d", n)
	}
}

func TestArchiveIsReadOnlyAndOutsideTheInbox(t *testing.T) {
	f := setup(t)
	org := f.org(t)
	f.run(t, org, bundle(t, "ro", "Fatura", 2))
	if _, err := f.pool.Exec(f.ctx, "UPDATE history_cases SET problem='değişti' WHERE organization_id=$1", org); err == nil || pgCode(err) != "23514" {
		t.Fatalf("archive text changed: %v", err)
	}
	var conversations int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM conversations WHERE organization_id=$1", org).Scan(&conversations); err != nil || conversations != 0 {
		t.Fatalf("history became inbox conversations: %d %v", conversations, err)
	}
}

func TestConsentCancelAndRollback(t *testing.T) {
	f := setup(t)
	org := f.org(t)
	if _, err := f.svc.Upload(f.ctx, org, f.user, model.UploadInput{}, "x.json", bundle(t, "n", "Fatura", 1)); !errors.Is(err, model.ErrInvalid) {
		t.Fatalf("upload without consent: %v", err)
	}
	waiting, err := f.svc.Upload(f.ctx, org, f.user, model.UploadInput{ConsentAnalysis: true, ConsentTraining: true}, "x.json", bundle(t, "w", "Fatura", 3))
	if err != nil || !waiting.ConsentTraining || waiting.ConsentVersion != model.ConsentVersion {
		t.Fatalf("%+v %v", waiting, err)
	}
	cancelled, err := f.svc.Cancel(f.ctx, org, waiting.Uuid)
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatalf("%+v %v", cancelled, err)
	}
	// A cancelled waiting run is never processed; the sweep drops its file.
	if err := f.svc.Process(f.ctx, org, waiting.Uuid); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.svc.Q.HistoryImportGet(f.ctx, db.HistoryImportGetParams{OrganizationID: org, Uuid: waiting.Uuid}); got.ProcessedRecords != 0 || got.RawKey != "" {
		t.Fatalf("cancelled run %+v", got)
	}
	done := f.run(t, org, bundle(t, "rb", "Fatura", 3))
	if len(f.pending(t, org)) != 1 {
		t.Fatal("expected a pending suggestion")
	}
	rolled, err := f.svc.Rollback(f.ctx, org, done.Uuid)
	if err != nil || rolled.Status != "rolled_back" {
		t.Fatalf("%+v %v", rolled, err)
	}
	if n, _ := f.svc.Q.HistoryCaseCount(f.ctx, db.HistoryCaseCountParams{OrganizationID: org}); n != 0 || len(f.pending(t, org)) != 0 {
		t.Fatalf("rollback left cases=%d or suggestions", n)
	}
	if n, _ := f.svc.Q.HistoryClusterCount(f.ctx, org); n != 0 {
		t.Fatalf("rollback left %d clusters", n)
	}
	if _, err := f.svc.Rollback(f.ctx, org, done.Uuid); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("second rollback: %v", err)
	}
}

func TestStaleRunFailsWhenRawRetentionPasses(t *testing.T) {
	f := setup(t)
	org := f.org(t)
	run, err := f.svc.Upload(f.ctx, org, f.user, model.UploadInput{ConsentAnalysis: true}, "x.json", bundle(t, "st", "Fatura", 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, "UPDATE history_imports SET raw_retain_until=now()-interval '1 minute' WHERE organization_id=$1 AND id=$2", org, run.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := f.svc.Q.HistoryImportGet(f.ctx, db.HistoryImportGetParams{OrganizationID: org, Uuid: run.Uuid})
	if ok, _ := f.store.Exists(f.ctx, run.RawKey); got.Status != "failed" || got.RawKey != "" || ok || got.ProcessedRecords != 0 {
		t.Fatalf("stale run %+v exists=%v", got, ok)
	}
}

// History suggestions are reviewed only through the history route, which
// requires a reason and records the decision memory.
func TestLegacyReviewCannotDecideHistorySuggestions(t *testing.T) {
	f := setup(t)
	org := f.org(t)
	f.run(t, org, bundle(t, "lg", "Fatura", 3))
	s := f.pending(t, org)[0]
	_, err := f.svc.Q.AIKnowledgeSuggestionReview(f.ctx, db.AIKnowledgeSuggestionReviewParams{OrganizationID: org, Uuid: s.Uuid, Status: "rejected"})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("legacy review changed a history suggestion: %v", err)
	}
}
