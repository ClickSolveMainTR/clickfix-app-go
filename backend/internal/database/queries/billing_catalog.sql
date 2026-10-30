-- name: BillingListPlans :many
SELECT * FROM billing_plans WHERE (NOT sqlc.arg(public_only)::boolean OR (visibility='public' AND archived_at IS NULL
 AND EXISTS(SELECT 1 FROM billing_plan_versions v WHERE v.plan_id=billing_plans.id AND v.status='published')))
 AND (sqlc.arg(q)::text='' OR name ILIKE '%' || sqlc.arg(q) || '%' OR code ILIKE '%' || sqlc.arg(q) || '%')
 ORDER BY sort_order,code LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);
-- name: BillingCountPlans :one
SELECT count(*) FROM billing_plans WHERE (NOT sqlc.arg(public_only)::boolean OR (visibility='public' AND archived_at IS NULL
 AND EXISTS(SELECT 1 FROM billing_plan_versions v WHERE v.plan_id=billing_plans.id AND v.status='published')))
 AND (sqlc.arg(q)::text='' OR name ILIKE '%' || sqlc.arg(q) || '%' OR code ILIKE '%' || sqlc.arg(q) || '%');
-- name: BillingGetPlanID :one
SELECT * FROM billing_plans WHERE id=$1;
-- name: BillingGetPlan :one
SELECT * FROM billing_plans WHERE uuid=$1;
-- name: BillingLockPlan :one
SELECT * FROM billing_plans WHERE uuid=$1 FOR UPDATE;
-- name: BillingCreatePlan :one
INSERT INTO billing_plans(code,name,description,visibility,sort_order) VALUES($1,$2,$3,$4,$5) RETURNING *;
-- name: BillingUpdatePlan :one
UPDATE billing_plans SET name=$2,description=$3,visibility=$4,sort_order=$5 WHERE uuid=$1 RETURNING *;
-- name: BillingArchivePlan :execrows
UPDATE billing_plans SET archived_at=now() WHERE uuid=$1;
-- name: BillingListVersions :many
SELECT * FROM billing_plan_versions WHERE plan_id=$1 ORDER BY version DESC;
-- name: BillingCreateVersion :one
INSERT INTO billing_plan_versions(plan_id,version,trial_days)
 SELECT sqlc.arg(plan_id),COALESCE(max(version),0)+1,sqlc.arg(trial_days) FROM billing_plan_versions WHERE plan_id=sqlc.arg(plan_id) RETURNING *;
-- name: BillingLockVersion :one
SELECT * FROM billing_plan_versions WHERE uuid=$1 FOR UPDATE;
-- name: BillingGetVersion :one
SELECT * FROM billing_plan_versions WHERE uuid=$1;
-- name: BillingUpdateVersion :one
UPDATE billing_plan_versions SET trial_days=$2 WHERE id=$1 AND status='draft' RETURNING *;
-- name: BillingPublishVersion :one
UPDATE billing_plan_versions SET status='published',published_at=now() WHERE id=$1 AND status='draft' RETURNING *;
-- name: BillingListFeatures :many
SELECT * FROM billing_features ORDER BY key;
-- name: BillingListPrices :many
SELECT * FROM billing_prices WHERE plan_version_id=$1 ORDER BY interval,currency;
-- name: BillingListTiers :many
SELECT t.*,f.key AS feature_key FROM billing_price_tiers t JOIN billing_features f ON f.id=t.feature_id WHERE price_id=$1 ORDER BY f.key,lower_bound;
-- name: BillingListPlanEntitlements :many
SELECT e.*,f.key AS feature_key FROM billing_plan_entitlements e JOIN billing_features f ON f.id=e.feature_id WHERE plan_version_id=$1 ORDER BY f.key;
-- name: BillingClearTiers :exec
DELETE FROM billing_price_tiers WHERE price_id IN (SELECT id FROM billing_prices WHERE plan_version_id=$1);
-- name: BillingClearPrices :exec
DELETE FROM billing_prices WHERE plan_version_id=$1;
-- name: BillingClearEntitlements :exec
DELETE FROM billing_plan_entitlements WHERE plan_version_id=$1;
-- name: BillingCreatePrice :one
INSERT INTO billing_prices(plan_version_id,interval,currency,amount_minor,seat_amount_minor,tax_inclusive,vat_basis_points)
 VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING *;
-- name: BillingCreateTier :exec
INSERT INTO billing_price_tiers(price_id,feature_id,lower_bound,upper_bound,unit_amount_minor) VALUES($1,$2,$3,$4,$5);
-- name: BillingCreateEntitlement :exec
INSERT INTO billing_plan_entitlements(plan_version_id,feature_id,kind,value_bool,value_limit,unlimited,enforcement,warn_percent)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8);
-- name: BillingGetPrice :one
SELECT * FROM billing_prices WHERE uuid=$1;
-- name: BillingGetVersionID :one
SELECT * FROM billing_plan_versions WHERE id=$1;
-- name: BillingGetPriceID :one
SELECT * FROM billing_prices WHERE id=$1;
