-- CF-217: history pipeline. Past support records (an uploaded export today,
-- connectors later, our own closed conversations every night) become
-- knowledge. Records are anonymized on the server before they are stored or
-- sent to a provider, kept in a read-only archive outside the inbox,
-- categorized and clustered by vector similarity. A cluster whose cases share
-- the same resolution often enough (evidence threshold) produces a
-- knowledge_suggestions row; approvals, rejections and corrections are kept
-- with their reason and embedding so a rejected suggestion does not return.
--
-- Vectors are pgvector vector(1536) (CF-214), compared with cosine distance
-- (<=>).
-- department_id on the history tables has no foreign key yet (follow-up to
-- CF-215); NULL means organization-wide. knowledge_suggestions.department_id
-- already comes from the departments migration (000226).

-- Per-organization settings. Missing row = defaults below.
CREATE TABLE history_settings (
    organization_id         BIGINT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    evidence_threshold      INTEGER NOT NULL DEFAULT 3 CHECK (evidence_threshold BETWEEN 2 AND 100),
    cluster_similarity      REAL NOT NULL DEFAULT 0.80 CHECK (cluster_similarity BETWEEN 0.5 AND 0.99),
    resolution_similarity   REAL NOT NULL DEFAULT 0.75 CHECK (resolution_similarity BETWEEN 0.5 AND 0.99),
    rejection_similarity    REAL NOT NULL DEFAULT 0.85 CHECK (rejection_similarity BETWEEN 0.5 AND 0.99),
    raw_retention_hours     INTEGER NOT NULL DEFAULT 24 CHECK (raw_retention_hours BETWEEN 1 AND 720),
    archive_retention_days  INTEGER NOT NULL DEFAULT 0 CHECK (archive_retention_days BETWEEN 0 AND 3650),
    nightly_enabled         BOOLEAN NOT NULL DEFAULT false,
    nightly_consent_analysis BOOLEAN NOT NULL DEFAULT false,
    nightly_consent_training BOOLEAN NOT NULL DEFAULT false,
    nightly_watermark       TIMESTAMPTZ,
    updated_by              BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (NOT nightly_enabled OR nightly_consent_analysis)
);
CREATE TRIGGER trg_history_settings_updated BEFORE UPDATE ON history_settings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- One pipeline run. Every run belongs to an organization; the consent given
-- by the data owner is recorded on the run itself.
CREATE TABLE history_imports (
    id                 BIGSERIAL PRIMARY KEY,
    uuid               UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id    BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    brand_id           UUID,
    department_id      BIGINT,
    source             TEXT NOT NULL CHECK (source IN ('upload', 'nightly', 'connector')),
    connector          TEXT NOT NULL DEFAULT '' CHECK (connector ~ '^[a-z0-9_]{0,40}$'),
    file_name          TEXT NOT NULL DEFAULT '' CHECK (char_length(file_name) <= 255),
    status             TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled', 'rolled_back')),
    stage              TEXT NOT NULL DEFAULT 'queued' CHECK (stage IN ('queued', 'reading', 'anonymizing', 'embedding', 'clustering', 'suggesting', 'done')),
    total_records      INTEGER NOT NULL DEFAULT 0 CHECK (total_records >= 0),
    processed_records  INTEGER NOT NULL DEFAULT 0 CHECK (processed_records >= 0),
    skipped_records    INTEGER NOT NULL DEFAULT 0 CHECK (skipped_records >= 0),
    clusters_touched   INTEGER NOT NULL DEFAULT 0 CHECK (clusters_touched >= 0),
    suggestions_created INTEGER NOT NULL DEFAULT 0 CHECK (suggestions_created >= 0),
    suggestions_suppressed INTEGER NOT NULL DEFAULT 0 CHECK (suggestions_suppressed >= 0),
    consent_analysis   BOOLEAN NOT NULL,
    consent_training   BOOLEAN NOT NULL DEFAULT false,
    consent_by         BIGINT REFERENCES users(id) ON DELETE SET NULL,
    consent_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    consent_version    TEXT NOT NULL DEFAULT '2026-11' CHECK (char_length(consent_version) <= 20),
    raw_key            TEXT NOT NULL DEFAULT '',
    raw_retain_until   TIMESTAMPTZ,
    raw_deleted_at     TIMESTAMPTZ,
    window_start       TIMESTAMPTZ,
    window_end         TIMESTAMPTZ,
    cancel_requested   BOOLEAN NOT NULL DEFAULT false,
    lease_until        TIMESTAMPTZ,
    error              TEXT NOT NULL DEFAULT '' CHECK (char_length(error) <= 500),
    created_by         BIGINT REFERENCES users(id) ON DELETE SET NULL,
    started_at         TIMESTAMPTZ,
    finished_at        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, brand_id) REFERENCES brands(organization_id, uuid) ON DELETE SET NULL (brand_id),
    -- Processing at all needs the analysis consent.
    CHECK (consent_analysis)
);
CREATE INDEX idx_history_imports_org ON history_imports (organization_id, created_at DESC);
CREATE INDEX idx_history_imports_due ON history_imports (status, created_at) WHERE status IN ('pending', 'running');
CREATE INDEX idx_history_imports_raw ON history_imports (raw_retain_until) WHERE raw_key <> '' AND raw_deleted_at IS NULL;
CREATE TRIGGER trg_history_imports_updated BEFORE UPDATE ON history_imports FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Groups of similar cases. Clusters live across runs, so evidence from an
-- upload and from later nightly runs adds up.
CREATE TABLE history_clusters (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id  BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    brand_id         UUID,
    department_id    BIGINT,
    category         TEXT NOT NULL DEFAULT '' CHECK (char_length(category) <= 100),
    label            TEXT NOT NULL DEFAULT '' CHECK (char_length(label) <= 300),
    centroid         vector(1536) NOT NULL,
    embedding_model  TEXT NOT NULL,
    size             INTEGER NOT NULL DEFAULT 0 CHECK (size >= 0),
    evidence         INTEGER NOT NULL DEFAULT 0 CHECK (evidence >= 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, brand_id) REFERENCES brands(organization_id, uuid) ON DELETE SET NULL (brand_id)
);
CREATE INDEX idx_history_clusters_org ON history_clusters (organization_id, category);
CREATE TRIGGER trg_history_clusters_updated BEFORE UPDATE ON history_clusters FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- The read-only archive. Text is stored already anonymized; it never becomes
-- an inbox conversation. Only classification columns may change afterwards.
CREATE TABLE history_cases (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id  BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    import_id        BIGINT NOT NULL,
    brand_id         UUID,
    department_id    BIGINT,
    source           TEXT NOT NULL CHECK (source IN ('upload', 'nightly', 'connector')),
    -- sha256 of the source system's ticket id (or our conversation uuid):
    -- deduplicates re-uploads without keeping the original identifier.
    external_ref     TEXT NOT NULL CHECK (external_ref ~ '^[0-9a-f]{64}$'),
    conversation_id  BIGINT,
    subject          TEXT NOT NULL DEFAULT '' CHECK (char_length(subject) <= 500),
    problem          TEXT NOT NULL CHECK (char_length(problem) BETWEEN 1 AND 20000),
    resolution       TEXT NOT NULL DEFAULT '' CHECK (char_length(resolution) <= 20000),
    message_count    INTEGER NOT NULL DEFAULT 0 CHECK (message_count >= 0),
    locale           TEXT NOT NULL DEFAULT 'tr' CHECK (locale IN ('tr', 'en')),
    training_allowed BOOLEAN NOT NULL DEFAULT false,
    category         TEXT NOT NULL DEFAULT '' CHECK (char_length(category) <= 100),
    embedding        vector(1536) NOT NULL,
    resolution_embedding vector(1536),
    embedding_model  TEXT NOT NULL,
    cluster_id       BIGINT,
    opened_at        TIMESTAMPTZ,
    closed_at        TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, external_ref),
    FOREIGN KEY (organization_id, import_id) REFERENCES history_imports(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, cluster_id) REFERENCES history_clusters(organization_id, id) ON DELETE SET NULL (cluster_id),
    FOREIGN KEY (organization_id, brand_id) REFERENCES brands(organization_id, uuid) ON DELETE SET NULL (brand_id),
    FOREIGN KEY (organization_id, conversation_id) REFERENCES conversations(organization_id, id) ON DELETE SET NULL (conversation_id)
);
CREATE INDEX idx_history_cases_org ON history_cases (organization_id, created_at DESC);
CREATE INDEX idx_history_cases_import ON history_cases (organization_id, import_id);
CREATE INDEX idx_history_cases_cluster ON history_cases (organization_id, cluster_id);

