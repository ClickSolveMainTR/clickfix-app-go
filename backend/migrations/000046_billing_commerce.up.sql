ALTER TABLE billing_subscriptions ADD COLUMN contracted_seats BIGINT NOT NULL DEFAULT 1 CHECK(contracted_seats>0);
ALTER TABLE billing_profiles ADD COLUMN kind TEXT NOT NULL DEFAULT 'company' CHECK(kind IN ('company','individual')),
 ADD COLUMN country_code TEXT NOT NULL DEFAULT 'TR' CHECK(length(country_code)=2),
 ADD COLUMN emails TEXT[] NOT NULL DEFAULT '{}',
 ADD COLUMN export_exemption BOOLEAN NOT NULL DEFAULT false;
CREATE TABLE billing_exchange_rates (
 id BIGSERIAL PRIMARY KEY, currency TEXT NOT NULL CHECK(currency IN ('USD','EUR')),
 rate NUMERIC(20,8) NOT NULL CHECK(rate>0), observed_on DATE NOT NULL, source TEXT NOT NULL DEFAULT 'TCMB',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(currency,observed_on)
);
CREATE TRIGGER trg_billing_fx_immutable BEFORE UPDATE OR DELETE ON billing_exchange_rates FOR EACH ROW EXECUTE FUNCTION billing_guard_append_only();
CREATE TABLE billing_invoice_sequences (
 series TEXT NOT NULL CHECK(series ~ '^[A-Z]{3}$'), year INTEGER NOT NULL, last_value BIGINT NOT NULL CHECK(last_value BETWEEN 1 AND 999999999), PRIMARY KEY(series,year)
);
ALTER TABLE billing_invoices ADD COLUMN issued_at TIMESTAMPTZ,
 ADD COLUMN original_invoice_id BIGINT,
 ADD FOREIGN KEY(organization_id,original_invoice_id) REFERENCES billing_invoices(organization_id,id);
ALTER TABLE billing_invoices DROP CONSTRAINT billing_invoices_kind_check;
ALTER TABLE billing_invoices ADD CHECK(kind IN ('renewal','upgrade','manual','credit_note','wallet','addon'));
CREATE TABLE billing_invoice_documents (
 id BIGSERIAL PRIMARY KEY, organization_id BIGINT NOT NULL REFERENCES organizations(id), invoice_id BIGINT NOT NULL UNIQUE,
 profile TEXT NOT NULL, xml BYTEA NOT NULL, pdf BYTEA NOT NULL, html BYTEA NOT NULL,
 provider TEXT NOT NULL, state TEXT NOT NULL DEFAULT 'prepared' CHECK(state IN ('prepared','submitted','issued','failed','void')),
 provider_reference TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,invoice_id) REFERENCES billing_invoices(organization_id,id)
);
CREATE INDEX idx_billing_documents_org ON billing_invoice_documents(organization_id);
CREATE FUNCTION billing_guard_invoice_document() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR (NEW.organization_id,NEW.invoice_id,NEW.profile,NEW.xml,NEW.pdf,NEW.html,NEW.provider) IS DISTINCT FROM (OLD.organization_id,OLD.invoice_id,OLD.profile,OLD.xml,OLD.pdf,OLD.html,OLD.provider) THEN
 RAISE EXCEPTION 'invoice document is immutable'; END IF; RETURN NEW;
