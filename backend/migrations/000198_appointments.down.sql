-- Drops every appointment, type and setting (data loss).
DROP TABLE IF EXISTS appointment_reminders_sent;
DROP TABLE IF EXISTS appointments;
DROP TABLE IF EXISTS appointment_type_hosts;
DROP TABLE IF EXISTS appointment_types;
DROP TABLE IF EXISTS appointment_settings;

DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('tenant.appointments.read', 'tenant.appointments.write', 'tenant.appointments.manage'));
DELETE FROM permissions WHERE slug IN ('tenant.appointments.read', 'tenant.appointments.write', 'tenant.appointments.manage');

-- Keep the catalog row when immutable billing history references it.
ALTER TABLE billing_plan_entitlements DISABLE TRIGGER trg_billing_entitlement_guard;
DO $$ BEGIN
    DELETE FROM billing_plan_entitlements WHERE feature_id = (SELECT id FROM billing_features WHERE key = 'module.appointments');
    DELETE FROM billing_features WHERE key = 'module.appointments';
EXCEPTION WHEN foreign_key_violation THEN NULL;
END $$;
ALTER TABLE billing_plan_entitlements ENABLE TRIGGER trg_billing_entitlement_guard;
