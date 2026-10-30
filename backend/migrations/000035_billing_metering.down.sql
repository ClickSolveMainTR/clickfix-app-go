-- Data-loss note: removes settlement snapshots; source usage ledger is preserved.
DROP TRIGGER trg_billing_usage_open ON billing_usage_records;
DROP FUNCTION billing_usage_open_guard();
DROP TABLE billing_usage_settlements;
DROP INDEX idx_billing_usage_daily;
-- Feature catalog entries are retained because published plans may reference them.
DROP TABLE billing_usage_denials;
