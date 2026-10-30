-- CF-153: knowledge articles whose embeddings could not be built because AI is
-- off, not configured or not in the plan are recorded as skipped (with the
-- reason) instead of failing the indexer; enabling AI re-indexes them.
CREATE TABLE knowledge_index_states (
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 article_id BIGINT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','indexed','skipped')),
 reason TEXT NOT NULL DEFAULT '' CHECK(length(reason)<=64),
 version INTEGER,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(organization_id,article_id),
 FOREIGN KEY(organization_id,article_id) REFERENCES knowledge_articles(organization_id,id) ON DELETE CASCADE
);
CREATE INDEX idx_knowledge_index_states_skipped ON knowledge_index_states(organization_id) WHERE status='skipped';
CREATE TRIGGER trg_knowledge_index_states_updated BEFORE UPDATE ON knowledge_index_states FOR EACH ROW EXECUTE FUNCTION set_updated_at();
