-- name: BillingPlanCreditGranted :one
SELECT COALESCE(sum(delta),0)::bigint FROM billing_credit_ledger
WHERE organization_id=$1 AND subscription_id=$2 AND unit=sqlc.arg(unit)::text AND delta>0
AND (idempotency_key=sqlc.arg(period_key)::text OR starts_with(idempotency_key,sqlc.arg(period_key)::text||':upgrade:'));

-- name: BillingPlanCreditPage :many
SELECT id,organization_id FROM billing_subscriptions
WHERE id>sqlc.arg(after_id)::bigint AND status IN ('active','trialing') AND period_start<=now() AND period_end>now()
ORDER BY id LIMIT 100;

-- name: AIWalletFunded :one
SELECT EXISTS(SELECT 1 FROM billing_credit_ledger WHERE organization_id=$1 AND unit='ai_credits' AND idempotency_key='ai:reserve:'||sqlc.arg(run_uuid)::uuid::text)::boolean;

-- name: AIConfiguredTokens :one
SELECT (EXISTS(SELECT 1 FROM billing_subscriptions s JOIN billing_plan_entitlements e ON e.plan_version_id=s.plan_version_id
JOIN billing_features f ON f.id=e.feature_id WHERE s.organization_id=$1 AND s.status IN ('active','trialing','past_due','grace') AND f.key='ai.tokens')
OR EXISTS(SELECT 1 FROM billing_entitlement_overrides o JOIN billing_features f ON f.id=o.feature_id WHERE o.organization_id=$1 AND f.key='ai.tokens' AND (o.expires_at IS NULL OR o.expires_at>now())))::boolean AS configured;
