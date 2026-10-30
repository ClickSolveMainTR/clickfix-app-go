-- Data-loss note: removes all AI trace, policy, article, version and embedding records.
DROP FUNCTION ai_wallet_reserve(BIGINT,UUID,BIGINT), ai_wallet_release(BIGINT,UUID,BIGINT);
DROP TABLE ai_credit_reservation_lots;
DROP TABLE ai_runs, ai_policies, knowledge_chunks;
DROP FUNCTION ai_cosine(REAL[],REAL[]);
ALTER TABLE knowledge_articles DROP CONSTRAINT fk_knowledge_published_version;
DROP TABLE knowledge_article_versions, knowledge_articles, knowledge_collections;
DROP FUNCTION knowledge_version_immutable();
