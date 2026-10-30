package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type TrialHooks struct{ Service *Service }

// SignupPaymentWindow is how long a self-serve signup on a plan without a
// trial waits in pending_payment for its first payment (CF-42).
const SignupPaymentWindow = 72 * time.Hour

func (h TrialHooks) OnDeleted(context.Context, orgmodel.Organization) error { return nil }
func (h TrialHooks) OnCreated(ctx context.Context, tx pgx.Tx, org orgmodel.Organization, owner orgmodel.User) error {
	return h.OnCreatedWithTrial(ctx, tx, org, owner, uuid.Nil)
}
func (h TrialHooks) OnCreatedWithTrial(ctx context.Context, tx pgx.Tx, org orgmodel.Organization, owner orgmodel.User, price uuid.UUID) error {
	// The signup and trial share the caller's transaction: no unbilled signup gap.
	return h.Service.startTrial(ctx, h.Service.repo.WithTx(tx), org.ID, price, owner.ID)
}
func (s *Service) StartTrial(ctx context.Context, org int64, price uuid.UUID, actor int64) error {
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		return s.startTrial(ctx, r, org, price, actor)
	})
}
func (s *Service) startTrial(ctx context.Context, r *repository.Repository, org int64, price uuid.UUID, actor int64) error {
	enrolled, err := r.Q.BillingTrialEnrolled(ctx, org)
	if err != nil {
		return err
	}
	if enrolled {
		return model.ErrConflict
	}
	if price != uuid.Nil {
		// CF-42: a plan picked on the pricing page without a trial opens in
		// pending_payment for a short payment window; paying the first
		// invoice (trial-invoice endpoint) activates it, otherwise the
		// lifecycle sweep expires it. Plans with a trial use the trial path.
		sp, e := r.Q.BillingSignupPrice(ctx, price)
		if e != nil {
			return e
		}
		if sp.TrialDays == 0 {
			now := time.Now().UTC()
			sub, e := r.Q.BillingCreateSubscription(ctx, db.BillingCreateSubscriptionParams{OrganizationID: org, PlanVersionID: sp.PlanVersionID, PriceID: sp.ID, Status: "pending_payment", PeriodStart: repository.TS(now), PeriodEnd: repository.TS(now.Add(SignupPaymentWindow))})
			if e != nil {
				return e
			}
			return recordTransition(ctx, r, sub, "", "Organization signup awaiting first payment", actor)
		}
	}
	if price == uuid.Nil {
		cfg, e := r.Q.BillingGatewaySettings(ctx)
		if e != nil {
			return e
		}
		if cfg.TrialPriceUuid.Valid {
			price = cfg.TrialPriceUuid.Bytes
		} else {
			price, e = r.Q.BillingDefaultTrialPrice(ctx)
			// A fresh installation has no commercial catalog yet. Onboarding still works;
			// its entitlement engine remains read-only until a trial plan is published.
			if errors.Is(e, pgx.ErrNoRows) {
				return nil
			}
			if e != nil {
				return e
			}
		}
	}
	p, err := r.Q.BillingTrialPrice(ctx, price)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	end := now.AddDate(0, 0, int(p.TrialDays))
	sub, err := r.Q.BillingCreateSubscription(ctx, db.BillingCreateSubscriptionParams{OrganizationID: org, PlanVersionID: p.PlanVersionID, PriceID: p.ID, Status: "trialing", PeriodStart: repository.TS(now), PeriodEnd: repository.TS(end), TrialEndsAt: repository.TS(end)})
	if err != nil {
		return err
	}
	if err = r.Q.BillingEnrollTrial(ctx, db.BillingEnrollTrialParams{OrganizationID: org, SubscriptionID: sub.ID}); err != nil {
		return err
	}
	return recordTransition(ctx, r, sub, "", "Organization signup trial", actor)
}
func (s *Service) ExtendTrial(ctx context.Context, org int64, id uuid.UUID, end time.Time, reason string, actor int64) error {
	if strings.TrimSpace(reason) == "" || len(reason) > 1000 || !end.After(time.Now()) || end.After(time.Now().AddDate(1, 0, 0)) {
		return model.ErrInvalid
	}
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		sub, err := r.Q.BillingLockSubscription(ctx, db.BillingLockSubscriptionParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		if sub.Status != "trialing" || !end.After(sub.TrialEndsAt.Time) {
			return model.ErrConflict
		}
		next, err := r.Q.BillingExtendTrial(ctx, db.BillingExtendTrialParams{OrganizationID: org, ID: sub.ID, TrialEndsAt: repository.TS(end)})
		if err != nil {
			return err
		}
		if err = extendPlanLots(ctx, r, next); err != nil {
			return err
		}
		return recordTransition(ctx, r, next, "trialing", reason, actor)
	})
}

