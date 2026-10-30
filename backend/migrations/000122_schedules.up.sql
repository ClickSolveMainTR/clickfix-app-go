-- CF-105: concrete UTC intervals; IANA timezone retained for calendar display.
CREATE TABLE shift_templates (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 name TEXT NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200),
 timezone TEXT NOT NULL, start_minute INTEGER NOT NULL CHECK(start_minute BETWEEN 0 AND 1439),
 duration_minutes INTEGER NOT NULL CHECK(duration_minutes BETWEEN 1 AND 1440),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(organization_id,id)
);
CREATE INDEX idx_shift_templates_org ON shift_templates(organization_id);
CREATE TABLE agent_shifts (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 user_id BIGINT NOT NULL REFERENCES users(id), template_id BIGINT,
 starts_at TIMESTAMPTZ NOT NULL, ends_at TIMESTAMPTZ NOT NULL CHECK(ends_at>starts_at),
 timezone TEXT NOT NULL DEFAULT 'Europe/Istanbul',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,template_id) REFERENCES shift_templates(organization_id,id),
 UNIQUE(organization_id,id), CHECK(ends_at<=starts_at+interval '24 hours')
);
CREATE INDEX idx_agent_shifts_org ON agent_shifts(organization_id,user_id,starts_at,ends_at);
CREATE TABLE agent_time_off (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 user_id BIGINT NOT NULL REFERENCES users(id),
 starts_at TIMESTAMPTZ NOT NULL, ends_at TIMESTAMPTZ NOT NULL CHECK(ends_at>starts_at),
 reason TEXT NOT NULL DEFAULT '' CHECK(length(reason)<=1000),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected','canceled')),
 reviewed_by BIGINT REFERENCES users(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_agent_time_off_org ON agent_time_off(organization_id,user_id,starts_at,ends_at);
CREATE TABLE schedule_settings (
 organization_id BIGINT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
 enforce_shifts BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_shift_templates_updated BEFORE UPDATE ON shift_templates FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_agent_shifts_updated BEFORE UPDATE ON agent_shifts FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_agent_time_off_updated BEFORE UPDATE ON agent_time_off FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_schedule_settings_updated BEFORE UPDATE ON schedule_settings FOR EACH ROW EXECUTE FUNCTION set_updated_at();
