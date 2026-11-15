package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	aimodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/deptscope"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/net/html"
)

// Lexical provides tenant-filtered Meilisearch candidates. Every hit is revalidated in
// Postgres: a stale index cannot expose a draft, deleted article or private revision.
type Lexical interface {
	Replace(context.Context, int64, uuid.UUID, []Document) error
	Search(context.Context, int64, string, string, bool) ([]uuid.UUID, error)
}
type Document struct {
	ID             string `json:"id"`
	OrganizationID int64  `json:"organization_id"`
	ArticleUUID    string `json:"article_uuid"`
	Locale         string `json:"locale"`
	Visibility     string `json:"visibility"`
	Content        string `json:"content"`
}
type Reranker interface {
	Rerank(context.Context, string, []model.Citation) ([]model.Citation, error)
}
type RAG struct {
	KB       *Service
	Embedder ai.Embedder
	Lexical  Lexical
	Reranker Reranker
}

func (r *RAG) Consumer() eventdispatch.Consumer {
	return eventdispatch.Consumer{Name: "knowledge.index", Events: []string{ArticlePublished, ArticleUnpublished}, Handle: func(ctx context.Context, ev events.Event) error {
		if ev.TenantID == nil {
			return model.ErrInvalid
		}
		raw, _ := ev.Payload["article_uuid"].(string)
		id, err := uuid.Parse(raw)
		if err != nil {
			return model.ErrInvalid
		}
		return r.Index(ctx, *ev.TenantID, id)
	}}
}

// Chunk preserves paragraph, heading and fenced-code boundaries when possible. Very
// large blocks are split on rune boundaries with overlap so no source bytes are dropped.
func Chunk(body string, maxRunes, overlap int) []string {
	if maxRunes < 128 {
		maxRunes = 1200
	}
	if overlap < 0 || overlap >= maxRunes {
		overlap = 100
	}
	if strings.Contains(body, "<") {
		tokenizer := html.NewTokenizer(strings.NewReader(body))
		var plain strings.Builder
		skip := false
		for {
			tt := tokenizer.Next()
			if tt == html.ErrorToken {
				break
			}
			token := tokenizer.Token()
			switch tt {
			case html.StartTagToken:
				if token.Data == "script" || token.Data == "style" {
					skip = true
				}
				if blockTags[token.Data] {
					plain.WriteString("\n\n")
				}
			case html.EndTagToken:
				if token.Data == "script" || token.Data == "style" {
					skip = false
				}
				if blockTags[token.Data] {
					plain.WriteString("\n\n")
				}
			case html.TextToken:
				if !skip {
					plain.WriteString(token.Data)
				}
			}
		}
		body = plain.String()
	}
	blocks := []string{}
	var block strings.Builder
	fenced := false
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
		}
		if !fenced && (strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#")) && block.Len() > 0 {
			blocks = append(blocks, block.String())
			block.Reset()
		}
		block.WriteString(line)
		block.WriteByte('\n')
	}
	if block.Len() > 0 {
		blocks = append(blocks, block.String())
	}
	out := []string{}
	current := ""
	flush := func() {
		if strings.TrimSpace(current) != "" {
			out = append(out, strings.TrimSpace(current))
		}
		current = ""
	}
	for _, b := range blocks {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		runes := []rune(b)
		if len(runes) > maxRunes {
			flush()
			for start := 0; start < len(runes); {
				end := min(start+maxRunes, len(runes))
				out = append(out, string(runes[start:end]))
				if end == len(runes) {
					break
				}
				start = end - overlap
			}
			continue
		}
		if utf8.RuneCountInString(current)+len(runes)+2 > maxRunes {
			flush()
		}
		if current != "" {
			current += "\n\n"
		}
		current += b
	}
	flush()
	return out
}

var (
	blockTags = map[string]bool{"p": true, "div": true, "br": true, "h1": true, "h2": true, "h3": true, "li": true, "pre": true, "table": true, "tr": true}
	blank     = regexp.MustCompile(`\s+`)
)

func validVectors(vectors [][]float32, n int) bool {
	if len(vectors) != n {
		return false
	}
	dim := 0
	for _, v := range vectors {
		if len(v) == 0 || len(v) > 8192 {
			return false
		}
		if dim == 0 {
			dim = len(v)
		}
		if len(v) != dim {
			return false
		}
		norm := float64(0)
		for _, x := range v {
			if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
				return false
			}
			norm += float64(x) * float64(x)
		}
		if norm == 0 {
			return false
		}
	}
	return true
}

