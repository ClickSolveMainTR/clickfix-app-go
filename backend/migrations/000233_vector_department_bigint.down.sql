-- Data-loss note: the UUID column cannot hold department ids; chunk
-- departments are dropped (NULL = organization wide) and article-level
-- department_visible stays the only department rule.
DROP TRIGGER IF EXISTS trg_knowledge_article_scope_sync ON knowledge_articles;
CREATE OR REPLACE FUNCTION knowledge_article_scope_sync() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 UPDATE knowledge_chunks SET brand_id=NEW.brand_id WHERE organization_id=NEW.organization_id AND article_id=NEW.id;
 RETURN NEW;
END $$;
CREATE TRIGGER trg_knowledge_article_scope_sync AFTER UPDATE OF brand_id ON knowledge_articles
 FOR EACH ROW WHEN (OLD.brand_id IS DISTINCT FROM NEW.brand_id) EXECUTE FUNCTION knowledge_article_scope_sync();
CREATE OR REPLACE FUNCTION knowledge_chunk_scope() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 SELECT brand_id INTO NEW.brand_id FROM knowledge_articles WHERE organization_id=NEW.organization_id AND id=NEW.article_id;
 RETURN NEW;
END $$;
DROP INDEX IF EXISTS idx_knowledge_chunks_department;
ALTER TABLE knowledge_chunks DROP CONSTRAINT IF EXISTS fk_knowledge_chunks_department;
ALTER TABLE knowledge_chunks ALTER COLUMN department_id TYPE UUID USING NULL::uuid;
