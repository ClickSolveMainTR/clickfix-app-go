-- Platform organization management (ported from rentopia-app-go): list with owner and counts,
-- export / bulk uuids, profile logo, activity.

-- name: ListOrganizationsWithStats :many
-- Access state: 'upcoming' before access_starts_at, 'expired' after
-- access_ends_at, 'open' otherwise. Window: 'unlimited' without an end.
SELECT o.id, o.uuid, o.slug, o.name, o.logo_object_key, o.status,
       o.access_starts_at, o.access_ends_at, o.settings, o.created_at, o.updated_at,
       o.timezone, o.locale, o.currency, o.country_code,
       mc.member_count::bigint AS member_count,
       la.last_activity_at::timestamptz AS last_activity_at,
       sc.billable_seats::bigint AS billable_seats,
       COALESCE(sub.plan_code, '')::text AS plan_code,
       COALESCE(sub.plan_name, '')::text AS plan_name,
       COALESCE(sub.subscription_status, '')::text AS subscription_status,
       sub.trial_ends_at::timestamptz AS trial_ends_at,
       sub.period_end::timestamptz AS period_end,
       wb.ai_credits::bigint AS ai_credits,
       wb.video_minutes::bigint AS video_minutes,
       ic.invitation_count::bigint AS pending_invitation_count,
       COALESCE(ow.owner_uuid, '')::text AS owner_uuid,
       COALESCE(ow.owner_email, '')::text AS owner_email,
       COALESCE(ow.owner_name, '')::text AS owner_name,
       COALESCE(ow.owner_surname, '')::text AS owner_surname
