-- CF-88..93: one organization-scoped issue model for native and external trackers.
CREATE TABLE engineering_projects (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE (organization_id,id), UNIQUE (organization_id,name)
);
CREATE TABLE engineering_releases (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 version TEXT NOT NULL CHECK(length(version) BETWEEN 1 AND 120), changelog_url TEXT NOT NULL DEFAULT '',
 released_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE (organization_id,id), UNIQUE(organization_id,version)
);
CREATE TABLE engineering_issues (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 project_id BIGINT, release_id BIGINT,
 title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 240), description TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL DEFAULT 'backlog' CHECK(status IN ('backlog','todo','in_progress','done','canceled')),
 priority TEXT NOT NULL DEFAULT 'normal' CHECK(priority IN ('low','normal','high','urgent')),
 labels TEXT[] NOT NULL DEFAULT '{}', cycle TEXT NOT NULL DEFAULT '',
 assignee_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
 created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
 context JSONB NOT NULL DEFAULT '{}', revision INTEGER NOT NULL DEFAULT 1,
 resolved_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id),
 FOREIGN KEY(organization_id,project_id) REFERENCES engineering_projects(organization_id,id),
 FOREIGN KEY(organization_id,release_id) REFERENCES engineering_releases(organization_id,id)
);
CREATE TABLE engineering_conversations (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 issue_id BIGINT NOT NULL, conversation_id BIGINT NOT NULL,
 notify_customer BOOLEAN NOT NULL DEFAULT true,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,issue_id) REFERENCES engineering_issues(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id) ON DELETE CASCADE,
 UNIQUE(organization_id,issue_id,conversation_id)
);
CREATE TABLE engineering_comments (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 issue_id BIGINT NOT NULL, author_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
 body TEXT NOT NULL CHECK(length(body) BETWEEN 1 AND 10000),
 source TEXT NOT NULL DEFAULT 'native', external_id TEXT NOT NULL DEFAULT '',
 UNIQUE(organization_id,id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,issue_id) REFERENCES engineering_issues(organization_id,id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX uq_engineering_comments_external ON engineering_comments(organization_id,issue_id,source,external_id) WHERE external_id <> '';
CREATE TABLE engineering_integrations (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 provider TEXT NOT NULL CHECK(provider IN ('linear','jira','github','sentry')),
 credentials_enc TEXT NOT NULL, config JSONB NOT NULL DEFAULT '{}', enabled BOOLEAN NOT NULL DEFAULT true,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id), UNIQUE(organization_id,provider)
);
CREATE TABLE engineering_external_issues (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 integration_id BIGINT NOT NULL, issue_id BIGINT NOT NULL,
 external_id TEXT NOT NULL, external_key TEXT NOT NULL DEFAULT '', external_url TEXT NOT NULL DEFAULT '',
 synced_revision INTEGER NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,integration_id) REFERENCES engineering_integrations(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,issue_id) REFERENCES engineering_issues(organization_id,id) ON DELETE CASCADE,
 UNIQUE(organization_id,integration_id,issue_id)
);
CREATE TABLE engineering_deliveries (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 issue_id BIGINT NOT NULL, conversation_id BIGINT NOT NULL, release_id BIGINT,
 generation TEXT NOT NULL, resolution_at TIMESTAMPTZ, content TEXT NOT NULL, actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','queued','skipped','failed')),
 attempts INTEGER NOT NULL DEFAULT 0, available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 message_uuid UUID, last_error TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,issue_id) REFERENCES engineering_issues(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,release_id) REFERENCES engineering_releases(organization_id,id),
 UNIQUE(organization_id,issue_id,conversation_id,generation)
);
CREATE TABLE engineering_oauth_states (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 integration_id BIGINT NOT NULL, user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 state_hash TEXT NOT NULL UNIQUE, expires_at TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,integration_id) REFERENCES engineering_integrations(organization_id,id) ON DELETE CASCADE
);
CREATE TABLE engineering_webhook_receipts (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 integration_id BIGINT NOT NULL, delivery_key TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,integration_id) REFERENCES engineering_integrations(organization_id,id) ON DELETE CASCADE,
 UNIQUE(organization_id,integration_id,delivery_key)
);
CREATE TABLE engineering_escalations (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 conversation_id BIGINT NOT NULL, tier TEXT NOT NULL CHECK(tier IN ('l1','l2','engineering')),
 team_id BIGINT, reason TEXT NOT NULL, due_at TIMESTAMPTZ NOT NULL,
 actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,team_id) REFERENCES teams(organization_id,id)
);
CREATE INDEX idx_engineering_projects_org ON engineering_projects(organization_id);
CREATE TRIGGER engineering_projects_updated BEFORE UPDATE ON engineering_projects FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX idx_engineering_releases_org ON engineering_releases(organization_id);
CREATE TRIGGER engineering_releases_updated BEFORE UPDATE ON engineering_releases FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX idx_engineering_issues_org ON engineering_issues(organization_id);
CREATE TRIGGER engineering_issues_updated BEFORE UPDATE ON engineering_issues FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX idx_engineering_conversations_org ON engineering_conversations(organization_id);
CREATE TRIGGER engineering_conversations_updated BEFORE UPDATE ON engineering_conversations FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX idx_engineering_comments_org ON engineering_comments(organization_id);
CREATE TRIGGER engineering_comments_updated BEFORE UPDATE ON engineering_comments FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX idx_engineering_integrations_org ON engineering_integrations(organization_id);
CREATE TRIGGER engineering_integrations_updated BEFORE UPDATE ON engineering_integrations FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX idx_engineering_external_issues_org ON engineering_external_issues(organization_id);
CREATE TRIGGER engineering_external_issues_updated BEFORE UPDATE ON engineering_external_issues FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX idx_engineering_deliveries_org ON engineering_deliveries(organization_id);
CREATE TRIGGER engineering_deliveries_updated BEFORE UPDATE ON engineering_deliveries FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX idx_engineering_oauth_states_org ON engineering_oauth_states(organization_id);
CREATE TRIGGER engineering_oauth_states_updated BEFORE UPDATE ON engineering_oauth_states FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX idx_engineering_webhook_receipts_org ON engineering_webhook_receipts(organization_id);
CREATE TRIGGER engineering_webhook_receipts_updated BEFORE UPDATE ON engineering_webhook_receipts FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX idx_engineering_escalations_org ON engineering_escalations(organization_id);
CREATE TRIGGER engineering_escalations_updated BEFORE UPDATE ON engineering_escalations FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX idx_engineering_issues_list ON engineering_issues(organization_id,status,updated_at DESC);
CREATE INDEX idx_engineering_deliveries_pending ON engineering_deliveries(available_at) WHERE status = 'pending';
CREATE UNIQUE INDEX uq_engineering_external_id ON engineering_external_issues(organization_id,integration_id,external_id) WHERE external_id<>'';
CREATE TABLE engineering_comment_sync (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 comment_id BIGINT NOT NULL,
 integration_id BIGINT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,integration_id) REFERENCES engineering_integrations(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,comment_id) REFERENCES engineering_comments(organization_id,id) ON DELETE CASCADE,
 UNIQUE(organization_id,comment_id,integration_id)
);
CREATE INDEX idx_engineering_comment_sync_org ON engineering_comment_sync(organization_id);
CREATE TRIGGER engineering_comment_sync_updated BEFORE UPDATE ON engineering_comment_sync FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE engineering_error_events (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 issue_id BIGINT NOT NULL, conversation_id BIGINT NOT NULL,
 sentry_issue_id TEXT NOT NULL,event_id TEXT NOT NULL,event_url TEXT NOT NULL,summary TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,issue_id) REFERENCES engineering_issues(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id) ON DELETE CASCADE,
 UNIQUE(organization_id,conversation_id,event_id)
);
CREATE INDEX idx_engineering_error_events_org ON engineering_error_events(organization_id,conversation_id);
CREATE TRIGGER engineering_error_events_updated BEFORE UPDATE ON engineering_error_events FOR EACH ROW EXECUTE FUNCTION set_updated_at();
