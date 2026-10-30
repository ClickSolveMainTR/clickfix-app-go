-- Data-loss note: imported statement rows and matching history are removed.
DROP TABLE billing_bank_statement_lines;
ALTER TABLE billing_bank_transfers DROP COLUMN sender_iban;
