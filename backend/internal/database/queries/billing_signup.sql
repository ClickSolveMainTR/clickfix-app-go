-- Self-serve signup on a chosen plan (CF-42).

-- name: BillingSignupPrice :one
-- A price of a published, unarchived plan version, with or without a trial.
SELECT p.id, p.uuid, p.plan_version_id, p.interval, v.trial_days
FROM billing_prices p
JOIN billing_plan_versions v ON v.id = p.plan_version_id
JOIN billing_plans b ON b.id = v.plan_id
WHERE p.uuid = $1 AND v.status = 'published' AND b.archived_at IS NULL;
