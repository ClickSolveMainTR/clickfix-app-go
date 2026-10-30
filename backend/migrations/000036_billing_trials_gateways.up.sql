-- CF-23/24: once-per-organization trial and durable gateway attempts.
CREATE TABLE billing_trial_enrollments (
 organization_id BIGINT PRIMARY KEY REFERENCES organizations(id),
 subscription_id BIGINT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,subscription_id) REFERENCES billing_subscriptions(organization_id,id)
);
CREATE TABLE billing_trial_reminders (
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 subscription_id BIGINT NOT NULL,
 trial_ends_at TIMESTAMPTZ NOT NULL,
 days INTEGER NOT NULL CHECK(days IN (7,3,1)),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,subscription_id) REFERENCES billing_subscriptions(organization_id,id),
 PRIMARY KEY(organization_id,subscription_id,trial_ends_at,days)
);
CREATE TABLE billing_gateway_settings (
 id BOOLEAN PRIMARY KEY DEFAULT true CHECK(id),
 default_provider TEXT NOT NULL DEFAULT '',
 currency_providers JSONB NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(currency_providers)='object'),
 trial_price_uuid UUID REFERENCES billing_prices(uuid),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO billing_gateway_settings(id) VALUES(true);
CREATE TRIGGER trg_billing_gateway_settings_updated BEFORE UPDATE ON billing_gateway_settings
 FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE billing_payment_attempts (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 idempotency_key TEXT NOT NULL CHECK(length(idempotency_key) BETWEEN 1 AND 200),
 provider TEXT NOT NULL,
 operation TEXT NOT NULL CHECK(operation IN ('checkout','three_ds','recurring','refund','tokenize','remove_card')),
 request_hash TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','unknown','succeeded','failed')),
 result JSONB,
 failure_code TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,idempotency_key)
);
CREATE INDEX idx_billing_payment_attempts_org ON billing_payment_attempts(organization_id,created_at);
CREATE TRIGGER trg_billing_payment_attempts_updated BEFORE UPDATE ON billing_payment_attempts
 FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE UNIQUE INDEX uq_notifications_source_event_channel ON notifications(user_id,channel,(payload->>'source_event_id'))
 WHERE payload ? 'source_event_id';
