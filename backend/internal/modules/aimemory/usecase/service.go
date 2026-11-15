// Package usecase builds and serves the CF-216 knowledge layer: knowledge
// units taken from articles and approved suggestions, the answer cache in
// front of them, the anonymized case memory of closed conversations and the
// unanswered questions that become knowledge gaps.
//
// Every model and embedding call goes through the AI core (Engine) and the
// metered embedder; nothing here talks to a provider directly.
package usecase

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	coremodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	core "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aimemory/model"
	kbusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Engine is the AI core's agent entry point (aicore usecase.Service).
type Engine interface {
	RunAgent(context.Context, int64, int64, coremodel.RunInput, core.AgentOptions) (core.RunResult, error)
}

// Event names this module consumes besides the knowledge/conversation ones.
const (
	articlePublished   = "knowledge.article_published"
	articleUnpublished = "knowledge.article_unpublished"
)

// Default thresholds (cosine similarity).
const (
	DefaultUnitThreshold = 0.90
	DefaultCaseThreshold = 0.78
)

type Service struct {
	Pool     *pgxpool.Pool
	Q        *db.Queries
	Engine   Engine
	Embedder ai.Embedder
	// UnitThreshold is the similarity above which a known unit answers a
	// question without a generation call.
	UnitThreshold float64
	// CaseThreshold is the similarity above which a case summary is offered.
	CaseThreshold float64
	Now           func() time.Time
}

func New(pool *pgxpool.Pool, engine Engine, embedder ai.Embedder) *Service {
	return &Service{Pool: pool, Q: db.New(pool), Engine: engine, Embedder: embedder, UnitThreshold: DefaultUnitThreshold, CaseThreshold: DefaultCaseThreshold, Now: time.Now}
}

// Ready reports whether lookups can run (an embedding model is configured).
func (s *Service) Ready() bool { return s != nil && s.Embedder != nil }

func (s *Service) within(ctx context.Context, fn func(*db.Queries) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = fn(s.Q.WithTx(tx)); err != nil {
		return normalize(err)
	}
	return normalize(tx.Commit(ctx))
}

func normalize(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505", "23503", "23514":
			return model.ErrConflict
		}
	}
	return err
}

// Consumer rebuilds units when a source changes and summarizes resolved
// conversations into case memory.
func (s *Service) Consumer() eventdispatch.Consumer {
	return eventdispatch.Consumer{Name: "ai.memory", Events: []string{articlePublished, articleUnpublished, model.SuggestionReviewed, events.ConversationsResolved}, Handle: func(ctx context.Context, ev events.Event) error {
		if ev.TenantID == nil {
			return model.ErrInvalid
		}
		org := *ev.TenantID
		var err error
		switch ev.Name {
		case articlePublished, articleUnpublished:
			raw, _ := ev.Payload["article_uuid"].(string)
			id, parseErr := uuid.Parse(raw)
			if parseErr != nil {
				return model.ErrInvalid
			}
			err = s.SyncArticle(ctx, org, id)
		case model.SuggestionReviewed:
			raw, _ := ev.Payload["suggestion_uuid"].(string)
			id, parseErr := uuid.Parse(raw)
			if parseErr != nil {
				return model.ErrInvalid
			}
			err = s.SyncSuggestion(ctx, org, id)
		case events.ConversationsResolved:
			if ev.EntityUUID == nil {
				return nil
			}
			err = s.Remember(ctx, org, *ev.EntityUUID)
		}
		if reason := kbusecase.SkipReason(err); reason != "" {
			// AI is off or not in the plan: nothing to build now, and a later
			// publish or resolution rebuilds it.
			slog.InfoContext(ctx, "ai_memory_skipped", "organization_id", org, "event", ev.Name, "reason", reason)
			return nil
		}
		if errors.Is(err, model.ErrNotFound) {
			return nil
		}
		return err
	}}
}
