-- CF-104. Configuration targets are validated against current org members before storage and delivery.
CREATE TABLE oncall_rotas (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 name TEXT NOT NULL CHECK(char_length(name) BETWEEN 1 AND 120),timezone TEXT NOT NULL,
 layers JSONB NOT NULL,overrides JSONB NOT NULL DEFAULT '[]',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),UNIQUE(organization_id,id)
);
CREATE INDEX idx_oncall_rotas_org ON oncall_rotas(organization_id);
CREATE TRIGGER trg_oncall_rotas_updated BEFORE UPDATE ON oncall_rotas FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE escalation_policies (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 name TEXT NOT NULL CHECK(char_length(name) BETWEEN 1 AND 120),steps JSONB NOT NULL,
 urgent_conversations BOOLEAN NOT NULL DEFAULT false,incidents BOOLEAN NOT NULL DEFAULT false,
 enabled BOOLEAN NOT NULL DEFAULT true,created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),UNIQUE(organization_id,id)
);
CREATE INDEX idx_escalation_policies_org ON escalation_policies(organization_id);
CREATE TRIGGER trg_escalation_policies_updated BEFORE UPDATE ON escalation_policies FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE oncall_alerts (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 policy_id BIGINT NOT NULL,incident_id BIGINT NULL,conversation_id BIGINT NULL,
 next_step INTEGER NOT NULL DEFAULT 0 CHECK(next_step>=0),next_notify_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 acknowledged_at TIMESTAMPTZ NULL,acknowledged_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,resolved_at TIMESTAMPTZ NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK((incident_id IS NOT NULL)::int+(conversation_id IS NOT NULL)::int=1),
 FOREIGN KEY(organization_id,policy_id) REFERENCES escalation_policies(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,incident_id) REFERENCES status_incidents(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id) ON DELETE CASCADE,
 UNIQUE(organization_id,policy_id,incident_id),UNIQUE(organization_id,policy_id,conversation_id)
);
CREATE INDEX idx_oncall_alerts_org ON oncall_alerts(organization_id,created_at DESC);
CREATE INDEX idx_oncall_alerts_due ON oncall_alerts(next_notify_at) WHERE acknowledged_at IS NULL AND resolved_at IS NULL;
CREATE TABLE uptime_monitors (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 name TEXT NOT NULL CHECK(char_length(name) BETWEEN 1 AND 120),url TEXT NOT NULL CHECK(char_length(url)<=2048),
 interval_seconds INTEGER NOT NULL CHECK(interval_seconds BETWEEN 60 AND 86400),
 expected_status INTEGER NOT NULL CHECK(expected_status BETWEEN 100 AND 599),keyword TEXT NOT NULL DEFAULT '' CHECK(char_length(keyword)<=1000),
 component_id BIGINT NOT NULL,policy_id BIGINT NULL,incident_id BIGINT NULL,
 enabled BOOLEAN NOT NULL DEFAULT true,next_check_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 last_checked_at TIMESTAMPTZ NULL,last_up BOOLEAN NULL,last_status INTEGER NOT NULL DEFAULT 0,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),UNIQUE(organization_id,id),
 FOREIGN KEY(organization_id,component_id) REFERENCES status_components(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,policy_id) REFERENCES escalation_policies(organization_id,id) ON DELETE SET NULL(policy_id),
 FOREIGN KEY(organization_id,incident_id) REFERENCES status_incidents(organization_id,id) ON DELETE SET NULL(incident_id)
);
CREATE INDEX idx_uptime_monitors_org ON uptime_monitors(organization_id);
CREATE INDEX idx_uptime_monitors_due ON uptime_monitors(next_check_at) WHERE enabled;
CREATE TRIGGER trg_uptime_monitors_updated BEFORE UPDATE ON uptime_monitors FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE uptime_results (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 monitor_id BIGINT NOT NULL,up BOOLEAN NOT NULL,status_code INTEGER NOT NULL,latency_ms BIGINT NOT NULL,
 checked_at TIMESTAMPTZ NOT NULL DEFAULT now(),FOREIGN KEY(organization_id,monitor_id) REFERENCES uptime_monitors(organization_id,id) ON DELETE CASCADE
);
CREATE INDEX idx_uptime_results_org ON uptime_results(organization_id,monitor_id,checked_at DESC);
