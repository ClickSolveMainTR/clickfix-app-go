-- CF-86: customer revenue data.

-- name: GetCompanyRevenue :one
SELECT * FROM company_revenue WHERE organization_id = $1 AND company_id = $2;

-- name: ListCompanyRevenueByCompanyIDs :many
SELECT * FROM company_revenue
WHERE organization_id = sqlc.arg(organization_id) AND company_id = ANY(sqlc.arg(company_ids)::bigint[]);

-- name: UpsertCompanyRevenue :one
INSERT INTO company_revenue (organization_id, company_id, mrr_minor, currency, plan, billing_status,
                             renewal_at, lifetime_value_minor, source, external_ref, updated_by_user_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(company_id), sqlc.arg(mrr_minor), sqlc.arg(currency), sqlc.arg(plan),
        sqlc.arg(billing_status), sqlc.narg(renewal_at), sqlc.arg(lifetime_value_minor), sqlc.arg(source),
        sqlc.arg(external_ref), sqlc.narg(updated_by_user_id))
ON CONFLICT (organization_id, company_id) DO UPDATE SET
    mrr_minor = EXCLUDED.mrr_minor,
    currency = EXCLUDED.currency,
    plan = EXCLUDED.plan,
    billing_status = EXCLUDED.billing_status,
    renewal_at = EXCLUDED.renewal_at,
    lifetime_value_minor = EXCLUDED.lifetime_value_minor,
    source = EXCLUDED.source,
    external_ref = EXCLUDED.external_ref,
    updated_by_user_id = EXCLUDED.updated_by_user_id
RETURNING *;

-- name: InsertCompanyRevenueHistory :exec
INSERT INTO company_revenue_history (organization_id, company_id, mrr_minor, currency, plan, billing_status,
                                     renewal_at, lifetime_value_minor, source, changed_by_user_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(company_id), sqlc.arg(mrr_minor), sqlc.arg(currency), sqlc.arg(plan),
        sqlc.arg(billing_status), sqlc.narg(renewal_at), sqlc.arg(lifetime_value_minor), sqlc.arg(source),
        sqlc.narg(changed_by_user_id));

-- name: ListCompanyRevenueHistory :many
SELECT h.*, u.uuid AS changed_by_uuid, u.name AS changed_by_name, u.surname AS changed_by_surname
FROM company_revenue_history h
LEFT JOIN users u ON u.id = h.changed_by_user_id
WHERE h.organization_id = sqlc.arg(organization_id) AND h.company_id = sqlc.arg(company_id)
ORDER BY h.created_at DESC, h.id DESC
LIMIT sqlc.arg(row_limit);

-- name: DeleteCompanyRevenue :execrows
DELETE FROM company_revenue WHERE organization_id = $1 AND company_id = $2;
