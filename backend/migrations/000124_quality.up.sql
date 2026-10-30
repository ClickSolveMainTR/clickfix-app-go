-- CF-108: rubrics are snapshotted in every review, so later edits cannot rewrite history.
CREATE TABLE qa_scorecards (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 name TEXT NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200),
 criteria JSONB NOT NULL CHECK(jsonb_typeof(criteria)='array'),
 threshold INTEGER NOT NULL DEFAULT 70 CHECK(threshold BETWEEN 0 AND 100),
 auto_score BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),UNIQUE(organization_id,id)
);
CREATE INDEX idx_qa_scorecards_org ON qa_scorecards(organization_id);
CREATE TABLE qa_reviews (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 scorecard_id BIGINT NOT NULL,conversation_id BIGINT NOT NULL,
 agent_id BIGINT REFERENCES users(id), reviewer_id BIGINT REFERENCES users(id),
 source TEXT NOT NULL CHECK(source IN ('manual','ai')),rubric JSONB NOT NULL,scores JSONB NOT NULL,
 score NUMERIC(6,2) NOT NULL CHECK(score BETWEEN 0 AND 100),
 critical_failure BOOLEAN NOT NULL DEFAULT false,needs_review BOOLEAN NOT NULL,reviewed_at TIMESTAMPTZ,
 ai_run_uuid UUID REFERENCES ai_runs(uuid),created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,scorecard_id) REFERENCES qa_scorecards(organization_id,id),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id)
);
CREATE INDEX idx_qa_reviews_org ON qa_reviews(organization_id,agent_id,created_at);
CREATE UNIQUE INDEX uq_qa_reviews_ai ON qa_reviews(organization_id,scorecard_id,conversation_id) WHERE source='ai';
CREATE TRIGGER trg_qa_scorecards_updated BEFORE UPDATE ON qa_scorecards FOR EACH ROW EXECUTE FUNCTION set_updated_at();
