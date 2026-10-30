-- name: LockAnalyticsConversation :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(organization_id)::text || ':' || sqlc.arg(conversation_id)::text, 110));

-- name: ProjectAnalyticsConversation :exec
INSERT INTO analytics_conversations (organization_id,conversation_id,conversation_uuid,channel_type,team_uuid,agent_uuid,tag_uuids,status,opened_at,first_response_at,resolved_at,sla_total,sla_breached,csat)
SELECT c.organization_id,c.id,c.uuid,c.channel_type,t.uuid,u.uuid,
 ARRAY(SELECT tags.uuid FROM conversation_tags ct JOIN tags ON tags.organization_id=ct.organization_id AND tags.id=ct.tag_id WHERE ct.organization_id=c.organization_id AND ct.conversation_id=c.id),
 c.status,c.created_at,c.first_response_at,COALESCE(c.resolved_at,c.closed_at),
 (SELECT count(*)::int FROM conversation_sla_metrics sm WHERE sm.organization_id=c.organization_id AND sm.conversation_id=c.id AND (sm.completed_at IS NOT NULL OR sm.breached_at IS NOT NULL)),
 (SELECT count(*)::int FROM conversation_sla_metrics sm WHERE sm.organization_id=c.organization_id AND sm.conversation_id=c.id AND sm.breached_at IS NOT NULL),
 (SELECT score FROM conversation_ratings cr WHERE cr.organization_id=c.organization_id AND cr.conversation_id=c.id)
FROM conversations c
LEFT JOIN teams t ON t.organization_id=c.organization_id AND t.id=c.assignee_team_id
LEFT JOIN users u ON u.id=c.assignee_user_id
WHERE c.organization_id=sqlc.arg(organization_id) AND c.id=sqlc.arg(conversation_id)
ON CONFLICT(organization_id,conversation_id) DO UPDATE SET
 channel_type=excluded.channel_type,team_uuid=excluded.team_uuid,agent_uuid=excluded.agent_uuid,
 tag_uuids=excluded.tag_uuids,status=excluded.status,opened_at=excluded.opened_at,
 first_response_at=excluded.first_response_at,resolved_at=excluded.resolved_at,
 sla_total=excluded.sla_total,sla_breached=excluded.sla_breached,csat=excluded.csat,projected_at=now();

-- name: AnalyticsBackfillIDs :many
SELECT id FROM conversations WHERE organization_id=$1 AND id > sqlc.arg(after_id)::bigint ORDER BY id LIMIT 500;

-- name: AnalyticsTimezone :one
SELECT timezone FROM organizations WHERE id=$1 AND deleted_at IS NULL;

-- name: AnalyticsDashboard :one
WITH scoped AS (
 SELECT a.* FROM analytics_conversations a
 JOIN conversations c ON c.organization_id=a.organization_id AND c.id=a.conversation_id AND c.deleted_at IS NULL
 WHERE a.organization_id=sqlc.arg(organization_id)
 AND a.opened_at >= sqlc.arg(from_at)::timestamptz AND a.opened_at < sqlc.arg(to_at)::timestamptz
 AND (sqlc.arg(channel)::text='' OR a.channel_type=sqlc.arg(channel)::text)
 AND (sqlc.narg(team)::uuid IS NULL OR a.team_uuid=sqlc.narg(team)::uuid)
 AND (sqlc.narg(agent)::uuid IS NULL OR a.agent_uuid=sqlc.narg(agent)::uuid)
 AND (sqlc.narg(tag)::uuid IS NULL OR sqlc.narg(tag)::uuid=ANY(a.tag_uuids))
), cards AS (
 SELECT count(*) AS volume,count(*) FILTER(WHERE status IN ('resolved','closed')) AS resolved,
 count(*) FILTER(WHERE status NOT IN ('resolved','closed')) AS backlog,
 avg(extract(epoch FROM first_response_at-opened_at)) FILTER(WHERE first_response_at>=opened_at) AS first_response_seconds,
 avg(extract(epoch FROM resolved_at-opened_at)) FILTER(WHERE resolved_at>=opened_at) AS resolution_seconds,
 sum(sla_total) AS sla_total,sum(sla_breached) AS sla_breached,
 CASE WHEN sum(sla_total)>0 THEN 100.0*(sum(sla_total)-sum(sla_breached))/sum(sla_total) END AS sla_hit_percent,
 avg(csat) AS csat_average,count(csat) AS csat_responses FROM scoped
), series AS (
 SELECT date_trunc(sqlc.arg(bucket)::text,opened_at AT TIME ZONE sqlc.arg(timezone)::text) AS bucket,
 count(*) AS volume,count(*) FILTER(WHERE status IN ('resolved','closed')) AS resolved,
 avg(extract(epoch FROM first_response_at-opened_at)) FILTER(WHERE first_response_at>=opened_at) AS first_response_seconds,
 avg(extract(epoch FROM resolved_at-opened_at)) FILTER(WHERE resolved_at>=opened_at) AS resolution_seconds,
 sum(sla_breached) AS sla_breached,avg(csat) AS csat_average FROM scoped GROUP BY 1
)
SELECT jsonb_build_object('cards',(SELECT to_jsonb(cards) FROM cards),
 'series',COALESCE((SELECT jsonb_agg(to_jsonb(series) ORDER BY bucket) FROM series),'[]'::jsonb),
 'by_channel',COALESCE((SELECT jsonb_agg(to_jsonb(x)) FROM (SELECT channel_type AS dimension,count(*) AS volume FROM scoped GROUP BY channel_type ORDER BY channel_type) x),'[]'::jsonb),
 'by_team',COALESCE((SELECT jsonb_agg(to_jsonb(x)) FROM (SELECT team_uuid AS dimension,count(*) AS volume FROM scoped GROUP BY team_uuid ORDER BY team_uuid) x),'[]'::jsonb),
 'by_agent',COALESCE((SELECT jsonb_agg(to_jsonb(x)) FROM (SELECT agent_uuid AS dimension,count(*) AS volume FROM scoped GROUP BY agent_uuid ORDER BY agent_uuid) x),'[]'::jsonb),
 'by_tag',COALESCE((SELECT jsonb_agg(to_jsonb(x)) FROM (SELECT unnest(tag_uuids) AS dimension,count(*) AS volume FROM scoped GROUP BY dimension ORDER BY dimension) x),'[]'::jsonb))::jsonb AS result;
