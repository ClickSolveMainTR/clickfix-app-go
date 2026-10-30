-- name: BillingInvoice :one
SELECT * FROM billing_invoices WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: BillingInvoiceID :one
SELECT * FROM billing_invoices WHERE organization_id=$1 AND id=$2 FOR UPDATE;
-- name: BillingSubscriptionID :one
SELECT * FROM billing_subscriptions WHERE organization_id=$1 AND id=$2 FOR UPDATE;
-- name: BillingInvoices :many
SELECT * FROM billing_invoices WHERE organization_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: BillingNewPayment :one
INSERT INTO billing_payments(organization_id,invoice_id,provider,merchant_account_uuid,kind,idempotency_key,amount_minor,payment_method_id,order_reference)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING *;
-- name: BillingPaymentKey :one
SELECT * FROM billing_payments WHERE organization_id=$1 AND idempotency_key=$2 FOR UPDATE;
-- name: BillingPaymentID :one
SELECT * FROM billing_payments WHERE organization_id=$1 AND id=$2 FOR UPDATE;
-- name: BillingPaymentOrder :one
SELECT * FROM billing_payments WHERE provider=$1 AND order_reference=$2;
-- name: BillingSetPayment :one
UPDATE billing_payments SET status=$3,provider_reference=$4,failure_code=$5,completed_at=$6 WHERE organization_id=$1 AND id=$2 RETURNING *;
-- name: BillingNewCheckoutSession :exec
INSERT INTO billing_checkout_sessions(organization_id,payment_id,store_card,auto_pay_consent) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING;
-- name: BillingSetCheckoutToken :exec
UPDATE billing_checkout_sessions SET token=$3 WHERE organization_id=$1 AND payment_id=$2;
-- name: BillingCheckoutSession :one
SELECT * FROM billing_checkout_sessions WHERE organization_id=$1 AND payment_id=$2;
-- name: BillingWebhookReceipt :execrows
INSERT INTO billing_webhook_receipts(organization_id,payment_id,provider,event_key,body_enc,payload_digest,signature)
VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING;
-- name: BillingGatewayFact :exec
INSERT INTO billing_gateway_transactions(organization_id,payment_id,event_key,operation,verified,payload_digest,result_code)
VALUES($1,$2,$3,$4,true,$5,$6) ON CONFLICT DO NOTHING;
-- name: BillingPayInvoice :exec
UPDATE billing_invoices SET status='paid',paid_at=now() WHERE organization_id=$1 AND id=$2 AND status='open';
-- name: BillingPaidTotal :one
SELECT COALESCE(sum(amount_minor),0)::bigint AS paid FROM billing_payments WHERE organization_id=$1 AND invoice_id=$2 AND status='succeeded' AND kind<>'refund';
-- name: BillingResolveDunning :exec
UPDATE billing_dunning SET resolved_at=now(),next_attempt_at=NULL WHERE organization_id=$1 AND invoice_id=$2 AND resolved_at IS NULL;
-- name: BillingCreditOverpayment :exec
INSERT INTO billing_credit_ledger(organization_id,subscription_id,invoice_id,unit,delta,reason,idempotency_key)
VALUES($1,$2,$3,'TRY',$4,'Invoice overpayment',$5) ON CONFLICT DO NOTHING;
-- name: BillingSaveCard :one
INSERT INTO billing_payment_methods(uuid,organization_id,provider,merchant_account_uuid,customer_token_enc,card_token_enc,token_hash,last4,brand)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT(organization_id,merchant_account_uuid,token_hash) WHERE status='active' DO UPDATE SET last4=excluded.last4,brand=excluded.brand RETURNING *;
-- name: BillingUseCard :exec
UPDATE billing_subscriptions SET payment_method_id=$3,auto_pay=$4,auto_pay_consented_at=CASE WHEN $4::boolean THEN now() ELSE NULL END,renewal_enabled=true WHERE organization_id=$1 AND id=$2;
-- name: BillingCards :many
SELECT uuid,provider,last4,brand,status FROM billing_payment_methods WHERE organization_id=$1 AND status='active' ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: BillingCardUUID :one
SELECT * FROM billing_payment_methods WHERE organization_id=$1 AND uuid=$2 AND status='active';
-- name: BillingRemoveCard :exec
UPDATE billing_payment_methods SET status='removed',removed_at=now() WHERE organization_id=$1 AND id=$2;
-- name: BillingDisableCardAutoPay :exec
UPDATE billing_subscriptions SET auto_pay=false,auto_pay_consented_at=NULL,payment_method_id=NULL WHERE organization_id=$1 AND payment_method_id=$2;
-- name: BillingBankAccounts :many
SELECT * FROM billing_bank_accounts WHERE active OR sqlc.arg(include_inactive)::boolean ORDER BY id LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: BillingSaveBankAccount :one
INSERT INTO billing_bank_accounts(uuid,bank_name,account_name,iban,currency,active,branch,description) VALUES($1,$2,$3,$4,'TRY',$5,$6,$7)
ON CONFLICT(uuid) DO UPDATE SET bank_name=excluded.bank_name,account_name=excluded.account_name,iban=excluded.iban,active=excluded.active,branch=excluded.branch,description=excluded.description RETURNING *;
-- name: BillingBankAccount :one
SELECT * FROM billing_bank_accounts WHERE uuid=$1 AND active;
-- name: BillingReceiptAttachment :one
SELECT * FROM attachments WHERE organization_id=$1 AND uuid=$2 AND deleted_at IS NULL AND scan_status IN ('clean','skipped') FOR UPDATE;
-- name: BillingNewTransfer :one
INSERT INTO billing_bank_transfers(organization_id,invoice_id,bank_account_id,amount_minor,receipt_object_key,attachment_uuid,note)
VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING *;
-- name: BillingLinkReceipt :exec
UPDATE attachments SET owner_type='billing_invoice',owner_id=$3 WHERE organization_id=$1 AND uuid=$2;
-- name: BillingTransfer :one
SELECT * FROM billing_bank_transfers WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: BillingTransfers :many
SELECT t.uuid,o.uuid AS organization_uuid,i.uuid AS invoice_uuid,i.payment_reference,t.amount_minor,t.status,t.attachment_uuid,t.note,t.created_at
FROM billing_bank_transfers t JOIN organizations o ON o.id=t.organization_id JOIN billing_invoices i ON i.organization_id=t.organization_id AND i.id=t.invoice_id
WHERE (sqlc.arg(organization_id)::bigint=0 OR t.organization_id=sqlc.arg(organization_id)::bigint)
ORDER BY t.id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: BillingReviewTransfer :exec
UPDATE billing_bank_transfers SET status=$3,reviewed_by=$4,reviewed_at=now(),rejection_reason=$5,payment_id=$6 WHERE organization_id=$1 AND id=$2;
-- name: BillingRenewalCandidates :many
SELECT * FROM billing_subscriptions WHERE renewal_enabled AND status='active' AND NOT cancel_at_period_end AND period_end<=$1 ORDER BY period_end LIMIT 100;
-- name: BillingRenewalInvoice :one
INSERT INTO billing_invoices(organization_id,subscription_id,period_start,period_end,idempotency_key,status,source_currency,fx_rate,fx_source,fx_observed_at,buyer_snapshot,seller_snapshot,subtotal_minor,tax_minor,total_minor,due_at)
VALUES($1,$2,$3,$4,$5,'open','TRY',1,'native',now(),$6,$7,$8,$9,$10,$11)
ON CONFLICT(subscription_id,period_start) WHERE kind='renewal' DO UPDATE SET idempotency_key=billing_invoices.idempotency_key RETURNING *;
-- name: BillingDueInvoices :many
SELECT i.* FROM billing_invoices i JOIN billing_subscriptions s ON s.organization_id=i.organization_id AND s.id=i.subscription_id
LEFT JOIN billing_dunning d ON d.organization_id=i.organization_id AND d.invoice_id=i.id
WHERE i.status='open' AND i.kind<>'credit_note' AND i.due_at<=sqlc.arg(at_time)::timestamptz AND s.renewal_enabled AND NOT s.cancel_at_period_end
 AND s.status IN ('active','past_due','grace','suspended')
 AND (d.id IS NULL OR (d.resolved_at IS NULL AND d.next_attempt_at<=sqlc.arg(at_time)::timestamptz AND d.grace_ends_at>sqlc.arg(at_time)::timestamptz))
