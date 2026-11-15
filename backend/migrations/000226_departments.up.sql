-- CF-215: departments.
--
-- A department is a business unit of the organization (technical support,
-- sales, billing ...), optionally under a brand. It is not a team: teams
-- belong to a department and people belong to teams. Scope order of the
-- knowledge layer: organization -> brand -> department -> team.
--
-- A department carries its own SLA policy, working hours (teams without
-- their own hours inherit them) and AI settings. Knowledge articles,
-- knowledge sources, business context sections, knowledge suggestions and
-- conversations carry department_id; NULL means organization-wide.
--
-- Visibility is closed by default: a department sees its own knowledge plus
-- organization-wide knowledge (the fallback). A department lead may share a
-- single item with another department (department_knowledge_shares).
-- Holders of tenant.departments.read_all see every department.

CREATE TABLE departments (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    brand_id         UUID         NULL,
    name             VARCHAR(120) NOT NULL,
    description      TEXT         NOT NULL DEFAULT '',
    color            VARCHAR(7)   NOT NULL DEFAULT '',
    -- Offered in the web widget's department picker.
    widget_visible   BOOLEAN      NOT NULL DEFAULT true,
    sort_order       INTEGER      NOT NULL DEFAULT 0,
    sla_policy_id    UUID         NULL,
    -- NULL = the organization's business hours (teams of the department
    -- without their own hours inherit these).
    timezone         VARCHAR(64)  NULL,
    working_hours    JSONB        NULL,
    -- AI settings: the customer AI may answer in this department; extra
    -- instructions for its replies; a hint the AI uses to route a first
    -- message to this department.
    ai_enabled       BOOLEAN      NOT NULL DEFAULT true,
    ai_instructions  TEXT         NOT NULL DEFAULT '',
    routing_hint     TEXT         NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_departments_uuid UNIQUE (uuid),
    CONSTRAINT uq_departments_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_departments_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT chk_departments_name CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    CONSTRAINT chk_departments_description CHECK (length(description) <= 1000),
    CONSTRAINT chk_departments_color CHECK (color = '' OR color ~ '^#[0-9A-Fa-f]{6}$'),
    CONSTRAINT chk_departments_ai_instructions CHECK (length(ai_instructions) <= 4000),
    CONSTRAINT chk_departments_routing_hint CHECK (length(routing_hint) <= 1000),
    CONSTRAINT fk_departments_brand FOREIGN KEY (organization_id, brand_id)
        REFERENCES brands (organization_id, uuid) ON DELETE SET NULL (brand_id),
    CONSTRAINT fk_departments_sla_policy FOREIGN KEY (organization_id, sla_policy_id)
        REFERENCES sla_policies (organization_id, uuid) ON DELETE SET NULL (sla_policy_id)
);

CREATE UNIQUE INDEX uq_departments_organization_name ON departments (organization_id, lower(name));
CREATE INDEX idx_departments_organization_sort ON departments (organization_id, sort_order, id);

CREATE TRIGGER trg_departments_set_updated_at
    BEFORE UPDATE ON departments
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Teams belong to a department (NULL = not in a department yet).
ALTER TABLE teams ADD COLUMN department_id BIGINT NULL;
ALTER TABLE teams ADD CONSTRAINT fk_teams_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id) ON DELETE SET NULL (department_id);
CREATE INDEX idx_teams_department ON teams (organization_id, department_id) WHERE department_id IS NOT NULL;

-- Knowledge scope. A department that still owns knowledge cannot be deleted
-- (NO ACTION); the use case moves the rows first when asked to.
ALTER TABLE knowledge_articles ADD COLUMN department_id BIGINT NULL;
ALTER TABLE knowledge_articles ADD CONSTRAINT fk_knowledge_articles_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id);
CREATE INDEX idx_knowledge_articles_department ON knowledge_articles (organization_id, department_id) WHERE department_id IS NOT NULL;

ALTER TABLE knowledge_sources ADD COLUMN department_id BIGINT NULL;
ALTER TABLE knowledge_sources ADD CONSTRAINT fk_knowledge_sources_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id);
CREATE INDEX idx_knowledge_sources_department ON knowledge_sources (organization_id, department_id) WHERE department_id IS NOT NULL;

ALTER TABLE business_context_sections ADD COLUMN department_id BIGINT NULL;
ALTER TABLE business_context_sections ADD CONSTRAINT fk_business_context_sections_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id);
CREATE INDEX idx_business_context_sections_department ON business_context_sections (organization_id, department_id) WHERE department_id IS NOT NULL;

ALTER TABLE knowledge_suggestions ADD COLUMN department_id BIGINT NULL;
ALTER TABLE knowledge_suggestions ADD CONSTRAINT fk_knowledge_suggestions_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id);
CREATE INDEX idx_knowledge_suggestions_department ON knowledge_suggestions (organization_id, department_id) WHERE department_id IS NOT NULL;

-- Conversations: the department, who set it and the AI's confidence.
-- department_routed_at marks that routing ran once (customer pick, AI
-- prediction or none), so the AI predicts only for a first message.
ALTER TABLE conversations
    ADD COLUMN department_id         BIGINT      NULL,
    ADD COLUMN department_source     TEXT        NULL,
    ADD COLUMN department_confidence REAL        NULL,
    ADD COLUMN department_routed_at  TIMESTAMPTZ NULL;
ALTER TABLE conversations ADD CONSTRAINT fk_conversations_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id) ON DELETE SET NULL (department_id);
ALTER TABLE conversations ADD CONSTRAINT chk_conversations_department_source
    CHECK (department_source IS NULL OR department_source IN ('customer', 'ai', 'agent', 'rule'));
