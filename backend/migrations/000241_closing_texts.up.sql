-- CF-250: closing texts. An agent picks one (or none) in the composer and it
-- is appended to the end of the reply on every channel. brand_id NULL is an
-- organization-wide text; a brand text is offered only on that brand's
-- conversations. One default per organization and per brand. body_en is
-- optional (empty falls back to body_tr). Bodies may hold variables
-- ({{agent.name}}, {{contact.first_name}}, {{brand.name}}, ...).
CREATE TABLE closing_texts (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    brand_id        UUID         NULL,
    name            TEXT         NOT NULL,
    body_tr         TEXT         NOT NULL,
    body_en         TEXT         NOT NULL DEFAULT '',
    is_default      BOOLEAN      NOT NULL DEFAULT FALSE,
    position        INTEGER      NOT NULL DEFAULT 0,
    created_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT closing_texts_uuid_uq UNIQUE (uuid),
    CONSTRAINT closing_texts_brand_fk FOREIGN KEY (organization_id, brand_id)
        REFERENCES brands (organization_id, uuid) ON DELETE CASCADE,
    CONSTRAINT closing_texts_name_chk CHECK (char_length(name) BETWEEN 1 AND 120),
    CONSTRAINT closing_texts_body_tr_chk CHECK (char_length(body_tr) BETWEEN 1 AND 2000),
    CONSTRAINT closing_texts_body_en_chk CHECK (char_length(body_en) <= 2000)
);

CREATE UNIQUE INDEX uq_closing_texts_default
    ON closing_texts (organization_id, COALESCE(brand_id, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE is_default;
CREATE INDEX idx_closing_texts_org ON closing_texts (organization_id, brand_id, position, id);

CREATE TRIGGER trg_closing_texts_set_updated_at
    BEFORE UPDATE ON closing_texts FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Organization settings. apply_to_ai: append the default closing text to
-- autonomous AI replies (off unless turned on).
CREATE TABLE closing_text_settings (
    organization_id BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    apply_to_ai     BOOLEAN      NOT NULL DEFAULT FALSE,
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- The agent's last pick, offered again in the composer. closing_text_id NULL
-- means the agent chose "no closing text".
CREATE TABLE closing_text_choices (
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    closing_text_id BIGINT       NULL REFERENCES closing_texts (id) ON DELETE CASCADE,
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, user_id)
);
