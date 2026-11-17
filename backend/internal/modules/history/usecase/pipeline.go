package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	aimodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/history/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/observability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// errCancelled stops a run between batches.
var errCancelled = errors.New("history: run cancelled")

// counts are the run's counters; a paused run saves them and continues.
type counts struct {
	processed, skipped                int32
	clusters, suggestions, suppressed int32
}

type progress struct {
	run   db.HistoryImport
	stage string
	total int32
	counts
	// offset is where a paused run continues: an index into the records,
	// then len(records)+i for the i-th cluster to evaluate (CF-228).
	offset int32
	// watermark is where the next nightly run starts; set when the window
	// held more conversations than one run reads.
	watermark pgtype.Timestamptz
}

func (s *Service) report(ctx context.Context, p *progress, stage string) error {
	p.stage = stage
	cancel, err := s.Q.HistoryImportProgress(ctx, db.HistoryImportProgressParams{OrganizationID: p.run.OrganizationID, ID: p.run.ID, Stage: stage,
		TotalRecords: p.total, ProcessedRecords: p.processed, SkippedRecords: p.skipped, ClustersTouched: p.clusters,
		SuggestionsCreated: p.suggestions, SuggestionsSuppressed: p.suppressed})
	if err != nil {
		return err
	}
	if cancel {
		return errCancelled
	}
	return nil
}

func (s *Service) source(run db.HistoryImport) (Source, error) {
	switch run.Source {
	case "upload":
		return FileSource{Blobs: s.Store}, nil
	case "nightly":
		return ConversationSource{Q: s.Q}, nil
	case "connector":
		if src, ok := s.Connectors[run.Connector]; ok {
			return src, nil
		}
	}
	return nil, fmt.Errorf("%w: no source for %s/%s", model.ErrUnavailable, run.Source, run.Connector)
}