END $$;
CREATE TRIGGER trg_billing_documents_immutable BEFORE UPDATE OR DELETE ON billing_invoice_documents FOR EACH ROW EXECUTE FUNCTION billing_guard_invoice_document();
CREATE FUNCTION billing_guard_numbered_invoice() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.number IS NOT NULL AND (NEW.organization_id,NEW.subscription_id,NEW.period_start,NEW.period_end,NEW.kind,NEW.number,NEW.buyer_snapshot,NEW.seller_snapshot,NEW.source_currency,NEW.fx_rate,NEW.subtotal_minor,NEW.discount_minor,NEW.tax_minor,NEW.total_minor) IS DISTINCT FROM (OLD.organization_id,OLD.subscription_id,OLD.period_start,OLD.period_end,OLD.kind,OLD.number,OLD.buyer_snapshot,OLD.seller_snapshot,OLD.source_currency,OLD.fx_rate,OLD.subtotal_minor,OLD.discount_minor,OLD.tax_minor,OLD.total_minor) THEN RAISE EXCEPTION 'numbered invoice is immutable'; END IF; RETURN NEW;
END $$;
CREATE TRIGGER trg_billing_numbered_invoice BEFORE UPDATE ON billing_invoices FOR EACH ROW EXECUTE FUNCTION billing_guard_numbered_invoice();
ALTER TABLE billing_coupons ADD COLUMN per_org_limit BIGINT NOT NULL DEFAULT 1 CHECK(per_org_limit>0),
 ADD COLUMN duration_months INTEGER CHECK(duration_months>0), ADD COLUMN recurring BOOLEAN NOT NULL DEFAULT false,
 ADD COLUMN plan_id BIGINT REFERENCES billing_plans(id), ADD COLUMN interval TEXT CHECK(interval IN ('monthly','yearly')),
 ADD COLUMN active BOOLEAN NOT NULL DEFAULT true;
