-- CF-201: mobile app identities of a web widget. A native app (iOS or
-- Android) has no page origin, so the visitor API accepts
-- `X-ClickFix-App-Key` + `X-ClickFix-App-Package` instead of the origin
-- allowlist. Only the sha256 hex of the key is stored; the key itself is
-- shown once when it is created. Revoking keeps the row (audit) and the
-- key stops working at once.
CREATE TABLE widget_app_keys (
    id                  BIGSERIAL PRIMARY KEY,
    uuid                UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id     BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_instance_id BIGINT NOT NULL,
    platform            TEXT NOT NULL CHECK (platform IN ('ios', 'android')),
    package_name        TEXT NOT NULL CHECK (char_length(package_name) BETWEEN 3 AND 200),
    label               TEXT NOT NULL DEFAULT '' CHECK (char_length(label) <= 100),
    key_prefix          TEXT NOT NULL CHECK (char_length(key_prefix) <= 16),
    key_hash            TEXT NOT NULL,
    created_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at        TIMESTAMPTZ NULL,
    revoked_at          TIMESTAMPTZ NULL,
    CONSTRAINT widget_app_keys_hash_uq UNIQUE (key_hash),
    FOREIGN KEY (organization_id, channel_instance_id) REFERENCES channel_instances(organization_id, id) ON DELETE CASCADE
);
CREATE INDEX widget_app_keys_instance_idx ON widget_app_keys (channel_instance_id, created_at DESC);
