-- CF-139: LiveKit video rooms, a metered `video_minutes` wallet unit and the
-- platform / plan / organization policy layers.

-- Billing: video minutes are a second metered credit unit next to ai_credits.
-- Same ledger, lots, FIFO allocations and expiry as AI credits.
ALTER TABLE billing_credit_ledger DROP CONSTRAINT billing_credit_ledger_unit_check;
ALTER TABLE billing_credit_ledger ADD CONSTRAINT billing_credit_ledger_unit_check CHECK (unit IN ('TRY','ai_credits','video_minutes'));
ALTER TABLE billing_credit_packages ADD COLUMN unit TEXT NOT NULL DEFAULT 'ai_credits' CHECK (unit IN ('ai_credits','video_minutes'));
ALTER TABLE billing_credit_purchases ADD COLUMN unit TEXT NOT NULL DEFAULT 'ai_credits' CHECK (unit IN ('ai_credits','video_minutes'));
INSERT INTO billing_features(key, kind, label_tr, label_en, unit, aggregation, period) VALUES
    ('video.enabled', 'boolean', 'Görüntülü görüşme', 'Video rooms', '', 'none', 'none'),
    ('video.minutes', 'limit', 'Görüntülü görüşme dakikası', 'Video minutes', 'minute', 'sum', 'month')
ON CONFLICT (key) DO NOTHING;

