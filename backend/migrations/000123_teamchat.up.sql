-- CF-106: explicit participants; realtime carries invalidation only.
CREATE TABLE internal_threads (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 kind TEXT NOT NULL CHECK(kind IN ('direct','group','swarm')),
 title TEXT NOT NULL DEFAULT '' CHECK(length(title)<=200),
 direct_key TEXT, conversation_id BIGINT,
 created_by BIGINT NOT NULL REFERENCES users(id), closed_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id), UNIQUE(organization_id,direct_key),
 CHECK((kind='direct')=(direct_key IS NOT NULL)),
 CHECK(kind<>'swarm' OR conversation_id IS NOT NULL),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id)
);
CREATE INDEX idx_internal_threads_org ON internal_threads(organization_id,id DESC);
CREATE TABLE internal_thread_members (
 id BIGSERIAL PRIMARY KEY, organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 thread_id BIGINT NOT NULL,user_id BIGINT NOT NULL REFERENCES users(id),last_read_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,thread_id) REFERENCES internal_threads(organization_id,id) ON DELETE CASCADE,
 UNIQUE(organization_id,thread_id,user_id)
);
CREATE INDEX idx_internal_thread_members_org ON internal_thread_members(organization_id,user_id,thread_id);
CREATE TABLE internal_messages (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 thread_id BIGINT NOT NULL,sender_id BIGINT NOT NULL REFERENCES users(id),
 body TEXT NOT NULL CHECK(length(trim(body)) BETWEEN 1 AND 8000),
 client_id UUID NOT NULL, is_ai BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,thread_id) REFERENCES internal_threads(organization_id,id) ON DELETE CASCADE,
 UNIQUE(organization_id,thread_id,client_id)
);
CREATE INDEX idx_internal_messages_org ON internal_messages(organization_id,thread_id,id);
CREATE TRIGGER trg_internal_threads_updated BEFORE UPDATE ON internal_threads FOR EACH ROW EXECUTE FUNCTION set_updated_at();
