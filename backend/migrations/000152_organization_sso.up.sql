CREATE TABLE organization_sso (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL UNIQUE REFERENCES organizations(id) ON DELETE CASCADE,
 protocol TEXT NOT NULL DEFAULT 'oidc' CHECK(protocol IN ('oidc','saml')),
 enabled BOOLEAN NOT NULL DEFAULT false, enforced BOOLEAN NOT NULL DEFAULT false,
 issuer TEXT NOT NULL DEFAULT '', client_id TEXT NOT NULL DEFAULT '', client_secret_enc TEXT NOT NULL DEFAULT '',
 authorization_url TEXT NOT NULL DEFAULT '', token_url TEXT NOT NULL DEFAULT '', jwks_url TEXT NOT NULL DEFAULT '',
 saml_metadata TEXT NOT NULL DEFAULT '', ip_allowlist TEXT[] NOT NULL DEFAULT '{}',
 scim_token_hash TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(NOT enforced OR enabled)
);
CREATE TRIGGER trg_organization_sso_updated BEFORE UPDATE ON organization_sso FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE sso_identities (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 external_id TEXT NOT NULL CHECK(length(external_id) BETWEEN 1 AND 500),
 active BOOLEAN NOT NULL DEFAULT true,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,external_id), UNIQUE(organization_id,user_id)
);
CREATE INDEX idx_sso_identities_org ON sso_identities(organization_id);
CREATE TRIGGER trg_sso_identities_updated BEFORE UPDATE ON sso_identities FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE sso_login_states (
 id BIGSERIAL PRIMARY KEY, organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 state_hash TEXT NOT NULL UNIQUE, nonce TEXT NOT NULL, verifier_enc TEXT NOT NULL,
 request_id TEXT NOT NULL DEFAULT '', expires_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '5 minutes',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sso_login_states_org ON sso_login_states(organization_id,expires_at);
CREATE TABLE scim_groups (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 team_id BIGINT NOT NULL, external_id TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,team_id) REFERENCES teams(organization_id,id) ON DELETE CASCADE,
 UNIQUE(organization_id,external_id), UNIQUE(organization_id,team_id)
);
CREATE INDEX idx_scim_groups_org ON scim_groups(organization_id);
CREATE TRIGGER trg_scim_groups_updated BEFORE UPDATE ON scim_groups FOR EACH ROW EXECUTE FUNCTION set_updated_at();
INSERT INTO billing_features(key,kind,label_tr,label_en,unit,aggregation,period)
VALUES('security.sso','boolean','Kurumsal SSO ve SCIM','Enterprise SSO and SCIM','','none','none') ON CONFLICT(key) DO NOTHING;