CREATE FUNCTION history_case_read_only() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.subject IS DISTINCT FROM OLD.subject OR NEW.problem IS DISTINCT FROM OLD.problem
  OR NEW.resolution IS DISTINCT FROM OLD.resolution OR NEW.external_ref IS DISTINCT FROM OLD.external_ref
  OR NEW.organization_id IS DISTINCT FROM OLD.organization_id OR NEW.import_id IS DISTINCT FROM OLD.import_id THEN
  RAISE EXCEPTION 'history cases are read-only' USING ERRCODE='23514';
 END IF;
 RETURN NEW; END $$;
CREATE TRIGGER trg_history_case_read_only BEFORE UPDATE ON history_cases FOR EACH ROW EXECUTE FUNCTION history_case_read_only();

-- Suggestions from the history pipeline reuse knowledge_suggestions.
ALTER TABLE knowledge_suggestions ALTER COLUMN conversation_id DROP NOT NULL;
ALTER TABLE knowledge_suggestions
    ADD COLUMN source TEXT NOT NULL DEFAULT 'conversation' CHECK (source IN ('conversation', 'history')),
    ADD COLUMN cluster_id BIGINT,
    ADD COLUMN import_id BIGINT,
    ADD COLUMN evidence_count INTEGER NOT NULL DEFAULT 0 CHECK (evidence_count >= 0),
    ADD COLUMN conflict_article_id BIGINT,
    ADD COLUMN conflict_score REAL NOT NULL DEFAULT 0 CHECK (conflict_score BETWEEN 0 AND 1),
    ADD CONSTRAINT knowledge_suggestions_cluster_fk FOREIGN KEY (organization_id, cluster_id) REFERENCES history_clusters(organization_id, id) ON DELETE SET NULL (cluster_id),
    ADD CONSTRAINT knowledge_suggestions_import_fk FOREIGN KEY (organization_id, import_id) REFERENCES history_imports(organization_id, id) ON DELETE SET NULL (import_id),
    ADD CONSTRAINT knowledge_suggestions_conflict_fk FOREIGN KEY (organization_id, conflict_article_id) REFERENCES knowledge_articles(organization_id, id),
    ADD CONSTRAINT knowledge_suggestions_source_chk CHECK ((source = 'conversation') = (conversation_id IS NOT NULL));
