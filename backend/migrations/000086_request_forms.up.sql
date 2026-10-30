-- CF-52: dynamic request forms. An organization defines forms whose fields
-- are system fields (name, e-mail, phone, subject, message, priority) or its
-- own custom fields (CF-7, conversation and contact). Fields can be required,
-- required only before the conversation is resolved, and conditional on the
-- value of another field of the form (a "topic" select drives what else is
-- asked). Forms are offered per channel: the web widget uses the active form
-- of the widget channel as its start (pre-chat) form.
--
-- fields: [{"key", "source": "system"|"conversation"|"contact",
--           "custom_field_uuid", "label", "help", "required",
--           "required_to_resolve", "condition": {"field", "operator", "value"}}]

CREATE TABLE request_forms (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            TEXT         NOT NULL,
    description     TEXT         NOT NULL DEFAULT '',
    is_active       BOOLEAN      NOT NULL DEFAULT TRUE,
    channels        TEXT[]       NOT NULL DEFAULT '{widget}',
    fields          JSONB        NOT NULL DEFAULT '[]'::jsonb,
    position        INTEGER      NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT request_forms_uuid_uq UNIQUE (uuid),
    CONSTRAINT request_forms_org_id_uq UNIQUE (organization_id, id),
    CONSTRAINT request_forms_name_chk CHECK (char_length(name) BETWEEN 1 AND 120),
    CONSTRAINT request_forms_fields_chk CHECK (jsonb_typeof(fields) = 'array')
);

CREATE INDEX idx_request_forms_org ON request_forms (organization_id, position, id);

CREATE TRIGGER trg_request_forms_set_updated_at
    BEFORE UPDATE ON request_forms FOR EACH ROW EXECUTE FUNCTION set_updated_at();
