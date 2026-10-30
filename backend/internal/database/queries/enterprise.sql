-- name: QuoteCreate :one
INSERT INTO enterprise_quotes(organization_id,base_version_id,title,currency,interval,amount_minor,vat_basis_points,lines,grants,contract_months,payment_terms_days,po_number,terms,valid_until,created_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING *;
-- name: QuoteList :many
SELECT q.uuid,o.uuid AS organization_uuid,q.title,q.status,q.currency,q.amount_minor,q.interval,q.valid_until,q.created_at FROM enterprise_quotes q JOIN organizations o ON o.id=q.organization_id WHERE q.organization_id=sqlc.arg(organization_id) ORDER BY q.id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: QuoteLock :one
SELECT * FROM enterprise_quotes WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: QuotePublic :one
SELECT q.* FROM enterprise_quotes q JOIN organizations o ON o.id=q.organization_id WHERE q.share_token=$1 AND q.status IN ('sent','accepted','rejected','converted') AND o.deleted_at IS NULL AND o.status='active';
-- name: QuotePublicLock :one
SELECT q.* FROM enterprise_quotes q JOIN organizations o ON o.id=q.organization_id WHERE q.share_token=$1 AND q.status IN ('sent','accepted','rejected') AND o.deleted_at IS NULL AND o.status='active' FOR UPDATE OF q;
-- name: QuoteSend :one
UPDATE enterprise_quotes SET status='sent',sent_at=now() WHERE organization_id=$1 AND uuid=$2 AND status='draft' AND valid_until>now() RETURNING *;
-- name: QuoteDecide :one
UPDATE enterprise_quotes SET status=$3,decided_at=now(),decision_ip=$4 WHERE organization_id=$1 AND uuid=$2 AND status='sent' AND valid_until>now() RETURNING *;
-- name: QuoteConvert :one
UPDATE enterprise_quotes SET status='converted',subscription_id=$3,requires_initial_invoice=$4,initial_period_start=$5,initial_period_end=$6 WHERE organization_id=$1 AND uuid=$2 AND status='accepted' AND subscription_id IS NULL RETURNING *;
-- name: QuoteCopyEntitlements :exec
INSERT INTO billing_plan_entitlements(plan_version_id,feature_id,kind,value_bool,value_limit,unlimited,enforcement,warn_percent)
SELECT $2,e.feature_id,e.kind,e.value_bool,e.value_limit,e.unlimited,e.enforcement,e.warn_percent FROM billing_plan_entitlements e WHERE e.plan_version_id=$1;
-- name: QuoteOverrideEntitlement :execrows
UPDATE billing_plan_entitlements SET value_bool=$3,value_limit=$4,unlimited=$5 WHERE plan_version_id=$1 AND feature_id=$2;
-- name: QuoteCopyTiers :exec
INSERT INTO billing_price_tiers(price_id,feature_id,lower_bound,upper_bound,unit_amount_minor)
SELECT $2,t.feature_id,t.lower_bound,t.upper_bound,t.unit_amount_minor FROM billing_price_tiers t WHERE t.price_id=$1;
-- name: QuoteAttachTerms :exec
UPDATE billing_profiles SET payment_terms_days=$2 WHERE organization_id=$1;
-- name: QuoteCount :one
SELECT count(*) FROM enterprise_quotes WHERE organization_id=$1;

-- name: QuoteLinkInitialInvoice :one
UPDATE enterprise_quotes SET initial_invoice_id=$3 WHERE organization_id=$1 AND uuid=$2 AND requires_initial_invoice AND (initial_invoice_id IS NULL OR initial_invoice_id=$3) RETURNING *;
