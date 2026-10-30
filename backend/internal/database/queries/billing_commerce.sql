-- name: BillingSaveProfile :one
INSERT INTO billing_profiles(organization_id,legal_name,tax_id,tax_office,address,email,kind,country_code,emails,export_exemption)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(organization_id) DO UPDATE SET legal_name=excluded.legal_name,tax_id=excluded.tax_id,tax_office=excluded.tax_office,address=excluded.address,email=excluded.email,kind=excluded.kind,country_code=excluded.country_code,emails=excluded.emails,export_exemption=excluded.export_exemption RETURNING *;
-- name: BillingSaveFX :exec
INSERT INTO billing_exchange_rates(currency,rate,observed_on) VALUES($1,$2,$3) ON CONFLICT DO NOTHING;
-- name: BillingLatestFX :one
SELECT * FROM billing_exchange_rates WHERE currency=$1 AND observed_on<=sqlc.arg(at_date)::date AND observed_on>=sqlc.arg(at_date)::date-interval '7 days' ORDER BY observed_on DESC LIMIT 1;
-- name: BillingNextInvoiceNumber :one
INSERT INTO billing_invoice_sequences(series,year,last_value) VALUES($1,$2,1) ON CONFLICT(series,year) DO UPDATE SET last_value=billing_invoice_sequences.last_value+1 RETURNING last_value;
-- name: BillingCreateInvoice :one
INSERT INTO billing_invoices(organization_id,subscription_id,period_start,period_end,kind,idempotency_key,status,source_currency,fx_rate,fx_source,fx_observed_at,buyer_snapshot,seller_snapshot,subtotal_minor,discount_minor,tax_minor,total_minor,due_at,original_invoice_id,request_digest)
VALUES($1,$2,$3,$4,$5,$6,'open',$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19) RETURNING *;
-- name: BillingInvoiceKey :one
SELECT * FROM billing_invoices WHERE organization_id=$1 AND idempotency_key=$2;
-- name: BillingNumberInvoice :one
UPDATE billing_invoices SET number=$3,issued_at=$4 WHERE organization_id=$1 AND id=$2 AND number IS NULL RETURNING *;
-- name: BillingCreateInvoiceLine :exec
INSERT INTO billing_invoice_lines(organization_id,invoice_id,description,quantity,unit_amount_minor,subtotal_minor,discount_minor,vat_basis_points,tax_minor,total_minor)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10);
-- name: BillingInvoiceLines :many
SELECT * FROM billing_invoice_lines WHERE organization_id=$1 AND invoice_id=$2 ORDER BY id;
-- name: BillingSaveDocument :exec
INSERT INTO billing_invoice_documents(organization_id,invoice_id,profile,xml,pdf,html,provider) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(invoice_id) DO NOTHING;
-- name: BillingDocument :one
SELECT * FROM billing_invoice_documents WHERE organization_id=$1 AND invoice_id=$2;
-- name: BillingDocumentState :exec
UPDATE billing_invoice_documents SET state=$3,provider_reference=$4,last_error=$5,updated_at=now() WHERE organization_id=$1 AND invoice_id=$2;
-- name: BillingEInvoiceState :exec
UPDATE billing_invoices SET einvoice_status=$3,einvoice_reference=$4 WHERE organization_id=$1 AND id=$2;
-- name: BillingPaidDocuments :many
SELECT i.* FROM billing_invoices i LEFT JOIN billing_invoice_documents d ON d.organization_id=i.organization_id AND d.invoice_id=i.id
WHERE (i.status='paid' OR i.kind='credit_note') AND (d.id IS NULL OR d.state IN ('prepared','submitted','failed')) AND i.id>sqlc.arg(after_id)::bigint ORDER BY i.id LIMIT 100;
-- name: BillingInvoiceUsage :many
SELECT p.id AS usage_period_id,f.key,COALESCE(sum(t.quantity),0)::bigint AS quantity
FROM billing_usage_settlements t JOIN billing_usage_periods p ON p.organization_id=t.organization_id AND p.id=t.usage_period_id JOIN billing_features f ON f.id=t.feature_id
WHERE t.organization_id=$1 AND p.ends_at<=sqlc.arg(period_end) AND p.closed_at IS NOT NULL
AND NOT EXISTS(SELECT 1 FROM billing_invoice_usage_periods claim WHERE claim.organization_id=p.organization_id AND claim.usage_period_id=p.id)
GROUP BY p.id,f.key ORDER BY p.id,f.key;
-- name: BillingInvoiceSeats :one
SELECT GREATEST(count(*),1)::bigint FROM organization_members WHERE organization_id=$1;
-- name: BillingAddonCatalog :many
SELECT a.*,f.key AS feature FROM billing_addons a JOIN billing_features f ON f.id=a.feature_id WHERE a.archived_at IS NULL ORDER BY a.id LIMIT $1 OFFSET $2;
-- name: BillingSaveAddon :one
INSERT INTO billing_addons(code,name,feature_id,units,amount_minor,currency) SELECT $1,$2,id,$4,$5,$6 FROM billing_features WHERE key=$3 RETURNING *;
-- name: BillingAddon :one
SELECT * FROM billing_addons WHERE uuid=$1 AND archived_at IS NULL;
-- name: BillingSubscriptionItems :many
SELECT i.*,a.name FROM billing_subscription_items i JOIN billing_addons a ON a.id=i.addon_id WHERE i.organization_id=$1 AND i.subscription_id=$2 ORDER BY i.id;
-- name: BillingSetItem :exec
INSERT INTO billing_subscription_items(organization_id,subscription_id,addon_id,quantity,unit_amount_minor,currency) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(subscription_id,addon_id) DO UPDATE SET quantity=excluded.quantity,pending_quantity=NULL;
-- name: BillingScheduleItem :exec
UPDATE billing_subscription_items SET pending_quantity=$4 WHERE organization_id=$1 AND subscription_id=$2 AND addon_id=$3;
-- name: BillingApplyItems :exec
UPDATE billing_subscription_items SET quantity=pending_quantity,pending_quantity=NULL WHERE organization_id=$1 AND subscription_id=$2 AND pending_quantity>0;
-- name: BillingDeleteItems :exec
DELETE FROM billing_subscription_items WHERE organization_id=$1 AND subscription_id=$2 AND pending_quantity=0;
-- name: BillingCommercePrice :one
SELECT p.*,v.plan_id FROM billing_prices p JOIN billing_plan_versions v ON v.id=p.plan_version_id JOIN billing_plans b ON b.id=v.plan_id WHERE p.uuid=$1 AND v.published_at IS NOT NULL AND b.archived_at IS NULL;
-- name: BillingNewChange :one
INSERT INTO billing_plan_changes(organization_id,subscription_id,price_id,invoice_id,idempotency_key,seats,effective_at,state,new_period_end) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING *;
-- name: BillingChangeKey :one
SELECT * FROM billing_plan_changes WHERE organization_id=$1 AND idempotency_key=$2;
-- name: BillingPaidChange :one
SELECT * FROM billing_plan_changes WHERE organization_id=$1 AND invoice_id=$2 AND state='awaiting_payment' FOR UPDATE;
-- name: BillingApplyChange :exec
UPDATE billing_subscriptions SET price_id=$3,plan_version_id=$4,contracted_seats=$5 WHERE organization_id=$1 AND id=$2;
-- name: BillingCompleteChange :exec
UPDATE billing_plan_changes SET state='applied' WHERE organization_id=$1 AND id=$2;
-- name: BillingScheduleChange :exec
UPDATE billing_subscriptions SET pending_price_id=$3,pending_plan_version_id=$4 WHERE organization_id=$1 AND id=$2;
-- name: BillingCouponCatalog :many
SELECT * FROM billing_coupons ORDER BY id DESC LIMIT $1 OFFSET $2;
-- name: BillingNewCoupon :one
INSERT INTO billing_coupons(code,percent_basis_points,amount_minor,currency,starts_at,ends_at,max_redemptions,per_org_limit,duration_months,recurring,plan_id,interval)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING *;
-- name: BillingLockCoupon :one
SELECT * FROM billing_coupons WHERE code=$1 FOR UPDATE;
-- name: BillingCouponUses :one
SELECT count(*)::bigint AS total,count(*) FILTER(WHERE organization_id=$2)::bigint AS org_total FROM billing_redemptions WHERE coupon_id=$1;
-- name: BillingAttachCoupon :exec
INSERT INTO billing_subscription_coupons(organization_id,subscription_id,coupon_id,expires_at) VALUES($1,$2,$3,$4) ON CONFLICT(subscription_id) DO UPDATE SET coupon_id=excluded.coupon_id,attached_at=now(),expires_at=excluded.expires_at;
-- name: BillingAttachedCoupon :one
SELECT c.* FROM billing_subscription_coupons s JOIN billing_coupons c ON c.id=s.coupon_id WHERE s.organization_id=$1 AND s.subscription_id=$2 AND (s.expires_at IS NULL OR s.expires_at>sqlc.arg(at_time)) FOR UPDATE OF c;
-- name: BillingRedeemCoupon :exec
INSERT INTO billing_redemptions(organization_id,coupon_id,invoice_id,discount_minor) VALUES($1,$2,$3,$4);
-- name: BillingWalletBalance :one
SELECT COALESCE(sum(delta),0)::bigint FROM billing_credit_ledger WHERE organization_id=$1 AND unit=$2;
-- name: BillingWalletEntries :many
SELECT * FROM billing_credit_ledger WHERE organization_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: BillingWalletKey :one
SELECT * FROM billing_credit_ledger WHERE organization_id=$1 AND unit=$2 AND idempotency_key=$3;
-- name: BillingWalletEntry :one
INSERT INTO billing_credit_ledger(organization_id,subscription_id,invoice_id,unit,delta,reason,idempotency_key,reverses_id,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING *;
-- name: BillingManualWalletEntry :one
INSERT INTO billing_credit_ledger(organization_id,subscription_id,unit,delta,reason,idempotency_key,expires_at,category,actor_user_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING *;
-- name: BillingExpiringCredits :many
SELECT * FROM billing_credit_ledger l WHERE l.delta>0 AND l.expires_at<=$1 AND NOT EXISTS(SELECT 1 FROM billing_credit_ledger r WHERE r.reverses_id=l.id) ORDER BY l.id LIMIT 1000;
-- name: BillingCreditPackages :many
SELECT * FROM billing_credit_packages WHERE active ORDER BY id;
-- name: BillingCreditPackage :one
SELECT * FROM billing_credit_packages WHERE uuid=$1 AND active;
-- name: BillingNewCreditPackage :one
INSERT INTO billing_credit_packages(code,name,credits,amount_minor,expires_days,unit,description,plan_ids,sort_order) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING *;
-- name: BillingAllCreditPackages :many
SELECT * FROM billing_credit_packages ORDER BY active DESC,unit,sort_order,id;
-- name: BillingCreditPackageAny :one
SELECT * FROM billing_credit_packages WHERE uuid=$1;
-- name: BillingUpdateCreditPackage :one
UPDATE billing_credit_packages SET name=$2,description=$3,credits=$4,amount_minor=$5,expires_days=$6,plan_ids=$7,sort_order=$8,active=$9
WHERE uuid=$1 RETURNING *;
-- name: BillingOrganizationCreditPackages :many
-- Packs an organization may buy: active, and either open to every plan or
-- listing the plan of its live subscription.
SELECT p.* FROM billing_credit_packages p
WHERE p.active AND (cardinality(p.plan_ids)=0 OR EXISTS(
 SELECT 1 FROM billing_subscriptions s JOIN billing_plan_versions v ON v.id=s.plan_version_id
 WHERE s.organization_id=$1 AND s.status IN ('trialing','active','past_due','grace','paused') AND v.plan_id=ANY(p.plan_ids)))
ORDER BY p.unit,p.sort_order,p.id;
-- name: BillingLowAICreditCandidates :many
-- Live subscriptions whose AI credit balance is under the alert threshold
-- (the organization's own, else a fifth of the plan allowance) and that were
-- not alerted in the last day.
SELECT s.organization_id,bal.balance::bigint AS balance FROM billing_subscriptions s
LEFT JOIN billing_wallet_settings w ON w.organization_id=s.organization_id
JOIN LATERAL (SELECT COALESCE(sum(l.delta),0)::bigint AS balance FROM billing_credit_ledger l WHERE l.organization_id=s.organization_id AND l.unit='ai_credits') bal ON true
LEFT JOIN LATERAL (SELECT e.value_limit FROM billing_plan_entitlements e JOIN billing_features f ON f.id=e.feature_id
 WHERE e.plan_version_id=s.plan_version_id AND f.key='ai.credits' AND NOT e.unlimited) lim ON true
WHERE s.status IN ('trialing','active','past_due','grace')
 AND bal.balance < COALESCE(w.alert_threshold,COALESCE(lim.value_limit,0)/5)
 AND (w.low_balance_notified_at IS NULL OR w.low_balance_notified_at<now()-interval '24 hours')
ORDER BY s.organization_id LIMIT 500;
-- name: BillingMarkLowBalance :execrows
INSERT INTO billing_wallet_settings(organization_id,low_balance_notified_at) VALUES($1,now())
ON CONFLICT(organization_id) DO UPDATE SET low_balance_notified_at=now()
WHERE billing_wallet_settings.low_balance_notified_at IS NULL OR billing_wallet_settings.low_balance_notified_at<now()-interval '24 hours';
-- name: BillingSetAlertThreshold :exec
INSERT INTO billing_wallet_settings(organization_id,alert_threshold) VALUES($1,$2)
ON CONFLICT(organization_id) DO UPDATE SET alert_threshold=excluded.alert_threshold;
-- name: BillingWalletSettings :one
SELECT s.*,p.uuid AS package_uuid FROM billing_wallet_settings s LEFT JOIN billing_credit_packages p ON p.id=s.package_id WHERE s.organization_id=$1;
-- name: BillingNewPurchase :exec
INSERT INTO billing_credit_purchases(organization_id,invoice_id,package_id,credits,expires_days,unit) VALUES($1,$2,$3,$4,$5,$6);
-- name: BillingPurchase :one
SELECT * FROM billing_credit_purchases WHERE organization_id=$1 AND invoice_id=$2 FOR UPDATE;
-- name: BillingGrantPurchase :exec
UPDATE billing_credit_purchases SET granted_at=now() WHERE organization_id=$1 AND invoice_id=$2;
-- name: BillingSaveWalletSettings :exec
INSERT INTO billing_wallet_settings(organization_id,auto_topup,threshold,package_id,consented_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(organization_id) DO UPDATE SET auto_topup=excluded.auto_topup,threshold=excluded.threshold,package_id=excluded.package_id,consented_at=excluded.consented_at;
-- name: BillingTopupCandidates :many
SELECT s.organization_id,p.uuid AS package_uuid FROM billing_wallet_settings s JOIN billing_credit_packages p ON p.id=s.package_id
WHERE s.auto_topup AND p.active AND COALESCE((SELECT sum(delta) FROM billing_credit_ledger WHERE organization_id=s.organization_id AND unit=p.unit),0)<s.threshold LIMIT 100;
-- name: BillingSaveStatement :one
INSERT INTO billing_provider_statement(provider,merchant_account_uuid,external_id,order_reference,operation,status,amount_minor,currency,observed_at,provider_reference)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(provider,merchant_account_uuid,external_id) DO UPDATE SET external_id=billing_provider_statement.external_id RETURNING *;
-- name: BillingMatchStatement :exec
UPDATE billing_provider_statement SET organization_id=$2,payment_id=$3,mismatch=$4 WHERE id=$1;
-- name: BillingStatementReport :many
SELECT * FROM billing_provider_statement WHERE mismatch<>'' ORDER BY id DESC LIMIT $1 OFFSET $2;
-- name: BillingRefundNote :exec
INSERT INTO billing_refund_notes(organization_id,payment_id,invoice_id) VALUES($1,$2,$3) ON CONFLICT(payment_id) DO NOTHING;
-- name: BillingRefundsWithoutNotes :many
SELECT p.* FROM billing_payments p WHERE p.kind='refund' AND p.status='succeeded' AND NOT EXISTS(SELECT 1 FROM billing_refund_notes n WHERE n.payment_id=p.id) ORDER BY p.id LIMIT 100;
-- name: BillingBillingRecipients :many
SELECT u.id,u.locale FROM users u JOIN organization_members m ON m.user_id=u.id JOIN roles r ON r.id=m.role_id WHERE m.organization_id=$1 AND r.slug IN ('owner','admin');
-- name: BillingNewItemChange :exec
INSERT INTO billing_item_changes(organization_id,invoice_id,subscription_id,addon_id,quantity,unit_amount_minor,currency) VALUES($1,$2,$3,$4,$5,$6,$7);
-- name: BillingPaidItemChange :one
SELECT * FROM billing_item_changes WHERE organization_id=$1 AND invoice_id=$2 AND applied_at IS NULL FOR UPDATE;
-- name: BillingCompleteItemChange :exec
UPDATE billing_item_changes SET applied_at=now() WHERE organization_id=$1 AND invoice_id=$2;
-- name: BillingScheduledChange :one
SELECT * FROM billing_plan_changes WHERE organization_id=$1 AND subscription_id=$2 AND state='scheduled' FOR UPDATE;
-- name: BillingInvoiceChange :one
SELECT * FROM billing_plan_changes WHERE organization_id=$1 AND invoice_id=$2;
-- name: BillingTargetLimits :many
SELECT f.key,e.value_limit,e.unlimited FROM billing_plan_entitlements e JOIN billing_features f ON f.id=e.feature_id WHERE e.plan_version_id=$1 AND f.kind='limit';
-- name: BillingSetContractedSeats :exec
UPDATE billing_subscriptions SET contracted_seats=$3 WHERE organization_id=$1 AND id=$2;
-- name: BillingCreditLots :many
SELECT l.*, (l.delta-COALESCE((SELECT sum(a.amount) FROM billing_credit_allocations a WHERE a.organization_id=l.organization_id AND a.credit_id=l.id),0))::bigint AS remaining
FROM billing_credit_ledger l WHERE l.organization_id=$1 AND l.unit=$2 AND l.delta>0 AND NOT EXISTS(SELECT 1 FROM billing_credit_ledger r WHERE r.organization_id=l.organization_id AND r.reverses_id=l.id)
ORDER BY l.expires_at NULLS LAST,l.id;
-- name: BillingAllocateCredits :exec
INSERT INTO billing_credit_allocations(organization_id,debit_id,credit_id,amount) VALUES($1,$2,$3,$4);
-- name: BillingUnsettledWalletInvoice :one
SELECT i.* FROM billing_invoices i WHERE i.organization_id=$1 AND i.kind='wallet' AND i.status='open' ORDER BY i.id LIMIT 1;
-- name: BillingAmbiguousRefunds :many
SELECT * FROM billing_payments WHERE kind='refund' AND status IN ('pending','unknown') AND created_at<now()-interval '2 minutes' AND id>sqlc.arg(after_id)::bigint ORDER BY id LIMIT 100;
-- name: BillingStatementPending :many
SELECT * FROM billing_provider_statement WHERE (payment_id IS NULL OR mismatch<>'') AND id>sqlc.arg(after_id)::bigint ORDER BY id LIMIT 1000;
-- name: BillingStatementPayment :one
SELECT * FROM billing_payments WHERE provider=$1 AND merchant_account_uuid=$2 AND order_reference=$3;
-- name: BillingAllPaymentRefunds :many
SELECT * FROM billing_payments WHERE organization_id=$1 AND original_payment_id=$2 ORDER BY id;
-- name: BillingReferralCode :one
INSERT INTO billing_referral_codes(organization_id) VALUES($1) ON CONFLICT(organization_id) DO UPDATE SET organization_id=excluded.organization_id RETURNING code;
-- name: BillingReferralOwner :one
SELECT organization_id FROM billing_referral_codes WHERE code=$1;
-- name: BillingNewReferral :exec
INSERT INTO billing_referrals(organization_id,inviter_organization_id,reward_credits) VALUES($1,$2,$3) ON CONFLICT(organization_id) DO NOTHING;
-- name: BillingReferral :one
SELECT * FROM billing_referrals WHERE organization_id=$1;
-- name: BillingAwardReferral :execrows
UPDATE billing_referrals SET awarded_at=now(),invoice_id=$2 WHERE organization_id=$1 AND awarded_at IS NULL;
-- name: BillingPaidInvoiceCount :one
SELECT count(*)::bigint FROM billing_invoices WHERE billing_invoices.organization_id=$1 AND status='paid' AND kind='renewal';
-- name: BillingCommerceCounts :one
SELECT
 (SELECT count(*) FROM billing_addons WHERE archived_at IS NULL)::bigint AS addons,
 (SELECT count(*) FROM billing_coupons)::bigint AS coupons,
 (SELECT count(*) FROM billing_provider_statement WHERE mismatch<>'')::bigint AS statements,
 (SELECT count(*) FROM billing_credit_ledger l WHERE l.organization_id=sqlc.arg(organization_id)::bigint)::bigint AS ledger;
-- name: BillingPendingReferralAwards :many
SELECT i.* FROM billing_referrals r JOIN billing_invoices i ON i.organization_id=r.organization_id
WHERE r.awarded_at IS NULL AND i.kind='renewal' AND i.status='paid' ORDER BY i.id LIMIT 100;

-- name: BillingChangePeriod :exec
UPDATE billing_subscriptions SET period_start=$3,period_end=$4 WHERE organization_id=$1 AND id=$2;
-- name: BillingPlatformRecipients :many
SELECT DISTINCT u.id,u.locale FROM users u JOIN user_roles ur ON ur.user_id=u.id JOIN roles r ON r.id=ur.role_id WHERE r.scope='platform' AND r.slug='super_admin';

-- name: BillingExpiredWalletOrganizations :many
SELECT DISTINCT organization_id FROM billing_credit_ledger l WHERE unit IN ('ai_credits','video_minutes') AND delta>COALESCE((SELECT sum(a.amount) FROM billing_credit_allocations a WHERE a.organization_id=l.organization_id AND a.credit_id=l.id),0) AND expires_at<=sqlc.arg(at_time)::timestamptz AND NOT EXISTS(SELECT 1 FROM billing_credit_ledger r WHERE r.reverses_id=l.id) LIMIT 1000;
-- name: BillingInvoiceRefundTotal :one
SELECT COALESCE(sum(amount_minor),0)::bigint FROM billing_payments WHERE organization_id=$1 AND invoice_id=$2 AND kind='refund' AND status IN ('pending','unknown','succeeded');
-- name: BillingReservedRefundCredits :one
SELECT COALESCE(sum(r.credits),0)::bigint FROM billing_refund_credit_reservations r JOIN billing_payments p ON p.id=r.payment_id AND p.organization_id=r.organization_id WHERE p.organization_id=$1 AND p.invoice_id=$2 AND r.released_at IS NULL;
-- name: BillingReserveRefundCredits :exec
INSERT INTO billing_refund_credit_reservations(organization_id,payment_id,debit_id,credits) VALUES($1,$2,$3,$4);
-- name: BillingRefundCreditReservation :one
SELECT r.*,l.subscription_id,l.expires_at FROM billing_refund_credit_reservations r JOIN billing_credit_ledger l ON l.id=r.debit_id AND l.organization_id=r.organization_id WHERE r.organization_id=$1 AND r.payment_id=$2 FOR UPDATE OF r;
-- name: BillingReleaseRefundCredits :exec
UPDATE billing_refund_credit_reservations SET released_at=now() WHERE organization_id=$1 AND payment_id=$2;
-- name: BillingItemChangeInvoice :one
SELECT * FROM billing_item_changes WHERE organization_id=$1 AND invoice_id=$2;

-- name: BillingClaimInvoiceUsage :exec
INSERT INTO billing_invoice_usage_periods(organization_id,usage_period_id,invoice_id) VALUES($1,$2,$3);
