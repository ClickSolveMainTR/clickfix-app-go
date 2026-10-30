-- name: BillingLockOrganization :one
SELECT id FROM organizations WHERE uuid=$1 FOR UPDATE;
-- name: BillingLockOrganizationID :one
SELECT id FROM organizations WHERE id=$1 FOR UPDATE;
-- name: BillingGetOrganizationID :one
SELECT id FROM organizations WHERE uuid=$1;
-- name: BillingRevision :one
SELECT billing_revision FROM organizations WHERE id=$1;
-- name: BillingGetSubscription :one
SELECT * FROM billing_subscriptions WHERE organization_id=$1 AND status NOT IN ('canceled','expired') ORDER BY id DESC LIMIT 1;
-- name: BillingLockSubscription :one
SELECT * FROM billing_subscriptions WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: BillingCreateSubscription :one
INSERT INTO billing_subscriptions(organization_id,plan_version_id,price_id,status,period_start,period_end,trial_ends_at)
 VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING *;
-- name: BillingSetSubscription :one
UPDATE billing_subscriptions SET status=$3,period_start=$4,period_end=$5,grace_ends_at=$6,canceled_at=$7,cancel_at_period_end=$8,
 plan_version_id=$9,price_id=$10,pending_plan_version_id=$11,pending_price_id=$12
 WHERE organization_id=$1 AND id=$2 RETURNING *;
-- name: BillingSchedulePlan :exec
UPDATE billing_subscriptions SET pending_plan_version_id=$3,pending_price_id=$4 WHERE organization_id=$1 AND id=$2;
-- name: BillingRecordTransition :exec
INSERT INTO billing_subscription_transitions(organization_id,subscription_id,from_status,to_status,reason,actor_user_id)
 VALUES($1,$2,$3,$4,$5,$6);
-- name: BillingListTransitions :many
SELECT * FROM billing_subscription_transitions WHERE organization_id=$1 AND subscription_id=$2 ORDER BY id DESC LIMIT 100;
-- name: BillingGetOverrideExpiry :one
SELECT min(expires_at)::timestamptz AS expires_at FROM billing_entitlement_overrides WHERE organization_id=$1 AND expires_at>now();
-- name: BillingResolveEntitlements :many
SELECT f.key,f.kind,f.aggregation,f.period,
 -- A version without a grant for the feature (added after it was published)
 -- falls back to the catalog default while a subscription is live.
 COALESCE(o.value_bool,e.value_bool,CASE WHEN s.id IS NOT NULL AND e.feature_id IS NULL THEN f.default_bool END,false)::boolean AS enabled,
 COALESCE(CASE WHEN o.id IS NOT NULL THEN o.value_limit WHEN e.feature_id IS NOT NULL THEN e.value_limit WHEN s.id IS NOT NULL THEN f.default_limit END,0)::bigint AS value_limit,
 COALESCE(o.unlimited,e.unlimited,CASE WHEN s.id IS NOT NULL AND e.feature_id IS NULL THEN f.default_unlimited END,false)::boolean AS unlimited,
 COALESCE(e.enforcement,'hard')::text AS enforcement,COALESCE(e.warn_percent,80)::integer AS warn_percent,
 COALESCE(a.units,0)::bigint AS addon_units,
 (o.id IS NOT NULL)::boolean AS overridden
 FROM billing_features f
 LEFT JOIN billing_subscriptions s ON s.organization_id=sqlc.arg(organization_id) AND s.status IN ('trialing','active','past_due','grace')
  -- A trial paid early is active with its first paid period starting at the
  -- trial end: it stays entitled in between.
  AND (s.period_start<=now() OR (s.trial_ends_at IS NOT NULL AND s.period_start<=s.trial_ends_at))
  AND (s.grace_ends_at IS NULL OR s.grace_ends_at>now())
  AND (NOT s.cancel_at_period_end OR s.period_end>now())
  AND (s.status<>'trialing' OR COALESCE(s.trial_ends_at,s.period_end)>now())
 LEFT JOIN billing_plan_entitlements e ON e.plan_version_id=s.plan_version_id AND e.feature_id=f.id
 LEFT JOIN LATERAL (SELECT sum(i.quantity*ad.units)::bigint AS units FROM billing_subscription_items i
   JOIN billing_addons ad ON ad.id=i.addon_id WHERE i.organization_id=sqlc.arg(organization_id) AND i.subscription_id=s.id AND ad.feature_id=f.id) a ON true
 LEFT JOIN billing_entitlement_overrides o ON o.organization_id=sqlc.arg(organization_id) AND o.feature_id=f.id
  AND (o.expires_at IS NULL OR o.expires_at>now())
 ORDER BY f.key;
-- name: BillingUpsertOverride :exec
INSERT INTO billing_entitlement_overrides(organization_id,feature_id,kind,value_bool,value_limit,unlimited,reason,expires_at,created_by)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
 ON CONFLICT(organization_id,feature_id) DO UPDATE SET value_bool=excluded.value_bool,value_limit=excluded.value_limit,
 unlimited=excluded.unlimited,reason=excluded.reason,expires_at=excluded.expires_at,created_by=excluded.created_by;
-- name: BillingDeleteOverride :exec
DELETE FROM billing_entitlement_overrides WHERE organization_id=$1 AND feature_id=$2;
-- name: BillingEnsureUsagePeriod :one
INSERT INTO billing_usage_periods(organization_id,starts_at,ends_at) VALUES($1,$2,$3)
 ON CONFLICT(organization_id,starts_at) DO UPDATE SET starts_at=excluded.starts_at RETURNING *;
-- name: BillingGetUsage :one
SELECT COALESCE(sum(t.quantity),0)::bigint AS used FROM billing_usage_totals t JOIN billing_usage_periods p ON p.organization_id=t.organization_id AND p.id=t.usage_period_id
 JOIN billing_features f ON f.id=t.feature_id WHERE t.organization_id=$1 AND f.key=$2 AND p.starts_at<=now() AND p.ends_at>now();
-- name: BillingUsageExists :one
SELECT EXISTS(SELECT 1 FROM billing_usage_records WHERE organization_id=$1 AND idempotency_key=$2);
-- name: BillingRecordUsage :exec
INSERT INTO billing_usage_records(organization_id,usage_period_id,feature_id,idempotency_key,quantity,occurred_at) VALUES($1,$2,$3,$4,$5,now());
-- name: BillingIncrementUsage :exec
INSERT INTO billing_usage_totals(organization_id,usage_period_id,feature_id,quantity) VALUES($1,$2,$3,$4)
 ON CONFLICT(organization_id,usage_period_id,feature_id) DO UPDATE SET quantity=billing_usage_totals.quantity+excluded.quantity;
-- name: BillingDueLifecycle :many
SELECT organization_id,uuid FROM billing_subscriptions WHERE status NOT IN ('canceled','expired') AND
 ((cancel_at_period_end AND period_end<=sqlc.arg(at_time)::timestamptz)
 OR (status='grace' AND grace_ends_at<=sqlc.arg(at_time)::timestamptz)
 OR (status='trialing' AND trial_ends_at<=sqlc.arg(at_time)::timestamptz)
 OR (status='pending_payment' AND period_end<=sqlc.arg(at_time)::timestamptz))
 ORDER BY organization_id LIMIT 100;
-- name: BillingGetUsageEvent :one
SELECT r.quantity,f.key FROM billing_usage_records r JOIN billing_features f ON f.id=r.feature_id
 WHERE r.organization_id=$1 AND r.idempotency_key=$2;
