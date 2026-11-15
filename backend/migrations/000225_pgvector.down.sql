-- Data-loss note: chunks without a vector (dimension mismatch or not yet
-- re-embedded) cannot satisfy the old NOT NULL REAL[] column and are deleted;
-- re-publishing their articles rebuilds them. Pending (re-embedding) vectors are dropped.
DROP INDEX IF EXISTS idx_knowledge_chunks_reembed;
DROP INDEX IF EXISTS idx_knowledge_chunks_brand;
DROP INDEX IF EXISTS idx_knowledge_chunks_embedding;
DROP TRIGGER IF EXISTS trg_knowledge_article_scope_sync ON knowledge_articles;
DROP FUNCTION IF EXISTS knowledge_article_scope_sync();
DROP TRIGGER IF EXISTS trg_knowledge_chunk_scope ON knowledge_chunks;
DROP FUNCTION IF EXISTS knowledge_chunk_scope();
DELETE FROM knowledge_chunks WHERE embedding IS NULL;
ALTER TABLE knowledge_chunks ADD COLUMN embedding_arr REAL[];
UPDATE knowledge_chunks SET embedding_arr=embedding::real[];
ALTER TABLE knowledge_chunks DROP COLUMN embedding;
ALTER TABLE knowledge_chunks RENAME COLUMN embedding_arr TO embedding;
ALTER TABLE knowledge_chunks ALTER COLUMN embedding SET NOT NULL;
ALTER TABLE knowledge_chunks ADD CONSTRAINT knowledge_chunks_embedding_check CHECK(cardinality(embedding)>0);
ALTER TABLE knowledge_chunks DROP COLUMN pending_model, DROP COLUMN pending_embedding,
 DROP COLUMN department_id, DROP COLUMN brand_id;
CREATE FUNCTION ai_cosine(a REAL[], b REAL[]) RETURNS DOUBLE PRECISION LANGUAGE sql IMMUTABLE STRICT AS $$
 SELECT CASE WHEN cardinality(a)<>cardinality(b) THEN 0 ELSE COALESCE(sum(x::double precision*y)/nullif(sqrt(sum(x::double precision*x))*sqrt(sum(y::double precision*y)),0),0) END
 FROM unnest(a,b) t(x,y)
$$;
DROP EXTENSION IF EXISTS vector;