CREATE UNIQUE INDEX uq_knowledge_suggestions_cluster_pending ON knowledge_suggestions (organization_id, cluster_id) WHERE cluster_id IS NOT NULL AND status = 'pending';

-- Decision memory: every approval, rejection and correction with its reason.
-- The embedding is of the suggested title and body, so a later candidate that
-- is close to a rejected one is suppressed.
CREATE TABLE history_decisions (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    suggestion_id   BIGINT REFERENCES knowledge_suggestions(id) ON DELETE SET NULL,
    cluster_id      BIGINT,
    department_id   BIGINT,
    decision        TEXT NOT NULL CHECK (decision IN ('approved', 'rejected', 'corrected')),
    reason          TEXT NOT NULL CHECK (char_length(reason) BETWEEN 1 AND 2000),
    title           TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 300),
    body            TEXT NOT NULL CHECK (char_length(body) <= 30000),
    correction      TEXT NOT NULL DEFAULT '' CHECK (char_length(correction) <= 30000),
    embedding       vector(1536) NOT NULL,
    embedding_model TEXT NOT NULL,
    decided_by      BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (organization_id, cluster_id) REFERENCES history_clusters(organization_id, id) ON DELETE SET NULL (cluster_id)
);
CREATE INDEX idx_history_decisions_org ON history_decisions (organization_id, decision, created_at DESC);

INSERT INTO permissions (name, slug) VALUES
    ('View the history archive and its runs', 'tenant.history.read'),
    ('Upload history files, cancel and roll back runs, change settings', 'tenant.history.manage'),
    ('Approve, reject or correct suggestions from history', 'tenant.history.review')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('owner', 'admin')
  AND p.slug IN ('tenant.history.read', 'tenant.history.manage', 'tenant.history.review')
ON CONFLICT DO NOTHING;

-- Supervisors lead departments until CF-215 brings a department lead role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug = 'supervisor'
  AND p.slug IN ('tenant.history.read', 'tenant.history.review')
ON CONFLICT DO NOTHING;
