-- CF-94/95/115: organization automations and developer integrations.
CREATE TABLE automation_rules (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 created_by BIGINT NOT NULL REFERENCES users(id), name TEXT NOT NULL,
 trigger_event TEXT NOT NULL, conditions JSONB NOT NULL DEFAULT '{}'::jsonb,
 actions JSONB NOT NULL DEFAULT '[]'::jsonb, position INTEGER NOT NULL DEFAULT 0,
 enabled BOOLEAN NOT NULL DEFAULT true, stop_on_match BOOLEAN NOT NULL DEFAULT false,
 schedule_minutes INTEGER NOT NULL DEFAULT 0 CHECK(schedule_minutes BETWEEN 0 AND 525600),
 next_run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), scan_lease_until TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 UNIQUE(organization_id,id)
);
CREATE INDEX idx_automation_rules_org_trigger ON automation_rules(organization_id,trigger_event,position,id) WHERE enabled;
CREATE INDEX idx_automation_rules_due ON automation_rules(next_run_at) WHERE enabled AND schedule_minutes > 0;
CREATE TRIGGER trg_automation_rules_updated BEFORE UPDATE ON automation_rules FOR EACH ROW EXECUTE FUNCTION set_updated_at();
ALTER TABLE conversations ADD CONSTRAINT uq_conversations_org_uuid UNIQUE(organization_id,uuid);
CREATE TABLE automation_executions (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 rule_id BIGINT NOT NULL, event_id UUID NOT NULL, conversation_uuid UUID NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('running','succeeded','failed','skipped')),
 matched BOOLEAN NOT NULL DEFAULT false, error TEXT NOT NULL DEFAULT '',
 action_index INTEGER NOT NULL DEFAULT 0, lease_until TIMESTAMPTZ NOT NULL DEFAULT NOW()+interval '2 minutes', created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), finished_at TIMESTAMPTZ,
 FOREIGN KEY(organization_id,conversation_uuid) REFERENCES conversations(organization_id,uuid) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,rule_id) REFERENCES automation_rules(organization_id,id) ON DELETE CASCADE,
 UNIQUE(organization_id,rule_id,event_id,conversation_uuid)
);
CREATE INDEX idx_automation_executions_org ON automation_executions(organization_id,created_at DESC);
CREATE TABLE outbound_webhooks (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 name TEXT NOT NULL, url TEXT NOT NULL, secret_enc TEXT NOT NULL, events TEXT[] NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT true, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 UNIQUE(organization_id,id)
);
CREATE INDEX idx_outbound_webhooks_org ON outbound_webhooks(organization_id);
CREATE TRIGGER trg_outbound_webhooks_updated BEFORE UPDATE ON outbound_webhooks FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE outbound_webhook_deliveries (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 webhook_id BIGINT NOT NULL, event_id UUID NOT NULL, event_name TEXT NOT NULL, payload JSONB NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','succeeded','failed')),
 attempts INTEGER NOT NULL DEFAULT 0, next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 lease_until TIMESTAMPTZ NOT NULL DEFAULT NOW(), last_status INTEGER NOT NULL DEFAULT 0,
 last_error TEXT NOT NULL DEFAULT '', delivered_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 FOREIGN KEY(organization_id,webhook_id) REFERENCES outbound_webhooks(organization_id,id) ON DELETE CASCADE,
 UNIQUE(organization_id,webhook_id,event_id)
);
CREATE INDEX idx_outbound_webhook_deliveries_org ON outbound_webhook_deliveries(organization_id,created_at DESC);
CREATE INDEX idx_outbound_webhook_deliveries_due ON outbound_webhook_deliveries(next_attempt_at,lease_until) WHERE status='pending';
CREATE TABLE public_api_keys (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 name TEXT NOT NULL, token_hash TEXT NOT NULL UNIQUE, key_prefix TEXT NOT NULL,
 scopes TEXT[] NOT NULL CHECK(cardinality(scopes)>0), rate_limit INTEGER NOT NULL DEFAULT 120 CHECK(rate_limit BETWEEN 1 AND 6000),
 expires_at TIMESTAMPTZ, revoked_at TIMESTAMPTZ, last_used_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), UNIQUE(organization_id,id)
);
CREATE INDEX idx_public_api_keys_org ON public_api_keys(organization_id,created_at DESC);
CREATE TABLE public_api_idempotency (
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 key_id BIGINT NOT NULL,
 request_key TEXT NOT NULL, request_hash TEXT NOT NULL,
 status_code INTEGER, response_enc TEXT, expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW()+interval '24 hours',
 FOREIGN KEY(organization_id,key_id) REFERENCES public_api_keys(organization_id,id) ON DELETE CASCADE,
 PRIMARY KEY(organization_id,key_id,request_key)
);
