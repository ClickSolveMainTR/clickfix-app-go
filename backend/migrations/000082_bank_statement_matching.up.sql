ALTER TABLE billing_bank_transfers ADD COLUMN sender_iban TEXT NOT NULL DEFAULT '';
CREATE TABLE billing_bank_statement_lines (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 bank_account_id BIGINT NOT NULL REFERENCES billing_bank_accounts(id),
 external_id TEXT NOT NULL CHECK(length(external_id) BETWEEN 1 AND 160),
 description TEXT NOT NULL DEFAULT '',
 reference TEXT NOT NULL DEFAULT '',
 sender_iban TEXT NOT NULL DEFAULT '',
 amount_minor BIGINT NOT NULL CHECK(amount_minor BETWEEN 1 AND 99999999999999),
 currency TEXT NOT NULL DEFAULT 'TRY' CHECK(currency='TRY'),
 booked_at TIMESTAMPTZ NOT NULL,
 status TEXT NOT NULL DEFAULT 'review' CHECK(status IN ('review','approved','ignored')),
 confidence INTEGER NOT NULL DEFAULT 0 CHECK(confidence BETWEEN 0 AND 100),
 transfer_id BIGINT REFERENCES billing_bank_transfers(id),
 candidate_uuid UUID REFERENCES billing_bank_transfers(uuid),
 reviewed_by BIGINT REFERENCES users(id),
 reason TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,bank_account_id,external_id),
 UNIQUE(transfer_id),
 UNIQUE(bank_account_id,external_id)
);
CREATE INDEX idx_billing_bank_statement_org ON billing_bank_statement_lines(organization_id,status,id);
CREATE TRIGGER trg_billing_bank_statement_updated BEFORE UPDATE ON billing_bank_statement_lines
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
