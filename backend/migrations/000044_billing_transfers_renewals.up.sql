ALTER TABLE billing_invoices ADD COLUMN payment_reference TEXT NOT NULL DEFAULT 'CF'||replace(gen_random_uuid()::text,'-','');
CREATE UNIQUE INDEX uq_billing_invoice_reference ON billing_invoices(payment_reference);
ALTER TABLE billing_bank_accounts ADD COLUMN branch TEXT NOT NULL DEFAULT '';
ALTER TABLE billing_bank_accounts ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE billing_bank_transfers ADD COLUMN attachment_uuid UUID REFERENCES attachments(uuid);
CREATE UNIQUE INDEX uq_billing_transfer_attachment ON billing_bank_transfers(attachment_uuid) WHERE attachment_uuid IS NOT NULL;
ALTER TABLE billing_dunning ADD COLUMN last_reminder_day INTEGER NOT NULL DEFAULT 0;
ALTER TABLE billing_subscriptions ADD COLUMN renewal_enabled BOOLEAN NOT NULL DEFAULT false;
CREATE TABLE billing_renewal_reminders (
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 subscription_id BIGINT NOT NULL,
 period_end TIMESTAMPTZ NOT NULL,
 days INTEGER NOT NULL CHECK(days IN (30,7)),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,subscription_id) REFERENCES billing_subscriptions(organization_id,id),
 PRIMARY KEY(organization_id,subscription_id,period_end,days)
);
CREATE TABLE billing_payment_notices (
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 notice_key TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(organization_id,notice_key)
);
