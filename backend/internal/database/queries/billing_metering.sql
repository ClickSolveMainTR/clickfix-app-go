-- name: BillingUsagePeriod :one
SELECT * FROM billing_usage_periods WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: BillingCloseUsagePeriod :exec
UPDATE billing_usage_periods SET closed_at=now() WHERE organization_id=$1 AND id=$2 AND closed_at IS NULL;
-- name: BillingSettleUsage :many
INSERT INTO billing_usage_settlements(organization_id,usage_period_id,feature_id,quantity)
 SELECT t.organization_id,t.usage_period_id,t.feature_id,t.quantity FROM billing_usage_totals t
 WHERE t.organization_id=$1 AND t.usage_period_id=$2
 ON CONFLICT DO NOTHING RETURNING *;
-- name: BillingUsageDaily :many
SELECT f.key, to_char(r.occurred_at AT TIME ZONE 'Europe/Istanbul','YYYY-MM-DD')::text AS day,
 CASE WHEN f.aggregation='gauge' THEN max(r.quantity) ELSE sum(r.quantity) END::bigint AS quantity
 FROM billing_usage_records r JOIN billing_features f ON f.id=r.feature_id
 WHERE r.organization_id=$1 AND r.occurred_at>=sqlc.arg(starts_at)::timestamptz AND r.occurred_at<sqlc.arg(ends_at)::timestamptz
 GROUP BY f.key,f.aggregation,day ORDER BY day,f.key;
-- name: BillingUsageSummary :many
SELECT p.uuid AS period_uuid,p.starts_at,p.ends_at,p.closed_at,f.key,f.aggregation,t.quantity
 FROM billing_usage_periods p JOIN billing_usage_totals t ON t.usage_period_id=p.id AND t.organization_id=p.organization_id
 JOIN billing_features f ON f.id=t.feature_id WHERE p.organization_id=$1 ORDER BY p.starts_at DESC,f.key LIMIT 500;
-- name: BillingRecordUsageAt :exec
INSERT INTO billing_usage_records(organization_id,usage_period_id,feature_id,idempotency_key,quantity,occurred_at)
 VALUES($1,$2,$3,$4,$5,$6);
-- name: BillingUsageInPeriod :one
SELECT COALESCE(sum(quantity),0)::bigint AS quantity FROM billing_usage_totals
 WHERE organization_id=$1 AND usage_period_id=$2 AND feature_id=$3;
-- name: BillingSetGauge :execrows
INSERT INTO billing_usage_totals(organization_id,usage_period_id,feature_id,quantity,last_occurred_at) VALUES($1,$2,$3,$4,$5)
 ON CONFLICT(organization_id,usage_period_id,feature_id) DO UPDATE SET quantity=excluded.quantity,last_occurred_at=excluded.last_occurred_at
 WHERE billing_usage_totals.last_occurred_at IS NULL OR billing_usage_totals.last_occurred_at<=excluded.last_occurred_at;
-- name: BillingUsageDenial :execrows
INSERT INTO billing_usage_denials(organization_id,event_key,feature,quantity,code) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING;
-- name: BillingDueUsagePeriods :many
SELECT organization_id,uuid FROM billing_usage_periods WHERE closed_at IS NULL AND ends_at<=sqlc.arg(at_time)::timestamptz ORDER BY organization_id,id LIMIT 100;
-- name: BillingLiveGauges :many
-- Current counts of the gauge features nothing meters by event: the usage
-- view reads them live so seats and channels never show 0 while they exist.
SELECT g.key::text AS key, g.quantity::bigint AS quantity FROM (
 SELECT 'seats' AS key, count(*) AS quantity FROM organization_members WHERE organization_id=sqlc.arg(organization_id)::bigint
 UNION ALL SELECT 'brands', count(*) FROM brands WHERE organization_id=sqlc.arg(organization_id)::bigint
 UNION ALL SELECT 'channels', count(*) FROM channel_instances WHERE organization_id=sqlc.arg(organization_id)::bigint AND deleted_at IS NULL
 UNION ALL SELECT 'channels.email', count(*) FROM channel_instances WHERE organization_id=sqlc.arg(organization_id)::bigint AND deleted_at IS NULL AND channel_type='email'
 UNION ALL SELECT 'channels.whatsapp', count(*) FROM channel_instances WHERE organization_id=sqlc.arg(organization_id)::bigint AND deleted_at IS NULL AND channel_type='whatsapp'
 UNION ALL SELECT 'contacts', count(*) FROM contacts WHERE organization_id=sqlc.arg(organization_id)::bigint AND deleted_at IS NULL
 UNION ALL SELECT 'storage.bytes', COALESCE(sum(size_bytes),0) FROM attachments WHERE organization_id=sqlc.arg(organization_id)::bigint AND deleted_at IS NULL
) g;
