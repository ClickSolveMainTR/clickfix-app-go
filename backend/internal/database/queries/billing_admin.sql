-- Platform billing management lists (CF-39). Cross-organization reads for
-- platform staff only (platform.subscriptions.read); tenant code never calls them.

-- name: BillingAdminSubscriptions :many
SELECT s.uuid, s.status, s.period_start, s.period_end, s.trial_ends_at, s.grace_ends_at,
       s.canceled_at, s.cancel_at_period_end, s.auto_pay, s.created_at,
       o.uuid AS organization_uuid, o.name AS organization_name, o.slug AS organization_slug,
       p.uuid AS plan_uuid, p.code AS plan_code, p.name AS plan_name,
       v.uuid AS version_uuid, v.version AS version_number,
       pr.uuid AS price_uuid, pr.interval AS price_interval, pr.currency AS price_currency,
       pr.amount_minor AS price_amount_minor, pr.seat_amount_minor AS price_seat_amount_minor,
       pm.provider AS card_provider, pm.brand AS card_brand, pm.last4 AS card_last4,
       (SELECT count(*) FROM billing_bank_transfers t WHERE t.organization_id = s.organization_id AND t.status = 'reported')::bigint AS pending_transfers
FROM billing_subscriptions s
JOIN organizations o ON o.id = s.organization_id
JOIN billing_plan_versions v ON v.id = s.plan_version_id
JOIN billing_plans p ON p.id = v.plan_id
JOIN billing_prices pr ON pr.id = s.price_id
LEFT JOIN billing_payment_methods pm ON pm.organization_id = s.organization_id AND pm.id = s.payment_method_id AND pm.status = 'active'
WHERE (sqlc.arg(status)::text = '' OR s.status = sqlc.arg(status)::text)
  AND (sqlc.arg(live_only)::boolean = false OR s.status IN ('trialing','active','past_due','grace','paused'))
  AND (sqlc.arg(search)::text = '' OR o.name ILIKE '%' || sqlc.arg(search)::text || '%' OR o.slug ILIKE '%' || sqlc.arg(search)::text || '%')
ORDER BY s.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: BillingAdminSubscriptionCount :one
SELECT count(*)::bigint
FROM billing_subscriptions s
JOIN organizations o ON o.id = s.organization_id
WHERE (sqlc.arg(status)::text = '' OR s.status = sqlc.arg(status)::text)
  AND (sqlc.arg(live_only)::boolean = false OR s.status IN ('trialing','active','past_due','grace','paused'))
  AND (sqlc.arg(search)::text = '' OR o.name ILIKE '%' || sqlc.arg(search)::text || '%' OR o.slug ILIKE '%' || sqlc.arg(search)::text || '%');

-- name: BillingAdminSubscriptionStats :many
SELECT s.status, pr.interval AS price_interval, pr.currency AS price_currency,
       count(*)::bigint AS subscriptions, COALESCE(sum(pr.amount_minor), 0)::bigint AS amount_minor
FROM billing_subscriptions s
JOIN billing_prices pr ON pr.id = s.price_id
WHERE s.status IN ('trialing','active','past_due','grace','paused')
GROUP BY s.status, pr.interval, pr.currency
ORDER BY s.status, pr.interval, pr.currency;

-- name: BillingAdminPayments :many
SELECT pay.uuid, pay.provider, pay.kind, pay.amount_minor, pay.currency, pay.status, pay.failure_code,
       pay.created_at, pay.completed_at,
       o.uuid AS organization_uuid, o.name AS organization_name, o.slug AS organization_slug,
       i.uuid AS invoice_uuid, i.payment_reference AS invoice_reference
FROM billing_payments pay
JOIN organizations o ON o.id = pay.organization_id
JOIN billing_invoices i ON i.organization_id = pay.organization_id AND i.id = pay.invoice_id
WHERE (sqlc.arg(status)::text = '' OR pay.status = sqlc.arg(status)::text)
  AND (sqlc.arg(organization_id)::bigint = 0 OR pay.organization_id = sqlc.arg(organization_id)::bigint)
ORDER BY pay.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: BillingAdminPaymentCount :one
SELECT count(*)::bigint FROM billing_payments pay
WHERE (sqlc.arg(status)::text = '' OR pay.status = sqlc.arg(status)::text)
  AND (sqlc.arg(organization_id)::bigint = 0 OR pay.organization_id = sqlc.arg(organization_id)::bigint);

-- name: BillingAdminTransfers :many
SELECT t.uuid, t.amount_minor, t.status, t.note, t.created_at, t.reviewed_at, t.rejection_reason,
       (t.attachment_uuid IS NOT NULL)::boolean AS has_receipt,
       o.uuid AS organization_uuid, o.name AS organization_name, o.slug AS organization_slug,
       i.uuid AS invoice_uuid, i.payment_reference AS invoice_reference, i.total_minor AS invoice_total_minor,
       i.status AS invoice_status,
       a.bank_name, a.iban
FROM billing_bank_transfers t
JOIN organizations o ON o.id = t.organization_id
JOIN billing_invoices i ON i.organization_id = t.organization_id AND i.id = t.invoice_id
JOIN billing_bank_accounts a ON a.id = t.bank_account_id
WHERE (sqlc.arg(status)::text = '' OR t.status = sqlc.arg(status)::text)
ORDER BY (t.status = 'reported') DESC, t.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: BillingAdminTransferCount :one
SELECT count(*)::bigint FROM billing_bank_transfers t
WHERE (sqlc.arg(status)::text = '' OR t.status = sqlc.arg(status)::text);
