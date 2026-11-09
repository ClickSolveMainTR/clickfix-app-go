-- CF-25: contacts, companies and channel identities.

-- name: InsertCompany :one
INSERT INTO companies (organization_id, name, domain, external_id, notes, custom_fields, tier, account_owner_id, sla_policy_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(name), sqlc.narg(domain), sqlc.narg(external_id),
        sqlc.arg(notes), sqlc.arg(custom_fields), sqlc.arg(tier), sqlc.narg(account_owner_id), sqlc.narg(sla_policy_id))
RETURNING *;

-- name: GetCompanyByUUID :one
SELECT * FROM companies WHERE organization_id = $1 AND uuid = $2 AND deleted_at IS NULL;

-- name: GetCompanyByDomain :one
SELECT * FROM companies
WHERE organization_id = $1 AND lower(domain) = lower(sqlc.arg(domain)::text) AND deleted_at IS NULL;

-- name: ListCompanies :many
SELECT co.*, u.uuid AS owner_uuid, u.name AS owner_name, u.surname AS owner_surname,
       (SELECT COUNT(*) FROM contacts c WHERE c.organization_id = co.organization_id AND c.company_id = co.id AND c.deleted_at IS NULL)::bigint AS contact_count,
       (SELECT COUNT(*) FROM conversations v WHERE v.organization_id = co.organization_id AND v.company_id = co.id
          AND v.deleted_at IS NULL AND v.status IN ('open', 'pending'))::bigint AS open_conversations
FROM companies co
LEFT JOIN users u ON u.id = co.account_owner_id
WHERE co.organization_id = sqlc.arg(organization_id)
  AND co.deleted_at IS NULL
  AND (sqlc.narg(q)::text IS NULL OR co.name ILIKE '%' || sqlc.narg(q) || '%' OR co.domain ILIKE '%' || sqlc.narg(q) || '%'
       OR co.external_id ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(external_id)::text IS NULL OR co.external_id = sqlc.narg(external_id)::text)
ORDER BY lower(co.name), co.id
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountCompanies :one
SELECT COUNT(*)::bigint FROM companies
WHERE organization_id = sqlc.arg(organization_id)
  AND deleted_at IS NULL
  AND (sqlc.narg(q)::text IS NULL OR name ILIKE '%' || sqlc.narg(q) || '%' OR domain ILIKE '%' || sqlc.narg(q) || '%'
       OR external_id ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(external_id)::text IS NULL OR external_id = sqlc.narg(external_id)::text);

-- name: UpdateCompany :one
UPDATE companies SET
    name = sqlc.arg(name),
    domain = sqlc.narg(domain),
    external_id = sqlc.narg(external_id),
    notes = sqlc.arg(notes),
    custom_fields = sqlc.arg(custom_fields),
    tier = sqlc.arg(tier),
    account_owner_id = sqlc.narg(account_owner_id),
    sla_policy_id = sqlc.narg(sla_policy_id)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteCompany :one
UPDATE companies SET deleted_at = NOW()
WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: GetUserRefByID :one
SELECT id, uuid, name, surname FROM users WHERE id = $1;

-- name: LinkContactsToCompanyByDomain :execrows
-- A new company adopts the organization's contacts whose e-mail is on its
-- domain and that have no company yet.
UPDATE contacts SET company_id = sqlc.arg(company_id)
WHERE organization_id = sqlc.arg(organization_id) AND company_id IS NULL AND deleted_at IS NULL
  AND lower(split_part(email, '@', 2)) = lower(sqlc.arg(domain)::text);

-- name: InsertContact :one
INSERT INTO contacts (organization_id, company_id, name, email, phone, locale, timezone, notes, custom_fields, job_title, external_id)
VALUES (sqlc.arg(organization_id), sqlc.narg(company_id), sqlc.arg(name), sqlc.narg(email), sqlc.narg(phone),
        sqlc.narg(locale), sqlc.narg(timezone), sqlc.arg(notes), sqlc.arg(custom_fields), sqlc.arg(job_title), sqlc.narg(external_id))
