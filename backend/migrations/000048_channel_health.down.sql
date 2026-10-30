DROP TABLE IF EXISTS channel_email_domains;
DROP INDEX IF EXISTS idx_messages_instance_out_recent;
ALTER TABLE channel_instances
    DROP COLUMN IF EXISTS token_expires_at,
    DROP COLUMN IF EXISTS health_checked_at,
    DROP COLUMN IF EXISTS health_detail,
    DROP COLUMN IF EXISTS health_ok,
    DROP COLUMN IF EXISTS last_webhook_at;
