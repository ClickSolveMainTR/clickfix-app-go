CREATE TABLE ai_procedures (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 intent TEXT NOT NULL,
 locale TEXT NOT NULL CHECK(locale IN ('tr','en')),
 title TEXT NOT NULL,
 draft JSONB NOT NULL,
 revision INTEGER NOT NULL DEFAULT 1,
 published JSONB,
 published_revision INTEGER,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,intent,locale),
 CHECK((published IS NULL)=(published_revision IS NULL))
);
CREATE INDEX idx_ai_procedures_org ON ai_procedures(organization_id,id);
CREATE TRIGGER trg_ai_procedures_updated BEFORE UPDATE ON ai_procedures FOR EACH ROW EXECUTE FUNCTION set_updated_at();
