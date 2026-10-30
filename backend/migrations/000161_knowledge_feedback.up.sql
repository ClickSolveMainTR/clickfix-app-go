-- CF-74: "Bu makale işe yaradı mı?" votes from the public help center.
-- voter_hash is an HMAC of (client IP, article, UTC day): no raw IP is stored,
-- and the same visitor voting twice on one day changes the vote instead of
-- counting twice (Redis rate limits sit in front of this).
CREATE TABLE knowledge_article_feedback (
 id BIGSERIAL PRIMARY KEY,
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 article_id BIGINT NOT NULL,
 version INTEGER NOT NULL CHECK(version>0),
 helpful BOOLEAN NOT NULL,
 voter_hash TEXT NOT NULL CHECK(length(voter_hash) BETWEEN 16 AND 128),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,article_id) REFERENCES knowledge_articles(organization_id,id),
 UNIQUE(organization_id,article_id,voter_hash)
);
CREATE INDEX idx_knowledge_article_feedback_article ON knowledge_article_feedback(organization_id,article_id,helpful);
CREATE TRIGGER trg_knowledge_article_feedback_updated BEFORE UPDATE ON knowledge_article_feedback FOR EACH ROW EXECUTE FUNCTION set_updated_at();
