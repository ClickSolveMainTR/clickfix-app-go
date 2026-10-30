package demoseed

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	billingmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	billingusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
)

// devPlanCode is the plan ensureDevCatalog publishes on an empty catalog.
const devPlanCode = "demo-dev"

// ensureDevCatalog publishes a public 30 day trial plan with every feature
// unlimited when the catalog has no trial plan yet. It exists for fresh
// local and CI databases (E2E); production has a real catalog and the
// command refuses the flag there.
func (s *seeder) ensureDevCatalog(ctx context.Context, billing *billingusecase.Service) error {
	_, err := s.q.BillingDefaultTrialPrice(ctx)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	features, err := billing.Features(ctx)
	if err != nil {
		return err
	}
	on := true
	grants := make([]billingmodel.Grant, 0, len(features))
	for _, f := range features {
		g := billingmodel.Grant{Feature: f.Key}
		if f.Kind == "boolean" {
			g.Enabled = &on
		} else {
			g.Unlimited = true
		}
		grants = append(grants, g)
	}
	plan, err := billing.Create(ctx, billingmodel.PlanInput{
		Code: devPlanCode, Name: "Demo (geliştirme)", Description: "Yerel ve CI ortamları için seed-demo -dev-catalog planı", Visibility: "public",
	})
	if err != nil {
		return fmt.Errorf("plan: %w", err)
	}
	v, err := billing.SaveVersion(ctx, plan.UUID, uuid.Nil, billingmodel.VersionInput{
		TrialDays: 30, Entitlements: grants,
		Prices: []billingmodel.Price{
			{Interval: "monthly", Currency: "TRY", Amount: 99900, VAT: 2000},
			{Interval: "yearly", Currency: "TRY", Amount: 999000, VAT: 2000},
		},
	})
	if err != nil {
		return fmt.Errorf("version: %w", err)
	}
	if _, err := billing.Publish(ctx, plan.UUID, v.UUID, s.users[demoUsers[0].key].ID); err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	return nil
}

// ensureSubscription starts the signup trial for a demo organization that
// was created before a catalog existed.
func (s *seeder) ensureSubscription(ctx context.Context, billing *billingusecase.Service) error {
	_, err := s.q.BillingGetSubscription(ctx, s.org.ID)
	if err == nil || !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if _, err := s.q.BillingDefaultTrialPrice(ctx); errors.Is(err, pgx.ErrNoRows) {
		s.opts.Log.Warn("seed-demo: no published trial plan; the demo organization stays read-only until one exists")
		return nil
	}
	err = billing.StartTrial(ctx, s.org.ID, uuid.Nil, s.users[demoUsers[0].key].ID)
	if errors.Is(err, billingmodel.ErrConflict) {
		s.opts.Log.Warn("seed-demo: the demo trial was already used; extend it from platform billing")
		return nil
	}
	return err
}
