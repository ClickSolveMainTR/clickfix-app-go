-- CF-46: business hours, holidays, SLA policies and conversation SLA clocks.

-- name: ListBusinessHours :many
SELECT * FROM business_hours WHERE organization_id = $1 ORDER BY is_default DESC, name, id;

-- name: GetBusinessHoursByUUID :one
SELECT * FROM business_hours WHERE organization_id = $1 AND uuid = $2;

-- name: GetBusinessHoursByID :one
SELECT * FROM business_hours WHERE organization_id = $1 AND id = $2;

-- name: GetDefaultBusinessHours :one
SELECT * FROM business_hours WHERE organization_id = $1 AND is_default;

-- name: GetTeamBusinessHours :one
SELECT * FROM business_hours WHERE organization_id = $1 AND team_id = $2;

-- name: ClearDefaultBusinessHours :exec
UPDATE business_hours SET is_default = FALSE WHERE organization_id = $1 AND is_default;

-- name: InsertBusinessHours :one
INSERT INTO business_hours (organization_id, name, timezone, working_hours, use_public_holidays, holiday_country, team_id, is_default)
VALUES (sqlc.arg(organization_id), sqlc.arg(name), sqlc.arg(timezone), sqlc.arg(working_hours),
        sqlc.arg(use_public_holidays), sqlc.arg(holiday_country), sqlc.narg(team_id), sqlc.arg(is_default))
RETURNING *;

-- name: UpdateBusinessHours :one
UPDATE business_hours
SET name = sqlc.arg(name), timezone = sqlc.arg(timezone), working_hours = sqlc.arg(working_hours),
    use_public_holidays = sqlc.arg(use_public_holidays), holiday_country = sqlc.arg(holiday_country),
    team_id = sqlc.narg(team_id), is_default = sqlc.arg(is_default)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: DeleteBusinessHours :execrows
DELETE FROM business_hours WHERE organization_id = $1 AND id = $2;

-- name: ListPublicHolidays :many
SELECT * FROM public_holidays
WHERE country_code = sqlc.arg(country_code) AND date BETWEEN sqlc.arg(from_date)::date AND sqlc.arg(to_date)::date
ORDER BY date;

-- name: ListBusinessHolidays :many
-- Holidays of one schedule plus the organization-wide ones.
SELECT * FROM business_holidays
WHERE organization_id = sqlc.arg(organization_id)
  AND (business_hours_id IS NULL OR business_hours_id = sqlc.narg(business_hours_id)::bigint)
  AND date BETWEEN sqlc.arg(from_date)::date AND sqlc.arg(to_date)::date
ORDER BY date, id;

-- name: ListAllBusinessHolidays :many
SELECT h.*, b.uuid AS business_hours_uuid
FROM business_holidays h
LEFT JOIN business_hours b ON b.organization_id = h.organization_id AND b.id = h.business_hours_id
WHERE h.organization_id = sqlc.arg(organization_id) AND h.date >= sqlc.arg(from_date)::date
ORDER BY h.date, h.id;

-- name: InsertBusinessHoliday :one
INSERT INTO business_holidays (organization_id, business_hours_id, date, name, closed, open_time, close_time)
VALUES (sqlc.arg(organization_id), sqlc.narg(business_hours_id), sqlc.arg(date), sqlc.arg(name),
        sqlc.arg(closed), sqlc.narg(open_time), sqlc.narg(close_time))
RETURNING *;

-- name: DeleteBusinessHoliday :execrows
DELETE FROM business_holidays WHERE organization_id = $1 AND uuid = $2;

-- name: ListSLAPolicies :many
SELECT * FROM sla_policies WHERE organization_id = $1 ORDER BY position, id;

-- name: ListActiveSLAPolicies :many
SELECT * FROM sla_policies WHERE organization_id = $1 AND is_active ORDER BY position, id;

-- name: GetSLAPolicyByUUID :one
SELECT * FROM sla_policies WHERE organization_id = $1 AND uuid = $2;

