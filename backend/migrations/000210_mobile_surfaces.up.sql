-- CF-152: mobile OS surfaces (widgets, Live Activities, App Intents).

-- Narrow, short-lived bearer for widget extensions and App Intents
-- (`Authorization: Widget <token>`). Only the sha256 hex of the opaque token
-- is stored. session_uuid is the refresh session the issuing access token was
-- bound to; the token dies with that session (rotation re-binds it).
CREATE TABLE widget_tokens (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    user_id         BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    device_id       VARCHAR(128) NOT NULL,
    token_hash      TEXT         NOT NULL,
    session_uuid    UUID         NULL,
    expires_at      TIMESTAMPTZ  NOT NULL,
    revoked_at      TIMESTAMPTZ  NULL,
    last_used_at    TIMESTAMPTZ  NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_widget_tokens_uuid UNIQUE (uuid),
    CONSTRAINT uq_widget_tokens_token_hash UNIQUE (token_hash)
);

-- One active token per (user, device): issuing revokes the previous ones.
CREATE INDEX idx_widget_tokens_user_device ON widget_tokens (user_id, device_id)
    WHERE revoked_at IS NULL;
CREATE INDEX idx_widget_tokens_session ON widget_tokens (session_uuid)
    WHERE revoked_at IS NULL AND session_uuid IS NOT NULL;

CREATE TRIGGER trg_widget_tokens_set_updated_at
    BEFORE UPDATE ON widget_tokens
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ActivityKit push tokens (hex APNs tokens). push_to_start tokens start a new
-- activity of activity_type; update tokens belong to one running activity
-- (activity_id) about ref (e.g. the conversation uuid).
CREATE TABLE live_activity_tokens (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    user_id         BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    device_id       VARCHAR(128) NOT NULL,
    kind            TEXT         NOT NULL,
    activity_type   TEXT         NOT NULL,
    activity_id     VARCHAR(128) NOT NULL DEFAULT '',
    ref             VARCHAR(64)  NOT NULL DEFAULT '',
    token           VARCHAR(255) NOT NULL,
    environment     TEXT         NOT NULL DEFAULT 'production',
    disabled_at     TIMESTAMPTZ  NULL,
    disabled_reason VARCHAR(64)  NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_live_activity_tokens_uuid UNIQUE (uuid),
    CONSTRAINT uq_live_activity_tokens_token UNIQUE (token),
    CONSTRAINT chk_live_activity_tokens_kind CHECK (kind IN ('push_to_start', 'update')),
    CONSTRAINT chk_live_activity_tokens_activity_type CHECK (activity_type IN ('call', 'urgent_conversation')),
    CONSTRAINT chk_live_activity_tokens_environment CHECK (environment IN ('production', 'sandbox'))
);

CREATE INDEX idx_live_activity_tokens_user_active ON live_activity_tokens (user_id)
    WHERE disabled_at IS NULL;
-- Running activities about one conversation (updated / ended by events).
CREATE INDEX idx_live_activity_tokens_ref_active ON live_activity_tokens (activity_type, ref)
    WHERE kind = 'update' AND disabled_at IS NULL;

CREATE TRIGGER trg_live_activity_tokens_set_updated_at
    BEFORE UPDATE ON live_activity_tokens
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Per-user switch for Live Activity pushes (absent row = enabled).
CREATE TABLE mobile_surface_preferences (
    user_id                 BIGINT      PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    live_activities_enabled BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_mobile_surface_preferences_set_updated_at
    BEFORE UPDATE ON mobile_surface_preferences
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
