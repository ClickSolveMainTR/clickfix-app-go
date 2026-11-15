-- CF-221: contact detail page. Agent notes on a contact (the company has
-- company_notes since CF-85) and the cached AI summary of a contact's history.

CREATE TABLE contact_notes (
    id              BIGSERIAL   PRIMARY KEY,
    uuid            UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    contact_id      BIGINT      NOT NULL,
    author_user_id  BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    body            TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT contact_notes_uuid_uq UNIQUE (uuid),
    CONSTRAINT contact_notes_contact_fk FOREIGN KEY (organization_id, contact_id)
        REFERENCES contacts (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT contact_notes_body_chk CHECK (char_length(body) BETWEEN 1 AND 10000)
);

CREATE INDEX idx_contact_notes_contact ON contact_notes (organization_id, contact_id, created_at DESC);
CREATE TRIGGER trg_contact_notes_updated BEFORE UPDATE ON contact_notes FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- One cached summary per contact and audience. audience is 'all' for members
-- who see every conversation and 'user:<id>' for a member limited by team
-- visibility, so a summary never shows a member conversations they cannot
-- open. source_at is the newest activity the summary saw: the page marks it
-- stale when newer activity exists. Erasing or merging a contact deletes it.
CREATE TABLE contact_ai_summaries (
    id              BIGSERIAL   PRIMARY KEY,
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    contact_id      BIGINT      NOT NULL,
    audience        TEXT        NOT NULL DEFAULT 'all',
    summary         TEXT        NOT NULL,
    open_topics     JSONB       NOT NULL DEFAULT '[]',
    sentiment       TEXT        NOT NULL DEFAULT 'unknown',
    locale          TEXT        NOT NULL DEFAULT 'tr',
    run_uuid        UUID        NULL,
    source_at       TIMESTAMPTZ NULL,
    generated_by    BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT contact_ai_summaries_contact_uq UNIQUE (organization_id, contact_id, audience),
    CONSTRAINT contact_ai_summaries_audience_chk CHECK (audience ~ '^(all|user:[0-9]+)$'),
    CONSTRAINT contact_ai_summaries_contact_fk FOREIGN KEY (organization_id, contact_id)
        REFERENCES contacts (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT contact_ai_summaries_summary_chk CHECK (char_length(summary) BETWEEN 1 AND 4000),
    CONSTRAINT contact_ai_summaries_topics_chk CHECK (jsonb_typeof(open_topics) = 'array'),
    CONSTRAINT contact_ai_summaries_sentiment_chk CHECK (sentiment IN ('positive', 'neutral', 'negative', 'unknown')),
    CONSTRAINT contact_ai_summaries_locale_chk CHECK (locale IN ('tr', 'en'))
);

CREATE TRIGGER trg_contact_ai_summaries_updated BEFORE UPDATE ON contact_ai_summaries FOR EACH ROW EXECUTE FUNCTION set_updated_at();
