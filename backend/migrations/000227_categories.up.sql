-- CF-218. Category taxonomy, per-record category assignments and the label
-- store that the organization's own classification model (CF-219) trains on.
--
-- Scope order: organization -> brand -> department -> team. Departments come
-- with CF-215; until it lands department_id has no foreign key and NULL means
-- organization-wide. CF-215 adds the FK to departments(organization_id,id).
--
-- Vectors: REAL[] + ai_cosine, the same portable representation as
-- knowledge_chunks. CF-214 moves every vector column (this one included) to
-- pgvector vector(N) with an HNSW index in one migration.

CREATE TABLE categories (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 department_id BIGINT,
 parent_id BIGINT,
 name TEXT NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 120),
 slug TEXT NOT NULL CHECK(slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(slug)<=80),
 description TEXT NOT NULL DEFAULT '' CHECK(length(description)<=1000),
 examples TEXT[] NOT NULL DEFAULT '{}' CHECK(cardinality(examples)<=50),
 position INTEGER NOT NULL DEFAULT 0,
 depth SMALLINT NOT NULL DEFAULT 0 CHECK(depth BETWEEN 0 AND 4),
 archived_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id), UNIQUE(organization_id,slug),
 FOREIGN KEY(organization_id,parent_id) REFERENCES categories(organization_id,id),
 CHECK(parent_id IS NULL OR parent_id<>id)
);
CREATE INDEX idx_categories_org ON categories(organization_id, department_id, parent_id);
CREATE TRIGGER trg_categories_updated BEFORE UPDATE ON categories FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Embedded example texts per category (name/description/examples). Rebuilt
-- lazily when the text or the embedding model changes.
CREATE TABLE category_examples (
 id BIGSERIAL PRIMARY KEY,
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 category_id BIGINT NOT NULL,
 content TEXT NOT NULL CHECK(length(content) BETWEEN 1 AND 4000),
 embedding REAL[] NOT NULL CHECK(cardinality(embedding)>0),
 embedding_model TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,category_id) REFERENCES categories(organization_id,id) ON DELETE CASCADE,
 UNIQUE(organization_id,category_id,embedding_model,content)
);
CREATE INDEX idx_category_examples_org ON category_examples(organization_id, embedding_model);

-- The current category of one record. subject_type is open for the record
-- kinds of the knowledge layer: conversation, archive_record (CF-217),
-- knowledge_unit (CF-216) and suggestion (knowledge_suggestions). subject_id
-- is the record's internal id; it has no FK because the archive and unit
-- tables arrive on other branches (each one deletes its assignments).
CREATE TABLE category_assignments (
 id BIGSERIAL PRIMARY KEY,
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 subject_type TEXT NOT NULL CHECK(subject_type IN ('conversation','archive_record','knowledge_unit','suggestion')),
 subject_id BIGINT NOT NULL,
 subject_uuid UUID NOT NULL,
 department_id BIGINT,
 category_id BIGINT,
 confidence REAL NOT NULL DEFAULT 0 CHECK(confidence BETWEEN 0 AND 1),
 source TEXT NOT NULL CHECK(source IN ('embedding','ai','human','model')),
 classifier TEXT NOT NULL DEFAULT '',
 needs_review BOOLEAN NOT NULL DEFAULT false,
 assigned_by BIGINT REFERENCES users(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,category_id) REFERENCES categories(organization_id,id),
 UNIQUE(organization_id,subject_type,subject_id),
 UNIQUE(organization_id,subject_type,subject_uuid)
);
CREATE INDEX idx_category_assignments_review ON category_assignments(organization_id, needs_review, updated_at DESC);
CREATE INDEX idx_category_assignments_category ON category_assignments(organization_id, category_id);
CREATE TRIGGER trg_category_assignments_updated BEFORE UPDATE ON category_assignments FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Every agent confirmation or correction: masked text, its vector, the old
-- and the new category, who and when. Append-only training data (CF-219).
CREATE TABLE category_labels (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 department_id BIGINT,
 subject_type TEXT NOT NULL CHECK(subject_type IN ('conversation','archive_record','knowledge_unit','suggestion')),
 subject_uuid UUID NOT NULL,
 action TEXT NOT NULL CHECK(action IN ('confirm','correct')),
 content TEXT NOT NULL CHECK(length(content) BETWEEN 1 AND 24000),
 embedding REAL[] CHECK(embedding IS NULL OR cardinality(embedding)>0),
 embedding_model TEXT NOT NULL DEFAULT '',
 previous_category_id BIGINT,
 previous_source TEXT CHECK(previous_source IS NULL OR previous_source IN ('embedding','ai','human','model')),
 previous_confidence REAL CHECK(previous_confidence IS NULL OR previous_confidence BETWEEN 0 AND 1),
 category_id BIGINT NOT NULL,
 labeled_by BIGINT NOT NULL REFERENCES users(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,previous_category_id) REFERENCES categories(organization_id,id),
 FOREIGN KEY(organization_id,category_id) REFERENCES categories(organization_id,id),
 CHECK((embedding IS NULL) = (embedding_model=''))
);
CREATE INDEX idx_category_labels_org ON category_labels(organization_id, id);
CREATE FUNCTION category_labels_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='UPDATE' AND OLD.embedding IS NULL AND NEW.embedding IS NOT NULL
    AND (OLD.content, OLD.category_id, OLD.action, OLD.labeled_by, OLD.created_at) = (NEW.content, NEW.category_id, NEW.action, NEW.labeled_by, NEW.created_at) THEN
  RETURN NEW; -- late vectorization only
 END IF;
 RAISE EXCEPTION 'category labels are append-only' USING ERRCODE='23514'; END $$;
CREATE TRIGGER trg_category_labels_immutable BEFORE UPDATE ON category_labels FOR EACH ROW EXECUTE FUNCTION category_labels_immutable();

INSERT INTO permissions (name, slug) VALUES
    ('View categories', 'tenant.categories.read'),
    ('Manage the category tree', 'tenant.categories.manage'),
    ('Confirm or correct record categories', 'tenant.categories.label'),
    ('Export category labels', 'tenant.categories.export')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('owner', 'admin')
  AND p.slug IN ('tenant.categories.read', 'tenant.categories.manage', 'tenant.categories.label', 'tenant.categories.export')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug = 'supervisor'
  AND p.slug IN ('tenant.categories.read', 'tenant.categories.label', 'tenant.categories.export')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('member', 'agent')
  AND p.slug IN ('tenant.categories.read', 'tenant.categories.label')
ON CONFLICT DO NOTHING;
