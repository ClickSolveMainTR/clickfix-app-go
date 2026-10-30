-- name: BillingTrialEnrolled :one
SELECT EXISTS(SELECT 1 FROM billing_trial_enrollments WHERE organization_id=$1);
-- name: BillingEnrollTrial :exec
INSERT INTO billing_trial_enrollments(organization_id,subscription_id) VALUES($1,$2);
-- name: BillingGatewaySettings :one
SELECT * FROM billing_gateway_settings WHERE id=true;
-- name: BillingSaveGatewaySettings :exec
UPDATE billing_gateway_settings SET default_provider=$1,currency_providers=$2,trial_price_uuid=$3 WHERE id=true;
-- name: BillingTrialPrice :one
SELECT p.*,v.trial_days,v.uuid AS version_uuid FROM billing_prices p
 JOIN billing_plan_versions v ON v.id=p.plan_version_id JOIN billing_plans b ON b.id=v.plan_id
 WHERE p.uuid=$1 AND v.status='published' AND b.archived_at IS NULL AND v.trial_days>0;
-- name: BillingListTrials :many
SELECT s.* FROM billing_subscriptions s WHERE s.status='trialing'
 AND s.trial_ends_at<=sqlc.arg(at_time)::timestamptz+interval '7 days'
 AND (s.trial_ends_at<=sqlc.arg(at_time)::timestamptz OR NOT EXISTS(
  SELECT 1 FROM billing_trial_reminders r WHERE r.organization_id=s.organization_id AND r.subscription_id=s.id
   AND r.trial_ends_at=s.trial_ends_at AND r.days=CASE
    WHEN s.trial_ends_at<=sqlc.arg(at_time)::timestamptz+interval '1 day' THEN 1
    WHEN s.trial_ends_at<=sqlc.arg(at_time)::timestamptz+interval '3 days' THEN 3 ELSE 7 END))
 ORDER BY s.trial_ends_at,s.organization_id LIMIT 1000;
-- name: BillingTrialReminder :execrows
INSERT INTO billing_trial_reminders(organization_id,subscription_id,trial_ends_at,days)
 VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING;
-- name: BillingExtendTrial :one
UPDATE billing_subscriptions SET trial_ends_at=$3,period_end=GREATEST(period_end,$3)
 WHERE organization_id=$1 AND id=$2 AND status='trialing' RETURNING *;
-- name: BillingTrialRecipients :many
SELECT m.user_id FROM organization_members m JOIN roles r ON r.id=m.role_id
 WHERE m.organization_id=$1 AND r.slug IN ('owner','admin');
-- name: BillingPaymentMethod :one
SELECT * FROM billing_payment_methods WHERE organization_id=$1 AND id=$2 AND status='active';
-- name: BillingAttempt :one
SELECT * FROM billing_payment_attempts WHERE organization_id=$1 AND idempotency_key=$2 FOR UPDATE;
-- name: BillingCreateAttempt :one
INSERT INTO billing_payment_attempts(organization_id,idempotency_key,provider,operation,request_hash)
 VALUES($1,$2,$3,$4,$5) RETURNING *;
-- name: BillingFinishAttempt :exec
UPDATE billing_payment_attempts SET status=$3,result=$4,failure_code=$5 WHERE organization_id=$1 AND id=$2;
-- name: BillingDefaultTrialPrice :one
SELECT p.uuid FROM billing_prices p JOIN billing_plan_versions v ON v.id=p.plan_version_id
 JOIN billing_plans b ON b.id=v.plan_id
 WHERE v.status='published' AND v.trial_days>0 AND b.visibility='public' AND b.archived_at IS NULL
 ORDER BY b.sort_order,b.code,v.version DESC,p.interval,p.currency LIMIT 1;