FROM organizations o
CROSS JOIN LATERAL (
    SELECT COUNT(*) AS member_count
    FROM organization_members om
    INNER JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
    WHERE om.organization_id = o.id
) mc
CROSS JOIN LATERAL (
    -- Paid seats (CF-4): members holding a billable role.
    SELECT COUNT(*) AS billable_seats
    FROM organization_members om
    INNER JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
    WHERE om.organization_id = o.id
      AND EXISTS (
        SELECT 1 FROM organization_member_roles omr
        INNER JOIN roles r ON r.id = omr.role_id AND r.scope = 'tenant' AND r.is_billable
        WHERE omr.organization_id = om.organization_id AND omr.member_id = om.id
      )
) sc
LEFT JOIN LATERAL (
    -- Current plan (CF-14 platform panel): the newest subscription.
    SELECT bp.code AS plan_code, bp.name AS plan_name, bs.status AS subscription_status,
           bs.trial_ends_at, bs.period_end
    FROM billing_subscriptions bs
    INNER JOIN billing_plan_versions bv ON bv.id = bs.plan_version_id
    INNER JOIN billing_plans bp ON bp.id = bv.plan_id
    WHERE bs.organization_id = o.id
    ORDER BY bs.created_at DESC, bs.id DESC
    LIMIT 1
) sub ON TRUE
CROSS JOIN LATERAL (
    -- Wallet balances (CF-143): AI credits and video minutes left.
    SELECT COALESCE(SUM(l.delta) FILTER (WHERE l.unit = 'ai_credits'), 0) AS ai_credits,
           COALESCE(SUM(l.delta) FILTER (WHERE l.unit = 'video_minutes'), 0) AS video_minutes
    FROM billing_credit_ledger l
    WHERE l.organization_id = o.id
) wb
CROSS JOIN LATERAL (
    SELECT MAX(e.created_at) AS last_activity_at
    FROM activity_events e
    WHERE e.organization_id = o.id
) la
CROSS JOIN LATERAL (
    SELECT COUNT(*) AS invitation_count
    FROM organization_invitations oi
    WHERE oi.organization_id = o.id
      AND oi.accepted_at IS NULL AND oi.revoked_at IS NULL AND oi.expires_at > NOW()
) ic
LEFT JOIN LATERAL (
    -- The longest-standing owner.
    SELECT u.uuid::text AS owner_uuid, u.email::text AS owner_email,
           u.name::text AS owner_name, u.surname::text AS owner_surname
    FROM organization_members om
    INNER JOIN organization_member_roles omr
        ON omr.organization_id = om.organization_id AND omr.member_id = om.id
    INNER JOIN roles r
        ON r.id = omr.role_id AND r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug = 'owner'
    INNER JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
    WHERE om.organization_id = o.id
    ORDER BY om.created_at ASC, om.id ASC
    LIMIT 1
) ow ON TRUE
WHERE o.deleted_at IS NULL
  AND (sqlc.narg(organization_uuid)::uuid IS NULL OR o.uuid = sqlc.narg(organization_uuid)::uuid)
  AND (sqlc.narg(statuses)::text[] IS NULL OR o.status = ANY(sqlc.narg(statuses)::text[]))
  AND (
    sqlc.narg(access_states)::text[] IS NULL
    OR (CASE
          WHEN o.access_starts_at > NOW() THEN 'upcoming'
          WHEN o.access_ends_at IS NOT NULL AND o.access_ends_at <= NOW() THEN 'expired'
          ELSE 'open'
        END) = ANY(sqlc.narg(access_states)::text[])
  )
  AND (
    sqlc.narg(windows)::text[] IS NULL
    OR (CASE WHEN o.access_ends_at IS NULL THEN 'unlimited' ELSE 'limited' END) = ANY(sqlc.narg(windows)::text[])
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR o.name ILIKE '%' || sqlc.narg(q) || '%'
    OR o.slug ILIKE '%' || sqlc.narg(q) || '%'
    OR COALESCE(ow.owner_email, '') ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN o.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN o.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'slug' AND NOT sqlc.arg(sort_desc)::boolean THEN o.slug END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'slug' AND sqlc.arg(sort_desc)::boolean THEN o.slug END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN o.status END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN o.status END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'member_count' AND NOT sqlc.arg(sort_desc)::boolean THEN mc.member_count END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'member_count' AND sqlc.arg(sort_desc)::boolean THEN mc.member_count END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'last_activity_at' AND NOT sqlc.arg(sort_desc)::boolean THEN la.last_activity_at END ASC NULLS FIRST,
  CASE WHEN sqlc.arg(sort_field)::text = 'last_activity_at' AND sqlc.arg(sort_desc)::boolean THEN la.last_activity_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'access_ends_at' AND NOT sqlc.arg(sort_desc)::boolean THEN o.access_ends_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'access_ends_at' AND sqlc.arg(sort_desc)::boolean THEN o.access_ends_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN o.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN o.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN o.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN o.created_at END DESC,
  o.created_at DESC, o.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountOrganizationsWithStats :one
SELECT COUNT(*)::bigint
FROM organizations o
LEFT JOIN LATERAL (
    SELECT u.email::text AS owner_email
    FROM organization_members om
    INNER JOIN organization_member_roles omr
        ON omr.organization_id = om.organization_id AND omr.member_id = om.id
    INNER JOIN roles r
        ON r.id = omr.role_id AND r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug = 'owner'
    INNER JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
    WHERE om.organization_id = o.id
    ORDER BY om.created_at ASC, om.id ASC
    LIMIT 1
) ow ON TRUE
WHERE o.deleted_at IS NULL
  AND (sqlc.narg(organization_uuid)::uuid IS NULL OR o.uuid = sqlc.narg(organization_uuid)::uuid)
  AND (sqlc.narg(statuses)::text[] IS NULL OR o.status = ANY(sqlc.narg(statuses)::text[]))
  AND (
    sqlc.narg(access_states)::text[] IS NULL
    OR (CASE
          WHEN o.access_starts_at > NOW() THEN 'upcoming'
          WHEN o.access_ends_at IS NOT NULL AND o.access_ends_at <= NOW() THEN 'expired'
          ELSE 'open'
        END) = ANY(sqlc.narg(access_states)::text[])
  )
  AND (
    sqlc.narg(windows)::text[] IS NULL
    OR (CASE WHEN o.access_ends_at IS NULL THEN 'unlimited' ELSE 'limited' END) = ANY(sqlc.narg(windows)::text[])
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR o.name ILIKE '%' || sqlc.narg(q) || '%'
    OR o.slug ILIKE '%' || sqlc.narg(q) || '%'
    OR COALESCE(ow.owner_email, '') ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: SetOrganizationLogo :one
UPDATE organizations
SET logo_object_key = sqlc.narg(logo_object_key)
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: ListPlatformOrganizationActivity :many
-- Platform operations on the organization plus everything recorded inside it.
SELECT
    e.uuid, e.action, e.resource, e.resource_uuid, e.payload, e.created_at,
    u.uuid AS actor_uuid, u.name AS actor_name, u.surname AS actor_surname, u.email AS actor_email
FROM activity_events e
LEFT JOIN users u ON u.id = e.actor_user_id
WHERE (e.resource = 'platform.organizations' AND e.resource_uuid = sqlc.arg(organization_uuid)::uuid)
   OR e.organization_id = sqlc.arg(organization_id)::bigint
ORDER BY e.created_at DESC, e.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountPlatformOrganizationActivity :one
SELECT COUNT(*)::bigint
FROM activity_events e
WHERE (e.resource = 'platform.organizations' AND e.resource_uuid = sqlc.arg(organization_uuid)::uuid)
   OR e.organization_id = sqlc.arg(organization_id)::bigint;

-- name: UpdateOrganizationProfile :one
-- Tenant organization profile (CF-3): name, regional defaults and the
-- validated settings document. NULL arguments keep the current value.
UPDATE organizations
SET name = COALESCE(sqlc.narg(name), name),
    timezone = COALESCE(sqlc.narg(timezone), timezone),
    locale = COALESCE(sqlc.narg(locale), locale),
    currency = COALESCE(sqlc.narg(currency), currency),
    country_code = COALESCE(sqlc.narg(country_code), country_code),
    settings = COALESCE(sqlc.narg(settings), settings)
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;