// extendPlanLots carries the unspent trial allowance (AI credits, video
// minutes) to the extended period end. Lots are append-only, so each lot that
// would expire early is reversed and re-issued with the new expiry (CF-143).
func extendPlanLots(ctx context.Context, r *repository.Repository, sub db.BillingSubscription) error {
	end := sub.PeriodEnd.Time
	for _, grant := range planAllowances {
		lots, err := r.Q.BillingCreditLots(ctx, db.BillingCreditLotsParams{OrganizationID: sub.OrganizationID, Unit: grant.unit})
		if err != nil {
			return err
		}
		for _, lot := range lots {
			if lot.SubscriptionID != sub.ID || lot.Remaining <= 0 || !lot.ExpiresAt.Valid || !lot.ExpiresAt.Time.Before(end) || !strings.HasPrefix(lot.IdempotencyKey, grant.keyPrefix) {
				continue
			}
			suffix := fmt.Sprintf("%d:%d", lot.ID, end.UnixNano())
			if _, err = r.Q.BillingWalletEntry(ctx, db.BillingWalletEntryParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID, Unit: lot.Unit, Delta: -lot.Remaining, Reason: "Trial extended", IdempotencyKey: "trial-extend:" + suffix, ReversesID: pgtype.Int8{Int64: lot.ID, Valid: true}}); err != nil {
				return err
			}
			if _, err = r.Q.BillingWalletEntry(ctx, db.BillingWalletEntryParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID, Unit: lot.Unit, Delta: lot.Remaining, Reason: grant.reason, IdempotencyKey: grant.keyPrefix + "extend:" + suffix, ExpiresAt: sub.PeriodEnd}); err != nil {
				return err
			}
		}
	}
	return nil
}

// TrialPayments is supplied by gateway configuration. A successful result must
// mean a settled charge, never an accepted request or a browser return.
type TrialPayments interface {
	ChargeTrial(context.Context, db.BillingSubscription) (bool, error)
}

func (s *Service) SetTrialPayments(p TrialPayments) { s.trialPayments = p }
func (s *Service) TrialSweep(ctx context.Context, now time.Time) error {
	rows, err := s.repo.Q.BillingListTrials(ctx, repository.TS(now))
	if err != nil {
		return err
	}
	failures := []error{}
	for _, row := range rows {
		if err = s.trialStep(ctx, row, now); err != nil {
			failures = append(failures, fmt.Errorf("trial %s: %w", row.Uuid, err))
		}
	}
	return errors.Join(failures...)
}
func (s *Service) trialStep(ctx context.Context, row db.BillingSubscription, now time.Time) error {
	var err error
	if !now.Before(row.TrialEndsAt.Time) {
		if row.CancelAtPeriodEnd {
			if err = s.Advance(ctx, row.OrganizationID, row.Uuid, now); err != nil {
				return err
			}
			return nil
		}
		paid := false
		if row.AutoPay && row.PaymentMethodID.Valid && s.trialPayments == nil {
			return payments.ErrNotConfigured
		}
		if row.AutoPay && row.PaymentMethodID.Valid && s.trialPayments != nil {
			paid, err = s.trialPayments.ChargeTrial(ctx, row)
			if err != nil {
				return err
			}
		}
		if paid {
			current, e := s.repo.Q.BillingSubscriptionID(ctx, db.BillingSubscriptionIDParams{OrganizationID: row.OrganizationID, ID: row.ID})
			if e != nil {
				return e
			}
			if current.Status == "active" {
				return nil
			}
			price, e := s.repo.Q.BillingGetPriceID(ctx, row.PriceID)
			if e != nil {
				return e
			}
			start := row.PeriodEnd.Time
			end := start.AddDate(0, 1, 0)
			if price.Interval == "yearly" {
				end = start.AddDate(1, 0, 0)
			}
			if _, err = s.Transition(ctx, row.OrganizationID, row.Uuid, model.Transition{To: "active", Reason: "Trial payment settled", PeriodStart: &start, PeriodEnd: &end}, 0); err != nil {
				return err
			}
		} else if err = s.Advance(ctx, row.OrganizationID, row.Uuid, now); err != nil {
			return err
		}
		return nil
	}
	err = s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, row.OrganizationID); e != nil {
			return e
		}
		sub, e := r.Q.BillingLockSubscription(ctx, db.BillingLockSubscriptionParams{OrganizationID: row.OrganizationID, Uuid: row.Uuid})
		if e != nil {
			return e
		}
		if sub.Status != "trialing" || !now.Before(sub.TrialEndsAt.Time) {
			return nil
		}
		remaining := sub.TrialEndsAt.Time.Sub(now)
		// Send the nearest crossed threshold only after downtime, avoiding a burst.
		days := int32(0)
		for _, d := range []int32{1, 3, 7} {
			if remaining <= time.Duration(d)*24*time.Hour {
				days = d
				break
			}
		}
		if days == 0 {
			return nil
		}
		n, e := r.Q.BillingTrialReminder(ctx, db.BillingTrialReminderParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID, TrialEndsAt: sub.TrialEndsAt, Days: days})
		if e != nil || n == 0 {
			return e
		}
		return r.Emit(ctx, events.New("billing.trial_reminder").WithTenant(sub.OrganizationID).WithEntity("billing_subscriptions", &sub.ID, &sub.Uuid).WithPayload(map[string]any{"days": days, "trial_ends_at": sub.TrialEndsAt.Time}))
	})
	if err != nil {
		return err
	}
	return nil
}

