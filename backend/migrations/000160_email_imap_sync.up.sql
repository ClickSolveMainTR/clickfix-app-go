-- CF-56: cursor and retry state only; mailbox secrets stay in channel_instances.credentials_enc.
CREATE TABLE channel_email_imap_states (
 id BIGSERIAL PRIMARY KEY,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 channel_instance_id BIGINT NOT NULL REFERENCES channel_instances(id) ON DELETE CASCADE,
 mailbox_key TEXT NOT NULL DEFAULT '',
 uid_validity BIGINT NOT NULL DEFAULT 0 CHECK (uid_validity BETWEEN 0 AND 4294967295),
 initial_uid BIGINT NOT NULL DEFAULT 0 CHECK (initial_uid BETWEEN 0 AND 4294967295),
 last_uid BIGINT NOT NULL DEFAULT 0 CHECK (last_uid BETWEEN 0 AND 4294967295),
 failures INTEGER NOT NULL DEFAULT 0 CHECK (failures >= 0),
 next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 UNIQUE (organization_id, channel_instance_id)
);
CREATE INDEX idx_channel_email_imap_states_org ON channel_email_imap_states(organization_id);
CREATE TRIGGER trg_channel_email_imap_states_updated_at BEFORE UPDATE ON channel_email_imap_states
 FOR EACH ROW EXECUTE FUNCTION set_updated_at();
