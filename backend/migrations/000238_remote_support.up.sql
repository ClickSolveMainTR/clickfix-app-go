-- CF-233: remote support sessions (docs/design/masaustu.md §5, §7). An agent
-- starts a short-lived session from a conversation; the customer's desktop
-- app resolves it with a 9-digit code or a one-time join link, then gives
-- separate consents for viewing, control and recording. Media flows through
-- LiveKit (room rs_<uuid>). Only HMAC digests of the code, the join token and
-- the customer's client secret are stored.

-- Plan features: on by default in the catalog (published plan versions are
-- immutable, see the db-migration skill); minutes are metered, unlimited by
-- default until a plan or override sets a limit.
INSERT INTO billing_features(key, kind, label_tr, label_en, unit, aggregation, period, default_bool) VALUES
    ('remote_support.enabled', 'boolean', 'Uzaktan destek', 'Remote support', '', 'none', 'none', true)
ON CONFLICT (key) DO NOTHING;
INSERT INTO billing_features(key, kind, label_tr, label_en, unit, aggregation, period, default_unlimited) VALUES
    ('remote_support.minutes', 'limit', 'Uzaktan destek dakikası', 'Remote support minutes', 'minute', 'sum', 'month', true)
ON CONFLICT (key) DO NOTHING;

-- Organization settings plus the platform state (kill switch / automatic
-- suspension). A missing row means the defaults below.
CREATE TABLE remote_support_settings (
    organization_id BIGINT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    -- Opt-in: an organization admin turns remote support on.
    enabled BOOLEAN NOT NULL DEFAULT false,
    allow_control BOOLEAN NOT NULL DEFAULT true,
    recording TEXT NOT NULL DEFAULT 'off' CHECK (recording IN ('off','optional')),
    max_duration_minutes INTEGER NOT NULL DEFAULT 60 CHECK (max_duration_minutes BETWEEN 5 AND 240),
    recording_retention_days INTEGER NOT NULL DEFAULT 30 CHECK (recording_retention_days BETWEEN 1 AND 365),
    -- Platform side: 'disabled' is the platform admin's kill switch,
    -- 'suspended' is set automatically after too many fraud reports.
    platform_state TEXT NOT NULL DEFAULT 'active' CHECK (platform_state IN ('active','disabled','suspended')),
    platform_reason TEXT NOT NULL DEFAULT '' CHECK (char_length(platform_reason) <= 500),
    platform_changed_at TIMESTAMPTZ,
    platform_changed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_remote_support_settings_state ON remote_support_settings(platform_state) WHERE platform_state <> 'active';
CREATE TRIGGER trg_remote_support_settings_updated BEFORE UPDATE ON remote_support_settings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE remote_support_sessions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    conversation_id BIGINT NOT NULL,
    agent_user_id BIGINT,
    contact_id BIGINT,
    -- code_lookup: HMAC of the first five digits, unique among pending
    -- sessions, so a wrong guess can be charged to one session (five wrong
    -- attempts invalidate it). code_hash: HMAC of all nine digits.
    code_lookup TEXT NOT NULL,
    code_hash TEXT NOT NULL,
    code_attempts INTEGER NOT NULL DEFAULT 0 CHECK (code_attempts >= 0),
    code_expires_at TIMESTAMPTZ NOT NULL,
    join_token_hash TEXT NOT NULL UNIQUE,
    -- Issued once at resolve; authenticates the customer's later calls.
    client_secret_hash TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','joined','active','ended','expired','cancelled')),
    mode_requested TEXT NOT NULL CHECK (mode_requested IN ('view','control')),
    resolved_at TIMESTAMPTZ,
    view_consent_at TIMESTAMPTZ,
    control_consent_at TIMESTAMPTZ,
    control_revoked_at TIMESTAMPTZ,
    -- NULL: not asked (organization recording off); false: declined.
    recording_consent BOOLEAN,
    recording_consent_at TIMESTAMPTZ,
    room_name TEXT NOT NULL UNIQUE,
    egress_id TEXT NOT NULL DEFAULT '',
    recording_status TEXT NOT NULL DEFAULT 'none' CHECK (recording_status IN ('none','starting','active','complete','failed','deleted')),
    recording_key TEXT NOT NULL DEFAULT '',
    recording_size_bytes BIGINT NOT NULL DEFAULT 0,
    recording_expires_at TIMESTAMPTZ,
    client_os TEXT NOT NULL DEFAULT '' CHECK (char_length(client_os) <= 32),
    client_version TEXT NOT NULL DEFAULT '' CHECK (char_length(client_version) <= 32),
    max_duration_minutes INTEGER NOT NULL CHECK (max_duration_minutes BETWEEN 5 AND 240),
    started_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,
    ended_reason TEXT NOT NULL DEFAULT '' CHECK (ended_reason IN ('','agent','customer','declined','code_expired','timeout','attempts','max_duration','conversation_closed','platform','reported')),
    ended_by TEXT NOT NULL DEFAULT '' CHECK (ended_by IN ('','agent','customer','system','platform')),
    ended_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reported_at TIMESTAMPTZ,
    report_reason TEXT NOT NULL DEFAULT '' CHECK (char_length(report_reason) <= 1000),
    billed_minutes BIGINT NOT NULL DEFAULT 0 CHECK (billed_minutes >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    CHECK ((status IN ('ended','expired','cancelled')) = (ended_at IS NOT NULL)),
    CHECK (control_consent_at IS NULL OR mode_requested = 'control'),
    FOREIGN KEY (organization_id, conversation_id) REFERENCES conversations(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, agent_user_id) REFERENCES organization_members(organization_id, user_id) ON DELETE SET NULL (agent_user_id),
    FOREIGN KEY (organization_id, contact_id) REFERENCES contacts(organization_id, id) ON DELETE SET NULL (contact_id)
);
CREATE INDEX idx_remote_support_sessions_org ON remote_support_sessions(organization_id, created_at DESC);
CREATE INDEX idx_remote_support_sessions_conversation ON remote_support_sessions(organization_id, conversation_id, created_at DESC);
CREATE INDEX idx_remote_support_sessions_open ON remote_support_sessions(status) WHERE status IN ('pending','joined','active');
CREATE INDEX idx_remote_support_sessions_reported ON remote_support_sessions(organization_id, reported_at DESC) WHERE reported_at IS NOT NULL;
CREATE UNIQUE INDEX uq_remote_support_sessions_code ON remote_support_sessions(code_lookup) WHERE status = 'pending';
CREATE UNIQUE INDEX uq_remote_support_sessions_egress ON remote_support_sessions(egress_id) WHERE egress_id <> '';
CREATE INDEX idx_remote_support_sessions_recording_expiry ON remote_support_sessions(recording_expires_at) WHERE recording_status = 'complete';
CREATE TRIGGER trg_remote_support_sessions_updated BEFORE UPDATE ON remote_support_sessions FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO permissions(name, slug) VALUES
    ('Start remote support sessions', 'tenant.remote_support.use'),
    ('Request control in remote support sessions', 'tenant.remote_support.control'),
    ('Manage remote support settings', 'tenant.remote_support.settings'),
    ('Manage remote support across organizations', 'platform.remote_support.manage')
ON CONFLICT (slug) DO NOTHING;
INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE p.slug IN ('tenant.remote_support.use','tenant.remote_support.control','tenant.remote_support.settings','platform.remote_support.manage')
  AND ((r.scope = 'platform' AND r.slug = 'super_admin' AND p.slug LIKE 'platform.%')
    OR (r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('owner','admin') AND p.slug LIKE 'tenant.%'))
ON CONFLICT DO NOTHING;
