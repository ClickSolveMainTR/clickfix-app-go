package billing_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/google/uuid"
)

// CF-143 commerce simulation: one organization walks the whole platform
// commerce lifecycle the way a platform admin and its owner would drive it,
// against the real schema. Each step asserts the balances an operator would
// look at; a failing step names the scenario it belongs to.

type scenario struct {
	*billingFixture
	starter, pro model.Version
	orgs         *orgusecase.Service
	mgmt         *orgusecase.Management
	org          int64
	orgUUID      uuid.UUID
	owner        int64
	bank         uuid.UUID
}

// scenarioPlan publishes a plan version whose AI credits / video minutes are
// funded each period; limits elsewhere stay generous.
func scenarioPlan(t *testing.T, f *billingFixture, name string, trialDays int32, amount, aiCredits, videoMinutes int64) model.Version {
	t.Helper()
	plan, err := f.svc.Create(f.ctx, model.PlanInput{Code: "sim-" + uuid.NewString(), Name: name, Visibility: "public"})
	if err != nil {
		t.Fatal(err)
	}
	in := model.VersionInput{TrialDays: trialDays, Prices: []model.Price{{Interval: "monthly", Currency: "TRY", Amount: amount, VAT: 2000}}}
	for _, g := range f.input.Entitlements {
		g := g
		if g.Limit != nil {
			v := int64(100)
			switch g.Feature {
			case "ai.credits":
				v = aiCredits
			case "video.minutes":
				v = videoMinutes
			}
			g.Limit = &v
		}
		in.Entitlements = append(in.Entitlements, g)
	}
	v, err := f.svc.SaveVersion(f.ctx, plan.UUID, uuid.Nil, in)
	if err != nil {
		t.Fatal(err)
	}
	v, err = f.svc.Publish(f.ctx, plan.UUID, v.UUID, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func newScenario(t *testing.T) *scenario {
	t.Helper()
	s := &scenario{billingFixture: fixture(t)}
	s.starter = scenarioPlan(t, s.billingFixture, "Başlangıç", 14, 99000, 100, 60)
	s.pro = scenarioPlan(t, s.billingFixture, "Profesyonel", 14, 249000, 500, 300)
	repo := orgrepository.NewPostgres(s.pool, s.q)
	s.orgs = orgusecase.New(repo, usecase.TrialHooks{Service: s.svc})
	s.mgmt = orgusecase.NewManagement(s.orgs, repo, orgusecase.ManagementConfig{FrontendURL: "https://example.test"})
	if err := s.pool.QueryRow(s.ctx, "INSERT INTO users(email,password_hash,name,surname) VALUES($1,'x','Ayşe','Yılmaz') RETURNING id", "owner-"+uuid.NewString()+"@example.test").Scan(&s.owner); err != nil {
		t.Fatal(err)
	}
	bank, err := s.svc.SaveBankAccount(s.ctx, usecase.BankAccountInput{BankName: "Simülasyon Bankası", AccountName: "ClickFix", IBAN: "TR330006100519786457841326", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	s.bank = bank.UUID
	return s
}

func (s *scenario) wallet(t *testing.T) usecase.WalletSummary {
	t.Helper()
	w, err := s.svc.Wallet(s.ctx, s.org, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// payByTransfer reports a bank transfer for the invoice and approves it as
// the platform admin, the path every tenant has when no gateway is set up.
func (s *scenario) payByTransfer(t *testing.T, invoice uuid.UUID, amount int64) uuid.UUID {
	t.Helper()
	var attachment uuid.UUID
	if err := s.pool.QueryRow(s.ctx, "INSERT INTO attachments(organization_id,filename,content_type,size_bytes,sha256,object_key) VALUES($1,'dekont.pdf','application/pdf',1,'digest',$2) RETURNING uuid", s.org, "test/"+uuid.NewString()).Scan(&attachment); err != nil {
		t.Fatal(err)
	}
	id, err := s.svc.ReportTransfer(s.ctx, s.org, usecase.TransferInput{InvoiceUUID: invoice, BankAccountUUID: s.bank, AttachmentUUID: attachment, AmountMinor: amount})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.svc.ReviewTransfer(s.ctx, s.org, id, usecase.TransferReview{Approve: true}, s.actor); err != nil {
		t.Fatal(err)
	}
	var payment uuid.UUID
	if err = s.pool.QueryRow(s.ctx, "SELECT p.uuid FROM billing_payments p JOIN billing_invoices i ON i.id=p.invoice_id WHERE i.uuid=$1 AND p.provider='bank_transfer' AND p.status='succeeded' ORDER BY p.id DESC LIMIT 1", invoice).Scan(&payment); err != nil {
		t.Fatal(err)
	}
	return payment
}

func TestCommerceScenarioPlatformAdminLifecycle(t *testing.T) {
	s := newScenario(t)
	noGateway := usecase.New(repository.New(s.pool, s.q))
	for _, svc := range []*usecase.Service{noGateway, s.svc} {
		if err := svc.ConfigureInvoicing(&usecase.FakeEInvoiceProvider{}, validBillingProfile(), "CFX", "test"); err != nil {
			t.Fatal(err)
		}
	}

	// 1. Platform admin opens the organization for an existing account on
	// the starter plan with its trial.
	var owner db.User
	if err := s.pool.QueryRow(s.ctx, "SELECT uuid FROM users WHERE id=$1", s.owner).Scan(&owner.Uuid); err != nil {
		t.Fatal(err)
	}
	org, err := s.orgs.CreateOrganization(s.ctx, orgmodel.CreateOrganizationInput{Name: "Simülasyon AŞ", OwnerUserUUID: owner.Uuid, TrialPriceUUID: s.starter.Prices[0].UUID})
	if err != nil {
		t.Fatal(err)
	}
	s.org, s.orgUUID = org.ID, org.UUID
	sub, err := s.svc.Subscription(s.ctx, s.org)
	if err != nil || sub == nil || sub.Status != "trialing" || sub.VersionUUID != s.starter.UUID || sub.TrialDaysRemaining != 14 {
		t.Fatalf("1. trial subscription %+v %v", sub, err)
	}
	if scope, err := s.orgs.Resolve(s.ctx, org.UUID.String(), s.owner); err != nil || scope.MemberRole != orgctx.RoleOwner {
		t.Fatalf("1. owner membership %+v %v", scope, err)
	}
	if w := s.wallet(t); w.AICredits != 100 || w.VideoMinutes != 60 {
		t.Fatalf("1. trial allowance ai=%d video=%d", w.AICredits, w.VideoMinutes)
	}
	if _, err = s.svc.SaveProfile(s.ctx, s.org, validBillingProfile()); err != nil {
		t.Fatal(err)
	}

	// 2. Sales extends the trial by a week: trial credits must last as long
	// as the trial does.
	trialEnd := *sub.TrialEndsAt
	if err = s.svc.ExtendTrial(s.ctx, s.org, sub.UUID, trialEnd.AddDate(0, 0, 7), "Satış görüşmesi uzadı", s.actor); err != nil {
		t.Fatal(err)
	}
	if err = s.svc.ExpireWallet(s.ctx, s.org, trialEnd.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if w := s.wallet(t); w.AICredits != 100 || w.VideoMinutes != 60 {
		t.Fatalf("2. extended trial lost its allowance ai=%d video=%d", w.AICredits, w.VideoMinutes)
	}

	// 3. No payment provider: the owner pays the first period by bank
	// transfer; approval activates the plan from the trial end.
	if _, err = noGateway.Checkout(s.ctx, s.org, usecase.CheckoutInput{InvoiceUUID: uuid.New(), Provider: "fake", IdempotencyKey: "x"}, payments.Buyer{}); !errors.Is(err, payments.ErrNotConfigured) {
		t.Fatalf("3. checkout without provider: %v", err)
	}
	first, err := noGateway.TrialInvoice(s.ctx, s.org, sub.UUID)
	if err != nil {
		t.Fatal(err)
	}
	s.payByTransfer(t, first.UUID, first.TotalMinor)
	sub, err = s.svc.Subscription(s.ctx, s.org)
	if err != nil || sub.Status != "active" {
		t.Fatalf("3. paid trial %+v %v", sub, err)
	}
	snap, err := s.ent.Resolve(s.ctx, s.org)
	if err != nil || !snap.Writable {
		t.Fatalf("3. writable after payment %+v %v", snap, err)
	}

	// Paying early keeps the trial allowance until the trial ends and funds
	// the first paid period on top of it.
	if w := s.wallet(t); w.AICredits != 200 || w.VideoMinutes != 120 {
		t.Fatalf("3. paid period allowance ai=%d video=%d", w.AICredits, w.VideoMinutes)
	}

	// 4. Upgrade to Pro through the sandbox provider: proration invoice,
	// plan applied only after settlement, AI allowance topped up to 500.
	fake := payments.NewFake("https://example.test")
	configureGateway(t, s.billingFixture, fake)
	change, err := s.svc.ApplyPlanChange(s.ctx, s.org, usecase.PlanChangeInput{PriceUUID: s.pro.Prices[0].UUID, Seats: 1, IdempotencyKey: "upgrade"})
	if err != nil || change.State != "awaiting_payment" || !change.InvoiceID.Valid {
		t.Fatalf("4. upgrade %+v %v", change, err)
	}
	var upgradeInvoice uuid.UUID
	if err = s.pool.QueryRow(s.ctx, "SELECT uuid FROM billing_invoices WHERE id=$1", change.InvoiceID.Int64).Scan(&upgradeInvoice); err != nil {
		t.Fatal(err)
	}
	if sub, _ = s.svc.Subscription(s.ctx, s.org); sub.VersionUUID != s.starter.UUID {
		t.Fatal("4. upgrade applied before payment")
	}
	f := s.billingFixture
	saved := f.org
	f.org = s.org
	payWithFake(t, f, fake, upgradeInvoice, "upgrade-pay")
	f.org = saved
	if sub, _ = s.svc.Subscription(s.ctx, s.org); sub.VersionUUID != s.pro.UUID {
		t.Fatalf("4. upgrade not applied %+v", sub)
	}
	if w := s.wallet(t); w.AICredits != 600 || w.VideoMinutes != 360 {
		t.Fatalf("4. pro allowance ai=%d video=%d", w.AICredits, w.VideoMinutes)
	}

	// 5. Downgrade back to the starter plan waits for the period end.
	change, err = s.svc.ApplyPlanChange(s.ctx, s.org, usecase.PlanChangeInput{PriceUUID: s.starter.Prices[0].UUID, Seats: 1, IdempotencyKey: "downgrade"})
	if err != nil || change.State != "scheduled" {
		t.Fatalf("5. downgrade %+v %v", change, err)
	}
	if sub, _ = s.svc.Subscription(s.ctx, s.org); sub.VersionUUID != s.pro.UUID || sub.PendingVersionUUID == nil || *sub.PendingVersionUUID != s.starter.UUID {
		t.Fatalf("5. downgrade must be pending %+v", sub)
	}

	// 6. Credit packs: an AI pack paid by bank transfer, a video-minute pack
	// through the sandbox provider. Nothing is granted before payment.
	aiPack, err := s.svc.CreateCreditPackage(s.ctx, usecase.CreditPackageInput{Code: "SIM_AI_" + strings.ToUpper(uuid.NewString()[:6]), Name: "1.000 AI kredisi", Credits: 1000, AmountMinor: 50000, ExpiresDays: ptr32(365)})
	if err != nil {
		t.Fatal(err)
	}
	videoPack, err := s.svc.CreateCreditPackage(s.ctx, usecase.CreditPackageInput{Unit: usecase.UnitVideoMinutes, Code: "SIM_VID_" + strings.ToUpper(uuid.NewString()[:6]), Name: "500 dakika", Credits: 500, AmountMinor: 75000})
	if err != nil {
		t.Fatal(err)
	}
	aiInvoice, err := noGateway.PurchaseCredits(s.ctx, s.org, usecase.CreditPurchaseInput{PackageUUID: aiPack.UUID, IdempotencyKey: "ai-pack"})
	if err != nil {
		t.Fatal(err)
	}
	if w := s.wallet(t); w.AICredits != 600 {
		t.Fatalf("6. AI pack granted before payment: %d", w.AICredits)
	}
	aiPayment := s.payByTransfer(t, aiInvoice.Uuid, aiInvoice.TotalMinor)
	videoInvoice, err := s.svc.PurchaseCredits(s.ctx, s.org, usecase.CreditPurchaseInput{PackageUUID: videoPack.UUID, IdempotencyKey: "video-pack"})
	if err != nil {
		t.Fatal(err)
	}
	f.org = s.org
	videoPayment := payWithFake(t, f, fake, videoInvoice.Uuid, "video-pay")
	f.org = saved
	if w := s.wallet(t); w.AICredits != 1600 || w.VideoMinutes != 860 {
		t.Fatalf("6. packs ai=%d video=%d", w.AICredits, w.VideoMinutes)
	}

	// 7. Usage: AI runs reserve credits (plan lots expire first), video
	// minutes are metered after the call.
	var ok bool
	if err = s.pool.QueryRow(s.ctx, "SELECT ai_wallet_reserve($1,$2,$3)", s.org, uuid.New(), int64(600)).Scan(&ok); err != nil || !ok {
		t.Fatalf("7. AI reserve %v %v", ok, err)
	}
	debited, left, err := s.svc.MeterUsage(s.ctx, s.org, usecase.UnitVideoMinutes, 45, "call:"+uuid.NewString(), "Görüntülü görüşme")
	if err != nil || debited != 45 || left != 815 {
		t.Fatalf("7. video metering %d %d %v", debited, left, err)
	}
	if w := s.wallet(t); w.AICredits != 1000 {
		t.Fatalf("7. AI balance %d", w.AICredits)
	}

	// 8. Manual adjustment with a reason; never below zero.
	if _, err = s.svc.WalletCommand(s.ctx, s.org, usecase.WalletInput{Unit: usecase.UnitAICredits, Delta: 250, Reason: "Hizmet kesintisi telafisi", IdempotencyKey: "goodwill"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.svc.WalletCommand(s.ctx, s.org, usecase.WalletInput{Unit: usecase.UnitAICredits, Delta: -5000, Reason: "Hatalı yükleme", IdempotencyKey: "too-much"}); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("8. negative wallet allowed: %v", err)
	}
	if w := s.wallet(t); w.AICredits != 1250 {
		t.Fatalf("8. adjusted balance %d", w.AICredits)
	}

	// 9. Refund of the AI pack paid by transfer: the refund goes back by
	// bank and claws back only the unspent purchased credits.
	if err = s.svc.RefundPayment(s.ctx, s.org, aiPayment, usecase.RefundInput{Destination: "manual_bank", EvidenceReference: "EFT-2026-1029", AmountMinor: aiInvoice.TotalMinor, IdempotencyKey: "ai-refund"}); err != nil {
		t.Fatalf("9. refund %v", err)
	}
	if w := s.wallet(t); w.AICredits != 250 {
		t.Fatalf("9. AI pack refund must claw back the unspent pack: %d", w.AICredits)
	}

	// A video-minute pack refund claws back minutes, not AI credits.
	before := s.wallet(t)
	if err = s.svc.RefundPayment(s.ctx, s.org, videoPayment, usecase.RefundInput{Destination: "wallet", AmountMinor: videoInvoice.TotalMinor, IdempotencyKey: "video-refund"}); err != nil {
		t.Fatalf("9. video refund %v", err)
	}
	if w := s.wallet(t); w.VideoMinutes != before.VideoMinutes-500 || w.AICredits != before.AICredits || w.TRYMinor != before.TRYMinor+videoInvoice.TotalMinor {
		t.Fatalf("9. video refund wallet ai=%d video=%d try=%d", w.AICredits, w.VideoMinutes, w.TRYMinor)
	}

	// 10. Organization suspension blocks members; reactivation restores.
	if _, err = s.orgs.SetStatus(s.ctx, s.orgUUID, orgctx.StatusSuspended); err != nil {
		t.Fatal(err)
	}
	if _, err = s.orgs.Resolve(s.ctx, s.orgUUID.String(), s.owner); !errors.Is(err, orgctx.ErrSuspended) {
		t.Fatalf("10. suspended org resolved: %v", err)
	}
	if _, err = s.orgs.SetStatus(s.ctx, s.orgUUID, orgctx.StatusActive); err != nil {
		t.Fatal(err)
	}
	if _, err = s.orgs.Resolve(s.ctx, s.orgUUID.String(), s.owner); err != nil {
		t.Fatalf("10. reactivated org: %v", err)
	}

	// 11. Dunning suspension of the subscription and reactivation.
	for _, step := range []model.Transition{
		{To: "past_due", Reason: "Ödeme alınamadı"},
		{To: "grace", Reason: "Ek süre", GraceEndsAt: ptrTime(time.Now().Add(72 * time.Hour))},
		{To: "suspended", Reason: "Ek süre doldu"},
	} {
		if _, err = s.svc.Transition(s.ctx, s.org, sub.UUID, step, s.actor); err != nil {
			t.Fatalf("11. %s: %v", step.To, err)
		}
	}
	if snap, err = s.ent.Resolve(s.ctx, s.org); err != nil || snap.Writable {
		t.Fatalf("11. suspended subscription writable %+v %v", snap.Writable, err)
	}
	if _, err = s.svc.Transition(s.ctx, s.org, sub.UUID, model.Transition{To: "active", Reason: "Borç ödendi"}, s.actor); err != nil {
		t.Fatal(err)
	}
	if snap, err = s.ent.Resolve(s.ctx, s.org); err != nil || !snap.Writable {
		t.Fatalf("11. reactivated subscription %+v %v", snap.Writable, err)
	}
}

// A feature added after plans were published must reach existing
// subscribers through its catalog default, without editing published
// versions (the migration 000194 incident), and new drafts must include it.
func TestCommerceScenarioNewFeatureAfterPublish(t *testing.T) {
	s := newScenario(t)
	org, err := s.orgs.CreateOrganization(s.ctx, orgmodel.CreateOrganizationInput{Name: "Özellik AŞ", OwnerUserID: s.owner, TrialPriceUUID: s.starter.Prices[0].UUID})
	if err != nil {
		t.Fatal(err)
	}
	key := "module.sim_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	t.Cleanup(func() {
		_, _ = s.pool.Exec(s.ctx, "DELETE FROM billing_features WHERE key=$1", key)
	})
	if _, err = s.pool.Exec(s.ctx, "INSERT INTO billing_features(key,kind,label_tr,label_en,default_bool) VALUES($1,'boolean','Simülasyon modülü','Simulation module',true)", key); err != nil {
		t.Fatal(err)
	}
	snap, err := s.ent.Resolve(s.ctx, org.ID)
	if err != nil || !snap.Values[key].Enabled {
		t.Fatalf("grandfathered default %+v %v", snap.Values[key], err)
	}
	// Published versions stay immutable.
	if _, err = s.pool.Exec(s.ctx, "INSERT INTO billing_plan_entitlements(plan_version_id,feature_id,kind,value_bool) SELECT v.id,f.id,'boolean',false FROM billing_plan_versions v, billing_features f WHERE v.uuid=$1 AND f.key=$2", s.starter.UUID, key); err == nil {
		t.Fatal("published version accepted a new grant")
	}
	// A new version that turns the module off publishes and migrates
	// explicitly; until then the subscriber keeps the default.
	in := model.VersionInput{TrialDays: 14, Prices: []model.Price{{Interval: "monthly", Currency: "TRY", Amount: 99000, VAT: 2000}}}
	features, err := s.svc.Features(s.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, feature := range features {
		g := model.Grant{Feature: feature.Key, Enforcement: "hard", WarnPercent: 80}
		if feature.Kind == "boolean" {
			v := feature.Key != key
			g.Enabled = &v
		} else {
			v := int64(100)
			g.Limit = &v
		}
		in.Entitlements = append(in.Entitlements, g)
	}
	plan, err := s.svc.Get(s.ctx, mustPlanOf(t, s, s.starter.UUID))
	if err != nil {
		t.Fatal(err)
	}
	next, err := s.svc.SaveVersion(s.ctx, plan.UUID, uuid.Nil, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.svc.Publish(s.ctx, plan.UUID, next.UUID, s.actor); err != nil {
		t.Fatal(err)
	}
	if snap, err = s.ent.Resolve(s.ctx, org.ID); err != nil || !snap.Values[key].Enabled {
		t.Fatalf("publish changed a grandfathered subscriber %+v %v", snap.Values[key], err)
	}
}

func mustPlanOf(t *testing.T, s *scenario, version uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := s.pool.QueryRow(s.ctx, "SELECT p.uuid FROM billing_plans p JOIN billing_plan_versions v ON v.plan_id=p.id WHERE v.uuid=$1", version).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func ptr32(v int32) *int32 { return &v }

// Wallet automation: the low AI credit alert fires once a day, auto top-up
// buys and charges the chosen pack on the saved card, and packs limited to
// another plan are neither listed nor purchasable.
func TestCommerceScenarioWalletAutomation(t *testing.T) {
	s := newScenario(t)
	if err := s.svc.ConfigureInvoicing(&usecase.FakeEInvoiceProvider{}, validBillingProfile(), "CFX", "test"); err != nil {
		t.Fatal(err)
	}
	org, err := s.orgs.CreateOrganization(s.ctx, orgmodel.CreateOrganizationInput{Name: "Cüzdan AŞ", OwnerUserID: s.owner, TrialPriceUUID: s.starter.Prices[0].UUID})
	if err != nil {
		t.Fatal(err)
	}
	s.org, s.orgUUID = org.ID, org.UUID
	t.Cleanup(func() {
		// Sweeps are global: leave no auto-pay or auto top-up behind.
		_, _ = s.pool.Exec(s.ctx, "UPDATE billing_wallet_settings SET auto_topup=false WHERE organization_id=$1", s.org)
		_, _ = s.pool.Exec(s.ctx, "UPDATE billing_subscriptions SET auto_pay=false,auto_pay_consented_at=NULL,payment_method_id=NULL WHERE organization_id=$1", s.org)
	})
	if _, err = s.svc.SaveProfile(s.ctx, s.org, validBillingProfile()); err != nil {
		t.Fatal(err)
	}
	alerts := func() int {
		var n int
		if err := s.pool.QueryRow(s.ctx, "SELECT count(*) FROM outbox_events WHERE event_name='billing.wallet_low' AND organization_id=$1", s.org).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Default threshold is a fifth of the plan allowance (100 → 20).
	var ok bool
	if err = s.pool.QueryRow(s.ctx, "SELECT ai_wallet_reserve($1,$2,$3)", s.org, uuid.New(), int64(75)).Scan(&ok); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err = s.svc.WalletSweep(s.ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if n := alerts(); n != 0 {
		t.Fatalf("alert above threshold: %d", n)
	}
	if err = s.pool.QueryRow(s.ctx, "SELECT ai_wallet_reserve($1,$2,$3)", s.org, uuid.New(), int64(10)).Scan(&ok); err != nil || !ok {
		t.Fatal(ok, err)
	}
	for range 2 {
		if err = s.svc.WalletSweep(s.ctx, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if n := alerts(); n != 1 {
		t.Fatalf("low balance alerts %d, want 1", n)
	}

	// Packs: one for every plan, one only for Pro.
	open, err := s.svc.CreateCreditPackage(s.ctx, usecase.CreditPackageInput{Code: "SIM_ALL_" + strings.ToUpper(uuid.NewString()[:6]), Name: "500 AI kredisi", Credits: 500, AmountMinor: 25000})
	if err != nil {
		t.Fatal(err)
	}
	proOnly, err := s.svc.CreateCreditPackage(s.ctx, usecase.CreditPackageInput{Code: "SIM_PRO_" + strings.ToUpper(uuid.NewString()[:6]), Name: "Pro paketi", Credits: 5000, AmountMinor: 200000, PlanUUIDs: []uuid.UUID{mustPlanOf(t, s, s.pro.UUID)}})
	if err != nil || len(proOnly.PlanUUIDs) != 1 {
		t.Fatalf("pro pack %+v %v", proOnly, err)
	}
	visible, err := s.svc.OrganizationCreditPackages(s.ctx, s.org)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range visible {
		if p.UUID == proOnly.UUID {
			t.Fatal("pro-only pack listed for a starter subscriber")
		}
	}
	if _, err = s.svc.PurchaseCredits(s.ctx, s.org, usecase.CreditPurchaseInput{PackageUUID: proOnly.UUID, IdempotencyKey: "pro-only"}); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("pro-only pack bought: %v", err)
	}
	archived := false
	if _, err = s.svc.UpdateCreditPackage(s.ctx, proOnly.UUID, usecase.CreditPackageInput{Name: proOnly.Name, Credits: proOnly.Credits, AmountMinor: proOnly.AmountMinor, Active: &archived}); err != nil {
		t.Fatal(err)
	}

	// Auto top-up needs a saved card with auto-pay.
	if err = s.svc.SaveWalletSettings(s.ctx, s.org, usecase.WalletSettingsInput{AutoTopup: true, Threshold: 50, PackageUUID: open.UUID, Consent: true}); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("auto top-up without a card: %v", err)
	}
	fake := payments.NewFake("https://example.test")
	account := configureGateway(t, s.billingFixture, fake)
	vault, err := payments.NewCardVault(paymentTestKey)
	if err != nil {
		t.Fatal(err)
	}
	cardID := uuid.New()
	owner, token, err := vault.Seal(fmt.Sprintf("org:%d", s.org), cardID, payments.SavedCard{OwnerKey: "owner", Token: "card", Last4: "4242"})
	if err != nil {
		t.Fatal(err)
	}
	method, err := s.q.BillingSaveCard(s.ctx, db.BillingSaveCardParams{Uuid: cardID, OrganizationID: s.org, Provider: "fake", MerchantAccountUuid: uuid.MustParse(account.UUID), CustomerTokenEnc: owner, CardTokenEnc: token, TokenHash: vault.TokenHash(fmt.Sprintf("org:%d", s.org), "fake", "card"), Last4: "4242"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(s.ctx, "UPDATE billing_subscriptions SET payment_method_id=$2,auto_pay=true,auto_pay_consented_at=now() WHERE organization_id=$1 AND status='trialing'", s.org, method.ID); err != nil {
		t.Fatal(err)
	}
	alert := int64(5)
	if err = s.svc.SaveWalletSettings(s.ctx, s.org, usecase.WalletSettingsInput{AutoTopup: true, Threshold: 50, PackageUUID: open.UUID, Consent: true, AlertThreshold: &alert}); err != nil {
		t.Fatal(err)
	}
	settings, err := s.svc.WalletSettings(s.ctx, s.org)
	if err != nil || !settings.AutoTopup || !settings.AutoTopupAvailable || settings.AlertThreshold == nil || *settings.AlertThreshold != 5 || settings.PackageUUID == nil || *settings.PackageUUID != open.UUID {
		t.Fatalf("wallet settings %+v %v", settings, err)
	}
	if err = s.svc.WalletSweep(s.ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if w := s.wallet(t); w.AICredits != 515 {
		t.Fatalf("auto top-up balance %d", w.AICredits)
	}
	// Above the threshold now: the next sweep buys nothing.
	if err = s.svc.WalletSweep(s.ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if w := s.wallet(t); w.AICredits != 515 {
		t.Fatalf("second top-up %d", w.AICredits)
	}
}

// Sales-led plan change by a platform admin: "now" switches a trialing
// organization without an invoice and tops up the allowance; "period_end"
// only schedules; an empty reason is refused.
func TestCommerceScenarioAdminPlanChange(t *testing.T) {
	s := newScenario(t)
	org, err := s.orgs.CreateOrganization(s.ctx, orgmodel.CreateOrganizationInput{Name: "Satış AŞ", OwnerUserID: s.owner, TrialPriceUUID: s.starter.Prices[0].UUID})
	if err != nil {
		t.Fatal(err)
	}
	s.org = org.ID
	if _, err = s.svc.AdminChangePlan(s.ctx, s.org, usecase.AdminPlanChange{PriceUUID: s.pro.Prices[0].UUID, Timing: "now"}, s.actor); !errors.Is(err, model.ErrInvalid) {
		t.Fatalf("change without reason: %v", err)
	}
	sub, err := s.svc.AdminChangePlan(s.ctx, s.org, usecase.AdminPlanChange{PriceUUID: s.pro.Prices[0].UUID, Timing: "now", Reason: "Yıllık sözleşme imzalandı"}, s.actor)
	if err != nil || sub.VersionUUID != s.pro.UUID || sub.Status != "trialing" {
		t.Fatalf("now %+v %v", sub, err)
	}
	if w := s.wallet(t); w.AICredits != 500 || w.VideoMinutes != 300 {
		t.Fatalf("allowance after upgrade ai=%d video=%d", w.AICredits, w.VideoMinutes)
	}
	sub, err = s.svc.AdminChangePlan(s.ctx, s.org, usecase.AdminPlanChange{PriceUUID: s.starter.Prices[0].UUID, Timing: "period_end", Reason: "Müşteri küçültmek istedi"}, s.actor)
	if err != nil || sub.VersionUUID != s.pro.UUID || sub.PendingVersionUUID == nil || *sub.PendingVersionUUID != s.starter.UUID {
		t.Fatalf("period_end %+v %v", sub, err)
	}
	history, err := s.svc.History(s.ctx, s.org, sub.UUID)
	if err != nil || len(history) < 3 {
		t.Fatalf("history %d %v", len(history), err)
	}
}
