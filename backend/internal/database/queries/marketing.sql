-- CF-144 marketing tracking settings and signup attribution.

-- name: MarketingSettings :one
SELECT * FROM marketing_settings WHERE id = 1;

-- name: MarketingSaveConfig :one
UPDATE marketing_settings SET config = $1, updated_by = $2 WHERE id = 1 RETURNING *;

-- name: MarketingSetOGImage :one
UPDATE marketing_settings SET og_image_key = sqlc.narg(og_image_key), updated_by = sqlc.narg(updated_by) WHERE id = 1 RETURNING *;

-- name: MarketingRecordUserAttribution :exec
INSERT INTO signup_attributions (user_id, first_touch, last_touch, source, medium, campaign)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id) DO NOTHING;

-- name: MarketingRecordOrganizationAttribution :exec
INSERT INTO signup_attributions (organization_id, first_touch, last_touch, source, medium, campaign)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (organization_id) DO NOTHING;

-- name: MarketingOrganizationAttribution :one
SELECT a.* FROM signup_attributions a
JOIN organizations o ON o.id = a.organization_id
WHERE o.uuid = sqlc.arg(organization_uuid);

-- name: MarketingOrganizationIDByUUID :one
SELECT id FROM organizations WHERE uuid = $1;

-- name: MarketingSourcesReport :many
SELECT COALESCE(a.source, '')::text AS source, COALESCE(a.medium, '')::text AS medium,
       COALESCE(a.campaign, '')::text AS campaign, COUNT(*)::bigint AS organizations
FROM organizations o
LEFT JOIN signup_attributions a ON a.organization_id = o.id
WHERE o.created_at >= sqlc.arg(from_at) AND o.created_at < sqlc.arg(to_at)
GROUP BY 1, 2, 3
ORDER BY organizations DESC, source, medium, campaign
LIMIT 500;

-- name: MarketingCopyOwnerAttribution :exec
INSERT INTO signup_attributions (organization_id, first_touch, last_touch, source, medium, campaign)
SELECT sqlc.arg(organization_id)::bigint, a.first_touch, a.last_touch, a.source, a.medium, a.campaign
FROM signup_attributions a
WHERE a.user_id = (
    SELECT m.user_id FROM organization_members m
    WHERE m.organization_id = sqlc.arg(organization_id)::bigint
    ORDER BY m.id LIMIT 1
)
ON CONFLICT (organization_id) DO NOTHING;

-- name: MarketingUserIDByUUID :one
SELECT id FROM users WHERE uuid = $1;
