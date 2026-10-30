-- CF-126. Opt-in is a widget setting AND a plan feature, never inferred from file upload.
CREATE TABLE widget_sessions (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 conversation_id BIGINT NOT NULL,
 visitor_id TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('replay','cobrowse')),
 status TEXT NOT NULL CHECK(status IN ('requested','active','ended','revoked','declined')),
 control_enabled BOOLEAN NOT NULL DEFAULT false,
 requested_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
 consent_version TEXT NOT NULL DEFAULT '2026-10-29',
 accepted_at TIMESTAMPTZ,
 ended_at TIMESTAMPTZ,
 timeout_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '15 minutes',
 expires_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '7 days',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id) ON DELETE CASCADE,
 CHECK(status <> 'active' OR accepted_at IS NOT NULL),
 CHECK(NOT control_enabled OR kind='cobrowse')
);
CREATE INDEX idx_widget_sessions_conversation ON widget_sessions(organization_id,conversation_id,created_at DESC);
CREATE INDEX idx_widget_sessions_expiry ON widget_sessions(expires_at);
CREATE TRIGGER trg_widget_sessions_updated BEFORE UPDATE ON widget_sessions FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE widget_replay_chunks (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 session_id BIGINT NOT NULL,
 sequence INTEGER NOT NULL CHECK(sequence BETWEEN 0 AND 299),
 object_key TEXT NOT NULL UNIQUE,
 size_bytes BIGINT NOT NULL CHECK(size_bytes BETWEEN 1 AND 524288),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,session_id,sequence),
 FOREIGN KEY(organization_id,session_id) REFERENCES widget_sessions(organization_id,id) ON DELETE CASCADE
);
-- Invalidate playback (including signed links) synchronously; retain keys until physical purge.
CREATE FUNCTION erase_conversation_replays() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.anonymized_at IS NOT NULL AND OLD.anonymized_at IS NULL THEN
  UPDATE widget_sessions SET status='revoked', visitor_id='erased', ended_at=now(), expires_at=now()
   WHERE organization_id=NEW.organization_id AND conversation_id=NEW.id;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER trg_conversation_replay_erasure AFTER UPDATE OF anonymized_at ON conversations
 FOR EACH ROW EXECUTE FUNCTION erase_conversation_replays();
INSERT INTO permissions(name,slug) VALUES
 ('Read session replays','tenant.replays.read'),('Request co-browse','tenant.cobrowse.use'),('Control co-browse','tenant.cobrowse.control')
 ON CONFLICT(slug) DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id)
 SELECT r.id,p.id FROM roles r CROSS JOIN permissions p WHERE r.scope='tenant' AND r.organization_id IS NULL
 AND r.slug IN ('owner','admin') AND p.slug IN ('tenant.replays.read','tenant.cobrowse.use','tenant.cobrowse.control') ON CONFLICT DO NOTHING;
INSERT INTO billing_features(key,kind,label_tr,label_en,unit,aggregation,period) VALUES
 ('replay.enabled','boolean','Oturum kaydı','Session replay','','none','none'),
 ('cobrowse.enabled','boolean','Ortak tarama','Co-browse','','none','none'),
 ('replay.bytes','limit','Kayıt depolama','Replay storage','byte','gauge','none'),
 ('replay.retention_days','limit','Kayıt saklama süresi','Replay retention','day','gauge','none'),
 ('cobrowse.sessions','limit','Günlük ortak tarama','Daily co-browse sessions','session','gauge','none')
 ON CONFLICT(key) DO NOTHING;
-- No automatic grants: billing must explicitly enable these features per plan/override.