RETURNING *;

-- name: GetContactByID :one
SELECT * FROM contacts WHERE organization_id = $1 AND id = $2;

-- name: GetContactByUUID :one
SELECT * FROM contacts WHERE organization_id = $1 AND uuid = $2 AND deleted_at IS NULL;

-- name: GetContactByExternalID :one
-- CF-195: the customer's own user id (unique per organization).
SELECT * FROM contacts
WHERE organization_id = $1 AND external_id = sqlc.arg(external_id)::text AND deleted_at IS NULL;

-- name: MarkContactVerified :execrows
-- CF-195: a user_hash check succeeded. The external id is taken only when
-- the contact has none and no other live contact holds it, so a contact
-- matched by e-mail never steals another user's id; and the contact is
-- marked verified only when its external id is the proved user id (a
-- contact of another user never gets the badge). Returns 0 rows when it
-- was not verified.
UPDATE contacts c SET
    verified_at = sqlc.arg(verified_at),
    external_id = COALESCE(c.external_id, sqlc.arg(external_id)::text)
WHERE c.organization_id = sqlc.arg(organization_id) AND c.id = sqlc.arg(id) AND c.deleted_at IS NULL
  AND (c.external_id = sqlc.arg(external_id)::text
       OR (c.external_id IS NULL AND NOT EXISTS (
            SELECT 1 FROM contacts o
            WHERE o.organization_id = c.organization_id AND o.external_id = sqlc.arg(external_id)::text
              AND o.deleted_at IS NULL AND o.id <> c.id)));

-- name: FindContactByEmail :one
SELECT * FROM contacts
WHERE organization_id = $1 AND lower(email) = lower(sqlc.arg(email)::text) AND deleted_at IS NULL
ORDER BY id
LIMIT 1;

-- name: FindContactByPhone :one
SELECT * FROM contacts
WHERE organization_id = $1 AND phone = sqlc.arg(phone)::text AND deleted_at IS NULL
ORDER BY id
LIMIT 1;

-- name: ListContacts :many
SELECT c.*, co.uuid AS company_uuid, co.name AS company_name,
       (SELECT COUNT(*) FROM conversations v WHERE v.organization_id = c.organization_id AND v.contact_id = c.id AND v.deleted_at IS NULL)::bigint AS conversation_count,
       (SELECT array_agg(DISTINCT ci.channel_type) FROM contact_identities ci WHERE ci.organization_id = c.organization_id AND ci.contact_id = c.id)::text[] AS channel_types
FROM contacts c
LEFT JOIN companies co ON co.organization_id = c.organization_id AND co.id = c.company_id AND co.deleted_at IS NULL
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.deleted_at IS NULL
  AND (sqlc.narg(company_id)::bigint IS NULL OR c.company_id = sqlc.narg(company_id))
  AND (sqlc.narg(q)::text IS NULL
       OR c.name ILIKE '%' || sqlc.narg(q) || '%'
       OR c.email ILIKE '%' || sqlc.narg(q) || '%'
       OR c.phone ILIKE '%' || sqlc.narg(q) || '%'
       OR c.external_id ILIKE '%' || sqlc.narg(q) || '%')
  -- CF-195: exact external id and identity state (verified / anonymous /
  -- self_named, see contactIdentityState).
  AND (sqlc.narg(external_id)::text IS NULL OR c.external_id = sqlc.narg(external_id)::text)
  AND (sqlc.narg(identity_state)::text IS NULL
       OR (sqlc.narg(identity_state)::text = 'verified' AND c.verified_at IS NOT NULL)
       OR (sqlc.narg(identity_state)::text = 'anonymous' AND c.verified_at IS NULL AND c.name = '' AND c.email IS NULL AND c.phone IS NULL)
       OR (sqlc.narg(identity_state)::text = 'self_named' AND c.verified_at IS NULL AND (c.name <> '' OR c.email IS NOT NULL OR c.phone IS NOT NULL)))
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN lower(c.name) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN lower(c.name) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'last_seen_at' AND NOT sqlc.arg(sort_desc)::boolean THEN c.last_seen_at END ASC NULLS FIRST,
  CASE WHEN sqlc.arg(sort_field)::text = 'last_seen_at' AND sqlc.arg(sort_desc)::boolean THEN c.last_seen_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN c.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN c.created_at END DESC,
  c.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountContacts :one
