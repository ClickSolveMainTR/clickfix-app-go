DELETE FROM notification_templates WHERE code IN (
    'conversation.assigned', 'message.received', 'conversation.mentioned', 'ticket.assigned',
    'ticket.commented', 'sla.warning', 'sla.breached', 'csat.submitted', 'notifications.digest'
);
DROP TABLE IF EXISTS notification_digest_items;
ALTER TABLE notification_preferences
    DROP CONSTRAINT IF EXISTS notification_preferences_quiet_chk,
    DROP CONSTRAINT IF EXISTS notification_preferences_digest_hour_chk,
    DROP CONSTRAINT IF EXISTS notification_preferences_digest_chk,
    DROP COLUMN IF EXISTS last_digest_at,
    DROP COLUMN IF EXISTS digest_hour,
    DROP COLUMN IF EXISTS email_digest,
    DROP COLUMN IF EXISTS timezone,
    DROP COLUMN IF EXISTS quiet_hours_end,
    DROP COLUMN IF EXISTS quiet_hours_start,
    DROP COLUMN IF EXISTS quiet_hours_enabled,
    DROP COLUMN IF EXISTS event_channels;
