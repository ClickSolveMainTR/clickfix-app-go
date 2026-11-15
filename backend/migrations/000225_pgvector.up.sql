-- CF-214: pgvector. Embeddings move from REAL[] + ai_cosine (full scan) to a
-- fixed-size vector column with an HNSW index. The dimension is fixed by the
-- single embedding model (platform/ai.EmbeddingDimensions); a model change keeps
-- the dimension and is re-embedded in the background (pending_* columns) while
-- search keeps using the old vectors.
CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE knowledge_chunks
 ADD COLUMN embedding_vec vector(1536),
 ADD COLUMN brand_id UUID,
 ADD COLUMN department_id UUID,
 ADD COLUMN pending_embedding vector(1536),
 ADD COLUMN pending_model TEXT;

-- Vectors of the fixed dimension are copied as is. Any other dimension cannot be
-- compared with the new column: it stays NULL and the re-embedding job rebuilds
-- it from the stored (already masked) chunk text.
UPDATE knowledge_chunks SET embedding_vec=embedding::vector(1536) WHERE cardinality(embedding)=1536;
ALTER TABLE knowledge_chunks DROP COLUMN embedding;
ALTER TABLE knowledge_chunks RENAME COLUMN embedding_vec TO embedding;
DROP FUNCTION ai_cosine(REAL[], REAL[]);

-- Scope columns are copied from the article so vector filters stay on one table.
-- department_id stays NULL (organization wide) until departments land (CF-215).
UPDATE knowledge_chunks c SET brand_id=a.brand_id FROM knowledge_articles a
 WHERE a.organization_id=c.organization_id AND a.id=c.article_id AND a.brand_id IS NOT NULL;
ALTER TABLE knowledge_chunks ADD CONSTRAINT fk_knowledge_chunks_brand FOREIGN KEY(organization_id,brand_id)
 REFERENCES brands(organization_id,uuid) ON DELETE SET NULL (brand_id);
ALTER TABLE knowledge_chunks ADD CONSTRAINT ck_knowledge_chunks_pending
 CHECK((pending_embedding IS NULL)=(pending_model IS NULL));

CREATE FUNCTION knowledge_chunk_scope() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 SELECT brand_id INTO NEW.brand_id FROM knowledge_articles WHERE organization_id=NEW.organization_id AND id=NEW.article_id;
 RETURN NEW;
END $$;
CREATE TRIGGER trg_knowledge_chunk_scope BEFORE INSERT ON knowledge_chunks FOR EACH ROW EXECUTE FUNCTION knowledge_chunk_scope();
-- Moving an article to another brand moves its chunks without re-indexing.
CREATE FUNCTION knowledge_article_scope_sync() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 UPDATE knowledge_chunks SET brand_id=NEW.brand_id WHERE organization_id=NEW.organization_id AND article_id=NEW.id;
 RETURN NEW;
END $$;
CREATE TRIGGER trg_knowledge_article_scope_sync AFTER UPDATE OF brand_id ON knowledge_articles
 FOR EACH ROW WHEN (OLD.brand_id IS DISTINCT FROM NEW.brand_id) EXECUTE FUNCTION knowledge_article_scope_sync();

-- The migration runs in one transaction; more memory keeps the HNSW build in RAM.
-- A parallel build needs that much shared memory, and the Docker default
-- /dev/shm (64MB) is smaller, so the build runs in the migrating backend only.
SET LOCAL maintenance_work_mem='256MB';
SET LOCAL max_parallel_maintenance_workers=0;
CREATE INDEX idx_knowledge_chunks_embedding ON knowledge_chunks USING hnsw (embedding vector_cosine_ops);
CREATE INDEX idx_knowledge_chunks_brand ON knowledge_chunks(organization_id,brand_id);
-- Rows the re-embedding job still has to visit.
CREATE INDEX idx_knowledge_chunks_reembed ON knowledge_chunks(organization_id,embedding_model);