CREATE TABLE billing_subscription_coupons (
 organization_id BIGINT NOT NULL REFERENCES organizations(id), subscription_id BIGINT NOT NULL, coupon_id BIGINT NOT NULL REFERENCES billing_coupons(id),
 attached_at TIMESTAMPTZ NOT NULL DEFAULT now(), expires_at TIMESTAMPTZ, PRIMARY KEY(subscription_id),
 FOREIGN KEY(organization_id,subscription_id) REFERENCES billing_subscriptions(organization_id,id)
);
CREATE INDEX idx_billing_subscription_coupons_org ON billing_subscription_coupons(organization_id);
ALTER TABLE billing_subscription_items ADD COLUMN pending_quantity BIGINT CHECK(pending_quantity>=0);
CREATE TABLE billing_plan_changes (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE, organization_id BIGINT NOT NULL REFERENCES organizations(id),
 subscription_id BIGINT NOT NULL, price_id BIGINT NOT NULL REFERENCES billing_prices(id), invoice_id BIGINT,
 idempotency_key TEXT NOT NULL, seats BIGINT NOT NULL CHECK(seats>0), effective_at TIMESTAMPTZ NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('awaiting_payment','scheduled','applied','canceled')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(organization_id,idempotency_key),
 FOREIGN KEY(organization_id,subscription_id) REFERENCES billing_subscriptions(organization_id,id),
 FOREIGN KEY(organization_id,invoice_id) REFERENCES billing_invoices(organization_id,id)
);
CREATE UNIQUE INDEX uq_billing_plan_change_open ON billing_plan_changes(subscription_id) WHERE state IN ('awaiting_payment','scheduled');
CREATE INDEX idx_billing_changes_org ON billing_plan_changes(organization_id);
CREATE TABLE billing_credit_packages (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE, code TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
 credits BIGINT NOT NULL CHECK(credits>0), amount_minor BIGINT NOT NULL CHECK(amount_minor>0), active BOOLEAN NOT NULL DEFAULT true,
 expires_days INTEGER CHECK(expires_days>0), created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE billing_credit_purchases (
 id BIGSERIAL PRIMARY KEY, organization_id BIGINT NOT NULL REFERENCES organizations(id), invoice_id BIGINT NOT NULL UNIQUE,
 package_id BIGINT NOT NULL REFERENCES billing_credit_packages(id), credits BIGINT NOT NULL CHECK(credits>0), expires_days INTEGER,
 granted_at TIMESTAMPTZ, FOREIGN KEY(organization_id,invoice_id) REFERENCES billing_invoices(organization_id,id)
);
CREATE INDEX idx_billing_purchases_org ON billing_credit_purchases(organization_id);
ALTER TABLE billing_credit_ledger ADD COLUMN expires_at TIMESTAMPTZ;
CREATE TABLE billing_wallet_settings (
 organization_id BIGINT PRIMARY KEY REFERENCES organizations(id), auto_topup BOOLEAN NOT NULL DEFAULT false,
 threshold BIGINT NOT NULL DEFAULT 0 CHECK(threshold>=0), package_id BIGINT REFERENCES billing_credit_packages(id),
 consented_at TIMESTAMPTZ, CHECK(NOT auto_topup OR (consented_at IS NOT NULL AND package_id IS NOT NULL))
);
CREATE TABLE billing_provider_statement (
 id BIGSERIAL PRIMARY KEY, provider TEXT NOT NULL, merchant_account_uuid UUID NOT NULL, external_id TEXT NOT NULL,
 order_reference TEXT NOT NULL, operation TEXT NOT NULL CHECK(operation IN ('charge','refund')), status TEXT NOT NULL CHECK(status IN ('succeeded','failed','pending')),
 amount_minor BIGINT NOT NULL CHECK(amount_minor>0), currency TEXT NOT NULL CHECK(currency='TRY'), observed_at TIMESTAMPTZ NOT NULL,
 organization_id BIGINT REFERENCES organizations(id), payment_id BIGINT, mismatch TEXT NOT NULL DEFAULT '',
 UNIQUE(provider,merchant_account_uuid,external_id),
 FOREIGN KEY(organization_id,payment_id) REFERENCES billing_payments(organization_id,id),
 CHECK((organization_id IS NULL)=(payment_id IS NULL))
);
CREATE INDEX idx_billing_statement_org ON billing_provider_statement(organization_id);
CREATE TABLE billing_refund_notes (
 organization_id BIGINT NOT NULL REFERENCES organizations(id), payment_id BIGINT NOT NULL UNIQUE, invoice_id BIGINT NOT NULL UNIQUE,
 FOREIGN KEY(organization_id,payment_id) REFERENCES billing_payments(organization_id,id), FOREIGN KEY(organization_id,invoice_id) REFERENCES billing_invoices(organization_id,id)
);
CREATE INDEX idx_billing_refund_notes_org ON billing_refund_notes(organization_id);
CREATE TABLE billing_item_changes (
 organization_id BIGINT NOT NULL REFERENCES organizations(id), invoice_id BIGINT PRIMARY KEY, subscription_id BIGINT NOT NULL,
 addon_id BIGINT NOT NULL REFERENCES billing_addons(id), quantity BIGINT NOT NULL CHECK(quantity>0), unit_amount_minor BIGINT NOT NULL, currency TEXT NOT NULL,
 applied_at TIMESTAMPTZ, FOREIGN KEY(organization_id,invoice_id) REFERENCES billing_invoices(organization_id,id), FOREIGN KEY(organization_id,subscription_id) REFERENCES billing_subscriptions(organization_id,id)
);
CREATE INDEX idx_billing_item_changes_org ON billing_item_changes(organization_id);
CREATE TABLE billing_credit_allocations (
 organization_id BIGINT NOT NULL REFERENCES organizations(id), debit_id BIGINT NOT NULL, credit_id BIGINT NOT NULL, amount BIGINT NOT NULL CHECK(amount>0),
 PRIMARY KEY(debit_id,credit_id), FOREIGN KEY(organization_id,debit_id) REFERENCES billing_credit_ledger(organization_id,id), FOREIGN KEY(organization_id,credit_id) REFERENCES billing_credit_ledger(organization_id,id)
);
CREATE INDEX idx_billing_allocations_org ON billing_credit_allocations(organization_id);
CREATE TRIGGER trg_billing_allocations_immutable BEFORE UPDATE OR DELETE ON billing_credit_allocations FOR EACH ROW EXECUTE FUNCTION billing_guard_append_only();
CREATE TABLE billing_referral_codes (
 organization_id BIGINT PRIMARY KEY REFERENCES organizations(id), code TEXT NOT NULL UNIQUE DEFAULT upper(substr(replace(gen_random_uuid()::text,'-',''),1,12)), created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE billing_referrals (
 organization_id BIGINT PRIMARY KEY REFERENCES organizations(id), inviter_organization_id BIGINT NOT NULL REFERENCES organizations(id),
 reward_credits BIGINT NOT NULL CHECK(reward_credits>0), awarded_at TIMESTAMPTZ, invoice_id BIGINT,
 CHECK(organization_id<>inviter_organization_id), FOREIGN KEY(organization_id,invoice_id) REFERENCES billing_invoices(organization_id,id), created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_billing_referrals_inviter ON billing_referrals(inviter_organization_id);

-- Durable billing templates: notification service selects recipient locale.
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.welcome','email','tr','ClickFix’e hoş geldiniz','Aboneliğiniz etkin. Faturalama: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.welcome','inapp','tr','ClickFix’e hoş geldiniz','Aboneliğiniz etkin. Faturalama: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.welcome','email','en','Welcome to ClickFix','Your subscription is active. Billing: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.welcome','inapp','en','Welcome to ClickFix','Your subscription is active. Billing: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.trial_ending','email','tr','Deneme süreniz bitiyor','Denemenizin bitmesine {{days}} gün kaldı. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.trial_ending','inapp','tr','Deneme süreniz bitiyor','Denemenizin bitmesine {{days}} gün kaldı. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.trial_ending','email','en','Your trial is ending','Your trial ends in {{days}} days. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.trial_ending','inapp','en','Your trial is ending','Your trial ends in {{days}} days. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.payment_received','email','tr','Ödeme alındı','Ödemeniz alındı. Fatura: {{invoice_uuid}}. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.payment_received','inapp','tr','Ödeme alındı','Ödemeniz alındı. Fatura: {{invoice_uuid}}. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.payment_received','email','en','Payment received','Your payment was received. Invoice: {{invoice_uuid}}. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.payment_received','inapp','en','Payment received','Your payment was received. Invoice: {{invoice_uuid}}. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.invoice_issued','email','tr','Faturanız hazır','Faturanızı faturalama sayfasından indirebilirsiniz. {{invoice_url}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.invoice_issued','inapp','tr','Faturanız hazır','Faturanızı faturalama sayfasından indirebilirsiniz. {{invoice_url}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.invoice_issued','email','en','Your invoice is ready','Download your invoice from the billing page. {{invoice_url}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.invoice_issued','inapp','en','Your invoice is ready','Download your invoice from the billing page. {{invoice_url}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.payment_failed','email','tr','Ödeme başarısız','Ödemeniz tamamlanamadı. Kartınızı güncelleyin: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.payment_failed','inapp','tr','Ödeme başarısız','Ödemeniz tamamlanamadı. Kartınızı güncelleyin: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.payment_failed','email','en','Payment failed','Your payment could not be completed. Update your card: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.payment_failed','inapp','en','Payment failed','Your payment could not be completed. Update your card: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.card_expiring','email','tr','Kartınızın süresi doluyor','Kayıtlı kartınızı güncelleyin: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.card_expiring','inapp','tr','Kartınızın süresi doluyor','Kayıtlı kartınızı güncelleyin: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.card_expiring','email','en','Your card expires soon','Update your saved card: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.card_expiring','inapp','en','Your card expires soon','Update your saved card: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.renewal_reminder','email','tr','Yenileme yaklaşıyor','Aboneliğiniz {{days}} gün içinde yenilenecek. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.renewal_reminder','inapp','tr','Yenileme yaklaşıyor','Aboneliğiniz {{days}} gün içinde yenilenecek. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.renewal_reminder','email','en','Renewal approaching','Your subscription renews in {{days}} days. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.renewal_reminder','inapp','en','Renewal approaching','Your subscription renews in {{days}} days. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.transfer_pending','email','tr','Havale bekleniyor','Havale açıklamasında ödeme referansını kullanın. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.transfer_pending','inapp','tr','Havale bekleniyor','Havale açıklamasında ödeme referansını kullanın. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.transfer_pending','email','en','Bank transfer pending','Use the payment reference in your bank transfer. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.transfer_pending','inapp','en','Bank transfer pending','Use the payment reference in your bank transfer. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.transfer_approved','email','tr','Havale onaylandı','Havale bildiriminiz onaylandı. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.transfer_approved','inapp','tr','Havale onaylandı','Havale bildiriminiz onaylandı. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.transfer_approved','email','en','Bank transfer approved','Your bank transfer was approved. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.transfer_approved','inapp','en','Bank transfer approved','Your bank transfer was approved. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.transfer_rejected','email','tr','Havale reddedildi','Havale bildiriminiz reddedildi. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.transfer_rejected','inapp','tr','Havale reddedildi','Havale bildiriminiz reddedildi. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.transfer_rejected','email','en','Bank transfer rejected','Your bank transfer was rejected. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.transfer_rejected','inapp','en','Bank transfer rejected','Your bank transfer was rejected. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.quota_warning','email','tr','Kota uyarısı','{{feature}} kotanızın %{{percent}} sınırına ulaştınız. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.quota_warning','inapp','tr','Kota uyarısı','{{feature}} kotanızın %{{percent}} sınırına ulaştınız. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.quota_warning','email','en','Quota warning','You reached {{percent}}% of your {{feature}} quota. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.quota_warning','inapp','en','Quota warning','You reached {{percent}}% of your {{feature}} quota. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.quota_exhausted','email','tr','Kota doldu','{{feature}} kotanız doldu. Planınızı güncelleyin: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.quota_exhausted','inapp','tr','Kota doldu','{{feature}} kotanız doldu. Planınızı güncelleyin: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.quota_exhausted','email','en','Quota exhausted','Your {{feature}} quota is exhausted. Update your plan: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.quota_exhausted','inapp','en','Quota exhausted','Your {{feature}} quota is exhausted. Update your plan: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.suspended','email','tr','Abonelik askıya alındı','Ödemenizi tamamlayarak aboneliğinizi yeniden etkinleştirin. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.suspended','inapp','tr','Abonelik askıya alındı','Ödemenizi tamamlayarak aboneliğinizi yeniden etkinleştirin. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.suspended','email','en','Subscription suspended','Complete your payment to reactivate your subscription. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.suspended','inapp','en','Subscription suspended','Complete your payment to reactivate your subscription. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.canceled','email','tr','İptal onayı','Aboneliğinizin iptali onaylandı. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.canceled','inapp','tr','İptal onayı','Aboneliğinizin iptali onaylandı. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.canceled','email','en','Cancellation confirmed','Your subscription cancellation was confirmed. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.canceled','inapp','en','Cancellation confirmed','Your subscription cancellation was confirmed. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.payment_required','email','tr','Ödeme bekleniyor','Ödemenizi tamamlayın veya kartınızı güncelleyin: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.payment_required','inapp','tr','Ödeme bekleniyor','Ödemenizi tamamlayın veya kartınızı güncelleyin: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.payment_required','email','en','Payment required','Complete your payment or update your card: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.payment_required','inapp','en','Payment required','Complete your payment or update your card: {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.reconciliation_mismatch','email','tr','Ödeme inceleniyor','Ödemeniz incelemeye alındı. Destek ekibimizle iletişime geçin.',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.reconciliation_mismatch','inapp','tr','Ödeme inceleniyor','Ödemeniz incelemeye alındı. Destek ekibimizle iletişime geçin.',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.reconciliation_mismatch','email','en','Payment under review','Your payment requires review. Please contact support.',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.reconciliation_mismatch','inapp','en','Payment under review','Your payment requires review. Please contact support.',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.refund_succeeded','email','tr','İade tamamlandı','İadeniz işlendi. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.refund_succeeded','inapp','tr','İade tamamlandı','İadeniz işlendi. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.refund_succeeded','email','en','Refund completed','Your refund has been processed. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES('billing.refund_succeeded','inapp','en','Refund completed','Your refund has been processed. {{billing_path}}',true) ON CONFLICT(code,channel,language) DO NOTHING;
ALTER TABLE billing_invoices ADD COLUMN request_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE billing_payments ADD COLUMN refund_destination TEXT NOT NULL DEFAULT 'card' CHECK(refund_destination IN ('card','wallet','manual_bank')),
 ADD COLUMN refund_cancel BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE billing_plan_changes ADD COLUMN new_period_end TIMESTAMPTZ;
CREATE UNIQUE INDEX uq_billing_item_change_pending ON billing_item_changes(subscription_id,addon_id) WHERE applied_at IS NULL;
ALTER TABLE billing_provider_statement ADD COLUMN provider_reference TEXT NOT NULL DEFAULT '';
CREATE FUNCTION billing_guard_invoice_line() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE parent_id BIGINT;
BEGIN
 IF TG_OP='DELETE' THEN parent_id=OLD.invoice_id; ELSE parent_id=NEW.invoice_id; END IF;
 IF EXISTS(SELECT 1 FROM billing_invoices WHERE id=parent_id AND number IS NOT NULL) THEN RAISE EXCEPTION 'numbered invoice lines are immutable'; END IF;
 IF TG_OP='UPDATE' AND OLD.invoice_id<>NEW.invoice_id AND EXISTS(SELECT 1 FROM billing_invoices WHERE id=OLD.invoice_id AND number IS NOT NULL) THEN RAISE EXCEPTION 'numbered invoice lines are immutable'; END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF; RETURN NEW;
END $$;
CREATE TRIGGER trg_billing_numbered_lines BEFORE INSERT OR UPDATE OR DELETE ON billing_invoice_lines FOR EACH ROW EXECUTE FUNCTION billing_guard_invoice_line();
ALTER TABLE billing_payments DROP CONSTRAINT billing_payments_provider_check,DROP CONSTRAINT billing_payments_kind_check;
ALTER TABLE billing_payments ADD CHECK(provider IN ('paytr','iyzico','bank_transfer','fake','wallet')),
 ADD CHECK(kind IN ('checkout','recurring','bank_transfer','refund','wallet'));
CREATE TABLE billing_refund_credit_reservations (
 organization_id BIGINT NOT NULL REFERENCES organizations(id), payment_id BIGINT PRIMARY KEY, debit_id BIGINT NOT NULL UNIQUE,
 credits BIGINT NOT NULL CHECK(credits>0), released_at TIMESTAMPTZ,
 FOREIGN KEY(organization_id,payment_id) REFERENCES billing_payments(organization_id,id), FOREIGN KEY(organization_id,debit_id) REFERENCES billing_credit_ledger(organization_id,id)
);
CREATE INDEX idx_billing_refund_credits_org ON billing_refund_credit_reservations(organization_id);
CREATE TABLE billing_invoice_usage_periods (
 organization_id BIGINT NOT NULL REFERENCES organizations(id), usage_period_id BIGINT PRIMARY KEY, invoice_id BIGINT NOT NULL,
 FOREIGN KEY(organization_id,usage_period_id) REFERENCES billing_usage_periods(organization_id,id), FOREIGN KEY(organization_id,invoice_id) REFERENCES billing_invoices(organization_id,id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_billing_invoice_usage_org ON billing_invoice_usage_periods(organization_id);
CREATE TRIGGER trg_billing_invoice_usage_immutable BEFORE UPDATE OR DELETE ON billing_invoice_usage_periods FOR EACH ROW EXECUTE FUNCTION billing_guard_append_only();