-- name: GetSLAPolicyByID :one
SELECT * FROM sla_policies WHERE organization_id = $1 AND id = $2;

-- name: NextSLAPolicyPosition :one
SELECT COALESCE(MAX(position), 0)::int + 1 FROM sla_policies WHERE organization_id = $1;

-- name: InsertSLAPolicy :one
INSERT INTO sla_policies (organization_id, name, description, is_active, position, business_hours_id, conditions, warning_percent)
VALUES (sqlc.arg(organization_id), sqlc.arg(name), sqlc.arg(description), sqlc.arg(is_active), sqlc.arg(position),
        sqlc.narg(business_hours_id), sqlc.arg(conditions), sqlc.arg(warning_percent))
RETURNING *;

-- name: UpdateSLAPolicy :one
UPDATE sla_policies
SET name = sqlc.arg(name), description = sqlc.arg(description), is_active = sqlc.arg(is_active),
    position = sqlc.arg(position), business_hours_id = sqlc.narg(business_hours_id),
    conditions = sqlc.arg(conditions), warning_percent = sqlc.arg(warning_percent)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: DeleteSLAPolicy :execrows
DELETE FROM sla_policies WHERE organization_id = $1 AND id = $2;

-- name: ListSLAPolicyTargets :many
SELECT * FROM sla_policy_targets WHERE organization_id = $1 ORDER BY policy_id, priority;

-- name: ListSLAPolicyTargetsFor :many
SELECT * FROM sla_policy_targets WHERE organization_id = $1 AND policy_id = $2 ORDER BY priority;

-- name: DeleteSLAPolicyTargets :exec
DELETE FROM sla_policy_targets WHERE organization_id = $1 AND policy_id = $2;

-- name: InsertSLAPolicyTarget :exec
INSERT INTO sla_policy_targets (organization_id, policy_id, priority, first_response_minutes, next_response_minutes, resolution_minutes)
VALUES (sqlc.arg(organization_id), sqlc.arg(policy_id), sqlc.arg(priority), sqlc.narg(first_response_minutes),
        sqlc.narg(next_response_minutes), sqlc.narg(resolution_minutes));

-- name: ListConversationTagUUIDs :many
SELECT t.uuid FROM conversation_tags ct
JOIN tags t ON t.organization_id = ct.organization_id AND t.id = ct.tag_id
WHERE ct.organization_id = $1 AND ct.conversation_id = $2
ORDER BY t.uuid;

-- name: GetCompanySLA :one
-- CF-86: the customer's revenue plan and MRR feed the company_plans and
-- min_company_mrr_minor conditions.
SELECT co.uuid, co.tier, co.sla_policy_id,
       COALESCE(r.plan, '')::text AS revenue_plan, COALESCE(r.mrr_minor, 0)::bigint AS mrr_minor
FROM companies co
LEFT JOIN company_revenue r ON r.organization_id = co.organization_id AND r.company_id = co.id
WHERE co.organization_id = $1 AND co.id = $2;

-- name: SetCompanySLAPolicy :one
UPDATE companies SET sla_policy_id = sqlc.narg(sla_policy_id)
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND deleted_at IS NULL
RETURNING id;

-- name: CountCompaniesBySLAPolicy :many
SELECT sla_policy_id::bigint AS policy_id, COUNT(*)::int AS companies
FROM companies
WHERE organization_id = $1 AND sla_policy_id IS NOT NULL AND deleted_at IS NULL
GROUP BY sla_policy_id;

-- name: ListOpenConversationIDsForCompany :many
SELECT id FROM conversations
WHERE organization_id = $1 AND company_id = $2 AND deleted_at IS NULL AND status IN ('open', 'pending', 'snoozed');

-- name: ListConversationSLAMetrics :many
SELECT * FROM conversation_sla_metrics WHERE organization_id = $1 AND conversation_id = $2 ORDER BY metric;

