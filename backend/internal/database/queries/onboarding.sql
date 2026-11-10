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

-- name: OrganizationSetupCounts :one
-- The facts behind the set-up checklist (CF-198): one row per
-- organization, read by the onboarding checklist and the assistant.
SELECT
    (SELECT COUNT(*) FROM channel_instances ci
        WHERE ci.organization_id = sqlc.arg(organization_id) AND ci.deleted_at IS NULL
          AND ci.channel_type IN ('widget', 'web_widget', 'webchat'))::bigint AS widgets,
    (SELECT COUNT(*) FROM contacts ct
        WHERE ct.organization_id = sqlc.arg(organization_id) AND ct.deleted_at IS NULL
          AND ct.external_id IS NOT NULL)::bigint AS external_contacts,
    (SELECT COUNT(*) FROM companies co
        WHERE co.organization_id = sqlc.arg(organization_id) AND co.deleted_at IS NULL
          AND co.external_id IS NOT NULL)::bigint AS external_companies,
    (SELECT COUNT(*) FROM companies co
        LEFT JOIN company_revenue r ON r.organization_id = co.organization_id AND r.company_id = co.id
        WHERE co.organization_id = sqlc.arg(organization_id) AND co.deleted_at IS NULL
          AND (co.tier <> '' OR COALESCE(r.plan, '') <> ''))::bigint AS packaged_companies,
    (SELECT COALESCE(
        NULLIF(o.settings->'company'->>'legal_name', '') IS NOT NULL
        OR NULLIF(o.settings->'company'->>'website', '') IS NOT NULL
        OR NULLIF(o.settings->'company'->>'phone', '') IS NOT NULL, FALSE)
        FROM organizations o WHERE o.id = sqlc.arg(organization_id))::boolean AS company_profile,
    (SELECT COUNT(*) FROM integration_connections ic
        WHERE ic.organization_id = sqlc.arg(organization_id))::bigint AS systems,
    COALESCE((SELECT ap.enabled FROM ai_policies ap WHERE ap.organization_id = sqlc.arg(organization_id)), FALSE)::boolean AS ai_enabled,
    (SELECT COUNT(*) FROM ai_procedures p
        WHERE p.organization_id = sqlc.arg(organization_id) AND p.published IS NOT NULL)::bigint AS procedures,
    (SELECT COUNT(*) FROM knowledge_articles ka
        WHERE ka.organization_id = sqlc.arg(organization_id) AND ka.deleted_at IS NULL)::bigint AS articles,
    (SELECT COUNT(*) FROM sla_policies sp
        WHERE sp.organization_id = sqlc.arg(organization_id) AND sp.is_active)::bigint AS sla_policies,
    ((SELECT COUNT(*) FROM automation_rules ar WHERE ar.organization_id = sqlc.arg(organization_id))
        + (SELECT COUNT(*) FROM workflows w WHERE w.organization_id = sqlc.arg(organization_id)))::bigint AS automations;

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
