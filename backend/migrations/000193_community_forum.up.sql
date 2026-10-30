-- CF-78: all public writes are moderated before becoming visible.
CREATE TABLE forum_categories (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 slug TEXT NOT NULL CHECK(slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'), name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 120),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id), UNIQUE(organization_id,slug)
);
CREATE TABLE forum_posts (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 category_id BIGINT NOT NULL, topic_id BIGINT, contact_id BIGINT NOT NULL,
 slug TEXT NOT NULL CHECK(slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
 title TEXT NOT NULL DEFAULT '' CHECK(length(title)<=200), body TEXT NOT NULL CHECK(length(body) BETWEEN 1 AND 16000),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','published','rejected')),
 flag_reason TEXT NOT NULL DEFAULT '' CHECK(length(flag_reason)<=500),
 accepted_reply_id BIGINT, conversation_id BIGINT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id), UNIQUE(organization_id,slug), UNIQUE(organization_id,conversation_id),
 FOREIGN KEY(organization_id,category_id) REFERENCES forum_categories(organization_id,id),
 FOREIGN KEY(organization_id,topic_id) REFERENCES forum_posts(organization_id,id),
 FOREIGN KEY(organization_id,accepted_reply_id) REFERENCES forum_posts(organization_id,id),
 FOREIGN KEY(organization_id,contact_id) REFERENCES contacts(organization_id,id),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id),
 CHECK(topic_id IS NOT NULL OR length(title)>0), CHECK(topic_id IS NULL OR accepted_reply_id IS NULL), CHECK(topic_id IS NULL OR topic_id<>id)
);
CREATE INDEX idx_forum_posts_public ON forum_posts(organization_id,status,category_id,created_at DESC);
CREATE INDEX idx_forum_posts_topic ON forum_posts(organization_id,topic_id,created_at);
CREATE TABLE forum_votes (
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE, post_id BIGINT NOT NULL, contact_id BIGINT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(organization_id,post_id,contact_id),
 FOREIGN KEY(organization_id,post_id) REFERENCES forum_posts(organization_id,id),
 FOREIGN KEY(organization_id,contact_id) REFERENCES contacts(organization_id,id)
);
CREATE TABLE forum_subscriptions (
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE, topic_id BIGINT NOT NULL, contact_id BIGINT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(organization_id,topic_id,contact_id),
 FOREIGN KEY(organization_id,topic_id) REFERENCES forum_posts(organization_id,id),
 FOREIGN KEY(organization_id,contact_id) REFERENCES contacts(organization_id,id)
);
-- Durable visitor notification inbox, created atomically when a reply is approved.
CREATE TABLE forum_notifications (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE, post_id BIGINT NOT NULL, contact_id BIGINT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), read_at TIMESTAMPTZ,
 UNIQUE(organization_id,post_id,contact_id),
 FOREIGN KEY(organization_id,post_id) REFERENCES forum_posts(organization_id,id),
 FOREIGN KEY(organization_id,contact_id) REFERENCES contacts(organization_id,id)
);
CREATE INDEX idx_forum_notifications_contact ON forum_notifications(organization_id,contact_id,created_at DESC);
CREATE TRIGGER trg_forum_categories_updated BEFORE UPDATE ON forum_categories FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_forum_posts_updated BEFORE UPDATE ON forum_posts FOR EACH ROW EXECUTE FUNCTION set_updated_at();