// Process runs one claimed run end to end. A run is resumable: cases already
// archived (same external_ref) are skipped, so a worker restart costs only
// the batch in flight.
func (s *Service) Process(ctx context.Context, org int64, id uuid.UUID) error {
	run, err := s.Q.HistoryImportClaim(ctx, db.HistoryImportClaimParams{OrganizationID: org, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// CF-228: the pipeline is batch work; its AI calls use only the batch
	// share of the daily limits, so live AI keeps working.
	ctx = ai.WithBatch(orgctx.WithOrganizationID(ctx, org))
	// A run that fails before its first stage report keeps its stored stage
	// ("" would violate history_imports_stage_check).
	p := &progress{run: run, stage: run.Stage}
	runErr := s.process(ctx, p)
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if resume, ok := s.resumeAt(runErr); ok && !errors.Is(runErr, errCancelled) {
		// A limit is not a failure: keep the raw file and continue later
		// from the batch that hit it.
		code := failureCode(runErr)
		s.Log.InfoContext(ctx, "history_run_paused", "organization_id", org, "run_uuid", id, "code", code, "resume_at", resume, "offset", p.offset)
		return s.Q.HistoryImportPause(finishCtx, db.HistoryImportPauseParams{OrganizationID: org, ID: run.ID, Stage: p.stage, Error: truncate(publicError(runErr), 500),
			ErrorCode: code, ResumeAt: pgtype.Timestamptz{Time: resume, Valid: true}, ResumeOffset: p.offset, ProcessedRecords: p.processed,
			SkippedRecords: p.skipped, ClustersTouched: p.clusters, SuggestionsCreated: p.suggestions, SuggestionsSuppressed: p.suppressed})
	}
	status, stage, message, code := "completed", "done", "", ""
	switch {
	case errors.Is(runErr, errCancelled):
		status, stage = "cancelled", p.stage
	case runErr != nil:
		code = failureCode(runErr)
		status, stage, message = "failed", p.stage, truncate(publicError(runErr), 500)
		s.Log.WarnContext(ctx, "history_run_failed", "organization_id", org, "run_uuid", id, "code", code, "error", runErr)
		// The worker log alone hid the reason of a production failure.
		observability.CaptureError(ctx, runErr, observability.Options{Level: sentry.LevelWarning,
			Tags:        map[string]string{"module": "history", "history_error_code": code, "history_stage": p.stage},
			Fingerprint: []string{"history_run_failed", code}, Transaction: "history.Process"})
	}
	if status == "completed" && run.Source == "nightly" && run.WindowEnd.Valid {
		mark := run.WindowEnd
		if p.watermark.Valid {
			mark = p.watermark
		}
		if err := s.Q.HistorySettingsWatermark(finishCtx, db.HistorySettingsWatermarkParams{OrganizationID: org, NightlyWatermark: mark}); err != nil {
			return err
		}
	}
	if err := s.Q.HistoryImportFinish(finishCtx, db.HistoryImportFinishParams{OrganizationID: org, ID: run.ID, Status: status, Stage: stage, Error: message, ErrorCode: code}); err != nil {
		return err
	}
	// The raw file goes as soon as the run ends, however it ended.
	s.deleteRaw(finishCtx, run)
	return nil
}

// Reasons a run paused or failed (history_imports.error_code, CF-228). The
// panel shows them in the user's language; error keeps an English message.
const (
	CodeDailyLimit    = "daily_limit"
	CodeBudget        = "budget"
	CodeConcurrency   = "concurrency"
	CodeCredits       = "credits"
	CodeQuota         = "quota"
	CodeAIDisabled    = "ai_disabled"
	CodeNotConfigured = "not_configured"
	CodeProvider      = "provider"
	CodeInvalid       = "invalid"
	CodeUnavailable   = "unavailable"
	CodeUnsafe        = "unsafe_record"
	CodeUnknown       = "unknown"
)

// failureCode classifies why a run stopped.
func failureCode(err error) string {
	var ent *entitlement.Error
	switch {
	case errors.Is(err, aimodel.ErrDaily):
		return CodeDailyLimit
	case errors.Is(err, aimodel.ErrBudget):
		return CodeBudget
	case errors.Is(err, aimodel.ErrConcurrency):
		return CodeConcurrency
	case errors.Is(err, aimodel.ErrCredits):
		return CodeCredits
	case errors.As(err, &ent):
		return CodeQuota
	case errors.Is(err, aimodel.ErrDisabled):
		return CodeAIDisabled
	case errors.Is(err, ai.ErrNotConfigured):
		return CodeNotConfigured
	case errors.Is(err, ai.ErrInjection):
		return CodeUnsafe
	case errors.Is(err, model.ErrInvalid):
		return CodeInvalid
	case errors.Is(err, model.ErrUnavailable):
		return CodeUnavailable
	case errors.Is(err, ai.ErrUnavailable), errors.Is(err, ai.ErrRefused), errors.Is(err, ai.ErrTruncated),
		errors.Is(err, ai.ErrInvalidOutput), errors.Is(err, ai.ErrTooManyRounds):
		return CodeProvider
	}
	return CodeUnknown
}

// resumeAt reports whether err is a limit the run waits out, and until when:
// daily calls and budget reset at midnight in Istanbul (AIUsageToday), a
// concurrency slot frees within minutes.
func (s *Service) resumeAt(err error) (time.Time, bool) {
	now := s.now()
	switch failureCode(err) {
	case CodeDailyLimit, CodeBudget:
		return NextLimitReset(now).Add(time.Minute), true
	case CodeConcurrency:
		return now.Add(2 * time.Minute), true
	}
	return time.Time{}, false
}

// istanbul is the day boundary of the daily AI limits. Turkey has kept
// UTC+3 all year since 2016, so the fixed zone is the fallback without tzdata.
var istanbul = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return loc
	}
	return time.FixedZone("+03", 3*60*60)
}()

// NextLimitReset is the next midnight in Istanbul after t.
func NextLimitReset(t time.Time) time.Time {
	l := t.In(istanbul)
	return time.Date(l.Year(), l.Month(), l.Day()+1, 0, 0, 0, 0, istanbul).UTC()
}

