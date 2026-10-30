-- CF-139 video rooms.

-- name: VideoPlatformPolicy :one
SELECT policy FROM video_platform_settings WHERE id=1;
-- name: VideoSavePlatformPolicy :exec
UPDATE video_platform_settings SET policy=$1,updated_by=$2 WHERE id=1;
-- name: VideoPlanPolicies :many
SELECT p.uuid AS plan_uuid,p.code,p.name,v.policy FROM video_plan_policies v JOIN billing_plans p ON p.id=v.plan_id ORDER BY p.sort_order,p.id;
-- name: VideoSavePlanPolicy :exec
INSERT INTO video_plan_policies(plan_id,policy) SELECT id,$2 FROM billing_plans WHERE uuid=$1
ON CONFLICT(plan_id) DO UPDATE SET policy=excluded.policy;
-- name: VideoDeletePlanPolicy :execrows
DELETE FROM video_plan_policies WHERE plan_id=(SELECT id FROM billing_plans WHERE uuid=$1);
-- name: VideoOrgPlanPolicy :one
SELECT v.policy FROM billing_subscriptions s JOIN billing_plan_versions pv ON pv.id=s.plan_version_id JOIN video_plan_policies v ON v.plan_id=pv.plan_id
WHERE s.organization_id=$1;

-- name: VideoOrgSettings :one
SELECT * FROM video_org_settings WHERE organization_id=$1;
-- name: VideoSaveOrgSettings :one
INSERT INTO video_org_settings(organization_id,enabled,policy,storage_mode,s3_endpoint,s3_region,s3_bucket,s3_prefix,s3_path_style,s3_access_key_enc,s3_secret_key_enc)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT(organization_id) DO UPDATE SET enabled=excluded.enabled,policy=excluded.policy,storage_mode=excluded.storage_mode,s3_endpoint=excluded.s3_endpoint,
s3_region=excluded.s3_region,s3_bucket=excluded.s3_bucket,s3_prefix=excluded.s3_prefix,s3_path_style=excluded.s3_path_style,
s3_access_key_enc=excluded.s3_access_key_enc,s3_secret_key_enc=excluded.s3_secret_key_enc
RETURNING *;
-- name: VideoMarkLowBalanceNotified :execrows
INSERT INTO video_org_settings(organization_id,low_balance_notified_at) VALUES($1,now())
ON CONFLICT(organization_id) DO UPDATE SET low_balance_notified_at=now()
WHERE video_org_settings.organization_id=$1 AND (video_org_settings.low_balance_notified_at IS NULL OR video_org_settings.low_balance_notified_at<now()-interval '24 hours');
-- name: VideoClearLowBalance :exec
UPDATE video_org_settings SET low_balance_notified_at=NULL WHERE organization_id=$1 AND low_balance_notified_at IS NOT NULL;

-- name: VideoCreateRoom :one
INSERT INTO video_rooms(organization_id,name,livekit_name,host_user_id,created_by,appointment_uuid,conversation_id,pin_enc,lobby,guest_join,e2ee,e2ee_key_enc,
recording,allow_camera,allow_screen_share,allow_chat,max_participants,max_duration_minutes,scheduled_start_at,scheduled_end_at,links_expire_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21) RETURNING *;
-- name: VideoRoom :one
SELECT * FROM video_rooms WHERE organization_id=$1 AND uuid=$2;
-- name: VideoRoomByID :one
SELECT * FROM video_rooms WHERE organization_id=$1 AND id=$2;
-- name: VideoRoomForAppointment :one
SELECT * FROM video_rooms WHERE organization_id=$1 AND appointment_uuid=$2 AND status<>'ended';
-- name: VideoRooms :many
SELECT r.*,c.uuid AS conversation_uuid,
(SELECT count(*) FROM video_participants p WHERE p.organization_id=r.organization_id AND p.room_id=r.id AND p.status='waiting')::bigint AS waiting
FROM video_rooms r LEFT JOIN conversations c ON c.organization_id=r.organization_id AND c.id=r.conversation_id
WHERE r.organization_id=$1 AND (sqlc.narg(status)::text IS NULL OR r.status=sqlc.narg(status)::text)
ORDER BY r.id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: VideoRoomCount :one
SELECT count(*) FROM video_rooms WHERE organization_id=$1 AND (sqlc.narg(status)::text IS NULL OR status=sqlc.narg(status)::text);
-- name: VideoConversationUUID :one
SELECT uuid FROM conversations WHERE organization_id=$1 AND id=$2;
-- name: VideoStartRoom :exec
UPDATE video_rooms SET status='live',started_at=COALESCE(started_at,now()) WHERE organization_id=$1 AND id=$2 AND status<>'ended';
-- name: VideoEndRoom :one
UPDATE video_rooms SET status='ended',ended_at=COALESCE(ended_at,now()) WHERE organization_id=$1 AND id=$2 RETURNING *;
-- name: VideoSetRoomRecording :exec
UPDATE video_rooms SET recording=$3 WHERE organization_id=$1 AND id=$2;
-- name: VideoAddRoomBilling :exec
UPDATE video_rooms SET billed_minutes=billed_minutes+sqlc.arg(billed)::bigint,unbilled_minutes=unbilled_minutes+sqlc.arg(unbilled)::bigint WHERE organization_id=$1 AND id=$2;
-- name: VideoIdleRoom :exec
UPDATE video_rooms SET status='open',started_at=NULL WHERE organization_id=$1 AND id=$2 AND status='live';
-- name: VideoRoomByUUID :one
SELECT * FROM video_rooms WHERE uuid=$1;
-- name: VideoLiveRooms :many
SELECT * FROM video_rooms WHERE status<>'ended' ORDER BY id LIMIT 500;
-- name: VideoRoomByLivekitName :one
SELECT * FROM video_rooms WHERE livekit_name=$1;
-- name: VideoActiveRoomsInOrg :one
SELECT count(*) FROM video_rooms WHERE organization_id=$1 AND status='live';

