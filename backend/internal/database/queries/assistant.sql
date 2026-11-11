-- CF-141: ClickFix Asistan (platform-owned AI support/setup chat).

-- name: AssistantSettingsGet :one
SELECT * FROM assistant_settings WHERE id = 1;

-- name: AssistantSettingsSave :one
INSERT INTO assistant_settings (id, enabled, model_route, daily_limit_per_org, system_prompt_extra, support_org_uuid, updated_by)
VALUES (1, sqlc.arg(enabled), sqlc.arg(model_route), sqlc.arg(daily_limit_per_org), sqlc.arg(system_prompt_extra),
        sqlc.narg(support_org_uuid), sqlc.narg(updated_by))
ON CONFLICT (id) DO UPDATE SET
    enabled = EXCLUDED.enabled, model_route = EXCLUDED.model_route, daily_limit_per_org = EXCLUDED.daily_limit_per_org,
    system_prompt_extra = EXCLUDED.system_prompt_extra, support_org_uuid = EXCLUDED.support_org_uuid,
    updated_by = EXCLUDED.updated_by
RETURNING *;

-- name: AssistantThreadCreate :one
INSERT INTO assistant_threads (organization_id, user_id, title, context)
VALUES (sqlc.arg(organization_id), sqlc.arg(user_id), sqlc.arg(title), sqlc.arg(context))
RETURNING *;

-- name: AssistantThreadsList :many
SELECT * FROM assistant_threads
WHERE organization_id = sqlc.arg(organization_id) AND user_id = sqlc.arg(user_id)
ORDER BY updated_at DESC
LIMIT 50;

-- name: AssistantThreadGet :one
SELECT * FROM assistant_threads
WHERE organization_id = sqlc.arg(organization_id) AND user_id = sqlc.arg(user_id) AND uuid = sqlc.arg(uuid);

-- name: AssistantThreadTouch :exec
UPDATE assistant_threads SET
    title = CASE WHEN title = '' THEN sqlc.arg(title) ELSE title END,
    context = sqlc.arg(context)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: AssistantThreadEscalated :exec
UPDATE assistant_threads SET escalated_at = NOW(), ticket_ref = sqlc.arg(ticket_ref)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: AssistantMessageCreate :one
INSERT INTO assistant_messages (organization_id, thread_id, role, content, suggestions, actions)
VALUES (sqlc.arg(organization_id), sqlc.arg(thread_id), sqlc.arg(role), sqlc.arg(content), sqlc.arg(suggestions), sqlc.arg(actions))
RETURNING *;

-- name: AssistantMessagesList :many
SELECT * FROM assistant_messages
WHERE organization_id = sqlc.arg(organization_id) AND thread_id = sqlc.arg(thread_id) ORDER BY id;

-- name: AssistantMessagesRecent :many
SELECT * FROM (
    SELECT * FROM assistant_messages
    WHERE organization_id = sqlc.arg(organization_id) AND thread_id = sqlc.arg(thread_id)
    ORDER BY id DESC LIMIT sqlc.arg(max_rows)
) recent ORDER BY id;

-- name: AssistantMessageWithAction :one
SELECT * FROM assistant_messages
WHERE organization_id = sqlc.arg(organization_id) AND thread_id = sqlc.arg(thread_id) AND role = 'assistant'
  AND actions @> jsonb_build_array(jsonb_build_object('id', sqlc.arg(action_id)::text))
FOR UPDATE;

-- name: AssistantMessageSetActions :exec
UPDATE assistant_messages SET actions = sqlc.arg(actions)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: AssistantUsageRecord :exec
INSERT INTO assistant_usage (organization_id, user_id, thread_id, kind, provider, model, status, input_tokens, output_tokens, cost_micros, latency_ms, error_code)
VALUES (sqlc.arg(organization_id), sqlc.narg(user_id), sqlc.narg(thread_id), sqlc.arg(kind), sqlc.arg(provider), sqlc.arg(model),
        sqlc.arg(status), sqlc.arg(input_tokens), sqlc.arg(output_tokens), sqlc.arg(cost_micros), sqlc.arg(latency_ms), sqlc.arg(error_code));

