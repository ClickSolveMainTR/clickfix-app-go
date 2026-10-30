-- name: CreateOrganizationOnboarding :exec
INSERT INTO organization_onboarding (organization_id)
VALUES ($1)
ON CONFLICT (organization_id) DO NOTHING;

-- name: GetOrganizationOnboarding :one
SELECT * FROM organization_onboarding
WHERE organization_id = $1;

-- name: AddOrganizationOnboardingStep :execrows
UPDATE organization_onboarding
SET steps = array_append(steps, sqlc.arg(step)::text)
WHERE organization_id = sqlc.arg(organization_id)
  AND finished_at IS NULL
  AND NOT (sqlc.arg(step)::text = ANY (steps));

-- name: FinishOrganizationOnboarding :execrows
UPDATE organization_onboarding
SET finished_at = NOW()
WHERE organization_id = $1
  AND finished_at IS NULL;

-- name: OrganizationOnboardingCounts :one
SELECT
    (SELECT COUNT(*) FROM channel_instances ci
        WHERE ci.organization_id = sqlc.arg(organization_id) AND ci.deleted_at IS NULL)::bigint AS channels,
    (SELECT COUNT(*) FROM channel_instances ci
        WHERE ci.organization_id = sqlc.arg(organization_id) AND ci.deleted_at IS NULL
          AND ci.channel_type IN ('widget', 'web_widget', 'webchat'))::bigint AS widgets,
    (SELECT COUNT(*) FROM organization_members om
        WHERE om.organization_id = sqlc.arg(organization_id))::bigint AS members,
    (SELECT COUNT(*) FROM organization_invitations oi
        WHERE oi.organization_id = sqlc.arg(organization_id)
          AND oi.revoked_at IS NULL)::bigint AS invitations,
    (SELECT COUNT(*) FROM conversations c
        WHERE c.organization_id = sqlc.arg(organization_id))::bigint AS conversations;

-- name: CountOwnedOrganizations :one
SELECT COUNT(*)::bigint
FROM organization_members om
INNER JOIN roles r ON r.id = om.role_id
INNER JOIN organizations o ON o.id = om.organization_id
WHERE om.user_id = $1
  AND r.scope = 'tenant'
  AND r.slug = 'owner'
  AND o.deleted_at IS NULL;

-- name: SelfServeSignupPriceAllowed :one
-- CF-42: a sign-up may pick its plan: a price of the published version of a
-- public, unarchived plan (with a trial it starts trialing, without one it
-- waits for the first payment).
SELECT EXISTS (
  SELECT 1 FROM billing_prices p
  JOIN billing_plan_versions v ON v.id = p.plan_version_id
  JOIN billing_plans b ON b.id = v.plan_id
  WHERE p.uuid = $1 AND v.status = 'published'
    AND b.visibility = 'public' AND b.archived_at IS NULL
)::boolean;
