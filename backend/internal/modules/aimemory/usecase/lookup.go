package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aimemory/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// maxQuestionRunes bounds what is treated as one question; longer messages
// skip the cache and go to the normal AI path.
const maxQuestionRunes = 1000

// Normalize is the exact-cache key text: PII replaced by stable placeholders,
// lower-cased (Turkish I aware), punctuation dropped, spaces collapsed.
func Normalize(question string) string {
	question = lowerTR(Anonymize(question, Identity{}))
	var b strings.Builder
	space := false
	for _, r := range question {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r) || r == '[' || r == ']':
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			space = false
		default:
			space = true
		}
	}
	return b.String()
}

func questionHash(normalized string) string {
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func brandArg(s model.Scope) pgtype.UUID {
	if s.BrandID == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *s.BrandID, Valid: true}
}

func departmentArg(s model.Scope) pgtype.Int8 {
	if s.DepartmentID == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *s.DepartmentID, Valid: true}
}

func articleRef(id pgtype.UUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	v := uuid.UUID(id.Bytes)
	return &v
}

// Lookup answers a question from the knowledge layer before any generation:
//
//  1. exact repeat in the answer cache: no AI call at all;
//  2. one embedding call, then the closest knowledge unit above the threshold
//     in the most specific scope (department, brand, then organization);
//  3. otherwise case memory: approved and anonymized summaries for the
//     customer AI, every non-rejected one for the agent assistant.
//
// Every lookup is counted for the hit rate. A miss is not yet a knowledge
// gap: the caller records the gap when the normal AI path finds no source
// either (RecordGap).
func (s *Service) Lookup(ctx context.Context, q model.Query) (model.Result, error) {
	out := model.Result{Outcome: model.OutcomeMiss}
	if err := q.Validate(); err != nil {
		return out, err
	}
	question := strings.TrimSpace(q.Question)
	if question == "" || utf8.RuneCountInString(question) > maxQuestionRunes || !s.Ready() {
		return out, nil
	}
	if err := ai.Guard(question); err != nil {
		return out, nil
	}
	ctx = orgctx.WithOrganizationID(ctx, q.Organization)
	normalized := Normalize(question)
	if normalized == "" {
		return out, nil
	}
	hash := questionHash(normalized)
	scopeKey := q.Scope.Key()
	started := time.Now()
	defer func() {
		slog.InfoContext(ctx, "ai_stage", "stage", "memory_lookup", "organization_id", q.Organization, "audience", q.Audience, "outcome", out.Outcome, "duration_ms", time.Since(started).Milliseconds())
	}()

	cached, err := s.Q.AnswerCacheGet(ctx, db.AnswerCacheGetParams{OrganizationID: q.Organization, Audience: q.Audience, Locale: q.Locale, ScopeKey: scopeKey, QuestionHash: hash, EmbeddingModel: s.Embedder.Model()})
	switch err = normalize(err); {
	case err == nil:
		if err = s.Q.AnswerCacheTouch(ctx, db.AnswerCacheTouchParams{OrganizationID: q.Organization, ID: cached.ID}); err != nil {
			return out, err
		}
		out.Outcome = model.OutcomeExact
		out.Unit = &model.UnitHit{UUID: cached.UnitUuid, Question: cached.Question, Answer: cached.Answer, Category: cached.Category, Visibility: cached.Visibility, ArticleUUID: articleRef(cached.ArticleUuid), Score: float64(cached.Score)}
		return out, s.count(ctx, q, out.Outcome)
	case !errors.Is(err, model.ErrNotFound):
		return out, err
	}

	embedded, err := s.Embedder.Embed(ctx, []string{question})
	out.EmbeddingCalls = 1
	if err != nil {
		// AI off or budget exhausted: the caller's normal path decides.
		slog.InfoContext(ctx, "ai_memory_embed_failed", "organization_id", q.Organization, "error", err)
		out.EmbeddingCalls = 0
		return out, s.count(ctx, q, model.OutcomeMiss)
	}
	if len(embedded.Vectors) != 1 || len(embedded.Vectors[0]) == 0 {
		return out, ai.ErrInvalidOutput
	}
	vector := embedded.Vectors[0]
	units, err := s.Q.KnowledgeUnitMatch(ctx, db.KnowledgeUnitMatchParams{
		OrganizationID: q.Organization, Locale: q.Locale, Embedding: vector, EmbeddingModel: s.Embedder.Model(),
		PublicOnly: q.Audience == model.AudienceCustomer, BrandID: brandArg(q.Scope), DepartmentID: departmentArg(q.Scope),
	})
	if err != nil {
		return out, err
	}
	if best, ok := pickUnit(units, s.UnitThreshold); ok {
		if err = s.Q.AnswerCachePut(ctx, db.AnswerCachePutParams{OrganizationID: q.Organization, UnitID: best.ID, Audience: q.Audience, Locale: q.Locale, ScopeKey: scopeKey, QuestionHash: hash, Score: float32(best.Score)}); err != nil {
			return out, err
		}
		out.Outcome = model.OutcomeSemantic
		out.Unit = &model.UnitHit{UUID: best.Uuid, Question: best.Question, Answer: best.Answer, Category: best.Category, Visibility: best.Visibility, ArticleUUID: articleRef(best.ArticleUuid), Score: best.Score}
		return out, s.count(ctx, q, out.Outcome)
	}

	if err = s.cases(ctx, q, vector, &out); err != nil {
		return out, err
	}
	return out, s.count(ctx, q, model.OutcomeMiss)
}