SELECT COUNT(*)::bigint FROM contacts c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.deleted_at IS NULL
  AND (sqlc.narg(company_id)::bigint IS NULL OR c.company_id = sqlc.narg(company_id))
  AND (sqlc.narg(q)::text IS NULL
       OR c.name ILIKE '%' || sqlc.narg(q) || '%'
       OR c.email ILIKE '%' || sqlc.narg(q) || '%'
       OR c.phone ILIKE '%' || sqlc.narg(q) || '%'
       OR c.external_id ILIKE '%' || sqlc.narg(q) || '%')
  -- CF-195: exact external id and identity state (verified / anonymous /
  -- self_named, see contactIdentityState).
  AND (sqlc.narg(external_id)::text IS NULL OR c.external_id = sqlc.narg(external_id)::text)
  AND (sqlc.narg(identity_state)::text IS NULL
       OR (sqlc.narg(identity_state)::text = 'verified' AND c.verified_at IS NOT NULL)
       OR (sqlc.narg(identity_state)::text = 'anonymous' AND c.verified_at IS NULL AND c.name = '' AND c.email IS NULL AND c.phone IS NULL)
       OR (sqlc.narg(identity_state)::text = 'self_named' AND c.verified_at IS NULL AND (c.name <> '' OR c.email IS NOT NULL OR c.phone IS NOT NULL)));

-- name: UpdateContact :one
UPDATE contacts SET
    company_id = sqlc.narg(company_id),
    name = sqlc.arg(name),
    email = sqlc.narg(email),
    phone = sqlc.narg(phone),
    locale = sqlc.narg(locale),
    timezone = sqlc.narg(timezone),
    notes = sqlc.arg(notes),
    custom_fields = sqlc.arg(custom_fields),
    blocked = sqlc.arg(blocked),
    job_title = sqlc.arg(job_title),
    external_id = sqlc.narg(external_id)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: FillContactDetails :exec
-- Inbound messages only fill what is still empty: an agent's edit wins over
-- whatever the channel reports later.
UPDATE contacts SET
    name = CASE WHEN name = '' THEN sqlc.arg(name)::text ELSE name END,
    email = COALESCE(email, sqlc.narg(email)::text),
    phone = COALESCE(phone, sqlc.narg(phone)::text),
    last_seen_at = GREATEST(COALESCE(last_seen_at, sqlc.arg(seen_at)::timestamptz), sqlc.arg(seen_at)::timestamptz)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: SoftDeleteContact :one
UPDATE contacts SET deleted_at = NOW()
WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: MarkContactMerged :exec
UPDATE contacts SET merged_into_id = sqlc.arg(into_id), deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: FindContactIdentity :one
SELECT ci.* FROM contact_identities ci
WHERE ci.organization_id = sqlc.arg(organization_id)
  AND ci.channel_type = sqlc.arg(channel_type)
  AND COALESCE(ci.channel_instance_id, 0) = COALESCE(sqlc.narg(channel_instance_id)::bigint, 0)
  AND ci.external_id = sqlc.arg(external_id);

