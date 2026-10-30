-- CF-107/109: immutable activity, calibration rubric snapshots and agent development.
CREATE TABLE workforce_events (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 user_id BIGINT NOT NULL REFERENCES users(id), client_uuid UUID NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('active','away','handling','offline')),
 conversation_id BIGINT, occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id),
 UNIQUE(organization_id,user_id,client_uuid),
 CHECK((state='handling')=(conversation_id IS NOT NULL))
);
CREATE INDEX idx_workforce_events_org ON workforce_events(organization_id,user_id,occurred_at,id);
CREATE TABLE qa_calibrations (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 conversation_id BIGINT NOT NULL, scorecard_id BIGINT NOT NULL, rubric JSONB NOT NULL,
 name TEXT NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200),
 closed_at TIMESTAMPTZ, created_by BIGINT NOT NULL REFERENCES users(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id),
 FOREIGN KEY(organization_id,scorecard_id) REFERENCES qa_scorecards(organization_id,id)
);
CREATE INDEX idx_qa_calibrations_org ON qa_calibrations(organization_id,created_at);
CREATE TABLE qa_calibration_scores (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 calibration_id BIGINT NOT NULL, reviewer_id BIGINT NOT NULL REFERENCES users(id),
 scores JSONB NOT NULL, score DOUBLE PRECISION NOT NULL CHECK(score BETWEEN 0 AND 100),
 critical_failure BOOLEAN NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,calibration_id,reviewer_id),
 FOREIGN KEY(organization_id,calibration_id) REFERENCES qa_calibrations(organization_id,id) ON DELETE CASCADE
);
CREATE INDEX idx_qa_calibration_scores_org ON qa_calibration_scores(organization_id,calibration_id);
CREATE TABLE coaching_items (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 agent_id BIGINT NOT NULL REFERENCES users(id), coach_id BIGINT NOT NULL REFERENCES users(id),
 kind TEXT NOT NULL CHECK(kind IN ('note','goal','task')),
 body TEXT NOT NULL CHECK(length(trim(body)) BETWEEN 1 AND 5000),
 due_at TIMESTAMPTZ, completed_at TIMESTAMPTZ, parent_id BIGINT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id),
 FOREIGN KEY(organization_id,parent_id) REFERENCES coaching_items(organization_id,id)
);
CREATE INDEX idx_coaching_items_org ON coaching_items(organization_id,agent_id,created_at);
CREATE TABLE gamification_rules (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 name TEXT NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200),
 metric TEXT NOT NULL CHECK(metric IN ('csat','frt','resolutions','qa')),
 threshold DOUBLE PRECISION NOT NULL CHECK(threshold>=0 AND threshold<=1000000),
 points INTEGER NOT NULL CHECK(points BETWEEN 0 AND 10000),
 min_samples INTEGER NOT NULL DEFAULT 1 CHECK(min_samples BETWEEN 1 AND 100000),
 enabled BOOLEAN NOT NULL DEFAULT true,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,name)
);
CREATE INDEX idx_gamification_rules_org ON gamification_rules(organization_id);
CREATE TABLE gamification_preferences (
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 user_id BIGINT NOT NULL REFERENCES users(id), opted_out BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(organization_id,user_id)
);
CREATE TRIGGER trg_qa_calibrations_updated BEFORE UPDATE ON qa_calibrations FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_coaching_items_updated BEFORE UPDATE ON coaching_items FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_gamification_rules_updated BEFORE UPDATE ON gamification_rules FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_gamification_preferences_updated BEFORE UPDATE ON gamification_preferences FOR EACH ROW EXECUTE FUNCTION set_updated_at();
