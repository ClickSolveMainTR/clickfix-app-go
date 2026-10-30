-- CF-101: explicitly published roadmap cards sourced from external engineering issues.
-- source_issue_url is an external reference, deliberately independent of bridge provider tables.
CREATE TABLE roadmap_entries (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 title TEXT NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 200),
 body TEXT NOT NULL DEFAULT '' CHECK(length(body)<=16000),
 status TEXT NOT NULL CHECK(status IN ('planned','in_progress','released')),
 source_issue_url TEXT NOT NULL CHECK(source_issue_url LIKE 'https://%' AND length(source_issue_url)<=2000),
 is_public BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_roadmap_entries_org ON roadmap_entries(organization_id,status,id DESC);
CREATE TRIGGER trg_roadmap_entries_updated BEFORE UPDATE ON roadmap_entries FOR EACH ROW EXECUTE FUNCTION set_updated_at();
