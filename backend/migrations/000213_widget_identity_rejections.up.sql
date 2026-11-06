-- CF-176: the last rejected identify call of a widget, so the editor can tell
-- the integrator why signed-in users stay anonymous. One row per widget; it
-- holds a fixed error code and a time only (never user_id, user_hash or the
-- identity secret) and is removed by the next verified identify.
CREATE TABLE widget_identity_rejections (
    id                  BIGSERIAL PRIMARY KEY,
    organization_id     BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_instance_id BIGINT NOT NULL,
    code                TEXT NOT NULL CHECK (code IN ('WIDGET_IDENTITY_DISABLED', 'WIDGET_IDENTITY_INVALID')),
    rejected_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT widget_identity_rejections_instance_uq UNIQUE (channel_instance_id),
    FOREIGN KEY (organization_id, channel_instance_id) REFERENCES channel_instances(organization_id, id) ON DELETE CASCADE
);