// pickUnit prefers the most specific scope among matches above threshold
// (organization-wide units are the fallback), then the closest one.
func pickUnit(rows []db.KnowledgeUnitMatchRow, threshold float64) (db.KnowledgeUnitMatchRow, bool) {
	best, found, bestRank := db.KnowledgeUnitMatchRow{}, false, -1
	for _, r := range rows {
		if r.Score < threshold {
			continue
		}
		rank := 0
		if r.BrandID.Valid {
			rank++
		}
		if r.DepartmentID.Valid {
			rank += 2
		}
		if !found || rank > bestRank || (rank == bestRank && r.Score > best.Score) {
			best, found, bestRank = r, true, rank
		}
	}
	return best, found
}

func (s *Service) cases(ctx context.Context, q model.Query, vector []float32, out *model.Result) error {
	customer := q.Audience == model.AudienceCustomer
	rows, err := s.Q.CaseMemoryMatch(ctx, db.CaseMemoryMatchParams{
		OrganizationID: q.Organization, Locale: q.Locale, Embedding: vector, EmbeddingModel: s.Embedder.Model(),
		ExcludeConversationID: q.ExcludeConversationID, ApprovedOnly: customer, BrandID: brandArg(q.Scope), DepartmentID: departmentArg(q.Scope),
	})
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.Score < s.CaseThreshold {
			continue
		}
		if customer {
			// Approved summaries were anonymized when stored; scrub once more
			// against the source contact and company so a later edit of the
			// contact (or a model slip) still cannot reach another customer.
			id := Identity{Names: []string{r.ContactName, r.CompanyName}, Emails: []string{r.ContactEmail}, Phones: []string{r.ContactPhone}, Hosts: []string{r.CompanyDomain}}
			c := model.CustomerCase{Problem: Anonymize(r.Problem, id), Resolution: Anonymize(r.Resolution, id), Category: r.Category, Score: r.Score}
			if Leaks(c.Problem+"\n"+c.Resolution, id) {
				continue
			}
			out.CustomerCases = append(out.CustomerCases, c)
			continue
		}
		link := r.ConversationUuid
		out.AgentCases = append(out.AgentCases, model.AgentCase{UUID: r.Uuid, Problem: r.Problem, Resolution: r.Resolution, Category: r.Category, Status: r.Status, Score: r.Score, ConversationUUID: &link, SourceConversationID: r.ConversationID})
	}
	return nil
}

func (s *Service) count(ctx context.Context, q model.Query, outcome string) error {
	p := db.AnswerCacheStatParams{OrganizationID: q.Organization, Audience: q.Audience}
	switch outcome {
	case model.OutcomeExact:
		p.Exact = 1
	case model.OutcomeSemantic:
		p.Semantic = 1
	default:
		p.Miss = 1
	}
	return s.Q.AnswerCacheStat(ctx, p)
}

// RecordGap stores a question that neither the knowledge layer nor the
// normal AI path could answer from a source. It feeds the knowledge gaps
// report (AIKnowledgeGapTopics).
func (s *Service) RecordGap(ctx context.Context, q model.Query, conversationID int64, category string) error {
	if err := q.Validate(); err != nil {
		return err
	}
	question := clip(Anonymize(strings.TrimSpace(q.Question), Identity{}), 500)
	normalized := Normalize(question)
	if normalized == "" || ai.Guard(question) != nil {
		return nil
	}
	conv := pgtype.Int8{Int64: conversationID, Valid: conversationID > 0}
	return s.Q.KnowledgeGapQuestionRecord(ctx, db.KnowledgeGapQuestionRecordParams{
		OrganizationID: q.Organization, Audience: q.Audience, Locale: q.Locale, ScopeKey: q.Scope.Key(),
		BrandID: brandArg(q.Scope), DepartmentID: departmentArg(q.Scope), QuestionHash: questionHash(normalized),
		Question: question, Category: clip(strings.TrimSpace(category), 100), ConversationID: conv,
	})
}

// Stats returns the cache hit rate per audience since the given day.
func (s *Service) Stats(ctx context.Context, org int64, since time.Time) (model.Stats, error) {
	out := model.Stats{Since: since, Audiences: []model.AudienceStats{}}
	rows, err := s.Q.AnswerCacheStats(ctx, db.AnswerCacheStatsParams{OrganizationID: org, Since: pgtype.Date{Time: since, Valid: true}})
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		a := model.AudienceStats{Audience: r.Audience, ExactHits: r.ExactHits, SemanticHits: r.SemanticHits, Misses: r.Misses}
		a.Lookups = a.ExactHits + a.SemanticHits + a.Misses
		if a.Lookups > 0 {
			a.HitRate = float64(a.ExactHits+a.SemanticHits) / float64(a.Lookups)
		}
		out.Audiences = append(out.Audiences, a)
	}
	if out.Units, err = s.Q.KnowledgeUnitCount(ctx, org); err != nil {
		return out, err
	}
	out.Cached, err = s.Q.AnswerCacheCount(ctx, org)
	return out, err
}
