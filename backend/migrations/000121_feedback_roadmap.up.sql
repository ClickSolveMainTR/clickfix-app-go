-- CF-100/101: public content is explicitly published, never copied from inbox automatically.
CREATE TABLE feedback_settings (
 organization_id BIGINT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
 public_enabled BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE feature_requests (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 title TEXT NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 200),
 body TEXT NOT NULL DEFAULT '' CHECK(length(body)<=16000),
 status TEXT NOT NULL DEFAULT 'reviewing' CHECK(status IN ('reviewing','planned','in_progress','released','merged')),
 is_public BOOLEAN NOT NULL DEFAULT false,
 merged_into_id BIGINT, created_by BIGINT REFERENCES users(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id),
 FOREIGN KEY(organization_id,merged_into_id) REFERENCES feature_requests(organization_id,id),
 CHECK((status='merged')=(merged_into_id IS NOT NULL)), CHECK(id<>merged_into_id)
);
CREATE INDEX idx_feature_requests_org ON feature_requests(organization_id,status,id DESC);
CREATE TABLE feature_votes (
 id BIGSERIAL PRIMARY KEY, organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 request_id BIGINT NOT NULL, contact_id BIGINT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,request_id) REFERENCES feature_requests(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,contact_id) REFERENCES contacts(organization_id,id) ON DELETE CASCADE,
 UNIQUE(organization_id,request_id,contact_id)
);
CREATE INDEX idx_feature_votes_org ON feature_votes(organization_id,contact_id);
CREATE TABLE feature_sources (
 id BIGSERIAL PRIMARY KEY, organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 request_id BIGINT NOT NULL, conversation_id BIGINT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,request_id) REFERENCES feature_requests(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id) ON DELETE CASCADE,
 UNIQUE(organization_id,request_id,conversation_id)
);
CREATE INDEX idx_feature_sources_org ON feature_sources(organization_id,conversation_id);
CREATE TABLE feedback_comments (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 request_id BIGINT NOT NULL, contact_id BIGINT NOT NULL,
 body TEXT NOT NULL CHECK(length(trim(body)) BETWEEN 1 AND 4000),
 approved BOOLEAN NOT NULL DEFAULT false, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,request_id) REFERENCES feature_requests(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,contact_id) REFERENCES contacts(organization_id,id) ON DELETE CASCADE
);
CREATE INDEX idx_feedback_comments_org ON feedback_comments(organization_id,request_id,id);
CREATE TABLE changelog_entries (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 title TEXT NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 200),
 body TEXT NOT NULL CHECK(length(body) BETWEEN 1 AND 32000),
 tags TEXT[] NOT NULL DEFAULT '{}', request_id BIGINT,
 published_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,request_id) REFERENCES feature_requests(organization_id,id)
);
CREATE INDEX idx_changelog_entries_org ON changelog_entries(organization_id,published_at DESC);
CREATE TRIGGER trg_feedback_settings_updated BEFORE UPDATE ON feedback_settings FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_feature_requests_updated BEFORE UPDATE ON feature_requests FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_changelog_entries_updated BEFORE UPDATE ON changelog_entries FOR EACH ROW EXECUTE FUNCTION set_updated_at();
