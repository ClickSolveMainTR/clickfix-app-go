-- CF-154: payment state machine. Abandoned hosted checkouts become 'expired'
-- (they no longer block a new checkout of the same invoice). A provider result
-- that arrives after expiry still settles it: the provider is the source of truth.
-- Refunds stay separate kind='refund' rows linked to the original (ledger style).
ALTER TABLE billing_payments DROP CONSTRAINT billing_payments_status_check;
ALTER TABLE billing_payments ADD CONSTRAINT billing_payments_status_check
    CHECK (status IN ('pending','unknown','succeeded','failed','expired'));
ALTER TABLE billing_payments ADD CONSTRAINT billing_payments_expired_kind_check
    CHECK (status <> 'expired' OR kind = 'checkout');

-- Allowed moves (same status is a no-op):
--   pending -> unknown | succeeded | failed | expired
--   unknown -> succeeded | failed | expired
--   expired -> succeeded | failed  (late provider result)
-- succeeded and failed are final.
CREATE FUNCTION billing_payment_transition_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.status = OLD.status THEN
        RETURN NEW;
    END IF;
    IF (OLD.status = 'pending' AND NEW.status IN ('unknown','succeeded','failed','expired'))
        OR (OLD.status = 'unknown' AND NEW.status IN ('succeeded','failed','expired'))
        OR (OLD.status = 'expired' AND NEW.status IN ('succeeded','failed')) THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'billing payment % cannot move from % to %', OLD.id, OLD.status, NEW.status
        USING ERRCODE = 'check_violation';
END $$;
CREATE TRIGGER trg_billing_payment_transition BEFORE UPDATE OF status ON billing_payments
    FOR EACH ROW EXECUTE FUNCTION billing_payment_transition_guard();

CREATE INDEX idx_billing_payments_unresolved_checkout ON billing_payments(created_at)
    WHERE status IN ('pending','unknown') AND kind = 'checkout';