-- name: UpsertContactIdentity :one
INSERT INTO contact_identities (
    organization_id, contact_id, channel_type, channel_instance_id, external_id, display_name, username, meta, last_seen_at
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(contact_id), sqlc.arg(channel_type), sqlc.narg(channel_instance_id),
    sqlc.arg(external_id), sqlc.arg(display_name), sqlc.arg(username), sqlc.arg(meta), sqlc.narg(last_seen_at)
)
ON CONFLICT (organization_id, channel_type, COALESCE(channel_instance_id, 0), external_id) DO UPDATE SET
    display_name = CASE WHEN EXCLUDED.display_name <> '' THEN EXCLUDED.display_name ELSE contact_identities.display_name END,
    username = CASE WHEN EXCLUDED.username <> '' THEN EXCLUDED.username ELSE contact_identities.username END,
    last_seen_at = COALESCE(EXCLUDED.last_seen_at, contact_identities.last_seen_at)
RETURNING *;

-- name: ListContactIdentities :many
SELECT ci.*, inst.uuid AS channel_instance_uuid
FROM contact_identities ci
LEFT JOIN channel_instances inst ON inst.organization_id = ci.organization_id AND inst.id = ci.channel_instance_id
WHERE ci.organization_id = $1 AND ci.contact_id = $2
ORDER BY ci.id;

-- name: MoveContactIdentities :execrows
-- Merge: identities of the merged contact move to the survivor unless the
-- survivor already has the same identity (then the duplicate is dropped by
-- DeleteContactIdentities).
UPDATE contact_identities ci SET contact_id = sqlc.arg(into_id)
WHERE ci.organization_id = sqlc.arg(organization_id) AND ci.contact_id = sqlc.arg(from_id)
  AND NOT EXISTS (
    SELECT 1 FROM contact_identities o
    WHERE o.organization_id = ci.organization_id AND o.contact_id = sqlc.arg(into_id)
      AND o.channel_type = ci.channel_type
      AND COALESCE(o.channel_instance_id, 0) = COALESCE(ci.channel_instance_id, 0)
      AND o.external_id = ci.external_id
  );

-- name: DeleteContactIdentities :exec
DELETE FROM contact_identities WHERE organization_id = $1 AND contact_id = $2;

-- name: DeleteContactIdentity :execrows
DELETE FROM contact_identities WHERE organization_id = $1 AND contact_id = $2 AND id = $3;

-- name: MoveContactConversations :execrows
UPDATE conversations SET contact_id = sqlc.arg(into_id)
WHERE organization_id = sqlc.arg(organization_id) AND contact_id = sqlc.arg(from_id);

-- name: MoveContactMessages :execrows
UPDATE messages SET sender_contact_id = sqlc.arg(into_id)
WHERE organization_id = sqlc.arg(organization_id) AND sender_contact_id = sqlc.arg(from_id);

-- name: FindCompanyByName :one
-- CF-84 import dedupe: last resort when a row has no domain or external id.
SELECT * FROM companies
WHERE organization_id = $1 AND lower(name) = lower(sqlc.arg(name)::text) AND deleted_at IS NULL
ORDER BY id
LIMIT 1;

-- name: FindOrganizationMemberByEmail :one
-- CF-84 import: account owner column holds the CSM's e-mail.
SELECT u.id, u.uuid, u.name, u.surname, u.email
FROM organization_members om
JOIN users u ON u.id = om.user_id
WHERE om.organization_id = $1 AND lower(u.email) = lower(sqlc.arg(email)::text);

-- name: FindContactByPhoneLast10 :one
SELECT * FROM contacts WHERE organization_id=sqlc.arg(organization_id) AND deleted_at IS NULL
 AND length(regexp_replace(sqlc.arg(phone)::text,'[^0-9]','','g'))>=10
 AND right(regexp_replace(phone,'[^0-9]','','g'),10)=right(regexp_replace(sqlc.arg(phone)::text,'[^0-9]','','g'),10)
ORDER BY id LIMIT 1;

-- name: GetCompanyForAIContext :one
-- CF-195: the company the AI sees next to the contact.
SELECT name, external_id, tier, custom_fields FROM companies
WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL;