// TrialGateway bridges consented saved cards to the durable gateway executor.
type TrialGateway struct {
	Service  *Service
	Executor *GatewayExecutor
	Vault    *payments.CardVault
	Account  func(context.Context, string, string) (payments.Account, error)
}

func (g *TrialGateway) ChargeTrial(ctx context.Context, sub db.BillingSubscription) (bool, error) {
	if g.Service != nil {
		return g.Service.chargeTrialInvoice(ctx, sub)
	}
	method, err := g.Executor.repo.Q.BillingPaymentMethod(ctx, db.BillingPaymentMethodParams{OrganizationID: sub.OrganizationID, ID: sub.PaymentMethodID.Int64})
	if err != nil {
		return false, err
	}
	price, err := g.Executor.repo.Q.BillingGetPriceID(ctx, sub.PriceID)
	if err != nil {
		return false, err
	}
	// FX conversion is owned by invoicing; charge only an exact matching currency.
	if price.AmountMinor == 0 {
		return true, nil
	}
	provider, err := g.Executor.Registry.Get(method.Provider)
	if err != nil {
		return false, err
	}
	account, err := g.Account(ctx, method.Provider, method.MerchantAccountUuid.String())
	if err != nil {
		return false, err
	}
	owner, token, err := g.Vault.Open(fmt.Sprintf("org:%d", sub.OrganizationID), method.Uuid, method.CustomerTokenEnc, method.CardTokenEnc)
	if err != nil {
		return false, err
	}
	in := payments.ChargeRequest{Amount: price.AmountMinor, Currency: price.Currency, Card: payments.SavedCard{OwnerKey: owner, Token: token}, Recurring: true}
	key := "trial:" + sub.Uuid.String()
	_, err = g.Executor.Charge(ctx, sub.OrganizationID, key, provider, account, in)
	if errors.Is(err, model.ErrConflict) {
		_, err = g.Executor.ReconcileCharge(ctx, sub.OrganizationID, key, provider, account, in)
	}
	var refusal *payments.ProviderError
	if errors.As(err, &refusal) && !refusal.Temporary {
		return false, nil
	}
	return err == nil, err
}

// GatewaySettings contains public routing and trial selection; no credentials.
type GatewaySettings struct {
	DefaultProvider   string            `json:"default_provider"`
	CurrencyProviders map[string]string `json:"currency_providers"`
	TrialPriceUUID    *uuid.UUID        `json:"trial_price_uuid"`
}

func (s *Service) SaveGatewaySettings(ctx context.Context, in GatewaySettings) error {
	for currency, provider := range in.CurrencyProviders {
		if (currency != "TRY" && currency != "USD" && currency != "EUR") || (!validProvider(provider) || provider == "") {
			return model.ErrInvalid
		}
	}
	if !validProvider(in.DefaultProvider) {
		return model.ErrInvalid
	}
	if in.TrialPriceUUID != nil {
		if _, err := s.repo.Q.BillingTrialPrice(ctx, *in.TrialPriceUUID); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(in.CurrencyProviders)
	if err != nil {
		return err
	}
	if in.CurrencyProviders == nil {
		raw = []byte("{}")
	}
	var price pgtype.UUID
	if in.TrialPriceUUID != nil {
		price = pgtype.UUID{Bytes: *in.TrialPriceUUID, Valid: true}
	}
	return s.repo.Q.BillingSaveGatewaySettings(ctx, db.BillingSaveGatewaySettingsParams{DefaultProvider: in.DefaultProvider, CurrencyProviders: raw, TrialPriceUuid: price})
}
func validProvider(code string) bool {
	return code == "" || code == "paytr" || code == "iyzico" || code == "fake"
}
func (s *Service) GatewaySettings(ctx context.Context) (GatewaySettings, error) {
	row, err := s.repo.Q.BillingGatewaySettings(ctx)
	out := GatewaySettings{DefaultProvider: row.DefaultProvider, CurrencyProviders: map[string]string{}}
	if err != nil {
		return out, err
	}
	if row.TrialPriceUuid.Valid {
		u := uuid.UUID(row.TrialPriceUuid.Bytes)
		out.TrialPriceUUID = &u
	}
	err = json.Unmarshal(row.CurrencyProviders, &out.CurrencyProviders)
	return out, err
}
