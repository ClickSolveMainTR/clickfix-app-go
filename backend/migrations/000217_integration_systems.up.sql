-- CF-197: "Sistemini bağla". An organization connects its own system
-- (accounting, e-commerce, SaaS) on one screen: which concepts it has
-- (custom object types), where each concept is read from (an endpoint of
-- the organization's API keyed by the customer's external id), which
-- actions can be run against it and plain-language playbooks that tell the
-- AI how to work. Secrets are stored encrypted (APP_ENCRYPTION_KEY); call
-- logs keep a status and a masked path only, never a request or response
-- body.

-- One connection per organization: base URL and authentication.
CREATE TABLE integration_connections (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL DEFAULT '' CHECK (char_length(name) <= 120),
    base_url        TEXT NOT NULL CHECK (char_length(base_url) BETWEEN 8 AND 500),
    auth_type       TEXT NOT NULL DEFAULT 'none' CHECK (auth_type IN ('none', 'bearer', 'header', 'basic')),
    auth_header     TEXT NOT NULL DEFAULT '' CHECK (auth_header ~ '^[A-Za-z0-9-]{0,64}$'),
    auth_enc        TEXT NOT NULL DEFAULT '',
    timeout_ms      INTEGER NOT NULL DEFAULT 8000 CHECK (timeout_ms BETWEEN 1000 AND 15000),
    updated_by      BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT integration_connections_org_uq UNIQUE (organization_id)
);
CREATE TRIGGER trg_integration_connections_updated BEFORE UPDATE ON integration_connections FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- "Bilgi nereden gelsin": one read endpoint per concept. path_template is
-- relative to the connection base URL and holds {external_id} or
-- {company_external_id}; fields are the response paths picked from a real
-- answer ([{path, label}]).
CREATE TABLE integration_sources (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    object_type_id  BIGINT NOT NULL REFERENCES custom_object_types(id) ON DELETE CASCADE,
    path_template   TEXT NOT NULL CHECK (char_length(path_template) BETWEEN 1 AND 500),
    identity_key    TEXT NOT NULL DEFAULT 'external_id' CHECK (identity_key IN ('external_id', 'company_external_id')),
    items_path      TEXT NOT NULL DEFAULT '' CHECK (char_length(items_path) <= 200),
    fields          JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(fields) = 'array'),
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT integration_sources_type_uq UNIQUE (organization_id, object_type_id)
);
CREATE TRIGGER trg_integration_sources_updated BEFORE UPDATE ON integration_sources FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- "Yapılabilecek işler": a write endpoint of the organization's system.
-- allowed_for says who may run it: the customer-facing AI agent, a
-- member (assistant card, MCP, panel) or a workflow.
CREATE TABLE integration_actions (
    id                BIGSERIAL PRIMARY KEY,
    uuid              UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id   BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    key               TEXT NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{1,49}$'),
    title             TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
    description       TEXT NOT NULL DEFAULT '' CHECK (char_length(description) <= 1000),
    object_type_id    BIGINT REFERENCES custom_object_types(id) ON DELETE SET NULL,
    method            TEXT NOT NULL DEFAULT 'POST' CHECK (method IN ('POST', 'PUT', 'PATCH', 'DELETE')),
    path_template     TEXT NOT NULL CHECK (char_length(path_template) BETWEEN 1 AND 500),
    params            JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(params) = 'array'),
    requires_approval BOOLEAN NOT NULL DEFAULT TRUE,
    allowed_for       TEXT[] NOT NULL DEFAULT ARRAY['agent']::text[]
        CHECK (cardinality(allowed_for) > 0 AND allowed_for <@ ARRAY['ai', 'agent', 'workflow']::text[]),
    enabled           BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT integration_actions_key_uq UNIQUE (organization_id, key)
);
CREATE TRIGGER trg_integration_actions_updated BEFORE UPDATE ON integration_actions FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- "AI'a nasıl çalışacağını anlat": a plain-language playbook. The intent is
-- offered to the AI triage as a candidate; when the AI classifies a
-- message as this intent, the listed concepts are fetched and the
-- instructions (and the linked procedure, if any) go into the reply input.
CREATE TABLE integration_playbooks (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    intent          TEXT NOT NULL CHECK (intent ~ '^[a-z][a-z0-9_]{1,63}$'),
    title           TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    instructions    TEXT NOT NULL CHECK (char_length(instructions) BETWEEN 1 AND 3000),
    concept_keys    TEXT[] NOT NULL DEFAULT '{}'::text[] CHECK (cardinality(concept_keys) <= 10),
    action_keys     TEXT[] NOT NULL DEFAULT '{}'::text[] CHECK (cardinality(action_keys) <= 10),
    procedure_id    BIGINT REFERENCES ai_procedures(id) ON DELETE SET NULL,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT integration_playbooks_intent_uq UNIQUE (organization_id, intent)
);
CREATE TRIGGER trg_integration_playbooks_updated BEFORE UPDATE ON integration_playbooks FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Call log: who called what and how it ended. masked_path has secret
-- query values replaced; no bodies are stored.
CREATE TABLE integration_calls (
    id              BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN ('source', 'action', 'test')),
    target          TEXT NOT NULL CHECK (char_length(target) <= 100),
    channel         TEXT NOT NULL CHECK (channel IN ('ai', 'agent', 'workflow', 'test')),
    method          TEXT NOT NULL,
    masked_path     TEXT NOT NULL CHECK (char_length(masked_path) <= 600),
    status_code     INTEGER NOT NULL DEFAULT 0,
    ok              BOOLEAN NOT NULL,
    error_code      TEXT NOT NULL DEFAULT '',
    duration_ms     INTEGER NOT NULL DEFAULT 0,
    actor_user_id   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX integration_calls_org_idx ON integration_calls (organization_id, created_at DESC);

INSERT INTO permissions (name, slug) VALUES
    ('View connected system settings', 'tenant.integrations.read'),
    ('Manage connected system settings', 'tenant.integrations.manage'),
    ('Use connected system data and actions', 'tenant.integrations.use')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('owner', 'admin')
  AND p.slug IN ('tenant.integrations.read', 'tenant.integrations.manage', 'tenant.integrations.use')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('member', 'supervisor', 'agent')
  AND p.slug IN ('tenant.integrations.read', 'tenant.integrations.use')
ON CONFLICT DO NOTHING;