ORDER BY i.due_at LIMIT 100;
-- name: BillingDunning :one
SELECT * FROM billing_dunning WHERE organization_id=$1 AND invoice_id=$2 FOR UPDATE;
-- name: BillingBeginDunning :one
INSERT INTO billing_dunning(organization_id,subscription_id,invoice_id,first_failure_at,next_attempt_at,grace_ends_at)
VALUES($1,$2,$3,$4,$4,$5) ON CONFLICT(invoice_id) DO UPDATE SET invoice_id=billing_dunning.invoice_id RETURNING *;
-- name: BillingScheduleDunning :exec
UPDATE billing_dunning SET attempts=$3,next_attempt_at=$4,last_error_code=$5 WHERE organization_id=$1 AND invoice_id=$2;
-- name: BillingUnresolvedPayment :one
SELECT * FROM billing_payments WHERE organization_id=$1 AND invoice_id=$2 AND status IN ('pending','unknown') AND kind<>'refund' ORDER BY id LIMIT 1;
-- name: BillingReconcileCandidates :many
SELECT * FROM billing_payments WHERE provider IN ('paytr','iyzico','fake') AND kind IN ('checkout','recurring')
AND (status IN ('pending','unknown') OR (sqlc.arg(include_settled)::boolean AND status='succeeded' AND created_at>now()-interval '2 days')) AND created_at<now()-interval '2 minutes' AND id>sqlc.arg(after_id)::bigint
ORDER BY id LIMIT 1000;
-- name: BillingReconciliationRun :one
INSERT INTO billing_reconciliation_runs(run_date) VALUES($1) ON CONFLICT(run_date) DO UPDATE SET run_date=excluded.run_date RETURNING *;
-- name: BillingReconciliationMismatch :execrows
INSERT INTO billing_reconciliation_mismatches(organization_id,payment_id,run_id,reason) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING;
-- name: BillingCompleteReconciliation :exec
UPDATE billing_reconciliation_runs SET completed_at=now() WHERE id=$1;
-- name: BillingRenewalReminderCandidates :many
SELECT s.* FROM billing_subscriptions s JOIN billing_prices p ON p.id=s.price_id
WHERE s.renewal_enabled AND s.status='active' AND NOT s.cancel_at_period_end AND p.interval='yearly' AND s.period_end>sqlc.arg(at_time)::timestamptz AND s.period_end<=sqlc.arg(at_time)::timestamptz+interval '30 days' LIMIT 1000;
-- name: BillingRenewalReminder :execrows
INSERT INTO billing_renewal_reminders(organization_id,subscription_id,period_end,days) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING;
-- name: BillingPreviousInvoice :one
SELECT * FROM billing_invoices WHERE organization_id=$1 AND subscription_id=$2 AND status='paid' ORDER BY period_start DESC LIMIT 1;
-- name: BillingProfile :one
SELECT * FROM billing_profiles WHERE organization_id=$1;
-- name: BillingAnyPaymentMethod :one
SELECT * FROM billing_payment_methods WHERE organization_id=$1 AND id=$2;
-- name: BillingActiveCards :many
SELECT * FROM billing_payment_methods WHERE organization_id=$1 AND provider=$2 AND merchant_account_uuid=$3 AND status='active' ORDER BY id;
-- name: BillingEnableRenewal :exec
UPDATE billing_subscriptions SET renewal_enabled=$3 WHERE organization_id=$1 AND id=$2;
-- name: BillingNewRefund :one
INSERT INTO billing_payments(organization_id,invoice_id,provider,merchant_account_uuid,kind,original_payment_id,idempotency_key,amount_minor,order_reference,refund_destination,refund_cancel)
VALUES($1,$2,$3,$4,'refund',$5,$6,$7,$8,$9,$10) RETURNING *;
-- name: BillingRefundTotal :one
SELECT COALESCE(sum(amount_minor),0)::bigint FROM billing_payments WHERE organization_id=$1 AND original_payment_id=$2 AND status IN ('succeeded','pending','unknown');
-- name: BillingReconciliationReport :many
SELECT m.uuid,m.reason,m.created_at,p.uuid AS payment_uuid,p.provider,p.amount_minor FROM billing_reconciliation_mismatches m
JOIN billing_payments p ON p.organization_id=m.organization_id AND p.id=m.payment_id WHERE m.organization_id=$1 ORDER BY m.id DESC LIMIT $2 OFFSET $3;
-- name: BillingPaymentUUID :one
SELECT * FROM billing_payments WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: BillingPaymentNotice :execrows
INSERT INTO billing_payment_notices(organization_id,notice_key) VALUES($1,$2) ON CONFLICT DO NOTHING;

