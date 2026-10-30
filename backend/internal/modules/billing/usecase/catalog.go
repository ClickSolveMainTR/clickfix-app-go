// Package usecase implements versioned billing and subscription rules.
package usecase

import (
	"context"
	"fmt"
	platformcrypto "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"sort"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
)

type Service struct {
	repo            *repository.Repository
	trialPayments   TrialPayments
	gateway         *TrialGateway
	paymentBox      *platformcrypto.SecretBox
	paymentFrontend string
	// paymentAPI is the public API origin (PUBLIC_API_URL): the provider
	// return of a hosted checkout posts here, outside the browser BFF.
	paymentAPI    string
	einvoice      EInvoiceProvider
	seller        ProfileInput
	invoiceSeries string
	statements    StatementSource
}

func New(repo *repository.Repository) *Service { return &Service{repo: repo} }
func (s *Service) List(ctx context.Context, public bool, q string, limit, offset int32) ([]model.Plan, int64, error) {
	rows, err := s.repo.Q.BillingListPlans(ctx, db.BillingListPlansParams{PublicOnly: public, Q: q, LimitCount: limit, OffsetCount: offset})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Plan, 0, len(rows))
	for _, row := range rows {
		p, err := s.repo.Plan(ctx, row, public)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	total, err := s.repo.Q.BillingCountPlans(ctx, db.BillingCountPlansParams{PublicOnly: public, Q: q})
	return out, total, err
}
func (s *Service) Get(ctx context.Context, id uuid.UUID) (model.Plan, error) {
	p, err := s.repo.Q.BillingGetPlan(ctx, id)
	if err != nil {
		return model.Plan{}, repository.Normalize(err)
	}
	return s.repo.Plan(ctx, p, false)
}
func (s *Service) Create(ctx context.Context, in model.PlanInput) (model.Plan, error) {
	if err := in.Validate(true); err != nil {
		return model.Plan{}, err
	}
	p, err := s.repo.Q.BillingCreatePlan(ctx, db.BillingCreatePlanParams{Code: in.Code, Name: in.Name, Description: in.Description, Visibility: in.Visibility, SortOrder: in.SortOrder})
	if err != nil {
		return model.Plan{}, repository.Normalize(err)
	}
	return s.repo.Plan(ctx, p, false)
}
func (s *Service) Update(ctx context.Context, id uuid.UUID, in model.PlanInput) (model.Plan, error) {
	if err := in.Validate(false); err != nil {
		return model.Plan{}, err
	}
	p, err := s.repo.Q.BillingUpdatePlan(ctx, db.BillingUpdatePlanParams{Uuid: id, Name: in.Name, Description: in.Description, Visibility: in.Visibility, SortOrder: in.SortOrder})
	if err != nil {
		return model.Plan{}, repository.Normalize(err)
	}
	return s.repo.Plan(ctx, p, false)
}
func (s *Service) Archive(ctx context.Context, id uuid.UUID) error {
	n, err := s.repo.Q.BillingArchivePlan(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}
func (s *Service) Features(ctx context.Context) ([]model.Feature, error) {
	rows, err := s.repo.Q.BillingListFeatures(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Feature, 0, len(rows))
	for _, f := range rows {
		feature := model.Feature{UUID: f.Uuid, Key: f.Key, Kind: f.Kind, LabelTR: f.LabelTr, LabelEN: f.LabelEn, Unit: f.Unit, Aggregation: f.Aggregation, Period: f.Period, DefaultUnlimited: f.DefaultUnlimited}
		if f.DefaultBool.Valid {
			v := f.DefaultBool.Bool
			feature.DefaultEnabled = &v
		}
		if f.DefaultLimit.Valid {
			v := f.DefaultLimit.Int64
			feature.DefaultLimit = &v
		}
		out = append(out, feature)
	}
	return out, nil
}

func (s *Service) SaveVersion(ctx context.Context, planID, versionID uuid.UUID, in model.VersionInput) (model.Version, error) {
	var result model.Version
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		p, err := r.Q.BillingLockPlan(ctx, planID)
		if err != nil {
			return err
		}
		if p.ArchivedAt.Valid {
			return model.ErrConflict
		}
		fs, err := r.Features(ctx)
		if err != nil {
			return err
		}
		if err = in.Validate(repository.FeatureKinds(fs)); err != nil {
			return err
		}
		var v db.BillingPlanVersion
		if versionID == uuid.Nil {
			v, err = r.Q.BillingCreateVersion(ctx, db.BillingCreateVersionParams{PlanID: p.ID, TrialDays: in.TrialDays})
		} else {
			v, err = r.Q.BillingLockVersion(ctx, versionID)
			if err != nil {
				return err
			}
			if v.PlanID != p.ID {
				return model.ErrNotFound
			}
			if v.Status != "draft" {
				return model.ErrConflict
			}
			v, err = r.Q.BillingUpdateVersion(ctx, db.BillingUpdateVersionParams{ID: v.ID, TrialDays: in.TrialDays})
		}
		if err != nil {
			return err
		}
		if err = r.ReplaceVersion(ctx, v.ID, in, fs); err != nil {
			return err
		}
		result, err = r.Version(ctx, v)
		return err
	})
	return result, err
}
func (s *Service) Publish(ctx context.Context, planID, versionID uuid.UUID, actor int64) (model.Version, error) {
	var out model.Version
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		p, err := r.Q.BillingLockPlan(ctx, planID)
		if err != nil {
			return err
		}
		if p.ArchivedAt.Valid {
			return model.ErrConflict
		}
		v, err := r.Q.BillingLockVersion(ctx, versionID)
		if err != nil {
			return err
		}
		if v.PlanID != p.ID {
			return model.ErrNotFound
		}
		if v.Status == "published" {
			out, err = r.Version(ctx, v)
			return err
		}
		current, err := r.Version(ctx, v)
		if err != nil {
			return err
		}
		fs, err := r.Features(ctx)
		if err != nil {
			return err
		}
		if err = current.Validate(repository.FeatureKinds(fs)); err != nil {
			return err
		}
		// Versions publish monotonically; an older draft cannot replace current public pricing.
		all, err := r.Q.BillingListVersions(ctx, p.ID)
		if err != nil {
			return err
		}
		for _, other := range all {
			if other.Status == "published" && other.Version > v.Version {
				return model.ErrConflict
			}
		}
		v, err = r.Q.BillingPublishVersion(ctx, v.ID)
		if err != nil {
			return err
		}
		if err = r.Emit(ctx, events.New(events.BillingPlanPublished).WithActor(actor).WithEntity("billing_plans", &p.ID, &p.Uuid).WithPayload(map[string]any{"version_uuid": v.Uuid})); err != nil {
			return err
		}
		out, err = r.Version(ctx, v)
		return err
	})
	return out, err
}
func (s *Service) ScheduleMigration(ctx context.Context, in model.Migration, actor int64) error {
	n := len(in.SubscriptionUUIDs)
	if n == 0 || n > 100 || n != len(in.OrganizationUUIDs) || in.Reason == "" {
		return fmt.Errorf("%w: 1..100 matching subscriptions and organizations, reason required", model.ErrInvalid)
	}
	// Consistent lock order avoids deadlocks between overlapping batches.
	type pair struct{ org, sub uuid.UUID }
	pairs := make([]pair, n)
	seen := map[uuid.UUID]bool{}
	for i := range pairs {
		if seen[in.OrganizationUUIDs[i]] {
			return model.ErrInvalid
		}
		seen[in.OrganizationUUIDs[i]] = true
		pairs[i] = pair{in.OrganizationUUIDs[i], in.SubscriptionUUIDs[i]}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].org.String() < pairs[j].org.String() })
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		v, err := r.Q.BillingGetVersion(ctx, in.VersionUUID)
		if err != nil {
			return err
		}
		if v.Status != "published" {
			return model.ErrConflict
		}
		prices, err := r.Q.BillingListPrices(ctx, v.ID)
		if err != nil {
			return err
		}
		for _, pair := range pairs {
			subID := pair.sub
			org, err := r.Q.BillingLockOrganization(ctx, pair.org)
			if err != nil {
				return err
			}
			sub, err := r.Q.BillingLockSubscription(ctx, db.BillingLockSubscriptionParams{OrganizationID: org, Uuid: subID})
			if err != nil {
				return err
			}
			if sub.Status == "canceled" || sub.Status == "expired" || sub.CancelAtPeriodEnd {
				return model.ErrConflict
			}
			old, err := r.Q.BillingGetVersionID(ctx, sub.PlanVersionID)
			if err != nil {
				return err
			}
			if old.PlanID != v.PlanID {
				return model.ErrConflict
			}
			oldPrices, err := r.Q.BillingListPrices(ctx, sub.PlanVersionID)
			if err != nil {
				return err
			}
			var current db.BillingPrice
			for _, p := range oldPrices {
				if p.ID == sub.PriceID {
					current = p
				}
			}
			matched := false
			for _, p := range prices {
				if p.Currency != current.Currency || p.Interval != current.Interval {
					continue
				}
				matched = true
				if err = r.Q.BillingSchedulePlan(ctx, db.BillingSchedulePlanParams{OrganizationID: org, ID: sub.ID, PendingPlanVersionID: repository.Int(&v.ID), PendingPriceID: repository.Int(&p.ID)}); err != nil {
					return err
				}
			}
			if !matched {
				return fmt.Errorf("%w: target lacks matching interval/currency", model.ErrConflict)
			}
			if err = r.Emit(ctx, events.New(events.SubscriptionPlanScheduled).WithActor(actor).WithTenant(org).WithEntity("billing_subscriptions", &sub.ID, &sub.Uuid).WithPayload(map[string]any{"organization_id": org, "version_uuid": v.Uuid, "reason": in.Reason})); err != nil {
				return err
			}
		}
		return nil
	})
}
