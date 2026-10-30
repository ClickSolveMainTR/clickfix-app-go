-- Drops every video room, recording record and video_minutes wallet entry.
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.video.use','tenant.video.manage','platform.video.manage'));
DELETE FROM permissions WHERE slug IN ('tenant.video.use','tenant.video.manage','platform.video.manage');

DROP TABLE IF EXISTS video_webhook_events;
DROP TABLE IF EXISTS video_recordings;
DROP TABLE IF EXISTS video_sessions;
DROP TABLE IF EXISTS video_participants;
DROP TABLE IF EXISTS video_rooms;
DROP TABLE IF EXISTS video_org_settings;
DROP TABLE IF EXISTS video_plan_policies;
DROP TABLE IF EXISTS video_platform_settings;

DELETE FROM billing_plan_entitlements WHERE feature_id IN (SELECT id FROM billing_features WHERE key IN ('video.enabled','video.minutes'));
DELETE FROM billing_entitlement_overrides WHERE feature_id IN (SELECT id FROM billing_features WHERE key IN ('video.enabled','video.minutes'));
DELETE FROM billing_features WHERE key IN ('video.enabled','video.minutes');

-- The ledger is append-only; rolling the unit back removes its history.
ALTER TABLE billing_credit_ledger DISABLE TRIGGER trg_billing_credit_immutable;
DELETE FROM billing_credit_allocations WHERE debit_id IN (SELECT id FROM billing_credit_ledger WHERE unit = 'video_minutes')
    OR credit_id IN (SELECT id FROM billing_credit_ledger WHERE unit = 'video_minutes');
DELETE FROM billing_credit_ledger WHERE unit = 'video_minutes' AND reverses_id IS NOT NULL;
DELETE FROM billing_credit_ledger WHERE unit = 'video_minutes';
ALTER TABLE billing_credit_ledger ENABLE TRIGGER trg_billing_credit_immutable;
ALTER TABLE billing_credit_ledger DROP CONSTRAINT billing_credit_ledger_unit_check;
ALTER TABLE billing_credit_ledger ADD CONSTRAINT billing_credit_ledger_unit_check CHECK (unit IN ('TRY','ai_credits'));
DELETE FROM billing_wallet_settings WHERE package_id IN (SELECT id FROM billing_credit_packages WHERE unit = 'video_minutes');
ALTER TABLE billing_credit_purchases DROP COLUMN unit;
ALTER TABLE billing_credit_packages DROP COLUMN unit;
