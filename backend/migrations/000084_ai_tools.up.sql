CREATE TABLE ai_tools (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 name TEXT NOT NULL,
 description TEXT NOT NULL DEFAULT '',
 schema JSONB NOT NULL,
 url TEXT NOT NULL,
 auth_enc TEXT NOT NULL DEFAULT '',
 requires_approval BOOLEAN NOT NULL DEFAULT true,
 enabled BOOLEAN NOT NULL DEFAULT false,
 daily_limit INTEGER NOT NULL DEFAULT 100 CHECK(daily_limit BETWEEN 1 AND 1000),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,name)
);
CREATE INDEX idx_ai_tools_org ON ai_tools(organization_id,id);
CREATE TRIGGER trg_ai_tools_updated BEFORE UPDATE ON ai_tools FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE ai_tool_calls (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 conversation_id BIGINT NOT NULL,
 tool_name TEXT NOT NULL,
 invocation_key UUID NOT NULL,
 actor_id BIGINT REFERENCES users(id),
 dry_run BOOLEAN NOT NULL DEFAULT false,
 status TEXT NOT NULL DEFAULT 'started' CHECK(status IN ('started','completed','failed','dry_run')),
 error_code TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id),
 UNIQUE(organization_id,invocation_key)
);
CREATE INDEX idx_ai_tool_calls_org ON ai_tool_calls(organization_id,tool_name,created_at);