func (r *RAG) Index(ctx context.Context, org int64, id uuid.UUID) error {
	ctx = orgctx.WithOrganizationID(ctx, org)
	// Read first, embed masked data outside a DB lock, then compare the snapshot under lock.
	published, err := r.KB.Repo.Q.KnowledgePublished(ctx, db.KnowledgePublishedParams{OrganizationID: org, Uuid: id})
	missing := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !missing {
		return err
	}
	parts := []string{}
	vectors := [][]float32{}
	if !missing {
		if r.Embedder == nil {
			return r.skip(ctx, org, published, SkipAINotConfigured)
		}
		masker := ai.NewMasker()
		parts = Chunk(published.Title+"\n\n"+published.Body, 1200, 100)
		if len(parts) > 256 {
			return fmt.Errorf("knowledge: article exceeds 256 chunks")
		}
		for i := range parts {
			parts[i] = masker.Mask(parts[i])
		}
		if len(parts) > 0 {
			embedded, embedErr := r.Embedder.Embed(ctx, parts)
			vectors, err = embedded.Vectors, embedErr
			if err != nil {
				if reason := SkipReason(err); reason != "" {
					return r.skip(ctx, org, published, reason)
				}
				return err
			}
			if !validVectors(vectors, len(parts)) {
				return ai.ErrInvalidOutput
			}
		}
	}
	return r.KB.Repo.Within(ctx, func(q *db.Queries) error {
		current, err := q.KnowledgeLockArticle(ctx, db.KnowledgeLockArticleParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		nowMissing := current.DeletedAt.Valid || !current.PublishedVersion.Valid
		if missing != nowMissing || (!missing && current.PublishedVersion.Int32 != published.PublishedVersion.Int32) {
			return fmt.Errorf("knowledge: publication changed during indexing; retry")
		}
		if err = q.KnowledgeDeleteChunks(ctx, db.KnowledgeDeleteChunksParams{OrganizationID: org, ArticleID: current.ID}); err != nil {
			return err
		}
		docs := []Document{}
		if !missing {
			for i, part := range parts {
				c, err := q.KnowledgeInsertChunk(ctx, db.KnowledgeInsertChunkParams{OrganizationID: org, ArticleID: current.ID, Version: current.PublishedVersion.Int32, Ordinal: safeconv.ToInt32(i), Content: part, Embedding: vectors[i], EmbeddingModel: r.Embedder.Model()})
				if err != nil {
					return err
				}
				docs = append(docs, Document{ID: c.Uuid.String(), OrganizationID: org, ArticleUUID: id.String(), Locale: current.Locale, Visibility: published.Visibility, Content: part})
			}
		}
		if missing {
			if err = q.KnowledgeClearIndexState(ctx, db.KnowledgeClearIndexStateParams{OrganizationID: org, ArticleID: current.ID}); err != nil {
				return err
			}
		} else if err = q.KnowledgeSetIndexState(ctx, db.KnowledgeSetIndexStateParams{OrganizationID: org, ArticleID: current.ID, Status: IndexIndexed, Version: current.PublishedVersion}); err != nil {
			return err
		}
		if r.Lexical != nil {
			return r.Lexical.Replace(ctx, org, id, docs)
		}
		return nil
	})
}

func (r *RAG) vectorCandidates(ctx context.Context, org int64, query, locale string, publicOnly bool) ([]model.Citation, error) {
	started := time.Now()
	embedded, err := r.Embedder.Embed(ctx, []string{query})
	slog.InfoContext(ctx, "ai_stage", "stage", "embeddings", "organization_id", org, "duration_ms", time.Since(started).Milliseconds(), "failed", err != nil)
	if err != nil {
		return nil, err
	}
	if !validVectors(embedded.Vectors, 1) {
		return nil, ai.ErrInvalidOutput
	}
	restrict, viewer := deptscope.Effective(ctx).Args()
	rows, err := r.KB.Repo.Q.KnowledgeSearchVector(ctx, db.KnowledgeSearchVectorParams{OrganizationID: org, Locale: locale, Embedding: embedded.Vectors[0], PublicOnly: publicOnly, EmbeddingModel: r.Embedder.Model(), DeptRestrict: restrict, ViewerDepartments: viewer})
	if err != nil {
		return nil, err
	}
	list := []model.Citation{}
	for _, c := range rows {
		if c.Score > 0 {
			list = append(list, model.Citation{ChunkUUID: c.Uuid, ArticleUUID: c.ArticleUuid, Version: c.Version, Title: c.Title, Content: c.Content, Slug: c.Slug, Locale: c.Locale})
		}
	}
	return list, nil
}

// Index states and the reasons an article was skipped.
const (
	IndexIndexed = "indexed"
	IndexSkipped = "skipped"

	SkipAIDisabled         = "ai_disabled"
	SkipAINotConfigured    = "ai_not_configured"
	SkipAINotEntitled      = "ai_not_entitled"
	SkipAICreditsExhausted = "ai_credits_exhausted" // #nosec G101 -- skip reason / error code, not a credential
)

// SkipReason reports why embeddings cannot be built for the organization
// right now ("" when err is a real failure that should be retried). These
// are product states, not errors: the indexer records them and moves on.
func SkipReason(err error) string {
	switch {
	case errors.Is(err, aimodel.ErrDisabled):
		return SkipAIDisabled
	case errors.Is(err, ai.ErrNotConfigured):
		return SkipAINotConfigured
	case errors.Is(err, entitlement.ErrDenied):
		return SkipAINotEntitled
	case errors.Is(err, aimodel.ErrCredits):
		return SkipAICreditsExhausted
	}
	return ""
}

// skip records that the published version was not indexed and why. Turning
// AI on re-indexes skipped articles (aicore SavePolicy).
func (r *RAG) skip(ctx context.Context, org int64, a db.KnowledgePublishedRow, reason string) error {
	slog.InfoContext(ctx, "knowledge_index_skipped", "organization_id", org, "article_uuid", a.Uuid, "reason", reason)
	return r.KB.Repo.Q.KnowledgeSetIndexState(ctx, db.KnowledgeSetIndexStateParams{
		OrganizationID: org, ArticleID: a.ID, Status: IndexSkipped, Reason: reason, Version: a.PublishedVersion,
	})
}

// Search follows the department scope of ctx (CF-215, deptscope): without
// one only organization-wide articles are candidates.
func (r *RAG) Search(ctx context.Context, org int64, query, locale string, publicOnly bool) ([]model.Citation, error) {
	ctx = orgctx.WithOrganizationID(ctx, org)
	restrict, viewer := deptscope.Effective(ctx).Args()
	if len(query) == 0 || len(query) > 4000 || (locale != "tr" && locale != "en") {
		return nil, model.ErrInvalid
	}
	if err := ai.Guard(query); err != nil {
		return nil, err
	}
	query = ai.NewMasker().Mask(query)
	started := time.Now()
	defer func() {
		slog.InfoContext(ctx, "ai_stage", "stage", "knowledge_search_total", "organization_id", org, "duration_ms", time.Since(started).Milliseconds())
	}()
	lists := [][]model.Citation{}
	if r.Embedder != nil {
		list, err := r.vectorCandidates(ctx, org, query, locale, publicOnly)
		switch {
		case err == nil:
			lists = append(lists, list)
		case SkipReason(err) != "":
			// AI is off, not configured or not in the plan: search still
			// works on the lexical index alone.
			slog.InfoContext(ctx, "knowledge_search_lexical_only", "organization_id", org, "reason", SkipReason(err))
		default:
			return nil, err
		}
	}
	lexicalStarted := time.Now()
	lexical := []model.Citation{}
	if r.Lexical != nil {
		ids, err := r.Lexical.Search(ctx, org, query, locale, publicOnly)
		if err != nil {
			return nil, err
		}
		rows, err := r.KB.Repo.Q.KnowledgeValidChunks(ctx, db.KnowledgeValidChunksParams{OrganizationID: org, Locale: locale, Ids: ids, PublicOnly: publicOnly, DeptRestrict: restrict, ViewerDepartments: viewer})
		if err != nil {
			return nil, err
		}
		byID := map[uuid.UUID]model.Citation{}
		for _, c := range rows {
			byID[c.Uuid] = model.Citation{ChunkUUID: c.Uuid, ArticleUUID: c.ArticleUuid, Version: c.Version, Title: c.Title, Content: c.Content, Slug: c.Slug, Locale: c.Locale}
		}
		for _, id := range ids {
			if c, ok := byID[id]; ok {
				lexical = append(lexical, c)
			}
		}
	} else {
		rows, err := r.KB.Repo.Q.KnowledgeSearchLexical(ctx, db.KnowledgeSearchLexicalParams{OrganizationID: org, Locale: locale, Q: query, PublicOnly: publicOnly, DeptRestrict: restrict, ViewerDepartments: viewer})
		if err != nil {
			return nil, err
		}
		for _, c := range rows {
			lexical = append(lexical, model.Citation{ChunkUUID: c.Uuid, ArticleUUID: c.ArticleUuid, Version: c.Version, Title: c.Title, Content: c.Content, Slug: c.Slug, Locale: c.Locale})
		}
	}
	slog.InfoContext(ctx, "ai_stage", "stage", "lexical_search", "organization_id", org, "duration_ms", time.Since(lexicalStarted).Milliseconds())
	lists = append(lists, lexical)
	merged := map[uuid.UUID]model.Citation{}
	for _, list := range lists {
		for rank, c := range list {
			old := merged[c.ChunkUUID]
			c.Score = old.Score + 1/float64(60+rank+1)
			merged[c.ChunkUUID] = c
		}
	}
	out := []model.Citation{}
	for _, c := range merged {
		if ai.Guard(c.Content) != nil {
			continue
		}
		c.Title = ai.NewMasker().Mask(blank.ReplaceAllString(c.Title, " "))
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ChunkUUID.String() < out[j].ChunkUUID.String()
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > 10 {
		out = out[:10]
	}
	if r.Reranker != nil {
		ranked, err := r.Reranker.Rerank(ctx, query, out)
		if err != nil {
			return nil, err
		}
		// A reranker may reorder/drop existing candidates, never inject a citation or text.
		byID := map[uuid.UUID]model.Citation{}
		for _, c := range out {
			byID[c.ChunkUUID] = c
		}
		out = []model.Citation{}
		for _, c := range ranked {
			if original, ok := byID[c.ChunkUUID]; ok {
				out = append(out, original)
				delete(byID, c.ChunkUUID)
			}
		}
	}
	return out, nil
}
