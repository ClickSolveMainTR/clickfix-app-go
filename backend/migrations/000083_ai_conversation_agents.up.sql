CREATE TABLE ai_agent_configs (
 id BIGSERIAL PRIMARY KEY,
 organization_id BIGINT NOT NULL UNIQUE REFERENCES organizations(id) ON DELETE CASCADE,
 config JSONB NOT NULL DEFAULT '{}',
 revision INTEGER NOT NULL DEFAULT 1,
 updated_by BIGINT REFERENCES users(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_ai_agent_configs_updated BEFORE UPDATE ON ai_agent_configs FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE ai_agent_jobs (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 conversation_id BIGINT NOT NULL,
 message_id BIGINT NOT NULL,
 task TEXT NOT NULL CHECK(task IN ('triage','reply')),
 status TEXT NOT NULL DEFAULT 'running' CHECK(status IN ('running','completed','handoff','draft','skipped','unknown')),
 run_uuid UUID REFERENCES ai_runs(uuid),
 result JSONB NOT NULL DEFAULT '{}',
 reason TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id),
 FOREIGN KEY(organization_id,message_id) REFERENCES messages(organization_id,id),
 UNIQUE(organization_id,message_id,task)
);
CREATE INDEX idx_ai_agent_jobs_org ON ai_agent_jobs(organization_id,conversation_id,id DESC);
CREATE TRIGGER trg_ai_agent_jobs_updated BEFORE UPDATE ON ai_agent_jobs FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE ai_resolutions (
 id BIGSERIAL PRIMARY KEY,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 conversation_id BIGINT NOT NULL UNIQUE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id)
);
CREATE INDEX idx_ai_resolutions_org ON ai_resolutions(organization_id,created_at);
