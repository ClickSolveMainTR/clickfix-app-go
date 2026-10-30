-- CF-138: flexible appointments (onboarding sessions, demos, trainings, any
-- meeting type an organization defines) and the organization calendar.
--
-- appointment_settings: module switch, display time zone, public booking and
--   reminder offsets of an organization (one row, created on first save).
-- appointment_types: what can be booked. duration_options are the lengths an
--   invitee may pick; buffers block time around a booking; host_mode decides
--   who hosts (listed agents, a team, or round-robin over listed agents).
--   intake_fields reuse the CF-52 request form field format.
-- appointment_type_hosts: listed hosts with their round-robin cursor.
-- appointments: bookings. Times are UTC instants; timezone is the invitee's
--   zone (display only). buffer_* are copied from the type when booked so a
--   later type change does not move existing blocked time. token_version
--   invalidates older reschedule/cancel links. external_* and sequence are the
--   hook for a later Google / Microsoft calendar sync (not implemented yet).
--   video_room_uuid is the hook for the CF-139 video rooms module.
-- appointment_reminders_sent: one row per (appointment, offset) delivered.

CREATE TABLE appointment_settings (
    organization_id   BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    enabled           BOOLEAN      NOT NULL DEFAULT TRUE,
    timezone          TEXT         NOT NULL DEFAULT 'Europe/Istanbul',
    public_booking    BOOLEAN      NOT NULL DEFAULT TRUE,
    reminder_minutes  INTEGER[]    NOT NULL DEFAULT '{1440,60}',
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT appointment_settings_reminders_ck CHECK (cardinality(reminder_minutes) <= 5)
);

CREATE TRIGGER trg_appointment_settings_set_updated_at
    BEFORE UPDATE ON appointment_settings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE appointment_types (
    id                      BIGSERIAL    PRIMARY KEY,
    uuid                    UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id         BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name                    TEXT         NOT NULL,
    slug                    TEXT         NOT NULL,
    description             TEXT         NOT NULL DEFAULT '',
    color                   TEXT         NOT NULL DEFAULT 'blue',
    duration_options        INTEGER[]    NOT NULL DEFAULT '{30}',
    default_duration        INTEGER      NOT NULL DEFAULT 30,
    slot_interval           INTEGER      NOT NULL DEFAULT 30,
    buffer_before           INTEGER      NOT NULL DEFAULT 0,
    buffer_after            INTEGER      NOT NULL DEFAULT 0,
    location_kind           TEXT         NOT NULL DEFAULT 'video',
    location_value          TEXT         NOT NULL DEFAULT '',
    host_mode               TEXT         NOT NULL DEFAULT 'round_robin',
    team_id                 BIGINT       NULL,
    booking_window_days     INTEGER      NOT NULL DEFAULT 60,
    min_notice_minutes      INTEGER      NOT NULL DEFAULT 240,
    max_per_day             INTEGER      NOT NULL DEFAULT 0,
    intake_fields           JSONB        NOT NULL DEFAULT '[]'::jsonb,
    confirmation_template   TEXT         NOT NULL DEFAULT '',
    reminder_template       TEXT         NOT NULL DEFAULT '',
    requires_confirmation   BOOLEAN      NOT NULL DEFAULT FALSE,
    allow_cancel            BOOLEAN      NOT NULL DEFAULT TRUE,
    allow_reschedule        BOOLEAN      NOT NULL DEFAULT TRUE,
    cancel_notice_minutes   INTEGER      NOT NULL DEFAULT 0,
    is_public               BOOLEAN      NOT NULL DEFAULT TRUE,
    is_active               BOOLEAN      NOT NULL DEFAULT TRUE,
    position                INTEGER      NOT NULL DEFAULT 0,
    created_at              TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at              TIMESTAMPTZ  NULL,
    CONSTRAINT appointment_types_uuid_uq UNIQUE (uuid),
    CONSTRAINT appointment_types_org_id_uq UNIQUE (organization_id, id),
    CONSTRAINT appointment_types_team_fk FOREIGN KEY (organization_id, team_id)
        REFERENCES teams (organization_id, id) ON DELETE SET NULL (team_id),
    CONSTRAINT appointment_types_name_ck CHECK (char_length(name) BETWEEN 1 AND 120),
    CONSTRAINT appointment_types_slug_ck CHECK (slug ~ '^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$'),
    CONSTRAINT appointment_types_description_ck CHECK (char_length(description) <= 2000),
    CONSTRAINT appointment_types_color_ck CHECK (color IN ('blue','emerald','amber','rose','violet','sky','orange','teal','pink','slate')),
    CONSTRAINT appointment_types_durations_ck CHECK (cardinality(duration_options) BETWEEN 1 AND 6
        AND 5 <= ALL (duration_options) AND 480 >= ALL (duration_options) AND default_duration = ANY (duration_options)),
    CONSTRAINT appointment_types_interval_ck CHECK (slot_interval BETWEEN 5 AND 240),
    CONSTRAINT appointment_types_buffer_ck CHECK (buffer_before BETWEEN 0 AND 240 AND buffer_after BETWEEN 0 AND 240),
    CONSTRAINT appointment_types_location_ck CHECK (location_kind IN ('video','phone','in_person','custom_url')),
    CONSTRAINT appointment_types_location_value_ck CHECK (char_length(location_value) <= 500),
    CONSTRAINT appointment_types_host_mode_ck CHECK (host_mode IN ('agents','team','round_robin')),
    CONSTRAINT appointment_types_team_mode_ck CHECK (host_mode <> 'team' OR team_id IS NOT NULL OR deleted_at IS NOT NULL OR NOT is_active),
    CONSTRAINT appointment_types_window_ck CHECK (booking_window_days BETWEEN 1 AND 365),
    CONSTRAINT appointment_types_notice_ck CHECK (min_notice_minutes BETWEEN 0 AND 43200),
    CONSTRAINT appointment_types_max_ck CHECK (max_per_day BETWEEN 0 AND 100),
    CONSTRAINT appointment_types_templates_ck CHECK (char_length(confirmation_template) <= 4000 AND char_length(reminder_template) <= 4000),
    CONSTRAINT appointment_types_cancel_ck CHECK (cancel_notice_minutes BETWEEN 0 AND 43200)
);

