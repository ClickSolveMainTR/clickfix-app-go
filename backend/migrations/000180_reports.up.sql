CREATE TABLE custom_reports (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 created_by BIGINT NOT NULL REFERENCES users(id), name TEXT NOT NULL CHECK(char_length(name) BETWEEN 1 AND 120),
 definition JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),UNIQUE(organization_id,id)
);
CREATE INDEX idx_custom_reports_org ON custom_reports(organization_id,created_at DESC);
CREATE TRIGGER trg_custom_reports_updated BEFORE UPDATE ON custom_reports FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE report_schedules (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 report_id BIGINT NOT NULL, created_by BIGINT NOT NULL REFERENCES users(id),
 frequency TEXT NOT NULL CHECK(frequency IN ('daily','weekly','monthly')),timezone TEXT NOT NULL,
 format TEXT NOT NULL CHECK(format IN ('csv','xlsx')),recipients UUID[] NOT NULL CHECK(cardinality(recipients) BETWEEN 1 AND 20),
 enabled BOOLEAN NOT NULL DEFAULT true,next_run_at TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,report_id) REFERENCES custom_reports(organization_id,id) ON DELETE CASCADE,UNIQUE(organization_id,id)
);
CREATE INDEX idx_report_schedules_org ON report_schedules(organization_id);
CREATE INDEX idx_report_schedules_due ON report_schedules(next_run_at) WHERE enabled;
CREATE TRIGGER trg_report_schedules_updated BEFORE UPDATE ON report_schedules FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE report_deliveries (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 schedule_id BIGINT NOT NULL,scheduled_at TIMESTAMPTZ NOT NULL,format TEXT NOT NULL CHECK(format IN ('csv','xlsx')),content BYTEA NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),expires_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '30 days',
 FOREIGN KEY(organization_id,schedule_id) REFERENCES report_schedules(organization_id,id) ON DELETE CASCADE,UNIQUE(organization_id,schedule_id,scheduled_at)
);
CREATE INDEX idx_report_deliveries_org ON report_deliveries(organization_id,created_at DESC);
