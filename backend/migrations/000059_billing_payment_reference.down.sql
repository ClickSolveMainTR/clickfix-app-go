DROP TRIGGER IF EXISTS trg_billing_invoice_payment_reference ON billing_invoices;
DROP FUNCTION IF EXISTS billing_invoice_payment_reference();
ALTER TABLE billing_invoices ALTER COLUMN payment_reference SET DEFAULT 'CF' || replace(gen_random_uuid()::text, '-', '');
