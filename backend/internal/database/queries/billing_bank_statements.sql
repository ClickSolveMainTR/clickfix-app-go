-- name: BillingInsertBankStatement :one
INSERT INTO billing_bank_statement_lines(organization_id,bank_account_id,external_id,description,reference,sender_iban,amount_minor,currency,booked_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT(organization_id,bank_account_id,external_id) DO UPDATE SET external_id=billing_bank_statement_lines.external_id RETURNING *;
-- name: BillingBankStatement :one
SELECT * FROM billing_bank_statement_lines WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: BillingBankStatements :many
SELECT l.*, b.uuid AS bank_account_uuid FROM billing_bank_statement_lines l
JOIN billing_bank_accounts b ON b.id=l.bank_account_id
WHERE l.organization_id=$1 AND (sqlc.arg(status)::text='' OR l.status=sqlc.arg(status))
ORDER BY l.id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: BillingBankStatementCount :one
SELECT count(*) FROM billing_bank_statement_lines WHERE organization_id=$1 AND (sqlc.arg(status)::text='' OR status=sqlc.arg(status));
-- name: BillingBankStatementCandidates :many
SELECT t.*,i.payment_reference FROM billing_bank_transfers t
JOIN billing_invoices i ON i.id=t.invoice_id AND i.organization_id=t.organization_id
WHERE t.organization_id=$1 AND t.bank_account_id=$2 AND t.status='reported'
AND t.created_at BETWEEN sqlc.arg(booked_at)::timestamptz-interval '3 days' AND sqlc.arg(booked_at)::timestamptz+interval '3 days'
AND NOT EXISTS(SELECT 1 FROM billing_bank_statement_lines l WHERE l.organization_id=t.organization_id AND l.transfer_id=t.id)
ORDER BY t.id;
-- name: BillingResolveBankStatement :exec
UPDATE billing_bank_statement_lines SET status=$3,confidence=$4,transfer_id=$5,candidate_uuid=$6,reviewed_by=$7,reason=$8 WHERE organization_id=$1 AND id=$2;
-- name: BillingSetTransferSenderIBAN :exec
UPDATE billing_bank_transfers SET sender_iban=$3 WHERE organization_id=$1 AND uuid=$2;
