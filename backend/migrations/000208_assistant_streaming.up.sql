ALTER TABLE assistant_messages ADD COLUMN interrupted BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN guides_auto_start BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE assistant_usage DROP CONSTRAINT assistant_usage_kind_chk;
ALTER TABLE assistant_usage ADD CONSTRAINT assistant_usage_kind_chk CHECK (kind IN ('chat', 'guide_generate', 'title'));

-- Content differs by surface, while these topics have one shared first-view state.
CREATE FUNCTION guide_progress_topic(v TEXT) RETURNS TEXT LANGUAGE SQL IMMUTABLE STRICT AS $$
 SELECT CASE v
  WHEN 'mobile.more_ai' THEN 'settings_ai'
  WHEN 'mobile.more_channels' THEN 'channels'
  WHEN 'mobile.more_team' THEN 'teams'
  WHEN 'mobile.more_members' THEN 'settings_members'
  WHEN 'mobile.more_reports' THEN 'reports'
  WHEN 'mobile.agenda' THEN 'calendar'
  ELSE regexp_replace(v, '^(web|mobile)\.', '') END
$$;

-- A setup tour already seen on either surface is never offered again on the other.
INSERT INTO guide_progress (organization_id,user_id,guide_key,status,step_index)
SELECT DISTINCT p.organization_id,p.user_id,'__setup__','in_progress',0 FROM guide_progress p
WHERE p.status <> 'new' AND EXISTS (SELECT 1 FROM guides g WHERE g.key=p.guide_key AND g.setup_order > 0)
ON CONFLICT DO NOTHING;
