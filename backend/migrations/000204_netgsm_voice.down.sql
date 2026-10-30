-- Data-loss note: removes per-member SIP configuration and Netgsm lifecycle metadata.
DROP INDEX uq_channel_voice_client_event;
ALTER TABLE channel_voice_calls DROP COLUMN initiated_at,DROP COLUMN client_event_id,DROP COLUMN provider_final,DROP COLUMN missed_notified_at,DROP COLUMN ended_at, DROP COLUMN answered_at, DROP COLUMN agent_user_id,
 DROP COLUMN recording_url, DROP COLUMN internal_number;
DELETE FROM channel_voice_calls WHERE direction='internal';
ALTER TABLE channel_voice_calls DROP CONSTRAINT channel_voice_calls_direction_check,
 ADD CONSTRAINT channel_voice_calls_direction_check CHECK(direction IN ('inbound','outbound'));
DROP INDEX uq_organization_members_sip_extension;
ALTER TABLE organization_members DROP COLUMN sip_credentials_encrypted,DROP COLUMN sip_username,DROP COLUMN sip_extension;
