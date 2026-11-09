-- CF-191: documents and links a user shares with the assistant. Only the
-- extracted text is kept (no file bytes), already PII- and secret-masked,
-- for 7 days; the thread's owner reads it, the model reads it as data.
CREATE TABLE assistant_documents (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT      NOT NULL,
    thread_id       BIGINT      NOT NULL,
    kind            TEXT        NOT NULL,
    name            TEXT        NOT NULL,
    source_url      TEXT        NOT NULL DEFAULT '',
    content_type    TEXT        NOT NULL,
    size_bytes      INTEGER     NOT NULL,
    content         TEXT        NOT NULL,
    truncated       BOOLEAN     NOT NULL DEFAULT FALSE,
    masked          BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days',
    CONSTRAINT assistant_documents_uuid_uq UNIQUE (uuid),
    CONSTRAINT assistant_documents_thread_fk FOREIGN KEY (organization_id, thread_id)
        REFERENCES assistant_threads (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT assistant_documents_kind_chk CHECK (kind IN ('file', 'link')),
    CONSTRAINT assistant_documents_name_chk CHECK (char_length(name) BETWEEN 1 AND 200),
    CONSTRAINT assistant_documents_url_chk CHECK (char_length(source_url) <= 2048),
    CONSTRAINT assistant_documents_size_chk CHECK (size_bytes BETWEEN 0 AND 2097152),
    CONSTRAINT assistant_documents_content_chk CHECK (char_length(content) <= 60000)
);

CREATE INDEX idx_assistant_documents_thread ON assistant_documents (organization_id, thread_id, created_at);
CREATE INDEX idx_assistant_documents_expires ON assistant_documents (expires_at);
