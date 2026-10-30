ALTER TABLE organization_members
 ADD COLUMN sip_extension TEXT NOT NULL DEFAULT '' CHECK (sip_extension = '' OR sip_extension ~ '^[0-9]{1,6}$'),
 ADD COLUMN sip_username TEXT NOT NULL DEFAULT '',
 ADD COLUMN sip_credentials_encrypted TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX uq_organization_members_sip_extension ON organization_members(organization_id,sip_extension) WHERE sip_extension <> '';
ALTER TABLE channel_voice_calls
 DROP CONSTRAINT channel_voice_calls_direction_check,
 ADD CONSTRAINT channel_voice_calls_direction_check CHECK(direction IN ('inbound','outbound','internal')),
 ADD COLUMN internal_number TEXT NOT NULL DEFAULT '',
 ADD COLUMN recording_url TEXT NOT NULL DEFAULT '',
 ADD COLUMN agent_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
 ADD COLUMN answered_at TIMESTAMPTZ,
 ADD COLUMN ended_at TIMESTAMPTZ,
 ADD COLUMN initiated_at TIMESTAMPTZ,
 ADD COLUMN client_event_id UUID,
 ADD COLUMN provider_final BOOLEAN NOT NULL DEFAULT false,
 ADD COLUMN missed_notified_at TIMESTAMPTZ;
CREATE UNIQUE INDEX uq_channel_voice_client_event ON channel_voice_calls(organization_id,channel_instance_id,client_event_id) WHERE client_event_id IS NOT NULL;
