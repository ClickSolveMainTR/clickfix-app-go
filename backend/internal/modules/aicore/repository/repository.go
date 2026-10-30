// Package repository integrates AI reservations with CF-20 entitlement locking and
// the existing CF-37-compatible append-only ai_credits ledger. No second wallet exists.
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	billingrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/entitlements"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Store struct {
	Billing      *billingrepo.Repository
	Entitlements *entitlements.Service
}

func New(b *billingrepo.Repository, e *entitlements.Service) *Store {
	return &Store{Billing: b, Entitlements: e}
}

func (s *Store) Policy(ctx context.Context, org int64) (db.AiPolicy, error) {
	p, err := s.Billing.Q.AIPolicy(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.AiPolicy{OrganizationID: org, MaskPii: true, DailyCalls: 100, Concurrency: 3, DailyBudgetMicros: 10000000}, nil
	}
	return p, err
}

func (s *Store) SavePolicy(ctx context.Context, in db.AISavePolicyParams) (db.AiPolicy, error) {
	out := db.AiPolicy{}
	if in.DailyCalls < 1 || in.DailyCalls > 100000 || in.Concurrency < 1 || in.Concurrency > 20 || in.DailyBudgetMicros < 1 || in.DailyBudgetMicros > 1000000000000 {
		return out, model.ErrInvalid
	}
	err := s.Billing.Within(ctx, func(r *billingrepo.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, in.OrganizationID); err != nil {
			return err
		}
		var err error
		out, err = r.Q.AISavePolicy(ctx, in)
		if err != nil || !out.Enabled {
			return err
		}
		return requeueSkippedKnowledge(ctx, r.Q, in.OrganizationID, out.UpdatedAt.Time.UnixNano())
	})
	return out, err
}

// requeueSkippedKnowledge re-publishes the index event of articles the
// knowledge indexer skipped while AI was unavailable, in the same
// transaction as the policy that turns AI on.
func requeueSkippedKnowledge(ctx context.Context, q *db.Queries, org int64, revision int64) error {
	rows, err := q.KnowledgeSkippedArticles(ctx, org)
	if err != nil {
		return err
	}
	for _, a := range rows {
		id := a.Uuid
		ev := events.New(events.KnowledgeArticlePublished).WithOrganization(org).
			WithEntity("knowledge_articles", nil, &id).
			WithPayload(map[string]any{"article_uuid": id.String(), "version": a.PublishedVersion}).
			WithIdempotencyKey(fmt.Sprintf("reindex:%s:%d:%d", id, a.PublishedVersion, revision))
		if err := outbox.EnqueueQueries(ctx, q, ev); err != nil {
			return err
		}
	}
	return nil
}

// Reserve re-reads all mutable policy and quota state under the shared organization
// lock. Reservations are not reclaimed by TTL: a crashed call may still be billable.
func (s *Store) Reserve(ctx context.Context, in db.AIInsertRunParams) (db.AiRun, db.AiPolicy, error) {
	out := db.AiRun{}
	policy := db.AiPolicy{}
	err := s.Entitlements.WithAIReservation(ctx, in.OrganizationID, func(q *db.Queries, values map[string]entitlement.Value) error {
		p, err := q.AIPolicy(ctx, in.OrganizationID)
		if errors.Is(err, pgx.ErrNoRows) {
			return model.ErrDisabled
		}
		if err != nil {
			return err
		}
		policy = p
		if !p.Enabled {
			return model.ErrDisabled
		}
		daily, err := q.AIUsageToday(ctx, in.OrganizationID)
		if err != nil {
			return err
		}
		if daily.Calls >= int64(p.DailyCalls) {
			return model.ErrDaily
		}
		if in.ReservedCostMicros > p.DailyBudgetMicros-daily.Cost {
			return model.ErrBudget
		}
		active, err := q.AIActive(ctx, in.OrganizationID)
		if err != nil {
			return err
		}
		if active >= int64(p.Concurrency) {
			return model.ErrConcurrency
		}
		month, err := q.AIQuotaMonth(ctx, in.OrganizationID)
		if err != nil {
			return err
		}
		configured, err := q.AIConfiguredTokens(ctx, in.OrganizationID)
		if err != nil {
			return err
		}
		if !configured {
			return &entitlement.Error{Code: entitlement.CodeFeatureRequired, Feature: "ai.tokens"}
		}
		if _, err = entitlement.Check(values["ai.tokens"], month.Tokens, in.ReservedTokens); err != nil {
			return err
		}
		// Credits are funded by expiring period lots plus purchased lots. A
		// calendar-month cap would deny purchases and overlap renewal periods.
		credits, ok := values["ai.credits"]
		if !ok {
			return &entitlement.Error{Code: entitlement.CodeFeatureRequired, Feature: "ai.credits"}
		}
		if !credits.Unlimited {
			funded, e := q.AIWalletReserve(ctx, db.AIWalletReserveParams{OrganizationID: in.OrganizationID, RunUuid: in.Uuid, Credits: in.ReservedCredits})
			if e != nil {
				return e
			}
			if !funded {
				return model.ErrCredits
			}
		}
		out, err = q.AIInsertRun(ctx, in)
		return err
	})
	return out, policy, err
}