CREATE UNIQUE INDEX uq_appointment_types_slug ON appointment_types (organization_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX idx_appointment_types_org ON appointment_types (organization_id, position) WHERE deleted_at IS NULL;

CREATE TRIGGER trg_appointment_types_set_updated_at
    BEFORE UPDATE ON appointment_types FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE appointment_type_hosts (
    organization_id   BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    type_id           BIGINT       NOT NULL,
    user_id           BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    last_assigned_at  TIMESTAMPTZ  NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (type_id, user_id),
    CONSTRAINT appointment_type_hosts_type_fk FOREIGN KEY (organization_id, type_id)
        REFERENCES appointment_types (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_appointment_type_hosts_org ON appointment_type_hosts (organization_id, user_id);

CREATE TABLE appointments (
    id                    BIGSERIAL    PRIMARY KEY,
    uuid                  UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id       BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    type_id               BIGINT       NOT NULL,
    host_user_id          BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    status                TEXT         NOT NULL DEFAULT 'scheduled',
    starts_at             TIMESTAMPTZ  NOT NULL,
    ends_at               TIMESTAMPTZ  NOT NULL,
    buffer_before         INTEGER      NOT NULL DEFAULT 0,
    buffer_after          INTEGER      NOT NULL DEFAULT 0,
    timezone              TEXT         NOT NULL DEFAULT 'Europe/Istanbul',
    title                 TEXT         NOT NULL,
    notes                 TEXT         NOT NULL DEFAULT '',
    location_kind         TEXT         NOT NULL,
    location_value        TEXT         NOT NULL DEFAULT '',
    video_room_uuid       UUID         NULL,
    invitee_name          TEXT         NOT NULL DEFAULT '',
    invitee_email         TEXT         NOT NULL DEFAULT '',
    invitee_phone         TEXT         NOT NULL DEFAULT '',
    intake_values         JSONB        NOT NULL DEFAULT '{}'::jsonb,
    contact_id            BIGINT       NULL,
    company_id            BIGINT       NULL,
    conversation_id       BIGINT       NULL,
    source                TEXT         NOT NULL DEFAULT 'internal',
    booked_by_user_id     BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    token_version         INTEGER      NOT NULL DEFAULT 1,
    sequence              INTEGER      NOT NULL DEFAULT 0,
    cancel_reason         TEXT         NOT NULL DEFAULT '',
    cancelled_by          TEXT         NULL,
    cancelled_at          TIMESTAMPTZ  NULL,
    confirmed_at          TIMESTAMPTZ  NULL,
    completed_at          TIMESTAMPTZ  NULL,
    external_provider     TEXT         NULL,
    external_calendar_id  TEXT         NULL,
    external_event_id     TEXT         NULL,
    external_synced_at    TIMESTAMPTZ  NULL,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT appointments_uuid_uq UNIQUE (uuid),
    CONSTRAINT appointments_org_id_uq UNIQUE (organization_id, id),
    CONSTRAINT appointments_type_fk FOREIGN KEY (organization_id, type_id)
        REFERENCES appointment_types (organization_id, id),
    CONSTRAINT appointments_contact_fk FOREIGN KEY (organization_id, contact_id)
        REFERENCES contacts (organization_id, id) ON DELETE SET NULL (contact_id),
    CONSTRAINT appointments_company_fk FOREIGN KEY (organization_id, company_id)
        REFERENCES companies (organization_id, id) ON DELETE SET NULL (company_id),
    CONSTRAINT appointments_conversation_fk FOREIGN KEY (organization_id, conversation_id)
        REFERENCES conversations (organization_id, id) ON DELETE SET NULL (conversation_id),
    CONSTRAINT appointments_status_ck CHECK (status IN ('scheduled','confirmed','completed','no_show','cancelled')),
    CONSTRAINT appointments_range_ck CHECK (ends_at > starts_at AND ends_at <= starts_at + interval '8 hours'),
    CONSTRAINT appointments_buffer_ck CHECK (buffer_before BETWEEN 0 AND 240 AND buffer_after BETWEEN 0 AND 240),
    CONSTRAINT appointments_title_ck CHECK (char_length(title) BETWEEN 1 AND 200),
    CONSTRAINT appointments_notes_ck CHECK (char_length(notes) <= 5000 AND char_length(cancel_reason) <= 1000),
    CONSTRAINT appointments_location_ck CHECK (location_kind IN ('video','phone','in_person','custom_url') AND char_length(location_value) <= 500),
    CONSTRAINT appointments_invitee_ck CHECK (char_length(invitee_name) <= 120 AND char_length(invitee_email) <= 254 AND char_length(invitee_phone) <= 40),
    CONSTRAINT appointments_source_ck CHECK (source IN ('internal','public','widget','api')),
    CONSTRAINT appointments_cancelled_by_ck CHECK (cancelled_by IS NULL OR cancelled_by IN ('host','invitee')),
    CONSTRAINT appointments_external_ck CHECK (external_provider IS NULL OR external_provider IN ('google','microsoft'))
);

CREATE INDEX idx_appointments_org_time ON appointments (organization_id, starts_at);
CREATE INDEX idx_appointments_host_time ON appointments (organization_id, host_user_id, starts_at)
    WHERE status IN ('scheduled','confirmed');
CREATE INDEX idx_appointments_contact ON appointments (organization_id, contact_id) WHERE contact_id IS NOT NULL;
CREATE INDEX idx_appointments_company ON appointments (organization_id, company_id) WHERE company_id IS NOT NULL;
CREATE INDEX idx_appointments_conversation ON appointments (organization_id, conversation_id) WHERE conversation_id IS NOT NULL;
CREATE INDEX idx_appointments_reminders ON appointments (starts_at) WHERE status IN ('scheduled','confirmed');

CREATE TRIGGER trg_appointments_set_updated_at
    BEFORE UPDATE ON appointments FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE appointment_reminders_sent (
    organization_id  BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    appointment_id   BIGINT       NOT NULL,
    offset_minutes   INTEGER      NOT NULL,
    token_version    INTEGER      NOT NULL,
    sent_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (appointment_id, offset_minutes, token_version),
    CONSTRAINT appointment_reminders_sent_fk FOREIGN KEY (organization_id, appointment_id)
        REFERENCES appointments (organization_id, id) ON DELETE CASCADE
);

-- Permissions: every member reads the calendar and books for customers;
-- owners, admins and supervisors manage types and settings.
INSERT INTO permissions (name, slug) VALUES
    ('Read appointments and calendar', 'tenant.appointments.read'),
    ('Book and update appointments', 'tenant.appointments.write'),
    ('Manage appointment types and settings', 'tenant.appointments.manage')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE p.slug IN ('tenant.appointments.read', 'tenant.appointments.write', 'tenant.appointments.manage')
  AND r.scope = 'tenant' AND r.organization_id IS NULL
  AND (r.slug IN ('owner', 'admin', 'supervisor')
       OR (r.slug IN ('member', 'agent') AND p.slug IN ('tenant.appointments.read', 'tenant.appointments.write'))
       OR (r.slug = 'light_agent' AND p.slug = 'tenant.appointments.read'))
ON CONFLICT DO NOTHING;

-- Plan feature: on in every existing plan version (platform admins can turn
-- it off per plan or per organization override).
INSERT INTO billing_features (key, kind, label_tr, label_en, unit, aggregation, period)
VALUES ('module.appointments', 'boolean', 'Randevular ve takvim', 'Appointments and calendar', '', 'none', 'none')
ON CONFLICT (key) DO NOTHING;

-- Published plan versions are immutable (trg_billing_entitlement_guard); a new feature still has to
-- reach them, so the guard is lifted only for this insert.
ALTER TABLE billing_plan_entitlements DISABLE TRIGGER trg_billing_entitlement_guard;
INSERT INTO billing_plan_entitlements (plan_version_id, feature_id, kind, value_bool)
SELECT v.id, f.id, 'boolean', TRUE FROM billing_plan_versions v
CROSS JOIN billing_features f WHERE f.key = 'module.appointments'
ON CONFLICT (plan_version_id, feature_id) DO NOTHING;
ALTER TABLE billing_plan_entitlements ENABLE TRIGGER trg_billing_entitlement_guard;