-- Calls since the start of the current UTC day (the per-organization limit).
-- name: AssistantUsageToday :one
SELECT COUNT(*)::bigint FROM assistant_usage
WHERE organization_id = $1 AND kind = 'chat' AND created_at >= date_trunc('day', NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC';

-- name: AssistantUsageSummary :one
SELECT COUNT(*)::bigint AS calls,
       COUNT(DISTINCT organization_id)::bigint AS organizations,
       COALESCE(SUM(input_tokens), 0)::bigint AS input_tokens,
       COALESCE(SUM(output_tokens), 0)::bigint AS output_tokens,
       COALESCE(SUM(cost_micros), 0)::bigint AS cost_micros
FROM assistant_usage WHERE created_at >= sqlc.arg(since);

-- name: AssistantSetupState :one
SELECT
    (SELECT COUNT(*) FROM channel_instances ci WHERE ci.organization_id = sqlc.arg(organization_id) AND ci.deleted_at IS NULL)::bigint AS channels,
    (SELECT COUNT(*) FROM channel_instances ci WHERE ci.organization_id = sqlc.arg(organization_id) AND ci.deleted_at IS NULL
        AND ci.channel_type IN ('widget', 'web_widget', 'webchat'))::bigint AS widgets,
    (SELECT COUNT(*) FROM channel_instances ci WHERE ci.organization_id = sqlc.arg(organization_id) AND ci.deleted_at IS NULL
        AND ci.channel_type = 'email')::bigint AS email_channels,
    (SELECT COUNT(*) FROM organization_members om WHERE om.organization_id = sqlc.arg(organization_id))::bigint AS members,
    (SELECT COUNT(*) FROM organization_invitations oi WHERE oi.organization_id = sqlc.arg(organization_id)
        AND oi.revoked_at IS NULL AND oi.accepted_at IS NULL)::bigint AS pending_invitations,
    (SELECT COUNT(*) FROM teams t WHERE t.organization_id = sqlc.arg(organization_id))::bigint AS teams,
    (SELECT COUNT(*) FROM business_hours bh WHERE bh.organization_id = sqlc.arg(organization_id))::bigint AS business_hours,
    (SELECT COUNT(*) FROM sla_policies sp WHERE sp.organization_id = sqlc.arg(organization_id) AND sp.is_active)::bigint AS sla_policies,
    COALESCE((SELECT ap.enabled FROM ai_policies ap WHERE ap.organization_id = sqlc.arg(organization_id)), FALSE)::boolean AS ai_enabled,
    (SELECT COUNT(*) FROM ai_agent_configs ac WHERE ac.organization_id = sqlc.arg(organization_id))::bigint AS ai_agents,
    (SELECT COUNT(*) FROM knowledge_articles ka WHERE ka.organization_id = sqlc.arg(organization_id) AND ka.deleted_at IS NULL)::bigint AS knowledge_articles,
    (SELECT COUNT(*) FROM knowledge_articles ka WHERE ka.organization_id = sqlc.arg(organization_id) AND ka.deleted_at IS NULL
        AND ka.visibility = 'public' AND ka.published_version > 0)::bigint AS public_articles,
    (SELECT COUNT(*) FROM canned_responses cr WHERE cr.organization_id = sqlc.arg(organization_id))::bigint AS canned_replies,
    (SELECT COUNT(*) FROM push_devices pd JOIN organization_members om ON om.user_id = pd.user_id
        WHERE om.organization_id = sqlc.arg(organization_id) AND pd.disabled_at IS NULL)::bigint AS mobile_devices,
    (SELECT COUNT(*) FROM conversations c WHERE c.organization_id = sqlc.arg(organization_id))::bigint AS conversations;

-- The inbox escalations land in, in ClickFix's own support organization.
-- name: AssistantSupportInbox :one
SELECT ci.id, o.id AS organization_id FROM channel_instances ci
JOIN organizations o ON o.id = ci.organization_id
WHERE o.uuid = sqlc.arg(organization_uuid) AND o.deleted_at IS NULL AND ci.deleted_at IS NULL AND ci.status = 'active'
  AND ci.channel_type IN ('api', 'widget', 'email')
ORDER BY CASE ci.channel_type WHEN 'api' THEN 0 WHEN 'widget' THEN 1 ELSE 2 END, ci.id
LIMIT 1;

-- name: AssistantSupportWidget :one
-- The support organization's active widget (CF-193): the panel embeds it
-- and identifies the member with a server-side signature.
SELECT ci.id, ci.uuid AS public_uuid, o.id AS organization_id FROM channel_instances ci
JOIN organizations o ON o.id = ci.organization_id
WHERE o.uuid = sqlc.arg(organization_uuid) AND o.deleted_at IS NULL AND ci.deleted_at IS NULL AND ci.status = 'active'
  AND ci.channel_type = 'widget'
ORDER BY ci.id
LIMIT 1;

-- name: AssistantEscalatedConversation :one
SELECT c.uuid, c.number FROM messages m
JOIN conversations c ON c.organization_id = m.organization_id AND c.id = m.conversation_id
WHERE m.organization_id = sqlc.arg(organization_id) AND m.channel_instance_id = sqlc.arg(channel_instance_id)
  AND m.external_id = sqlc.arg(external_id)
LIMIT 1;

-- name: AssistantTenantRoleUUID :one
SELECT uuid FROM roles
WHERE scope = 'tenant' AND slug = sqlc.arg(slug) AND (organization_id IS NULL OR organization_id = sqlc.arg(organization_id))
ORDER BY organization_id NULLS LAST
LIMIT 1;

-- name: AssistantEnableAI :exec
INSERT INTO ai_policies (organization_id, enabled) VALUES ($1, TRUE)
ON CONFLICT (organization_id) DO UPDATE SET enabled = TRUE;

-- name: AssistantMessageInterrupted :exec
UPDATE assistant_messages SET interrupted = TRUE, error_code = sqlc.arg(error_code)
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: AssistantThreadTitle :exec
UPDATE assistant_threads SET title = sqlc.arg(title) WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: AssistantDocumentCreate :one
INSERT INTO assistant_documents (organization_id, thread_id, kind, name, source_url, content_type, size_bytes, content, truncated, masked)
VALUES (sqlc.arg(organization_id), sqlc.arg(thread_id), sqlc.arg(kind), sqlc.arg(name), sqlc.arg(source_url),
        sqlc.arg(content_type), sqlc.arg(size_bytes), sqlc.arg(content), sqlc.arg(truncated), sqlc.arg(masked))
RETURNING *;

-- name: AssistantDocumentsList :many
SELECT id, uuid, organization_id, thread_id, kind, name, source_url, content_type, size_bytes,
       char_length(content)::int AS chars, truncated, masked, created_at, expires_at
FROM assistant_documents
WHERE organization_id = sqlc.arg(organization_id) AND thread_id = sqlc.arg(thread_id) AND expires_at > NOW()
ORDER BY created_at
LIMIT 20;

-- name: AssistantDocumentGet :one
SELECT * FROM assistant_documents
WHERE organization_id = sqlc.arg(organization_id) AND thread_id = sqlc.arg(thread_id) AND uuid = sqlc.arg(uuid) AND expires_at > NOW();

-- name: AssistantDocumentDelete :execrows
DELETE FROM assistant_documents
WHERE organization_id = sqlc.arg(organization_id) AND thread_id = sqlc.arg(thread_id) AND uuid = sqlc.arg(uuid);

-- name: AssistantDocumentsPurgeExpired :execrows
DELETE FROM assistant_documents WHERE expires_at <= NOW();
