-- CF-110: replayable, org-scoped conversation projection. Source rows remain authoritative.
CREATE TABLE analytics_conversations (
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 conversation_id BIGINT NOT NULL,
 conversation_uuid UUID NOT NULL,
 channel_type TEXT NOT NULL,
 team_uuid UUID NULL,
 agent_uuid UUID NULL,
 tag_uuids UUID[] NOT NULL DEFAULT '{}',
 status TEXT NOT NULL,
 opened_at TIMESTAMPTZ NOT NULL,
 first_response_at TIMESTAMPTZ NULL,
 resolved_at TIMESTAMPTZ NULL,
 sla_total INTEGER NOT NULL DEFAULT 0,
 sla_breached INTEGER NOT NULL DEFAULT 0,
 csat SMALLINT NULL,
 projected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(organization_id,conversation_id),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id) ON DELETE CASCADE
);
CREATE INDEX idx_analytics_conversations_range ON analytics_conversations(organization_id,opened_at);
CREATE INDEX idx_analytics_conversations_tags ON analytics_conversations USING GIN(tag_uuids);
