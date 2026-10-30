-- CF-87: customer health score.

-- name: GetCompanyHealthSettings :one
SELECT * FROM company_health_settings WHERE organization_id = $1;

-- name: UpsertCompanyHealthSettings :one
INSERT INTO company_health_settings (organization_id, enabled, threshold, weights)
VALUES (sqlc.arg(organization_id), sqlc.arg(enabled), sqlc.arg(threshold), sqlc.arg(weights))
ON CONFLICT (organization_id) DO UPDATE SET
    enabled = EXCLUDED.enabled, threshold = EXCLUDED.threshold, weights = EXCLUDED.weights
RETURNING *;

-- name: ListHealthOrganizations :many
-- Organizations with at least one company and the score not switched off.
SELECT o.id FROM organizations o
WHERE EXISTS (SELECT 1 FROM companies c WHERE c.organization_id = o.id AND c.deleted_at IS NULL)
  AND NOT EXISTS (SELECT 1 FROM company_health_settings s WHERE s.organization_id = o.id AND NOT s.enabled)
ORDER BY o.id;

-- name: CompanyHealthStats :many
-- Signal inputs of a page of companies (keyset on id), or of one company.
SELECT co.id, co.uuid, co.name, co.account_owner_id,
       (SELECT COALESCE(AVG(r.score), 0)::float8 FROM conversation_ratings r
          JOIN conversations v ON v.organization_id = r.organization_id AND v.id = r.conversation_id
         WHERE r.organization_id = co.organization_id AND v.organization_id = co.organization_id
           AND v.company_id = co.id AND r.created_at >= sqlc.arg(csat_since)) AS csat_avg,
       (SELECT COUNT(*) FROM conversation_ratings r
          JOIN conversations v ON v.organization_id = r.organization_id AND v.id = r.conversation_id
         WHERE r.organization_id = co.organization_id AND v.organization_id = co.organization_id
           AND v.company_id = co.id AND r.created_at >= sqlc.arg(csat_since))::bigint AS csat_count,
       (SELECT COUNT(*) FROM conversations v
         WHERE v.organization_id = co.organization_id AND v.company_id = co.id AND v.deleted_at IS NULL
           AND v.status IN ('open', 'pending'))::bigint AS open_conversations,
       (SELECT COUNT(*) FROM conversations v
         WHERE v.organization_id = co.organization_id AND v.company_id = co.id AND v.deleted_at IS NULL
           AND v.sla_breached_at >= sqlc.arg(window_start))::bigint AS sla_breaches,
       (SELECT COUNT(*) FROM conversations v
         WHERE v.organization_id = co.organization_id AND v.company_id = co.id AND v.deleted_at IS NULL
           AND v.created_at >= sqlc.arg(window_start))::bigint AS recent_conversations,
       (SELECT COUNT(*) FROM conversations v
         WHERE v.organization_id = co.organization_id AND v.company_id = co.id AND v.deleted_at IS NULL
           AND v.created_at >= sqlc.arg(previous_start) AND v.created_at < sqlc.arg(window_start))::bigint AS previous_conversations,
       COALESCE(rv.billing_status, '')::text AS billing_status,
       h.usage_score, h.usage_reported_at,
       COALESCE(h.below_threshold, FALSE)::boolean AS below_threshold,
       h.score AS previous_score
FROM companies co
LEFT JOIN company_revenue rv ON rv.organization_id = co.organization_id AND rv.company_id = co.id
LEFT JOIN company_health h ON h.organization_id = co.organization_id AND h.company_id = co.id
WHERE co.organization_id = sqlc.arg(organization_id)
  AND co.deleted_at IS NULL
  AND co.id > sqlc.arg(after_id)
  AND (sqlc.narg(company_id)::bigint IS NULL OR co.id = sqlc.narg(company_id))
ORDER BY co.id
LIMIT sqlc.arg(row_limit);

-- name: UpsertCompanyHealth :one
INSERT INTO company_health (organization_id, company_id, score, signals, below_threshold, alerted_at, computed_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(company_id), sqlc.narg(score), sqlc.arg(signals),
        sqlc.arg(below_threshold), sqlc.narg(alerted_at), sqlc.arg(computed_at))
ON CONFLICT (organization_id, company_id) DO UPDATE SET
    score = EXCLUDED.score,
    signals = EXCLUDED.signals,
    below_threshold = EXCLUDED.below_threshold,
    alerted_at = COALESCE(EXCLUDED.alerted_at, company_health.alerted_at),
    computed_at = EXCLUDED.computed_at
RETURNING *;

-- name: InsertCompanyHealthHistory :exec
INSERT INTO company_health_history (organization_id, company_id, score, signals)
VALUES (sqlc.arg(organization_id), sqlc.arg(company_id), sqlc.arg(score), sqlc.arg(signals));

-- name: ListCompanyHealthHistory :many
SELECT score, created_at FROM company_health_history
WHERE organization_id = sqlc.arg(organization_id) AND company_id = sqlc.arg(company_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: ListCompanyHealthByCompanyIDs :many
SELECT * FROM company_health
WHERE organization_id = sqlc.arg(organization_id) AND company_id = ANY(sqlc.arg(company_ids)::bigint[]);

-- name: SetCompanyUsageScore :one
INSERT INTO company_health (organization_id, company_id, usage_score, usage_reported_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(company_id), sqlc.arg(usage_score), NOW())
ON CONFLICT (organization_id, company_id) DO UPDATE SET
    usage_score = EXCLUDED.usage_score, usage_reported_at = EXCLUDED.usage_reported_at
RETURNING *;
