DROP TABLE IF EXISTS event_dead_letters;
DROP TABLE IF EXISTS event_consumptions;
DROP TABLE IF EXISTS domain_event_log;
DROP INDEX IF EXISTS idx_outbox_events_organization;
ALTER TABLE outbox_events
    DROP CONSTRAINT IF EXISTS chk_outbox_events_version,
    DROP COLUMN IF EXISTS event_version,
    DROP COLUMN IF EXISTS organization_id;