-- Platform policy (single row) and per-plan overrides. Organizations can only
-- tighten the effective policy (video_org_settings.policy).
CREATE TABLE video_platform_settings (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    policy JSONB NOT NULL,
    updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO video_platform_settings(id, policy) VALUES (1, '{
  "enabled": false,
  "max_duration_minutes": 120,
  "max_participants": 10,
  "allow_camera": true,
  "allow_screen_share": true,
  "allow_chat": true,
  "allow_recording": false,
  "allow_guest_join": true,
  "require_pin": false,
  "require_lobby": true,
  "e2ee_required": false,
  "recording_retention_hours": 24,
  "max_concurrent_recordings": 2,
  "max_recording_minutes": 120,
  "low_balance_minutes": 60
}');
CREATE TRIGGER trg_video_platform_settings_updated BEFORE UPDATE ON video_platform_settings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE video_plan_policies (
    plan_id BIGINT PRIMARY KEY REFERENCES billing_plans(id) ON DELETE CASCADE,
    policy JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_video_plan_policies_updated BEFORE UPDATE ON video_plan_policies FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Custom recording storage: the S3 keys are sealed with APP_ENCRYPTION_KEY.
CREATE TABLE video_org_settings (
    organization_id BIGINT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT true,
    policy JSONB NOT NULL DEFAULT '{}',
    storage_mode TEXT NOT NULL DEFAULT 'platform' CHECK (storage_mode IN ('platform','custom')),
    s3_endpoint TEXT NOT NULL DEFAULT '' CHECK (char_length(s3_endpoint) <= 512),
    s3_region TEXT NOT NULL DEFAULT '' CHECK (char_length(s3_region) <= 64),
    s3_bucket TEXT NOT NULL DEFAULT '' CHECK (char_length(s3_bucket) <= 128),
    s3_prefix TEXT NOT NULL DEFAULT '' CHECK (char_length(s3_prefix) <= 256),
    s3_path_style BOOLEAN NOT NULL DEFAULT true,
    s3_access_key_enc TEXT NOT NULL DEFAULT '',
    s3_secret_key_enc TEXT NOT NULL DEFAULT '',
    low_balance_notified_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (storage_mode = 'platform' OR (s3_bucket <> '' AND s3_access_key_enc <> '' AND s3_secret_key_enc <> ''))
);
CREATE TRIGGER trg_video_org_settings_updated BEFORE UPDATE ON video_org_settings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE video_rooms (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 160),
    livekit_name TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','live','ended')),
    host_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    -- CF-138 appointments are built in parallel: kept as a bare reference.
    appointment_uuid UUID,
    conversation_id BIGINT,
    pin_enc TEXT NOT NULL DEFAULT '',
    lobby BOOLEAN NOT NULL DEFAULT true,
    guest_join BOOLEAN NOT NULL DEFAULT true,
    e2ee BOOLEAN NOT NULL DEFAULT true,
    e2ee_key_enc TEXT NOT NULL DEFAULT '',
    recording BOOLEAN NOT NULL DEFAULT false,
    allow_camera BOOLEAN NOT NULL DEFAULT true,
    allow_screen_share BOOLEAN NOT NULL DEFAULT true,
    allow_chat BOOLEAN NOT NULL DEFAULT true,
    max_participants INTEGER NOT NULL CHECK (max_participants BETWEEN 2 AND 500),
    max_duration_minutes INTEGER NOT NULL CHECK (max_duration_minutes BETWEEN 5 AND 1440),
    scheduled_start_at TIMESTAMPTZ,
    scheduled_end_at TIMESTAMPTZ,
    links_expire_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,
    billed_minutes BIGINT NOT NULL DEFAULT 0 CHECK (billed_minutes >= 0),
    unbilled_minutes BIGINT NOT NULL DEFAULT 0 CHECK (unbilled_minutes >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    -- E2EE media cannot be recorded by the server-side egress.
    CHECK (NOT (e2ee AND recording)),
    CHECK (scheduled_end_at IS NULL OR scheduled_start_at IS NULL OR scheduled_end_at > scheduled_start_at),
    FOREIGN KEY (organization_id, conversation_id) REFERENCES conversations(organization_id, id) ON DELETE SET NULL (conversation_id)
);
CREATE INDEX idx_video_rooms_org ON video_rooms(organization_id, created_at DESC);
CREATE UNIQUE INDEX uq_video_rooms_appointment ON video_rooms(organization_id, appointment_uuid) WHERE appointment_uuid IS NOT NULL AND status <> 'ended';
CREATE INDEX idx_video_rooms_active ON video_rooms(status) WHERE status <> 'ended';
CREATE TRIGGER trg_video_rooms_updated BEFORE UPDATE ON video_rooms FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- A participant is admitted through the lobby before any LiveKit token is
-- issued. Guests hold a random ticket; only its SHA-256 is stored.
CREATE TABLE video_participants (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    room_id BIGINT NOT NULL,
    identity TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('host','agent','guest')),
    display_name TEXT NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 120),
    email TEXT NOT NULL DEFAULT '' CHECK (char_length(email) <= 254),
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    status TEXT NOT NULL CHECK (status IN ('waiting','admitted','denied','removed')),
    ticket_hash TEXT NOT NULL UNIQUE,
    admitted_at TIMESTAMPTZ,
    admitted_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    UNIQUE (room_id, identity),
    FOREIGN KEY (organization_id, room_id) REFERENCES video_rooms(organization_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_video_participants_org ON video_participants(organization_id, room_id, status);
CREATE TRIGGER trg_video_participants_updated BEFORE UPDATE ON video_participants FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- One row per LiveKit participant connection (webhooks). Billing is
-- participant-minutes, rounded up per connection, charged when it closes.
CREATE TABLE video_sessions (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    room_id BIGINT NOT NULL,
    livekit_sid TEXT NOT NULL UNIQUE,
    identity TEXT NOT NULL,
    joined_at TIMESTAMPTZ NOT NULL,
    left_at TIMESTAMPTZ,
    minutes BIGINT NOT NULL DEFAULT 0 CHECK (minutes >= 0),
    billed_minutes BIGINT NOT NULL DEFAULT 0 CHECK (billed_minutes >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, room_id) REFERENCES video_rooms(organization_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_video_sessions_org ON video_sessions(organization_id, room_id);
CREATE INDEX idx_video_sessions_open ON video_sessions(room_id) WHERE left_at IS NULL;

CREATE TABLE video_recordings (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    room_id BIGINT NOT NULL,
    egress_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'starting' CHECK (status IN ('starting','active','complete','failed','deleted')),
    storage TEXT NOT NULL CHECK (storage IN ('platform','custom')),
    object_key TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    duration_seconds BIGINT NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    started_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at TIMESTAMPTZ,
    -- Platform storage only: NULL means the organization's own bucket keeps it.
    expires_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, room_id) REFERENCES video_rooms(organization_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_video_recordings_org ON video_recordings(organization_id, created_at DESC);
CREATE UNIQUE INDEX uq_video_recordings_egress ON video_recordings(egress_id) WHERE egress_id <> '';
CREATE INDEX idx_video_recordings_expiry ON video_recordings(expires_at) WHERE deleted_at IS NULL AND expires_at IS NOT NULL;
CREATE TRIGGER trg_video_recordings_updated BEFORE UPDATE ON video_recordings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- LiveKit retries webhooks; event ids are remembered for a day.
CREATE TABLE video_webhook_events (
    event_id TEXT PRIMARY KEY,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO permissions(name, slug) VALUES
    ('Use video rooms', 'tenant.video.use'),
    ('Manage video rooms, recordings and settings', 'tenant.video.manage'),
    ('Manage the video module', 'platform.video.manage')
ON CONFLICT (slug) DO NOTHING;
INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE p.slug IN ('tenant.video.use','tenant.video.manage','platform.video.manage')
  AND ((r.scope = 'platform' AND r.slug = 'super_admin' AND p.slug LIKE 'platform.%')
    OR (r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('owner','admin') AND p.slug LIKE 'tenant.%')
    OR (r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('supervisor','member','agent','light_agent') AND p.slug = 'tenant.video.use'))
ON CONFLICT DO NOTHING;