func publicError(err error) string {
	switch failureCode(err) {
	case CodeInvalid, CodeUnavailable:
		return err.Error()
	case CodeNotConfigured:
		return "AI embedding is not configured for this platform"
	case CodeUnsafe:
		return "a record looks like an instruction to the AI and was refused"
	case CodeDailyLimit:
		return "the daily AI call limit for batch jobs is used up; the run continues when the limit resets"
	case CodeBudget:
		return "the daily AI budget for batch jobs is used up; the run continues when the limit resets"
	case CodeConcurrency:
		return "too many AI calls are running; the run continues in a few minutes"
	case CodeCredits:
		return "the organization has no AI credits left; add credits and upload again"
	case CodeQuota:
		return "the monthly AI allowance of the plan is used up"
	case CodeAIDisabled:
		return "AI is turned off for this organization"
	case CodeProvider:
		return "the AI provider returned an error; roll the run back and upload it again"
	}
	return "processing failed; the run can be rolled back and uploaded again"
}

func (s *Service) deleteRaw(ctx context.Context, run db.HistoryImport) {
	if run.RawKey == "" {
		return
	}
	if s.Store != nil {
		if err := s.Store.Delete(ctx, run.RawKey); err != nil {
			s.Log.WarnContext(ctx, "history_raw_delete_failed", "run_uuid", run.Uuid, "error", err)
			return
		}
	}
	if err := s.Q.HistoryImportRawDeleted(ctx, db.HistoryImportRawDeletedParams{OrganizationID: run.OrganizationID, ID: run.ID}); err != nil {
		s.Log.WarnContext(ctx, "history_raw_mark_failed", "run_uuid", run.Uuid, "error", err)
	}
}

func (s *Service) process(ctx context.Context, p *progress) error {
	run := p.run
	if s.Embedder == nil {
		return ai.ErrNotConfigured
	}
	settings, err := s.Settings(ctx, run.OrganizationID)
	if err != nil {
		return err
	}
	src, err := s.source(run)
	if err != nil {
		return err
	}
	records, err := src.Records(ctx, run)
	if err != nil {
		return err
	}
	p.total = int32(len(records)) // #nosec G115 -- sources return at most model.MaxTickets records
	if run.Source == "nightly" && len(records) >= model.MaxTickets {
		// Records come ordered by close time: continue after the last one.
		if last := records[len(records)-1].ClosedAt; last != nil {
			p.watermark = pgtype.Timestamptz{Time: *last, Valid: true}
		}
	}
	touched := map[int64]bool{}
	offset := int(run.ResumeOffset)
	if offset > 0 {
		// A paused run continues where it stopped, with its saved counters
		// and the clusters its archived cases already touched.
		p.counts = counts{processed: run.ProcessedRecords, skipped: run.SkippedRecords, suggestions: run.SuggestionsCreated, suppressed: run.SuggestionsSuppressed}
		ids, err := s.Q.HistoryClustersOfImport(ctx, db.HistoryClustersOfImportParams{OrganizationID: run.OrganizationID, ImportID: run.ID})
		if err != nil {
			return err
		}
		for _, id := range ids {
			touched[id] = true
		}
		p.clusters = int32(len(touched)) // #nosec G115 -- at most one run's records (MaxTickets)
	}
	if err := s.report(ctx, p, "anonymizing"); err != nil {
		return err
	}
	clusters := map[string][]candidate{}
	size := max(1, min(s.BatchSize, model.MaxBatchRecords))
	for start := min(offset, len(records)); start < len(records); start += size {
		batch := records[start:min(start+size, len(records))]
		p.offset = int32(start) // #nosec G115 -- bounded by MaxTickets
		saved := p.counts
		ids, err := s.archiveBatch(ctx, p, settings, batch, clusters)
		if err != nil {
			// Nothing of this batch was archived: a resume repeats it whole.
			p.counts = saved
			return err
		}
		for _, id := range ids {
			if !touched[id] {
				touched[id] = true
				p.clusters++
			}
		}
		p.offset = int32(start + len(batch)) // #nosec G115 -- bounded by MaxTickets
		if err := s.report(ctx, p, "clustering"); err != nil {
			return err
		}
	}
	if err := s.report(ctx, p, "suggesting"); err != nil {
		return err
	}
	// Clusters are evaluated in id order so a resumed run skips the ones
	// already evaluated.
	order := make([]int64, 0, len(touched))
	for id := range touched {
		order = append(order, id)
	}
	slices.Sort(order)
	for i := max(0, offset-len(records)); i < len(order); i++ {
		p.offset = int32(len(records) + i) // #nosec G115 -- bounded by MaxTickets
		saved := p.counts
		if err := s.evaluate(ctx, p, settings, order[i]); err != nil {
			p.counts = saved
			return err
		}
		if err := s.report(ctx, p, "suggesting"); err != nil {
			return err
		}
	}
	return nil
}

