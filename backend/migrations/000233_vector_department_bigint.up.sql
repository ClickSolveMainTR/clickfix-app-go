-- CF-222: department scope of the vector store follows departments.
--
-- 000225_pgvector added knowledge_chunks.department_id as UUID before
-- departments existed; departments (000226) use BIGINT ids and the composite
-- key (organization_id, department_id) -> departments(organization_id, id).
-- The column was never written (always NULL), so it is converted without a
-- data copy; the check below stops the migration if that is not true.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM knowledge_chunks WHERE department_id IS NOT NULL) THEN
  RAISE EXCEPTION 'knowledge_chunks.department_id has values; CF-222 expects it to be NULL';
 END IF;
END $$;

ALTER TABLE knowledge_chunks ALTER COLUMN department_id TYPE BIGINT USING NULL::bigint;
-- Same delete rule as the article: a department that still owns knowledge
-- cannot be deleted (NO ACTION); moving the article moves its chunks.
ALTER TABLE knowledge_chunks ADD CONSTRAINT fk_knowledge_chunks_department FOREIGN KEY (organization_id, department_id)
 REFERENCES departments (organization_id, id);
CREATE INDEX idx_knowledge_chunks_department ON knowledge_chunks (organization_id, department_id) WHERE department_id IS NOT NULL;

-- Chunks copy brand and department from their article.
CREATE OR REPLACE FUNCTION knowledge_chunk_scope() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 SELECT brand_id, department_id INTO NEW.brand_id, NEW.department_id
  FROM knowledge_articles WHERE organization_id=NEW.organization_id AND id=NEW.article_id;
 RETURN NEW;
END $$;
-- Moving an article to another brand or department moves its chunks without re-indexing.
CREATE OR REPLACE FUNCTION knowledge_article_scope_sync() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 UPDATE knowledge_chunks SET brand_id=NEW.brand_id, department_id=NEW.department_id
  WHERE organization_id=NEW.organization_id AND article_id=NEW.id;
 RETURN NEW;
END $$;
DROP TRIGGER trg_knowledge_article_scope_sync ON knowledge_articles;
CREATE TRIGGER trg_knowledge_article_scope_sync AFTER UPDATE OF brand_id, department_id ON knowledge_articles
 FOR EACH ROW WHEN (OLD.brand_id IS DISTINCT FROM NEW.brand_id OR OLD.department_id IS DISTINCT FROM NEW.department_id)
 EXECUTE FUNCTION knowledge_article_scope_sync();

-- Chunks of articles that already belong to a department.
UPDATE knowledge_chunks c SET department_id=a.department_id FROM knowledge_articles a
 WHERE a.organization_id=c.organization_id AND a.id=c.article_id AND a.department_id IS NOT NULL;