-- name: VideoCreateParticipant :one
INSERT INTO video_participants(organization_id,room_id,identity,role,display_name,email,user_id,status,ticket_hash,admitted_at,admitted_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT(room_id,identity) DO UPDATE SET display_name=excluded.display_name,ticket_hash=excluded.ticket_hash,
status=CASE WHEN video_participants.status='removed' THEN video_participants.status ELSE excluded.status END,
admitted_at=COALESCE(video_participants.admitted_at,excluded.admitted_at)
WHERE video_participants.organization_id=$1
RETURNING *;
-- name: VideoParticipantByTicket :one
SELECT * FROM video_participants WHERE ticket_hash=$1;
-- name: VideoParticipant :one
SELECT * FROM video_participants WHERE organization_id=$1 AND room_id=$2 AND uuid=$3;
-- name: VideoParticipants :many
SELECT * FROM video_participants WHERE organization_id=$1 AND room_id=$2 ORDER BY id;
-- name: VideoSetParticipantStatus :one
UPDATE video_participants SET status=sqlc.arg(status)::text,
admitted_at=CASE WHEN sqlc.arg(status)::text='admitted' THEN now() ELSE admitted_at END,
admitted_by=CASE WHEN sqlc.arg(status)::text='admitted' THEN sqlc.narg(actor)::bigint ELSE admitted_by END
WHERE organization_id=$1 AND room_id=$2 AND uuid=$3 RETURNING *;
-- name: VideoAdmittedCount :one
SELECT count(*) FROM video_participants WHERE organization_id=$1 AND room_id=$2 AND status='admitted';

-- name: VideoOpenSession :exec
INSERT INTO video_sessions(organization_id,room_id,livekit_sid,identity,joined_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(livekit_sid) DO NOTHING;
-- name: VideoCloseSession :one
UPDATE video_sessions SET left_at=sqlc.arg(left_at)::timestamptz,
minutes=GREATEST(1,ceil(extract(epoch FROM (sqlc.arg(left_at)::timestamptz-joined_at))/60.0))::bigint
WHERE organization_id=$1 AND livekit_sid=$2 AND left_at IS NULL RETURNING *;
-- name: VideoOpenSessions :many
SELECT * FROM video_sessions WHERE organization_id=$1 AND room_id=$2 AND left_at IS NULL ORDER BY id;
-- name: VideoSetSessionBilled :exec
UPDATE video_sessions SET billed_minutes=$3 WHERE organization_id=$1 AND id=$2;
-- name: VideoOpenMinutes :one
SELECT COALESCE(sum(ceil(extract(epoch FROM (now()-joined_at))/60.0)),0)::bigint FROM video_sessions WHERE organization_id=$1 AND left_at IS NULL;
-- name: VideoUsage :one
SELECT COALESCE(sum(minutes),0)::bigint AS minutes,COALESCE(sum(billed_minutes),0)::bigint AS billed
FROM video_sessions WHERE organization_id=$1 AND joined_at>=sqlc.arg(since)::timestamptz;

-- name: VideoWebhookSeen :execrows
INSERT INTO video_webhook_events(event_id) VALUES($1) ON CONFLICT DO NOTHING;
-- name: VideoForgetWebhookEvent :exec
DELETE FROM video_webhook_events WHERE event_id=$1;
-- name: VideoForceLobby :exec
UPDATE video_rooms SET lobby=true WHERE organization_id=$1 AND id=$2;
-- name: VideoPurgeWebhookEvents :exec
DELETE FROM video_webhook_events WHERE received_at<now()-interval '1 day';

-- name: VideoCreateRecording :one
INSERT INTO video_recordings(organization_id,room_id,storage,object_key,started_by,expires_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING *;
-- name: VideoSetRecordingEgress :exec
UPDATE video_recordings SET egress_id=$3,status='active' WHERE organization_id=$1 AND id=$2;
-- name: VideoFailRecording :exec
UPDATE video_recordings SET status='failed',error=$3,ended_at=now() WHERE organization_id=$1 AND id=$2;
-- name: VideoRecordingByEgress :one
SELECT * FROM video_recordings WHERE egress_id=$1;
-- name: VideoFinishRecording :exec
UPDATE video_recordings SET status=$3,size_bytes=$4,duration_seconds=$5,error=$6,ended_at=COALESCE(ended_at,now()),
expires_at=CASE WHEN expires_at IS NULL THEN NULL ELSE now()+make_interval(hours=>sqlc.arg(retention_hours)::int) END
WHERE organization_id=$1 AND id=$2 AND status<>'deleted';
-- name: VideoActiveRecording :one
SELECT * FROM video_recordings WHERE organization_id=$1 AND room_id=$2 AND status IN ('starting','active') ORDER BY id DESC LIMIT 1;
-- name: VideoActiveRecordingCount :one
SELECT count(*) FROM video_recordings WHERE organization_id=$1 AND status IN ('starting','active');
-- name: VideoRecording :one
SELECT * FROM video_recordings WHERE organization_id=$1 AND uuid=$2;
-- name: VideoRecordings :many
SELECT v.*,r.uuid AS room_uuid,r.name AS room_name FROM video_recordings v JOIN video_rooms r ON r.organization_id=v.organization_id AND r.id=v.room_id
WHERE v.organization_id=$1 AND v.status<>'deleted' AND (sqlc.narg(room_uuid)::uuid IS NULL OR r.uuid=sqlc.narg(room_uuid)::uuid)
ORDER BY v.id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: VideoRecordingCount :one
SELECT count(*) FROM video_recordings v JOIN video_rooms r ON r.organization_id=v.organization_id AND r.id=v.room_id
WHERE v.organization_id=$1 AND v.status<>'deleted' AND (sqlc.narg(room_uuid)::uuid IS NULL OR r.uuid=sqlc.narg(room_uuid)::uuid);
-- name: VideoMarkRecordingDeleted :exec
UPDATE video_recordings SET status='deleted',deleted_at=now() WHERE organization_id=$1 AND id=$2;
-- name: VideoExpiredRecordings :many
SELECT * FROM video_recordings WHERE deleted_at IS NULL AND expires_at IS NOT NULL AND expires_at<=now() AND status NOT IN ('starting','active') ORDER BY id LIMIT 200;

-- name: VideoMoveAppointmentRoom :one
UPDATE video_rooms SET scheduled_start_at=$3,scheduled_end_at=$4,links_expire_at=$4,
 host_user_id=$5,max_duration_minutes=$6
WHERE organization_id=$1 AND uuid=$2 AND status='open' RETURNING *;

-- name: VideoAttendance :one
SELECT EXISTS(SELECT 1 FROM video_sessions s JOIN video_participants p
 ON p.organization_id=s.organization_id AND p.room_id=s.room_id AND p.identity=s.identity
 WHERE s.organization_id=r.organization_id AND s.room_id=r.id AND p.role='host'
 AND (r.scheduled_start_at IS NULL OR s.joined_at >= r.scheduled_start_at)
 AND (r.scheduled_end_at IS NULL OR s.joined_at < r.scheduled_end_at)) AS host_joined,
 EXISTS(SELECT 1 FROM video_sessions s JOIN video_participants p
 ON p.organization_id=s.organization_id AND p.room_id=s.room_id AND p.identity=s.identity
 WHERE s.organization_id=r.organization_id AND s.room_id=r.id AND p.role='guest'
 AND (r.scheduled_start_at IS NULL OR s.joined_at >= r.scheduled_start_at)
 AND (r.scheduled_end_at IS NULL OR s.joined_at < r.scheduled_end_at)) AS guest_joined
FROM video_rooms r WHERE r.organization_id=$1 AND r.uuid=$2;
