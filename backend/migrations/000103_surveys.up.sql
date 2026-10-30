-- CF-99: invitations and NPS/CES responses. CSAT keeps the widget's canonical rating row.
CREATE TABLE survey_templates (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 name TEXT NOT NULL CHECK(char_length(name) BETWEEN 1 AND 120),
 kind TEXT NOT NULL CHECK(kind IN ('csat','nps','ces')),
 question TEXT NOT NULL CHECK(char_length(question) BETWEEN 1 AND 1000),
 on_resolve BOOLEAN NOT NULL DEFAULT false,enabled BOOLEAN NOT NULL DEFAULT true,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id)
);
CREATE INDEX idx_survey_templates_org ON survey_templates(organization_id);
CREATE TRIGGER trg_survey_templates_updated BEFORE UPDATE ON survey_templates FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE survey_invitations (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 template_id BIGINT NOT NULL,conversation_id BIGINT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('csat','nps','ces')),question TEXT NOT NULL,
 period_key TEXT NOT NULL DEFAULT '' CHECK(char_length(period_key)<=64),
 expires_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '30 days',
 answered_at TIMESTAMPTZ NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id),UNIQUE(organization_id,template_id,conversation_id,period_key),
 FOREIGN KEY(organization_id,template_id) REFERENCES survey_templates(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id) ON DELETE CASCADE
);
CREATE INDEX idx_survey_invitations_org ON survey_invitations(organization_id,created_at);
CREATE TABLE survey_responses (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 invitation_id BIGINT NOT NULL,kind TEXT NOT NULL CHECK(kind IN ('nps','ces')),
 score SMALLINT NOT NULL,comment TEXT NOT NULL DEFAULT '' CHECK(char_length(comment)<=2000),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,invitation_id),
 FOREIGN KEY(organization_id,invitation_id) REFERENCES survey_invitations(organization_id,id) ON DELETE CASCADE,
 CHECK((kind='nps' AND score BETWEEN 0 AND 10) OR (kind='ces' AND score BETWEEN 1 AND 7))
);
CREATE INDEX idx_survey_responses_org ON survey_responses(organization_id,created_at);
