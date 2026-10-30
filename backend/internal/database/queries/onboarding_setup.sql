-- CF-140: AI-assisted set-up runs, their provenance ledger and the
-- lookups the apply job uses to stay idempotent (an entity with the same
-- natural key already exists → the item is skipped, never overwritten).

-- name: OnboardingCreateRun :one
INSERT INTO onboarding_setup_runs (organization_id, source, template_key, template_version, discovery, plan, ai_run_uuid, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(source), sqlc.arg(template_key), sqlc.arg(template_version),
        sqlc.arg(discovery), sqlc.arg(plan), sqlc.narg(ai_run_uuid), sqlc.narg(created_by))
RETURNING *;

-- name: OnboardingGetRun :one
SELECT * FROM onboarding_setup_runs WHERE organization_id = $1 AND uuid = $2;

-- name: OnboardingGetRunByID :one
SELECT * FROM onboarding_setup_runs WHERE organization_id = $1 AND id = $2;

-- name: OnboardingLatestRun :one
SELECT * FROM onboarding_setup_runs
WHERE organization_id = $1
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: OnboardingCountAIRunsSince :one
SELECT COUNT(*)::bigint FROM onboarding_setup_runs
WHERE organization_id = $1 AND source = 'ai' AND created_at >= sqlc.arg(since);

-- name: OnboardingQueueRun :one
-- Draft → queued with the owner's selection. The partial unique index
-- refuses a second active run of the same organization.
UPDATE onboarding_setup_runs
SET status = 'queued', selection = sqlc.arg(selection), total = sqlc.arg(total), done = 0, failed = 0, error = ''
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'draft'
RETURNING *;

-- name: OnboardingStartRun :execrows
UPDATE onboarding_setup_runs SET status = 'running', started_at = NOW()
WHERE organization_id = $1 AND id = $2 AND status = 'queued';

-- name: OnboardingProgressRun :exec
UPDATE onboarding_setup_runs SET done = sqlc.arg(done), failed = sqlc.arg(failed)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: OnboardingFinishRun :exec
UPDATE onboarding_setup_runs SET status = sqlc.arg(status), error = sqlc.arg(error), finished_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status IN ('queued', 'running');

-- name: OnboardingMarkRunUndone :execrows
UPDATE onboarding_setup_runs SET status = 'undone', undone_at = NOW()
WHERE organization_id = $1 AND id = $2 AND status IN ('succeeded', 'partial', 'failed');

-- name: OnboardingFailStaleRuns :exec
-- A process restart mid-apply leaves a run queued/running; after 15
-- minutes without progress it is closed as failed so the owner can retry.
UPDATE onboarding_setup_runs SET status = 'failed', error = 'interrupted', finished_at = NOW()
WHERE organization_id = $1 AND status IN ('queued', 'running') AND updated_at < NOW() - INTERVAL '15 minutes';

-- name: OnboardingUpsertItem :one
INSERT INTO onboarding_setup_items (organization_id, run_id, item_key, kind, label, status, entity_id, entity_uuid, previous, error)
VALUES (sqlc.arg(organization_id), sqlc.arg(run_id), sqlc.arg(item_key), sqlc.arg(kind), sqlc.arg(label), sqlc.arg(status),
        sqlc.narg(entity_id), sqlc.narg(entity_uuid), sqlc.narg(previous), sqlc.arg(error))
ON CONFLICT (run_id, item_key) DO UPDATE
SET status = excluded.status, entity_id = excluded.entity_id, entity_uuid = excluded.entity_uuid,
    previous = excluded.previous, error = excluded.error
RETURNING *;

-- name: OnboardingRunItems :many
SELECT * FROM onboarding_setup_items
WHERE organization_id = $1 AND run_id = $2
ORDER BY id;

-- name: OnboardingMarkItemUndone :exec
UPDATE onboarding_setup_items SET undone_at = NOW(), error = sqlc.arg(error)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: OnboardingEnsureAIPolicy :exec
-- Discovery turns AI on for an organization that never configured it
-- (default limits); an existing policy, enabled or not, is left alone.
INSERT INTO ai_policies (organization_id, enabled) VALUES ($1, TRUE)
ON CONFLICT (organization_id) DO NOTHING;

-- name: OnboardingFindTeam :one
SELECT id, uuid FROM teams WHERE organization_id = $1 AND lower(name) = lower(sqlc.arg(name)::text) LIMIT 1;

-- name: OnboardingFindTag :one
SELECT id, uuid FROM tags WHERE organization_id = $1 AND lower(name) = lower(sqlc.arg(name)::text) LIMIT 1;

-- name: OnboardingFindStatus :one
SELECT id, uuid FROM conversation_statuses WHERE organization_id = $1 AND slug = $2;

-- name: OnboardingNextStatusOrder :one
SELECT COALESCE(MAX(sort_order), 0)::int + 1 FROM conversation_statuses WHERE organization_id = $1;

-- name: OnboardingFindBusinessHours :one
SELECT id, uuid FROM business_hours WHERE organization_id = $1 AND lower(name) = lower(sqlc.arg(name)::text) LIMIT 1;

-- name: OnboardingFindSLAPolicy :one
SELECT id, uuid FROM sla_policies WHERE organization_id = $1 AND lower(name) = lower(sqlc.arg(name)::text) LIMIT 1;

