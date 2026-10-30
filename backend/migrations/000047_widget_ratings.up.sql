-- CF-55: CSAT ratings the contact gives a finished conversation (web widget).
-- One rating per conversation: a second submit returns the stored one
-- (docs/widget/07 terminal state rule).
CREATE TABLE conversation_ratings (
    id                  BIGSERIAL    PRIMARY KEY,
    uuid                UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    conversation_id     BIGINT       NOT NULL,
    contact_id          BIGINT       NULL,
    channel_instance_id BIGINT       NULL,
    score               SMALLINT     NOT NULL,
    comment             TEXT         NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT conversation_ratings_uuid_uq UNIQUE (uuid),
    CONSTRAINT conversation_ratings_conversation_uq UNIQUE (organization_id, conversation_id),
    CONSTRAINT conversation_ratings_score_ck CHECK (score BETWEEN 1 AND 5),
    CONSTRAINT conversation_ratings_conversation_fk FOREIGN KEY (organization_id, conversation_id)
        REFERENCES conversations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT conversation_ratings_contact_fk FOREIGN KEY (organization_id, contact_id)
        REFERENCES contacts (organization_id, id) ON DELETE SET NULL (contact_id),
    CONSTRAINT conversation_ratings_instance_fk FOREIGN KEY (organization_id, channel_instance_id)
        REFERENCES channel_instances (organization_id, id) ON DELETE SET NULL (channel_instance_id)
);

CREATE INDEX idx_conversation_ratings_org_created ON conversation_ratings (organization_id, created_at DESC);
