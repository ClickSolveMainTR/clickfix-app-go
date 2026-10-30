-- name: PurgeUserSignInData :exec
-- Sign-in methods, sessions and codes; nothing else can reach the account.
WITH oauth AS (DELETE FROM oauth_accounts WHERE oauth_accounts.user_id = sqlc.arg(user_id)::bigint),
     passkeys AS (DELETE FROM webauthn_credentials WHERE webauthn_credentials.user_id = sqlc.arg(user_id)::bigint),
     totp AS (DELETE FROM user_totp WHERE user_totp.user_id = sqlc.arg(user_id)::bigint),
     refresh AS (DELETE FROM refresh_tokens WHERE refresh_tokens.user_id = sqlc.arg(user_id)::bigint),
     otps AS (DELETE FROM otp_codes WHERE otp_codes.user_id = sqlc.arg(user_id)::bigint OR otp_codes.email=(SELECT users.email FROM users WHERE users.id=sqlc.arg(user_id)::bigint)),
     mcp AS (DELETE FROM mcp_oauth_grants WHERE mcp_oauth_grants.user_id = sqlc.arg(user_id)::bigint)
DELETE FROM user_roles WHERE user_roles.user_id = sqlc.arg(user_id)::bigint;

-- name: PurgeUserPersonalData :exec
-- Devices, preferences, inbox (every organization's notices to the person)
-- and starred or shared files.
WITH push_subs AS (DELETE FROM push_subscriptions WHERE push_subscriptions.user_id = sqlc.arg(user_id)::bigint),
     push_devs AS (DELETE FROM push_devices WHERE push_devices.user_id = sqlc.arg(user_id)::bigint),
     prefs AS (DELETE FROM notification_preferences WHERE notification_preferences.user_id = sqlc.arg(user_id)::bigint),
     digest AS (DELETE FROM notification_digest_items WHERE notification_digest_items.user_id = sqlc.arg(user_id)::bigint),
     stars AS (DELETE FROM storage_stars WHERE storage_stars.user_id = sqlc.arg(user_id)::bigint),
     shares AS (DELETE FROM storage_shares WHERE storage_shares.user_id = sqlc.arg(user_id)::bigint)
DELETE FROM notifications WHERE notifications.user_id = sqlc.arg(user_id)::bigint;


-- name: AccountDeletionLock :one
SELECT * FROM users WHERE id=$1 FOR UPDATE;
-- name: AccountDeletionDue :many
SELECT id,uuid FROM users WHERE deleted_at IS NULL AND deletion_due_at<=now() ORDER BY deletion_due_at,id LIMIT 100;
-- name: AccountDeletionOrganizations :many
SELECT organization_members.organization_id FROM organization_members WHERE organization_members.user_id=sqlc.arg(user_id)::bigint
UNION SELECT sso_identities.organization_id FROM sso_identities WHERE sso_identities.user_id=sqlc.arg(user_id)::bigint
UNION SELECT public_api_keys.organization_id FROM public_api_keys WHERE public_api_keys.user_id=sqlc.arg(user_id)::bigint
UNION SELECT agent_presence.organization_id FROM agent_presence WHERE agent_presence.user_id=sqlc.arg(user_id)::bigint
UNION SELECT agent_capacity.organization_id FROM agent_capacity WHERE agent_capacity.user_id=sqlc.arg(user_id)::bigint
UNION SELECT internal_thread_members.organization_id FROM internal_thread_members WHERE internal_thread_members.user_id=sqlc.arg(user_id)::bigint
ORDER BY organization_id;
-- name: AccountDeletionLockOrganization :one
SELECT id FROM organizations WHERE id=$1 AND deleted_at IS NULL FOR UPDATE;
-- name: AccountDeletionSoleOwner :one
SELECT EXISTS(SELECT 1 FROM organization_members m
 JOIN organization_member_roles mr ON mr.organization_id=m.organization_id AND mr.member_id=m.id
 JOIN roles r ON r.id=mr.role_id AND r.slug='owner' AND r.scope='tenant' AND r.organization_id IS NULL
 WHERE m.organization_id=$1 AND m.user_id=$2
 AND NOT EXISTS(SELECT 1 FROM organization_members other
 JOIN organization_member_roles omr ON omr.organization_id=other.organization_id AND omr.member_id=other.id
 JOIN roles orole ON orole.id=omr.role_id AND orole.slug='owner' AND orole.scope='tenant' AND orole.organization_id IS NULL
 JOIN users u ON u.id=other.user_id AND u.status='active' AND u.deleted_at IS NULL AND u.deactivated_at IS NULL
 WHERE other.organization_id=m.organization_id AND other.user_id<>m.user_id));
-- name: AccountDeletionAnonymize :exec
WITH audit AS (INSERT INTO activity_events(action,resource,resource_uuid,payload) SELECT 'auth.account_deactivated','auth.account',uuid,'{"stage":"erased"}'::jsonb FROM users WHERE users.id=sqlc.arg(user_id)::bigint)
UPDATE users SET email='deleted-'||uuid::text||'@deleted.invalid',name='Silinmiş kullanıcı',surname='',password_hash='',password_set=false,
 status='disabled',email_verified_at=NULL,last_login_at=NULL,locale='tr',deleted_at=now(),deletion_due_at=NULL WHERE users.id=sqlc.arg(user_id)::bigint;
-- name: AccountDeletionScrubAudit :exec
UPDATE activity_events SET payload='{}',ip_address=NULL,user_agent=NULL WHERE actor_user_id=$1;
-- name: AccountDeletionRevokeDelegatedSessions :exec
WITH grants AS (DELETE FROM mcp_oauth_grants WHERE mcp_oauth_grants.user_id=sqlc.arg(user_id)::bigint)
UPDATE public_api_keys SET revoked_at=now() WHERE public_api_keys.user_id=sqlc.arg(user_id)::bigint AND revoked_at IS NULL;
-- name: AccountDeletionPurgeOrganization :exec
WITH identities AS (DELETE FROM sso_identities WHERE sso_identities.organization_id=sqlc.arg(organization_id)::bigint AND sso_identities.user_id=sqlc.arg(user_id)::bigint),
 keys AS (DELETE FROM public_api_keys WHERE public_api_keys.organization_id=sqlc.arg(organization_id)::bigint AND public_api_keys.user_id=sqlc.arg(user_id)::bigint),
 presence AS (DELETE FROM agent_presence WHERE agent_presence.organization_id=sqlc.arg(organization_id)::bigint AND agent_presence.user_id=sqlc.arg(user_id)::bigint),
 capacity AS (DELETE FROM agent_capacity WHERE agent_capacity.organization_id=sqlc.arg(organization_id)::bigint AND agent_capacity.user_id=sqlc.arg(user_id)::bigint),
 threads AS (DELETE FROM internal_thread_members WHERE internal_thread_members.organization_id=sqlc.arg(organization_id)::bigint AND internal_thread_members.user_id=sqlc.arg(user_id)::bigint)
DELETE FROM organization_members WHERE organization_members.organization_id=sqlc.arg(organization_id)::bigint AND organization_members.user_id=sqlc.arg(user_id)::bigint;
-- name: AccountDeletionSearchMessages :many
SELECT organization_id,uuid FROM messages WHERE sender_user_id=$1 ORDER BY organization_id,id;
