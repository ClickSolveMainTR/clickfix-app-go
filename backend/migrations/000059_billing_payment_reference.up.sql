-- Human payment references (QA round 2): "CF" + year + invoice id, e.g.
-- CF2026000042, instead of "CF" + a random uuid. Letters and digits only so
-- bank transfer descriptions keep it intact; unique because the id is.
CREATE FUNCTION billing_invoice_payment_reference() RETURNS trigger AS $$
BEGIN
    IF NEW.payment_reference IS NULL OR NEW.payment_reference = '' THEN
        NEW.payment_reference := 'CF' || to_char(COALESCE(NEW.created_at, now()) AT TIME ZONE 'Europe/Istanbul', 'YYYY')
            || lpad(NEW.id::text, 6, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

ALTER TABLE billing_invoices ALTER COLUMN payment_reference DROP DEFAULT;

CREATE TRIGGER trg_billing_invoice_payment_reference
    BEFORE INSERT ON billing_invoices
    FOR EACH ROW EXECUTE FUNCTION billing_invoice_payment_reference();

UPDATE billing_invoices
SET payment_reference = 'CF' || to_char(created_at AT TIME ZONE 'Europe/Istanbul', 'YYYY') || lpad(id::text, 6, '0')
WHERE payment_reference ~ '^CF[0-9a-f]{32}$';