-- name: ListSLAMetricsForConversations :many
SELECT m.*, c.uuid AS conversation_uuid
FROM conversation_sla_metrics m
JOIN conversations c ON c.organization_id = m.organization_id AND c.id = m.conversation_id
WHERE m.organization_id = sqlc.arg(organization_id) AND c.uuid = ANY(sqlc.arg(uuids)::uuid[])
ORDER BY m.conversation_id, m.metric;

-- name: UpsertConversationSLAMetric :exec
INSERT INTO conversation_sla_metrics (
    organization_id, conversation_id, metric, policy_id, target_seconds, cycle, started_at, resumed_at,
    consumed_seconds, paused_at, warn_at, due_at, warned_at, breached_at, completed_at, updated_at
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(conversation_id), sqlc.arg(metric), sqlc.narg(policy_id), sqlc.arg(target_seconds),
    sqlc.arg(cycle), sqlc.arg(started_at), sqlc.arg(resumed_at), sqlc.arg(consumed_seconds), sqlc.narg(paused_at),
    sqlc.narg(warn_at), sqlc.narg(due_at), sqlc.narg(warned_at), sqlc.narg(breached_at), sqlc.narg(completed_at), NOW()
)
ON CONFLICT (conversation_id, metric) DO UPDATE SET
    policy_id = EXCLUDED.policy_id, target_seconds = EXCLUDED.target_seconds, cycle = EXCLUDED.cycle,
    started_at = EXCLUDED.started_at, resumed_at = EXCLUDED.resumed_at, consumed_seconds = EXCLUDED.consumed_seconds,
    paused_at = EXCLUDED.paused_at, warn_at = EXCLUDED.warn_at, due_at = EXCLUDED.due_at,
    warned_at = EXCLUDED.warned_at, breached_at = EXCLUDED.breached_at, completed_at = EXCLUDED.completed_at,
    updated_at = NOW();

-- name: DeleteConversationSLAMetric :exec
DELETE FROM conversation_sla_metrics WHERE organization_id = $1 AND conversation_id = $2 AND metric = $3;

-- name: ClaimDueSLAMetrics :many
-- Running clocks whose warning or due time has come, locked for the sweep.
SELECT * FROM conversation_sla_metrics
WHERE completed_at IS NULL AND paused_at IS NULL AND breached_at IS NULL
  AND (due_at <= sqlc.arg(now)::timestamptz OR (warned_at IS NULL AND warn_at <= sqlc.arg(now)::timestamptz))
ORDER BY COALESCE(LEAST(due_at, warn_at), due_at)
LIMIT sqlc.arg(max_rows)
FOR UPDATE SKIP LOCKED;

-- name: MarkSLAMetricWarned :exec
UPDATE conversation_sla_metrics SET warned_at = $4, updated_at = NOW()
WHERE organization_id = $1 AND conversation_id = $2 AND metric = $3;

-- name: MarkSLAMetricBreached :exec
UPDATE conversation_sla_metrics SET breached_at = $4, warned_at = COALESCE(warned_at, $4), updated_at = NOW()
WHERE organization_id = $1 AND conversation_id = $2 AND metric = $3;

-- name: MarkConversationSLABreached :exec
UPDATE conversations SET sla_breached_at = COALESCE(sla_breached_at, sqlc.arg(at)::timestamptz)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: ListOpenConversationIDsForSLA :many
-- Conversations whose clocks a policy or schedule change must recompute.
SELECT id FROM conversations
WHERE organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL
  AND status IN ('open', 'pending', 'snoozed')
  AND id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(max_rows);

-- name: ListContractedCompanies :many
SELECT c.uuid AS company_uuid, c.name, c.tier, p.uuid AS policy_uuid, p.name AS policy_name
FROM companies c
JOIN sla_policies p ON p.organization_id = c.organization_id AND p.id = c.sla_policy_id
WHERE c.organization_id = $1 AND c.deleted_at IS NULL
ORDER BY lower(c.name), c.id
LIMIT 500;

-- name: GetTeamWorkingHours :one
-- A team's own hours (CF-5); NULL working_hours means the organization's.
SELECT working_hours, timezone FROM teams WHERE organization_id = $1 AND id = $2;