-- name: BillingPaymentListCounts :one
SELECT
 (SELECT count(*) FROM billing_invoices WHERE organization_id=sqlc.arg(organization_id)::bigint)::bigint AS invoices,
 (SELECT count(*) FROM billing_payment_methods WHERE organization_id=sqlc.arg(organization_id)::bigint AND status='active')::bigint AS cards,
 (SELECT count(*) FROM billing_bank_accounts WHERE active OR sqlc.arg(include_inactive)::boolean)::bigint AS bank_accounts,
 (SELECT count(*) FROM billing_bank_transfers WHERE sqlc.arg(organization_id)::bigint=0 OR organization_id=sqlc.arg(organization_id)::bigint)::bigint AS transfers,
 (SELECT count(*) FROM billing_reconciliation_mismatches WHERE organization_id=sqlc.arg(organization_id)::bigint)::bigint AS mismatches;
-- name: BillingOpenTrialInvoice :one
SELECT * FROM billing_invoices WHERE organization_id=$1 AND subscription_id=$2 AND period_start=$3 AND status IN ('open','paid') ORDER BY id DESC LIMIT 1;
-- name: BillingLockUnresolvedPayment :one
SELECT * FROM billing_payments WHERE organization_id=$1 AND invoice_id=$2 AND status IN ('pending','unknown') AND kind<>'refund' FOR UPDATE;
-- name: BillingExpirePayment :execrows
UPDATE billing_payments SET status='expired',failure_code=$3,completed_at=now()
WHERE organization_id=$1 AND id=$2 AND kind='checkout' AND status IN ('pending','unknown');
-- name: BillingFailPayment :execrows
UPDATE billing_payments SET status='failed',failure_code=$3,completed_at=now()
WHERE organization_id=$1 AND id=$2 AND status IN ('pending','unknown');
-- name: BillingPaymentStatus :one
SELECT p.uuid,p.status,p.failure_code,p.provider,i.uuid AS invoice_uuid,i.status AS invoice_status
FROM billing_payments p JOIN billing_invoices i ON i.organization_id=p.organization_id AND i.id=p.invoice_id
WHERE p.organization_id=$1 AND p.order_reference=$2 AND p.kind='checkout' ORDER BY p.id DESC LIMIT 1;
