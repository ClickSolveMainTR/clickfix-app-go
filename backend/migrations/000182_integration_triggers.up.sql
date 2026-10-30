-- The per-organization counter serializes event cursor allocation with commit order.
CREATE TABLE integration_event_cursors (
 organization_id BIGINT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,last_sequence BIGINT NOT NULL DEFAULT 0,pruned_sequence BIGINT NOT NULL DEFAULT 0
);
CREATE TABLE integration_trigger_events (
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 sequence BIGINT NOT NULL,event_id UUID NOT NULL,event_name TEXT NOT NULL,payload JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),PRIMARY KEY(organization_id,sequence),UNIQUE(organization_id,event_id)
);
CREATE INDEX idx_integration_trigger_events_org ON integration_trigger_events(organization_id,event_name,sequence);