type prepared struct {
	rec      Record
	c        Case
	category string
}

// archiveBatch anonymizes, embeds, archives and clusters one batch and
// returns the clusters it touched.
func (s *Service) archiveBatch(ctx context.Context, p *progress, settings db.HistorySetting, batch []Record, clusters map[string][]candidate) ([]int64, error) {
	run := p.run
	org := run.OrganizationID
	var items []prepared
	refs := []string{}
	for _, r := range batch {
		c, ok := BuildCase(org, r)
		// A record that reads like an instruction to the model is skipped,
		// not sent: one bad ticket must not fail the whole run.
		if !ok || ai.Guard(c.EmbedText()) != nil || ai.Guard(c.Resolution) != nil {
			p.skipped++
			continue
		}
		category, err := s.Taxonomy.Categorize(ctx, org, r, c)
		if err != nil {
			return nil, err
		}
		items = append(items, prepared{rec: r, c: c, category: truncate(category, 100)})
		refs = append(refs, c.Ref)
	}
	if len(refs) > 0 {
		existing, err := s.Q.HistoryCaseRefsExisting(ctx, db.HistoryCaseRefsExistingParams{OrganizationID: org, Refs: refs})
		if err != nil {
			return nil, err
		}
		seen := make(map[string]bool, len(existing))
		for _, r := range existing {
			seen[r] = true
		}
		kept := items[:0]
		for _, it := range items {
			if seen[it.c.Ref] {
				p.skipped++
				continue
			}
			seen[it.c.Ref] = true
			kept = append(kept, it)
		}
		items = kept
	}
	if len(items) == 0 {
		return nil, nil
	}
	if err := s.report(ctx, p, "embedding"); err != nil {
		return nil, err
	}
	texts := make([]string, 0, 2*len(items))
	for _, it := range items {
		texts = append(texts, it.c.EmbedText(), it.c.Resolution)
	}
	vectors, err := s.embedAll(ctx, texts)
	if err != nil {
		return nil, err
	}
	embeddingModel := s.Embedder.Model()
	var touched []int64
	err = s.tx(ctx, func(q *db.Queries) error {
		for i, it := range items {
			problemVec, resolutionVec := vectors[2*i], vectors[2*i+1]
			opened, closed := ts(it.rec.OpenedAt), ts(it.rec.ClosedAt)
			brand := it.rec.BrandID
			if !brand.Valid {
				brand = run.BrandID
			}
			caseID, err := q.HistoryCaseInsert(ctx, db.HistoryCaseInsertParams{OrganizationID: org, ImportID: run.ID, BrandID: brand, DepartmentID: run.DepartmentID, Source: run.Source,
				ExternalRef: it.c.Ref, ConversationID: pgtype.Int8{Int64: it.rec.ConversationID, Valid: it.rec.ConversationID > 0}, Subject: it.c.Subject, Problem: it.c.Problem,
				Resolution: it.c.Resolution, MessageCount: it.c.Messages, Locale: locale(it.rec.Locale), TrainingAllowed: run.ConsentTraining, Category: it.category,
				Embedding: problemVec, ResolutionEmbedding: resolutionVec, EmbeddingModel: embeddingModel, OpenedAt: opened, ClosedAt: closed})
			if errors.Is(err, pgx.ErrNoRows) {
				p.skipped++
				continue
			}
			if err != nil {
				return err
			}
			// Clusters are scoped like the case: organization, brand, department, category.
			key := fmt.Sprintf("%s|%v|%v", it.category, brand, run.DepartmentID)
			cands, ok := clusters[key]
			if !ok {
				rows, err := q.HistoryClusterCandidates(ctx, db.HistoryClusterCandidatesParams{OrganizationID: org, Category: it.category, EmbeddingModel: embeddingModel, BrandID: brand, DepartmentID: run.DepartmentID})
				if err != nil {
					return err
				}
				for _, r := range rows {
					cands = append(cands, candidate{id: r.ID, centroid: r.Centroid, size: r.Size})
				}
			}
			var clusterID int64
			if i := nearest(cands, problemVec, float64(settings.ClusterSimilarity)); i >= 0 {
				c := &cands[i]
				c.centroid = grow(c.centroid, c.size, problemVec)
				c.size++
				clusterID = c.id
				if err := q.HistoryClusterGrow(ctx, db.HistoryClusterGrowParams{OrganizationID: org, ID: c.id, Centroid: c.centroid, Size: c.size}); err != nil {
					return err
				}
			} else {
				label := it.c.Subject
				if label == "" {
					label = truncate(it.c.Problem, 120)
				}
				clusterID, err = q.HistoryClusterCreate(ctx, db.HistoryClusterCreateParams{OrganizationID: org, BrandID: brand, DepartmentID: run.DepartmentID, Category: it.category,
					Label: truncate(label, 300), Centroid: problemVec, EmbeddingModel: embeddingModel})
				if err != nil {
					return err
				}
				cands = append(cands, candidate{id: clusterID, centroid: problemVec, size: 1})
			}
			clusters[key] = cands
			if err := q.HistoryCaseAssign(ctx, db.HistoryCaseAssignParams{OrganizationID: org, ID: caseID, ClusterID: pgtype.Int8{Int64: clusterID, Valid: true}}); err != nil {
				return err
			}
			p.processed++
			touched = append(touched, clusterID)
		}
		return nil
	})
	if err != nil {
		// The transaction rolled back; drop cached candidates so the next
		// attempt reloads clusters from the database.
		clear(clusters)
		return nil, err
	}
	return touched, nil
}

