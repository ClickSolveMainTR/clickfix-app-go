-- CF-10: agent presence per organization. status is what teammates see;
-- manual_status is what the agent picked (restored when an idle agent comes
-- back). The idle sweep turns agents without a heartbeat offline.
CREATE TABLE agent_presence (
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status          TEXT        NOT NULL DEFAULT 'offline',
    manual_status   TEXT        NOT NULL DEFAULT 'online',
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    changed_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, user_id),
    CONSTRAINT chk_agent_presence_status CHECK (status IN ('online', 'away', 'busy', 'offline')),
    CONSTRAINT chk_agent_presence_manual CHECK (manual_status IN ('online', 'away', 'busy', 'offline'))
);

CREATE INDEX idx_agent_presence_user ON agent_presence (user_id);
CREATE INDEX idx_agent_presence_active
    ON agent_presence (last_seen_at)
    WHERE status <> 'offline';