-- name: OnboardingNextSLAPosition :one
SELECT COALESCE(MAX(position), -1)::int + 1 FROM sla_policies WHERE organization_id = $1;

-- name: OnboardingFindCannedResponse :one
SELECT id, uuid FROM canned_responses
WHERE organization_id = $1 AND scope = 'organization' AND shortcut = $2 LIMIT 1;

-- name: OnboardingFindMacro :one
SELECT id, uuid FROM macros
WHERE organization_id = $1 AND scope = 'organization' AND lower(name) = lower(sqlc.arg(name)::text) LIMIT 1;

-- name: OnboardingFindCustomField :one
SELECT id, uuid, archived_at FROM custom_field_definitions
WHERE organization_id = $1 AND entity_type = $2 AND object_type_id IS NULL AND key = $3;

-- name: OnboardingDeleteCustomField :execrows
DELETE FROM custom_field_definitions WHERE organization_id = $1 AND id = $2;

-- name: OnboardingFindRequestForm :one
SELECT id, uuid FROM request_forms WHERE organization_id = $1 AND lower(name) = lower(sqlc.arg(name)::text) LIMIT 1;

-- name: OnboardingNextFormPosition :one
SELECT COALESCE(MAX(position), -1)::int + 1 FROM request_forms WHERE organization_id = $1;

-- name: OnboardingFindRoutingRule :one
SELECT id, uuid FROM routing_rules WHERE organization_id = $1 AND lower(name) = lower(sqlc.arg(name)::text) LIMIT 1;

-- name: OnboardingNextRoutingPosition :one
SELECT COALESCE(MAX(position), -1)::int + 1 FROM routing_rules WHERE organization_id = $1;

-- name: OnboardingFindView :one
SELECT id, uuid FROM conversation_views
WHERE organization_id = $1 AND scope = 'organization' AND lower(name) = lower(sqlc.arg(name)::text) LIMIT 1;

-- name: OnboardingNextViewPosition :one
SELECT COALESCE(MAX(position), -1)::int + 1 FROM conversation_views WHERE organization_id = $1;

-- name: OnboardingFindSurvey :one
SELECT id, uuid FROM survey_templates WHERE organization_id = $1 AND kind = $2 LIMIT 1;

-- name: OnboardingDeleteSurvey :execrows
-- Only a template nobody was invited to yet.
DELETE FROM survey_templates s
WHERE s.organization_id = $1 AND s.id = $2
  AND NOT EXISTS (SELECT 1 FROM survey_invitations i WHERE i.organization_id = s.organization_id AND i.template_id = s.id);

-- name: OnboardingDeleteStatus :execrows
-- Conversations keep their base category; status_id is set NULL by the FK.
DELETE FROM conversation_statuses WHERE organization_id = $1 AND id = $2;

-- name: OnboardingFindContextSection :one
SELECT id, uuid FROM business_context_sections
WHERE organization_id = $1 AND kind = $2 AND active AND lower(title) = lower(sqlc.arg(title)::text) LIMIT 1;

-- name: OnboardingCountContextSections :one
SELECT COUNT(*)::bigint FROM business_context_sections WHERE organization_id = $1;

-- name: OnboardingDeactivateContextSection :execrows
-- Undo turns the section off (its version history is immutable).
UPDATE business_context_sections SET active = FALSE, revision = revision + 1, updated_by = sqlc.arg(updated_by)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND active;

-- name: OnboardingFindProcedure :one
SELECT id, uuid FROM ai_procedures WHERE organization_id = $1 AND intent = $2 AND locale = $3;

-- name: OnboardingDeleteProcedure :execrows
-- Only while it is still an unpublished draft.
DELETE FROM ai_procedures WHERE organization_id = $1 AND id = $2 AND published IS NULL;

-- name: OnboardingFindCollection :one
SELECT id, uuid FROM knowledge_collections WHERE organization_id = $1 AND slug = $2;

-- name: OnboardingFindArticle :one
SELECT id, uuid FROM knowledge_articles
WHERE organization_id = $1 AND locale = $2 AND slug = $3 AND deleted_at IS NULL;

-- name: OnboardingDetachDeletedArticles :exec
-- Soft-deleted articles keep their collection reference; detach them so
-- an empty collection created by onboarding can be removed on undo.
UPDATE knowledge_articles SET collection_id = NULL
WHERE organization_id = $1 AND collection_id = $2 AND deleted_at IS NOT NULL;

-- name: OnboardingCountLiveArticles :one
SELECT COUNT(*)::bigint FROM knowledge_articles
WHERE organization_id = $1 AND collection_id = $2 AND deleted_at IS NULL;

-- name: OnboardingFirstWidget :one
SELECT id, uuid, settings FROM channel_instances
WHERE organization_id = $1 AND channel_type = 'widget' AND deleted_at IS NULL
ORDER BY id
LIMIT 1;

-- name: OnboardingSetWidgetSettings :execrows
UPDATE channel_instances SET settings = sqlc.arg(settings), updated_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: OnboardingDeleteNotificationPreferences :execrows
-- Undo of the owner's notification defaults: the row did not exist before
-- onboarding created it, so removing it restores the product defaults.
DELETE FROM notification_preferences WHERE user_id = $1;
