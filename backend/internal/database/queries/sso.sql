-- name: SSOConfig :one
SELECT * FROM organization_sso WHERE organization_id=$1;
-- name: SSOConfigSave :one
INSERT INTO organization_sso(organization_id,protocol,enabled,enforced,issuer,client_id,client_secret_enc,authorization_url,token_url,jwks_url,saml_metadata,ip_allowlist)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT(organization_id) DO UPDATE SET
protocol=excluded.protocol,enabled=excluded.enabled,enforced=excluded.enforced,issuer=excluded.issuer,client_id=excluded.client_id,
client_secret_enc=CASE WHEN excluded.client_secret_enc='' THEN organization_sso.client_secret_enc ELSE excluded.client_secret_enc END,
authorization_url=excluded.authorization_url,token_url=excluded.token_url,jwks_url=excluded.jwks_url,saml_metadata=excluded.saml_metadata,ip_allowlist=excluded.ip_allowlist RETURNING *;
-- name: SSOSetSCIMToken :execrows
-- SCIM provisioning works without SSO sign-in: the first rotation creates the
-- (disabled) settings row.
INSERT INTO organization_sso(organization_id,scim_token_hash) VALUES($1,$2)
ON CONFLICT(organization_id) DO UPDATE SET scim_token_hash=excluded.scim_token_hash;
-- name: SSOOrganization :one
SELECT id,uuid,slug FROM organizations WHERE uuid=$1 AND status='active' AND deleted_at IS NULL;
-- name: SSOState :exec
INSERT INTO sso_login_states(organization_id,state_hash,nonce,verifier_enc,request_id) VALUES($1,$2,$3,$4,$5);
-- name: SSOConsumeState :one
DELETE FROM sso_login_states WHERE organization_id=$1 AND state_hash=$2 AND expires_at>now() RETURNING *;
-- name: SSOExpireStates :exec
DELETE FROM sso_login_states WHERE expires_at<now();
-- name: SSOUser :one
SELECT u.id,u.uuid,u.status FROM sso_identities i
JOIN users u ON u.id=i.user_id JOIN organization_members m ON m.organization_id=i.organization_id AND m.user_id=i.user_id
WHERE i.organization_id=$1 AND i.external_id=$2 AND i.active AND u.status='active' AND u.deleted_at IS NULL;
-- name: SSOMembership :one
SELECT EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=$1 AND m.user_id=$2)::boolean;
-- name: SCIMUsers :many
SELECT i.uuid,i.external_id,i.active,u.email,u.name,u.surname FROM sso_identities i JOIN users u ON u.id=i.user_id
WHERE i.organization_id=$1 ORDER BY i.id LIMIT $2 OFFSET $3;
-- name: SCIMUserCount :one
SELECT count(*) FROM sso_identities WHERE organization_id=$1;
-- name: SCIMUser :one
SELECT i.uuid,i.external_id,i.active,i.user_id,u.email,u.name,u.surname FROM sso_identities i JOIN users u ON u.id=i.user_id
WHERE i.organization_id=$1 AND i.uuid=$2;
-- name: SCIMFindUser :one
SELECT id FROM users WHERE lower(email)=lower($1) AND deleted_at IS NULL;
-- name: SCIMCreateUser :one
INSERT INTO users(email,password_hash,name,surname,status,email_verified_at,password_set) VALUES($1,$2,$3,$4,'active',now(),false) RETURNING id;
-- name: SCIMMember :exec
INSERT INTO organization_members(organization_id,user_id,role_id)
SELECT $1,$2,r.id FROM roles r WHERE r.scope='tenant' AND r.organization_id IS NULL AND r.slug='agent'
ON CONFLICT(organization_id,user_id) DO NOTHING;
-- name: SCIMIdentity :one
INSERT INTO sso_identities(organization_id,user_id,external_id,active) VALUES($1,$2,$3,true) RETURNING *;
-- name: SCIMSetActive :exec
UPDATE sso_identities SET active=$3 WHERE organization_id=$1 AND uuid=$2;
-- name: SCIMRemoveMember :exec
DELETE FROM organization_members m WHERE m.organization_id=$1 AND m.user_id=$2 AND m.role_id IN(SELECT r.id FROM roles r WHERE r.slug NOT IN ('owner','admin'));
-- name: SCIMGroups :many
SELECT g.*,t.name FROM scim_groups g JOIN teams t ON t.organization_id=g.organization_id AND t.id=g.team_id
WHERE g.organization_id=$1 ORDER BY g.id LIMIT $2 OFFSET $3;
-- name: SCIMGroupCount :one
SELECT count(*) FROM scim_groups WHERE organization_id=$1;
-- name: SCIMGroup :one
SELECT g.*,t.name FROM scim_groups g JOIN teams t ON t.organization_id=g.organization_id AND t.id=g.team_id WHERE g.organization_id=$1 AND g.uuid=$2;
-- name: SCIMCreateTeam :one
INSERT INTO teams(organization_id,name) VALUES($1,$2) RETURNING id;
-- name: SCIMCreateGroup :one
INSERT INTO scim_groups(organization_id,team_id,external_id) VALUES($1,$2,$3) RETURNING *;
-- name: SCIMRenameTeam :exec
UPDATE teams SET name=$3 WHERE organization_id=$1 AND id=$2;
-- name: SCIMGroupMembers :many
SELECT i.uuid,u.name FROM team_members m JOIN sso_identities i ON i.organization_id=m.organization_id AND i.user_id=m.user_id
JOIN users u ON u.id=i.user_id WHERE m.organization_id=$1 AND m.team_id=$2 AND i.active ORDER BY i.id;
-- name: SCIMClearMembers :exec
DELETE FROM team_members WHERE organization_id=$1 AND team_id=$2;
-- name: SCIMAddMember :execrows
INSERT INTO team_members(organization_id,team_id,user_id)
SELECT $1,$2,i.user_id FROM sso_identities i JOIN organization_members m ON m.organization_id=i.organization_id AND m.user_id=i.user_id
WHERE i.organization_id=$1 AND i.uuid=$3 AND i.active ON CONFLICT DO NOTHING;
-- name: SCIMDeleteGroup :exec
DELETE FROM teams WHERE organization_id=$1 AND id=$2;
-- name: SSOActiveUser :one
SELECT EXISTS(SELECT 1 FROM sso_identities WHERE organization_id=$1 AND user_id=$2 AND active)::boolean;
-- name: SCIMProtectedMember :one
SELECT EXISTS(SELECT 1 FROM organization_members m JOIN organization_member_roles mr ON mr.organization_id=m.organization_id AND mr.member_id=m.id JOIN roles r ON r.id=mr.role_id
WHERE m.organization_id=$1 AND m.user_id=$2 AND r.slug IN ('owner','admin'))::boolean;
-- name: SCIMFilterUsers :many
SELECT i.uuid,i.external_id,i.active,u.email,u.name,u.surname FROM sso_identities i JOIN users u ON u.id=i.user_id
WHERE i.organization_id=$1 AND ((sqlc.arg(field)::text='userName' AND lower(u.email)=lower(sqlc.arg(value)::text))
 OR (sqlc.arg(field)::text='externalId' AND i.external_id=sqlc.arg(value)::text)) ORDER BY i.id LIMIT 100;
-- name: SCIMFilterGroups :many
SELECT g.uuid FROM scim_groups g JOIN teams t ON t.organization_id=g.organization_id AND t.id=g.team_id
WHERE g.organization_id=$1 AND ((sqlc.arg(field)::text='displayName' AND t.name=sqlc.arg(value)::text)
 OR (sqlc.arg(field)::text='externalId' AND g.external_id=sqlc.arg(value)::text)) ORDER BY g.id LIMIT $2 OFFSET $3;
-- name: SCIMFilterGroupCount :one
SELECT count(*) FROM scim_groups g JOIN teams t ON t.organization_id=g.organization_id AND t.id=g.team_id
WHERE g.organization_id=$1 AND ((sqlc.arg(field)::text='displayName' AND t.name=sqlc.arg(value)::text)
 OR (sqlc.arg(field)::text='externalId' AND g.external_id=sqlc.arg(value)::text));