// Finish is idempotent. Ledger release, actual usage signals and final trace commit
// together. Unknown outcomes keep their full debit and reserved cost for reconciliation.
func (s *Store) Finish(ctx context.Context, in db.AIFinishRunParams) error {
	return s.Billing.Within(ctx, func(r *billingrepo.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, in.OrganizationID); err != nil {
			return err
		}
		run, err := r.Q.AIGetRun(ctx, db.AIGetRunParams{OrganizationID: in.OrganizationID, Uuid: in.Uuid})
		if err != nil {
			return err
		}
		if run.Status != "running" {
			return nil
		}
		if in.ChargedCredits > run.ReservedCredits || in.InputTokens+in.OutputTokens > run.ReservedTokens {
			return model.ErrInvalid
		}
		n, err := r.Q.AIFinishRun(ctx, in)
		if err != nil || n == 0 {
			return err
		}
		release := run.ReservedCredits - in.ChargedCredits
		funded, err := r.Q.AIWalletFunded(ctx, db.AIWalletFundedParams{OrganizationID: in.OrganizationID, RunUuid: in.Uuid})
		if err != nil {
			return err
		}
		if release > 0 && funded {
			if err = r.Q.AIWalletRelease(ctx, db.AIWalletReleaseParams{OrganizationID: in.OrganizationID, RunUuid: in.Uuid, Credits: release}); err != nil {
				return err
			}
		}

		quantities := map[string]int64{"ai.tokens": in.InputTokens + in.OutputTokens, "ai.credits": in.ChargedCredits}
		if in.Status == "unknown" {
			quantities["ai.tokens"] = run.ReservedTokens
		}
		for feature, n := range quantities {
			if n == 0 {
				continue
			}
			ev := events.New(events.BillingUsageRecorded).WithOrganization(in.OrganizationID).WithPayload(map[string]any{"feature": feature, "quantity": n, "event_key": "ai:" + in.Uuid.String() + ":" + feature, "occurred_at": run.CreatedAt.Time.Format(time.RFC3339Nano)}).WithIdempotencyKey(in.Uuid.String() + ":" + feature)
			if err = r.Emit(ctx, ev); err != nil {
				return err
			}
		}
		return r.Emit(ctx, events.New("ai.run_completed").WithOrganization(in.OrganizationID).WithPayload(map[string]any{"run_uuid": in.Uuid.String(), "status": in.Status}).WithIdempotencyKey(in.Uuid.String()))
	})
}

// Block records a masked, pre-provider rejection without consuming a wallet reservation.
func (s *Store) Block(ctx context.Context, in db.AIInsertRunParams) error {
	in.Status = "blocked"
	in.ReservedCredits = 0
	in.ReservedCostMicros = 0
	in.ReservedTokens = 0
	_, err := s.Billing.Q.AIInsertRun(ctx, in)
	return err
}

func (s *Store) Get(ctx context.Context, org int64, id uuid.UUID) (db.AiRun, error) {
	return s.Billing.Q.AIGetRun(ctx, db.AIGetRunParams{OrganizationID: org, Uuid: id})
}

func (s *Store) List(ctx context.Context, org int64, limit, offset int32) ([]db.AiRun, int64, error) {
	rows, err := s.Billing.Q.AIListRuns(ctx, db.AIListRunsParams{OrganizationID: org, Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, err
	}
	count, err := s.Billing.Q.AICountRuns(ctx, org)
	return rows, count, err
}

func (s *Store) Organization(ctx context.Context, id uuid.UUID) (int64, error) {
	return s.Billing.Q.BillingGetOrganizationID(ctx, id)
}
func JSON(v any) []byte { out, _ := json.Marshal(v); return out }

// Stale is a bounded platform maintenance query; mutations reapply each run's org lock.
func (s *Store) Stale(ctx context.Context) ([]db.AiRun, error) { return s.Billing.Q.AIStaleRuns(ctx) }