// embedAll embeds texts in as few calls as the provider allows: at most
// model.MaxEmbedInputs texts and model.MaxEmbedBytes bytes per call (bytes
// bound tokens from above, CF-228).
func (s *Service) embedAll(ctx context.Context, texts []string) ([][]float32, error) {
	sizes := make([]int, len(texts))
	for i, t := range texts {
		sizes[i] = len(t)
	}
	out := make([][]float32, 0, len(texts))
	start := 0
	for _, n := range embedChunks(sizes, model.MaxEmbedInputs, model.MaxEmbedBytes) {
		res, err := s.Embedder.Embed(ctx, texts[start:start+n])
		if err != nil {
			return nil, err
		}
		if len(res.Vectors) != n {
			return nil, ai.ErrInvalidOutput
		}
		out = append(out, res.Vectors...)
		start += n
	}
	return out, nil
}

func ts(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// refreshEvidence recomputes how many of a cluster's cases agree on one
// resolution and returns that group's cases.
func (s *Service) refreshEvidence(ctx context.Context, org, clusterID int64, settings db.HistorySetting) ([]db.HistoryClusterCasesRow, error) {
	rows, err := s.Q.HistoryClusterCases(ctx, db.HistoryClusterCasesParams{OrganizationID: org, ClusterID: pgtype.Int8{Int64: clusterID, Valid: true}})
	if err != nil {
		return nil, err
	}
	resolutions := make([][]float32, len(rows))
	for i, r := range rows {
		resolutions[i] = r.ResolutionEmbedding
	}
	group := evidenceGroup(resolutions, float64(settings.ResolutionSimilarity))
	evidence := int32(len(group)) // #nosec G115 -- a group is at most one run's records (MaxTickets)
	if err := s.Q.HistoryClusterEvidence(ctx, db.HistoryClusterEvidenceParams{OrganizationID: org, ID: clusterID, Evidence: evidence}); err != nil {
		return nil, err
	}
	out := make([]db.HistoryClusterCasesRow, 0, len(group))
	for _, i := range group {
		out = append(out, rows[i])
	}
	return out, nil
}

const draftInstructions = "The cases below are resolved support tickets with the same problem; personal data is already replaced by [PII_*] placeholders. Compare the agents' resolutions, keep the solution they agree on and write one reusable internal help article: a short title and a step-by-step body. Use only facts from the cases. Never invent or restore personal data. If the cases do not share a reusable solution, return an empty title and body."

// evaluate turns a cluster into a suggestion when its evidence reaches the
// threshold, unless a reviewer already rejected this cluster or something
// close enough to the new draft.
func (s *Service) evaluate(ctx context.Context, p *progress, settings db.HistorySetting, clusterID int64) error {
	org := p.run.OrganizationID
	group, err := s.refreshEvidence(ctx, org, clusterID, settings)
	if err != nil {
		return err
	}
	evidence := int32(len(group)) // #nosec G115 -- a group is at most one run's records (MaxTickets)
	if evidence < settings.EvidenceThreshold {
		return nil
	}
	state, err := s.Q.HistoryClusterSuggestionState(ctx, db.HistoryClusterSuggestionStateParams{OrganizationID: org, ClusterID: pgtype.Int8{Int64: clusterID, Valid: true}})
	if err != nil {
		return err
	}
	if state.Pending > 0 || state.Approved > 0 {
		return nil
	}
	rejected, err := s.Q.HistoryClusterRejected(ctx, db.HistoryClusterRejectedParams{OrganizationID: org, ClusterID: pgtype.Int8{Int64: clusterID, Valid: true}})
	if err != nil {
		return err
	}
	if rejected > 0 {
		p.suppressed++
		return nil
	}
	if s.Drafter == nil {
		return nil
	}
	var input strings.Builder
	lang := "tr"
	for i, c := range group[:min(len(group), model.MaxDraftCases)] {
		lang = c.Locale
		fmt.Fprintf(&input, "Case %d\nSubject: %s\nProblem: %s\nResolution: %s\n\n", i+1, c.Subject, truncate(c.Problem, 1200), truncate(c.Resolution, 1200))
	}
	draft, err := s.Drafter.Draft(ctx, org, lang, truncate(input.String(), 20000))
	if err != nil {
		return err
	}
	draft.Title, draft.Body = truncate(draft.Title, 300), truncate(draft.Body, 30000)
	if draft.Title == "" || draft.Body == "" {
		return nil
	}
	vec, err := s.embedOne(ctx, draft.Title+"\n"+draft.Body)
	if err != nil {
		return err
	}
	cluster, err := s.Q.HistoryClusterGet(ctx, db.HistoryClusterGetParams{OrganizationID: org, ID: clusterID})
	if err != nil {
		return err
	}
	rej, err := s.Q.HistoryNearestRejection(ctx, db.HistoryNearestRejectionParams{OrganizationID: org, EmbeddingModel: s.Embedder.Model(), Embedding: vec, DepartmentID: cluster.DepartmentID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil && rej.Score >= float64(settings.RejectionSimilarity) {
		p.suppressed++
		return nil
	}
	conflict, conflictScore := pgtype.Int8{}, float32(0)
	art, err := s.Q.HistoryNearestArticle(ctx, db.HistoryNearestArticleParams{OrganizationID: org, EmbeddingModel: s.Embedder.Model(), Embedding: vec})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil && art.Score >= model.ConflictThreshold {
		// A published article covers the same ground: the reviewer compares
		// the two and either corrects the article or rejects the suggestion.
		conflict, conflictScore = pgtype.Int8{Int64: art.ID, Valid: true}, float32(min(art.Score, 1))
	}
	topic := cluster.Label
	if strings.TrimSpace(topic) == "" {
		topic = draft.Title
	}
	_, err = s.Q.HistorySuggestionCreate(ctx, db.HistorySuggestionCreateParams{OrganizationID: org, Topic: truncate(topic, 200), Title: draft.Title, Body: draft.Body,
		ClusterID: pgtype.Int8{Int64: clusterID, Valid: true}, ImportID: pgtype.Int8{Int64: p.run.ID, Valid: true}, DepartmentID: cluster.DepartmentID,
		EvidenceCount: int32(len(group)), ConflictArticleID: conflict, ConflictScore: conflictScore}) // #nosec G115 -- a group is at most one run's records (MaxTickets)
	if pgCode(err) == "23505" {
		return nil // another run created the pending suggestion first
	}
	if err != nil {
		return err
	}
	p.suggestions++
	return nil
}

func (s *Service) embedOne(ctx context.Context, text string) ([]float32, error) {
	if s.Embedder == nil {
		return nil, ai.ErrNotConfigured
	}
	if _, ok := orgctx.OrganizationID(ctx); !ok {
		return nil, fmt.Errorf("history: embedding needs an organization scope")
	}
	out, err := s.Embedder.Embed(ctx, []string{truncate(text, 6000)})
	if err != nil {
		return nil, err
	}
	if len(out.Vectors) != 1 || len(out.Vectors[0]) == 0 {
		return nil, ai.ErrInvalidOutput
	}
	return out.Vectors[0], nil
}

// Sweep is the worker's periodic step: delete raw files whose run ended or
// whose retention passed, apply archive retention, then process due runs.
func (s *Service) Sweep(ctx context.Context) error {
	// A run still waiting when its raw-file retention passes fails, so the
	// file is deleted below instead of outliving the configured retention.
	if n, err := s.Q.HistoryImportExpireStale(ctx); err != nil {
		return err
	} else if n > 0 {
		s.Log.InfoContext(ctx, "history_runs_expired", "runs", n)
	}
	expired, err := s.Q.HistoryImportRawExpired(ctx, 100)
	if err != nil {
		return err
	}
	for _, r := range expired {
		s.deleteRaw(ctx, db.HistoryImport{OrganizationID: r.OrganizationID, ID: r.ID, RawKey: r.RawKey})
	}
	if n, err := s.Q.HistoryCaseExpire(ctx); err != nil {
		return err
	} else if n > 0 {
		s.Log.InfoContext(ctx, "history_archive_expired", "cases", n)
	}
	due, err := s.Q.HistoryImportDue(ctx, 3)
	if err != nil {
		return err
	}
	for _, d := range due {
		if err := s.Process(ctx, d.OrganizationID, d.Uuid); err != nil {
			return err
		}
	}
	return nil
}

// Nightly queues a run over each opted-in organization's conversations
// closed since its watermark (at most the last 7 days on the first run).
// Consent comes from the organization's settings and is copied to the run.
func (s *Service) Nightly(ctx context.Context) error {
	orgs, err := s.Q.HistoryNightlyOrganizations(ctx)
	if err != nil {
		return err
	}
	end := s.now()
	for _, o := range orgs {
		if active, err := s.Q.HistoryImportActive(ctx, o.OrganizationID); err != nil {
			return err
		} else if active > 0 {
			continue
		}
		start := end.Add(-7 * 24 * time.Hour)
		if o.NightlyWatermark.Valid && o.NightlyWatermark.Time.After(start) {
			start = o.NightlyWatermark.Time
		}
		if _, err := s.Q.HistoryImportCreate(ctx, db.HistoryImportCreateParams{OrganizationID: o.OrganizationID, Source: "nightly", ConsentAnalysis: true,
			ConsentTraining: o.NightlyConsentTraining, ConsentBy: o.UpdatedBy, WindowStart: pgtype.Timestamptz{Time: start, Valid: true},
			WindowEnd: pgtype.Timestamptz{Time: end, Valid: true}}); err != nil {
			return err
		}
	}
	return nil
}