ALTER TABLE conversations ADD CONSTRAINT chk_conversations_department_confidence
    CHECK (department_confidence IS NULL OR (department_confidence >= 0 AND department_confidence <= 1));
CREATE INDEX idx_conversations_department ON conversations (organization_id, department_id, last_message_at DESC)
    WHERE department_id IS NOT NULL AND deleted_at IS NULL;

-- A suggestion drafted from a conversation starts in its department.
CREATE FUNCTION inherit_suggestion_department() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.department_id IS NULL THEN
        SELECT department_id INTO NEW.department_id FROM conversations
        WHERE organization_id = NEW.organization_id AND id = NEW.conversation_id;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_knowledge_suggestion_department BEFORE INSERT ON knowledge_suggestions
    FOR EACH ROW EXECUTE FUNCTION inherit_suggestion_department();

-- A department lead shares one item of the department's knowledge with
-- another department (department_id is the receiving department).
CREATE TABLE department_knowledge_shares (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID        NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    department_id   BIGINT      NOT NULL,
    resource_kind   TEXT        NOT NULL,
    resource_id     BIGINT      NOT NULL,
    shared_by       BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_department_knowledge_shares_kind CHECK (resource_kind IN ('article', 'source', 'context')),
    CONSTRAINT uq_department_knowledge_shares UNIQUE (organization_id, resource_kind, resource_id, department_id),
    CONSTRAINT fk_department_knowledge_shares_department FOREIGN KEY (organization_id, department_id)
        REFERENCES departments (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_department_knowledge_shares_department ON department_knowledge_shares (organization_id, department_id);

CREATE FUNCTION drop_department_knowledge_shares() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    DELETE FROM department_knowledge_shares
    WHERE organization_id = OLD.organization_id AND resource_kind = TG_ARGV[0] AND resource_id = OLD.id;
    RETURN OLD;
END $$;
CREATE TRIGGER trg_knowledge_articles_drop_shares AFTER DELETE ON knowledge_articles
    FOR EACH ROW EXECUTE FUNCTION drop_department_knowledge_shares('article');
CREATE TRIGGER trg_knowledge_sources_drop_shares AFTER DELETE ON knowledge_sources
    FOR EACH ROW EXECUTE FUNCTION drop_department_knowledge_shares('source');
CREATE TRIGGER trg_business_context_sections_drop_shares AFTER DELETE ON business_context_sections
    FOR EACH ROW EXECUTE FUNCTION drop_department_knowledge_shares('context');

-- The visibility rule in one place: organization-wide rows (NULL) are the
-- fallback of every department; a department row is visible to the viewer's
-- departments and to departments it was shared with.
CREATE FUNCTION department_visible(p_org BIGINT, p_kind TEXT, p_id BIGINT, p_department BIGINT, p_viewer BIGINT[])
RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT p_department IS NULL
        OR COALESCE(p_department = ANY (p_viewer), false)
        OR EXISTS (
            SELECT 1 FROM department_knowledge_shares s
            WHERE s.organization_id = p_org AND s.resource_kind = p_kind
              AND s.resource_id = p_id AND s.department_id = ANY (p_viewer)
        )
$$;

-- Department changes of a conversation: the customer's pick, the AI's
-- prediction or a transfer by an agent or the AI, with the context handed
-- to the receiving department.
CREATE TABLE conversation_department_transfers (
    id                 BIGSERIAL PRIMARY KEY,
    uuid               UUID        NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id    BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    conversation_id    BIGINT      NOT NULL,
    from_department_id BIGINT      NULL,
    to_department_id   BIGINT      NULL,
    actor_kind         TEXT        NOT NULL,
    actor_user_id      BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    confidence         REAL        NULL,
    reason             TEXT        NOT NULL DEFAULT '',
    context_summary    TEXT        NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_conversation_department_transfers_actor CHECK (actor_kind IN ('customer', 'ai', 'agent', 'rule')),
    CONSTRAINT chk_conversation_department_transfers_reason CHECK (length(reason) <= 1000),
    CONSTRAINT chk_conversation_department_transfers_summary CHECK (length(context_summary) <= 4000),
    CONSTRAINT chk_conversation_department_transfers_confidence CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
    CONSTRAINT fk_conversation_department_transfers_conversation FOREIGN KEY (organization_id, conversation_id)
        REFERENCES conversations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_conversation_department_transfers_from FOREIGN KEY (organization_id, from_department_id)
        REFERENCES departments (organization_id, id) ON DELETE SET NULL (from_department_id),
    CONSTRAINT fk_conversation_department_transfers_to FOREIGN KEY (organization_id, to_department_id)
        REFERENCES departments (organization_id, id) ON DELETE SET NULL (to_department_id)
);
CREATE INDEX idx_conversation_department_transfers_conversation
    ON conversation_department_transfers (organization_id, conversation_id, created_at DESC);

INSERT INTO permissions (name, slug) VALUES
    ('View departments', 'tenant.departments.read'),
    ('Manage departments', 'tenant.departments.write'),
    ('See the knowledge of every department', 'tenant.departments.read_all')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('owner', 'admin')
  AND p.slug IN ('tenant.departments.read', 'tenant.departments.write', 'tenant.departments.read_all')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('member', 'supervisor', 'agent', 'light_agent', 'viewer')
  AND p.slug = 'tenant.departments.read'
ON CONFLICT DO NOTHING;
