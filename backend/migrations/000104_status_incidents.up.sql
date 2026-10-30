-- CF-102/103. Public slugs resolve a page; every subsequent query carries its organization.
CREATE TABLE status_pages (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL UNIQUE REFERENCES organizations(id) ON DELETE CASCADE,
 slug TEXT NOT NULL UNIQUE CHECK(slug ~ '^[a-z0-9][a-z0-9-]{2,62}$'),
 title TEXT NOT NULL CHECK(char_length(title) BETWEEN 1 AND 120),
 custom_domain TEXT NOT NULL DEFAULT '',published BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_status_pages_updated BEFORE UPDATE ON status_pages FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE status_components (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 name TEXT NOT NULL CHECK(char_length(name) BETWEEN 1 AND 120),group_name TEXT NOT NULL DEFAULT '' CHECK(char_length(group_name)<=120),
 description TEXT NOT NULL DEFAULT '' CHECK(char_length(description)<=2000),sort_order INTEGER NOT NULL DEFAULT 0,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),UNIQUE(organization_id,id)
);
CREATE INDEX idx_status_components_org ON status_components(organization_id,sort_order,id);
CREATE TRIGGER trg_status_components_updated BEFORE UPDATE ON status_components FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE status_incidents (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 title TEXT NOT NULL CHECK(char_length(title) BETWEEN 1 AND 200),
 kind TEXT NOT NULL DEFAULT 'incident' CHECK(kind IN ('incident','maintenance')),
 severity TEXT NOT NULL CHECK(severity IN ('degraded','partial_outage','major_outage','maintenance')),
 status TEXT NOT NULL DEFAULT 'investigating' CHECK(status IN ('scheduled','investigating','identified','monitoring','resolved')),
 public BOOLEAN NOT NULL DEFAULT true,
 started_at TIMESTAMPTZ NOT NULL DEFAULT now(),ends_at TIMESTAMPTZ NULL,resolved_at TIMESTAMPTZ NULL,
 notification_body TEXT NOT NULL DEFAULT '' CHECK(char_length(notification_body)<=5000),
 notification_template JSONB NULL,
 postmortem TEXT NOT NULL DEFAULT '' CHECK(char_length(postmortem)<=20000),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),UNIQUE(organization_id,id),
 CHECK(ends_at IS NULL OR ends_at>started_at),CHECK(resolved_at IS NULL OR resolved_at>=started_at),
 CHECK(kind<>'maintenance' OR ends_at IS NOT NULL)
);
CREATE INDEX idx_status_incidents_org ON status_incidents(organization_id,started_at DESC);
CREATE TRIGGER trg_status_incidents_updated BEFORE UPDATE ON status_incidents FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE status_incident_components (
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,incident_id BIGINT NOT NULL,component_id BIGINT NOT NULL,
 PRIMARY KEY(organization_id,incident_id,component_id),
 FOREIGN KEY(organization_id,incident_id) REFERENCES status_incidents(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,component_id) REFERENCES status_components(organization_id,id) ON DELETE CASCADE
);
CREATE TABLE status_incident_updates (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,incident_id BIGINT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('scheduled','investigating','identified','monitoring','resolved')),
 body TEXT NOT NULL CHECK(char_length(body) BETWEEN 1 AND 10000),created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,incident_id) REFERENCES status_incidents(organization_id,id) ON DELETE CASCADE
);
CREATE INDEX idx_status_incident_updates_org ON status_incident_updates(organization_id,incident_id,id);
CREATE TABLE status_incident_conversations (
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,incident_id BIGINT NOT NULL,conversation_id BIGINT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),PRIMARY KEY(organization_id,incident_id,conversation_id),
 FOREIGN KEY(organization_id,incident_id) REFERENCES status_incidents(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id) ON DELETE CASCADE
);
CREATE INDEX idx_status_incident_conversations_org ON status_incident_conversations(organization_id,conversation_id);
CREATE TABLE status_subscriptions (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 email TEXT NOT NULL CHECK(char_length(email)<=320),confirmed_at TIMESTAMPTZ NULL,
 expires_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '1 day',unsubscribed_at TIMESTAMPTZ NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),UNIQUE(organization_id,email)
);
CREATE INDEX idx_status_subscriptions_org ON status_subscriptions(organization_id,id);

-- Freeze the destination at resolution. A later inbound message on another
-- linked conversation must not change the retry target (message client IDs
-- are unique inside a conversation, not globally).
CREATE TABLE status_incident_deliveries (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 incident_id BIGINT NOT NULL,contact_id BIGINT NOT NULL,conversation_id BIGINT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,incident_id,contact_id),
 FOREIGN KEY(organization_id,incident_id) REFERENCES status_incidents(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,contact_id) REFERENCES contacts(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id) ON DELETE SET NULL(conversation_id)
);
