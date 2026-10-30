-- Data-loss note: historical revenue snapshots cannot be reconstructed after rollback.
DROP TRIGGER trg_revenue_subscription ON billing_subscriptions;
DROP TRIGGER trg_revenue_items ON billing_subscription_items;
DROP TRIGGER trg_revenue_seats ON organization_members;
DROP FUNCTION billing_revenue_changed();
DROP FUNCTION billing_capture_revenue(BIGINT);
DROP TABLE billing_revenue_snapshots,billing_revenue_coverage;
