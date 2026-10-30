-- Data-loss note: provisioning links and SSO configuration will be lost; users/teams remain.
DROP TABLE scim_groups;
DROP TABLE sso_login_states;
DROP TABLE sso_identities;
DROP TABLE organization_sso;

-- Preserve the catalog identity when immutable historical billing rows use it.
-- The feature is inert after rollback because SSO configuration no longer exists.
DO $$ BEGIN
 DELETE FROM billing_features WHERE key='security.sso';
EXCEPTION WHEN foreign_key_violation THEN NULL;
END $$;
