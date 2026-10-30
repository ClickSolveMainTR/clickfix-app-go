-- CF-138: appointments, appointment types and the organization calendar.

-- name: AppointmentSettings :one
-- Defaults apply until the organization saves its settings once.
SELECT o.id AS organization_id, o.uuid AS organization_uuid, o.slug AS organization_slug, o.name AS organization_name,
       o.locale AS organization_locale,
       COALESCE(s.enabled, TRUE)::boolean AS enabled,
       COALESCE(s.timezone, o.timezone)::text AS timezone,
       COALESCE(s.public_booking, TRUE)::boolean AS public_booking,
       COALESCE(s.reminder_minutes, '{1440,60}'::integer[])::integer[] AS reminder_minutes
FROM organizations o
LEFT JOIN appointment_settings s ON s.organization_id = o.id
WHERE o.id = sqlc.arg(organization_id) AND o.deleted_at IS NULL;

-- name: AppointmentSaveSettings :exec
INSERT INTO appointment_settings (organization_id, enabled, timezone, public_booking, reminder_minutes)
VALUES (sqlc.arg(organization_id), sqlc.arg(enabled), sqlc.arg(timezone), sqlc.arg(public_booking), sqlc.arg(reminder_minutes)::integer[])
ON CONFLICT (organization_id) DO UPDATE SET enabled = excluded.enabled, timezone = excluded.timezone,
    public_booking = excluded.public_booking, reminder_minutes = excluded.reminder_minutes;

-- name: AppointmentPublicOrganization :one
SELECT id, uuid, slug, name, locale FROM organizations
WHERE slug = sqlc.arg(slug) AND deleted_at IS NULL AND status = 'active';

-- name: AppointmentTypes :many
SELECT * FROM appointment_types
WHERE organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL
  AND (sqlc.arg(only_public)::boolean = FALSE OR (is_public AND is_active))
ORDER BY position, name, id;

-- name: AppointmentTypeByUUID :one
SELECT * FROM appointment_types WHERE organization_id = $1 AND uuid = $2 AND deleted_at IS NULL;

-- name: AppointmentTypeByID :one
SELECT * FROM appointment_types WHERE organization_id = $1 AND id = $2;

-- name: AppointmentTypeBySlug :one
SELECT * FROM appointment_types
WHERE organization_id = $1 AND slug = $2 AND deleted_at IS NULL AND is_active AND is_public;

-- name: AppointmentSlugTaken :one
SELECT EXISTS (SELECT 1 FROM appointment_types
    WHERE organization_id = sqlc.arg(organization_id) AND slug = sqlc.arg(slug) AND deleted_at IS NULL
      AND id <> sqlc.arg(except_id)::bigint);

-- name: AppointmentNextTypePosition :one
SELECT COALESCE(MAX(position) + 1, 0)::integer FROM appointment_types WHERE organization_id = $1 AND deleted_at IS NULL;

-- name: AppointmentCreateType :one
INSERT INTO appointment_types (
    organization_id, name, slug, description, color, duration_options, default_duration, slot_interval,
    buffer_before, buffer_after, location_kind, location_value, host_mode, team_id, booking_window_days,
    min_notice_minutes, max_per_day, intake_fields, confirmation_template, reminder_template,
    requires_confirmation, allow_cancel, allow_reschedule, cancel_notice_minutes, is_public, is_active, position)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(name), sqlc.arg(slug), sqlc.arg(description), sqlc.arg(color),
    sqlc.arg(duration_options)::integer[], sqlc.arg(default_duration), sqlc.arg(slot_interval),
    sqlc.arg(buffer_before), sqlc.arg(buffer_after), sqlc.arg(location_kind), sqlc.arg(location_value),
    sqlc.arg(host_mode), sqlc.narg(team_id), sqlc.arg(booking_window_days), sqlc.arg(min_notice_minutes),
    sqlc.arg(max_per_day), sqlc.arg(intake_fields), sqlc.arg(confirmation_template), sqlc.arg(reminder_template),
    sqlc.arg(requires_confirmation), sqlc.arg(allow_cancel), sqlc.arg(allow_reschedule),
    sqlc.arg(cancel_notice_minutes), sqlc.arg(is_public), sqlc.arg(is_active), sqlc.arg(position))
RETURNING *;

-- name: AppointmentUpdateType :one
UPDATE appointment_types SET
    name = sqlc.arg(name), slug = sqlc.arg(slug), description = sqlc.arg(description), color = sqlc.arg(color),
    duration_options = sqlc.arg(duration_options)::integer[], default_duration = sqlc.arg(default_duration),
    slot_interval = sqlc.arg(slot_interval), buffer_before = sqlc.arg(buffer_before), buffer_after = sqlc.arg(buffer_after),
    location_kind = sqlc.arg(location_kind), location_value = sqlc.arg(location_value), host_mode = sqlc.arg(host_mode),
    team_id = sqlc.narg(team_id), booking_window_days = sqlc.arg(booking_window_days),
    min_notice_minutes = sqlc.arg(min_notice_minutes), max_per_day = sqlc.arg(max_per_day),
    intake_fields = sqlc.arg(intake_fields), confirmation_template = sqlc.arg(confirmation_template),
    reminder_template = sqlc.arg(reminder_template), requires_confirmation = sqlc.arg(requires_confirmation),
    allow_cancel = sqlc.arg(allow_cancel), allow_reschedule = sqlc.arg(allow_reschedule),
    cancel_notice_minutes = sqlc.arg(cancel_notice_minutes), is_public = sqlc.arg(is_public), is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: AppointmentDeleteType :execrows
UPDATE appointment_types SET deleted_at = NOW(), is_active = FALSE
WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: AppointmentTypeHosts :many
SELECT h.type_id, h.user_id, h.last_assigned_at, u.uuid AS user_uuid, u.name, u.surname, u.email
FROM appointment_type_hosts h
JOIN users u ON u.id = h.user_id AND u.deleted_at IS NULL
JOIN organization_members m ON m.organization_id = h.organization_id AND m.user_id = h.user_id
WHERE h.organization_id = sqlc.arg(organization_id) AND h.type_id = ANY(sqlc.arg(type_ids)::bigint[])
ORDER BY h.type_id, u.name, u.surname, u.id;

-- name: AppointmentClearHosts :exec
DELETE FROM appointment_type_hosts WHERE organization_id = $1 AND type_id = $2;

-- name: AppointmentAddHost :exec
INSERT INTO appointment_type_hosts (organization_id, type_id, user_id) VALUES ($1, $2, $3)
ON CONFLICT (type_id, user_id) DO NOTHING;

-- name: AppointmentTouchHost :exec
UPDATE appointment_type_hosts SET last_assigned_at = NOW()
WHERE organization_id = $1 AND type_id = $2 AND user_id = $3;

-- name: AppointmentMembers :many
-- Organization members by user uuid (host validation and filters).
SELECT u.id, u.uuid, u.name, u.surname, u.email
FROM organization_members m
JOIN users u ON u.id = m.user_id AND u.deleted_at IS NULL
WHERE m.organization_id = sqlc.arg(organization_id) AND u.uuid = ANY(sqlc.arg(user_uuids)::uuid[]);

-- name: AppointmentMembersByID :many
SELECT u.id, u.uuid, u.name, u.surname, u.email
FROM organization_members m
JOIN users u ON u.id = m.user_id AND u.deleted_at IS NULL
WHERE m.organization_id = sqlc.arg(organization_id) AND u.id = ANY(sqlc.arg(user_ids)::bigint[]);

-- name: AppointmentTeamHosts :many
SELECT tm.team_id, u.id, u.uuid, u.name, u.surname, u.email
FROM team_members tm
JOIN organization_members m ON m.organization_id = tm.organization_id AND m.user_id = tm.user_id
JOIN users u ON u.id = tm.user_id AND u.deleted_at IS NULL
WHERE tm.organization_id = sqlc.arg(organization_id) AND tm.team_id = ANY(sqlc.arg(team_ids)::bigint[])
ORDER BY tm.team_id, u.name, u.surname, u.id;

-- name: AppointmentTeamByID :one
SELECT id, uuid, name FROM teams WHERE organization_id = $1 AND id = $2;

-- name: AppointmentTeamByUUID :one
SELECT id, uuid, name FROM teams WHERE organization_id = $1 AND uuid = $2;

-- name: AppointmentHostShifts :many
SELECT user_id, starts_at, ends_at FROM agent_shifts
WHERE organization_id = sqlc.arg(organization_id) AND user_id = ANY(sqlc.arg(user_ids)::bigint[])
  AND starts_at < sqlc.arg(range_to) AND ends_at > sqlc.arg(range_from)
ORDER BY user_id, starts_at;

-- name: AppointmentHostTimeOff :many
SELECT user_id, starts_at, ends_at FROM agent_time_off
WHERE organization_id = sqlc.arg(organization_id) AND user_id = ANY(sqlc.arg(user_ids)::bigint[])
  AND status = 'approved' AND starts_at < sqlc.arg(range_to) AND ends_at > sqlc.arg(range_from)
ORDER BY user_id, starts_at;

-- name: AppointmentHostBusy :many
-- Active bookings of the hosts that overlap the range, widened by their own
-- buffers. except_id skips the booking being rescheduled.
SELECT host_user_id, type_id, starts_at, ends_at, buffer_before, buffer_after
FROM appointments
WHERE organization_id = sqlc.arg(organization_id) AND host_user_id = ANY(sqlc.arg(user_ids)::bigint[])
  AND status IN ('scheduled','confirmed') AND id <> sqlc.arg(except_id)::bigint
  AND starts_at - make_interval(mins => buffer_before) < sqlc.arg(range_to)
  AND ends_at + make_interval(mins => buffer_after) > sqlc.arg(range_from)
ORDER BY host_user_id, starts_at;

-- name: AppointmentLockHost :exec
-- Serializes bookings of one host so two requests cannot take the same slot.
SELECT pg_advisory_xact_lock(hashtextextended('appointments:' || sqlc.arg(organization_id)::bigint::text || ':' || sqlc.arg(user_id)::bigint::text, 0));

-- name: AppointmentCreate :one
INSERT INTO appointments (
    organization_id, type_id, host_user_id, status, starts_at, ends_at, buffer_before, buffer_after, timezone,
    title, notes, location_kind, location_value, video_room_uuid, invitee_name, invitee_email, invitee_phone,
    intake_values, contact_id, company_id, conversation_id, source, booked_by_user_id, confirmed_at)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(type_id), sqlc.narg(host_user_id), sqlc.arg(status), sqlc.arg(starts_at),
    sqlc.arg(ends_at), sqlc.arg(buffer_before), sqlc.arg(buffer_after), sqlc.arg(timezone), sqlc.arg(title),
    sqlc.arg(notes), sqlc.arg(location_kind), sqlc.arg(location_value), sqlc.narg(video_room_uuid),
    sqlc.arg(invitee_name), sqlc.arg(invitee_email), sqlc.arg(invitee_phone), sqlc.arg(intake_values),
    sqlc.narg(contact_id), sqlc.narg(company_id), sqlc.narg(conversation_id), sqlc.arg(source),
    sqlc.narg(booked_by_user_id), CASE WHEN sqlc.arg(status)::text = 'confirmed' THEN NOW() END)
RETURNING *;

-- name: AppointmentForUpdate :one
SELECT * FROM appointments WHERE organization_id = $1 AND uuid = $2 FOR UPDATE;

-- name: AppointmentByID :one
SELECT * FROM appointments WHERE organization_id = $1 AND id = $2;

-- name: AppointmentReschedule :one
UPDATE appointments SET starts_at = sqlc.arg(starts_at), ends_at = sqlc.arg(ends_at),
    host_user_id = sqlc.narg(host_user_id), token_version = token_version + 1, sequence = sequence + 1,
    status = CASE WHEN status = 'confirmed' THEN 'confirmed' ELSE 'scheduled' END
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: AppointmentSetStatus :one
UPDATE appointments SET status = sqlc.arg(status),
    confirmed_at = CASE WHEN sqlc.arg(status)::text = 'confirmed' THEN COALESCE(confirmed_at, NOW()) ELSE confirmed_at END,
    completed_at = CASE WHEN sqlc.arg(status)::text IN ('completed','no_show') THEN NOW() ELSE completed_at END,
    cancelled_at = CASE WHEN sqlc.arg(status)::text = 'cancelled' THEN NOW() ELSE cancelled_at END,
    cancelled_by = CASE WHEN sqlc.arg(status)::text = 'cancelled' THEN sqlc.narg(cancelled_by)::text ELSE cancelled_by END,
    cancel_reason = CASE WHEN sqlc.arg(status)::text = 'cancelled' THEN sqlc.arg(cancel_reason)::text ELSE cancel_reason END,
    token_version = CASE WHEN sqlc.arg(status)::text = 'cancelled' THEN token_version + 1 ELSE token_version END,
    sequence = sequence + 1
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: AppointmentUpdateDetails :one
UPDATE appointments SET notes = sqlc.arg(notes), title = sqlc.arg(title), location_value = sqlc.arg(location_value)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: AppointmentCountHostDay :one
SELECT COUNT(*)::bigint FROM appointments
WHERE organization_id = sqlc.arg(organization_id) AND type_id = sqlc.arg(type_id)
  AND host_user_id = sqlc.arg(host_user_id) AND status IN ('scheduled','confirmed','completed')
  AND id <> sqlc.arg(except_id)::bigint
  AND starts_at >= sqlc.arg(day_from) AND starts_at < sqlc.arg(day_to);

-- name: AppointmentCountRecentByEmail :one
-- Public booking abuse limit: bookings one e-mail made recently.
SELECT COUNT(*)::bigint FROM appointments
WHERE organization_id = sqlc.arg(organization_id) AND lower(invitee_email) = lower(sqlc.arg(email)::text)
  AND source IN ('public','widget') AND created_at > sqlc.arg(since);

-- name: AppointmentList :many
-- Calendar / list rows with everything the UI shows. Filters are optional.
SELECT a.*, t.uuid AS type_uuid, t.name AS type_name, t.color AS type_color,
       hu.uuid AS host_uuid, COALESCE(TRIM(hu.name || ' ' || hu.surname), '')::text AS host_name,
       ct.uuid AS contact_uuid, COALESCE(ct.name, '')::text AS contact_name,
       co.uuid AS company_uuid, COALESCE(co.name, '')::text AS company_name,
       cv.uuid AS conversation_uuid, cv.number AS conversation_number
FROM appointments a
JOIN appointment_types t ON t.organization_id = a.organization_id AND t.id = a.type_id
LEFT JOIN users hu ON hu.id = a.host_user_id
LEFT JOIN contacts ct ON ct.organization_id = a.organization_id AND ct.id = a.contact_id
LEFT JOIN companies co ON co.organization_id = a.organization_id AND co.id = a.company_id
LEFT JOIN conversations cv ON cv.organization_id = a.organization_id AND cv.id = a.conversation_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(range_from)::timestamptz IS NULL OR a.ends_at > sqlc.narg(range_from)::timestamptz)
  AND (sqlc.narg(range_to)::timestamptz IS NULL OR a.starts_at < sqlc.narg(range_to)::timestamptz)
  AND (sqlc.narg(statuses)::text[] IS NULL OR a.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(host_ids)::bigint[] IS NULL OR a.host_user_id = ANY(sqlc.narg(host_ids)::bigint[]))
  AND (sqlc.narg(type_ids)::bigint[] IS NULL OR a.type_id = ANY(sqlc.narg(type_ids)::bigint[]))
  AND (sqlc.narg(contact_id)::bigint IS NULL OR a.contact_id = sqlc.narg(contact_id)::bigint)
  AND (sqlc.narg(company_id)::bigint IS NULL OR a.company_id = sqlc.narg(company_id)::bigint)
  AND (sqlc.narg(conversation_id)::bigint IS NULL OR a.conversation_id = sqlc.narg(conversation_id)::bigint)
ORDER BY CASE WHEN sqlc.arg(newest_first)::boolean THEN -EXTRACT(EPOCH FROM a.starts_at) ELSE EXTRACT(EPOCH FROM a.starts_at) END, a.id
LIMIT sqlc.arg(row_limit);

-- name: AppointmentDetail :one
SELECT a.*, t.uuid AS type_uuid, t.name AS type_name, t.color AS type_color,
       hu.uuid AS host_uuid, COALESCE(TRIM(hu.name || ' ' || hu.surname), '')::text AS host_name,
       ct.uuid AS contact_uuid, COALESCE(ct.name, '')::text AS contact_name,
       co.uuid AS company_uuid, COALESCE(co.name, '')::text AS company_name,
       cv.uuid AS conversation_uuid, cv.number AS conversation_number
FROM appointments a
JOIN appointment_types t ON t.organization_id = a.organization_id AND t.id = a.type_id
LEFT JOIN users hu ON hu.id = a.host_user_id
LEFT JOIN contacts ct ON ct.organization_id = a.organization_id AND ct.id = a.contact_id
LEFT JOIN companies co ON co.organization_id = a.organization_id AND co.id = a.company_id
LEFT JOIN conversations cv ON cv.organization_id = a.organization_id AND cv.id = a.conversation_id
WHERE a.organization_id = sqlc.arg(organization_id) AND a.uuid = sqlc.arg(uuid);

-- name: AppointmentDueReminders :many
-- Platform-wide reminder sweep: bookings whose reminder offset has passed in
-- the last 30 minutes and was not delivered for the current booking version.
SELECT a.id, a.organization_id, off.minutes::integer AS offset_minutes, a.token_version
FROM appointments a
LEFT JOIN appointment_settings s ON s.organization_id = a.organization_id
CROSS JOIN LATERAL unnest(COALESCE(s.reminder_minutes, '{1440,60}'::integer[])) AS off(minutes)
WHERE a.status IN ('scheduled','confirmed') AND COALESCE(s.enabled, TRUE)
  AND a.starts_at > sqlc.arg(now)::timestamptz
  AND a.starts_at - make_interval(mins => off.minutes) <= sqlc.arg(now)::timestamptz
  AND a.starts_at - make_interval(mins => off.minutes) > sqlc.arg(now)::timestamptz - interval '30 minutes'
  AND NOT EXISTS (SELECT 1 FROM appointment_reminders_sent r
      WHERE r.organization_id = a.organization_id AND r.appointment_id = a.id
        AND r.offset_minutes = off.minutes AND r.token_version = a.token_version)
ORDER BY a.starts_at
LIMIT 200;

-- name: AppointmentMarkReminder :execrows
INSERT INTO appointment_reminders_sent (organization_id, appointment_id, offset_minutes, token_version)
VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING;

-- name: AppointmentActiveSMSChannel :one
SELECT EXISTS (SELECT 1 FROM channel_instances
    WHERE organization_id = $1 AND channel_type = 'sms' AND status = 'active' AND deleted_at IS NULL)::boolean;

-- name: AppointmentAttachVideo :exec
UPDATE appointments SET video_room_uuid=$3, location_value=$4 WHERE organization_id=$1 AND id=$2;

-- name: AppointmentVideoDue :many
-- Global minutely worker; every subsequent mutation uses the returned org.
SELECT a.* FROM appointments a
JOIN video_rooms r ON r.organization_id=a.organization_id AND r.uuid=a.video_room_uuid
WHERE a.status IN ('scheduled','confirmed') AND r.scheduled_end_at <= $1
ORDER BY a.id LIMIT 500;

-- name: AppointmentVideoFinish :execrows
UPDATE appointments SET status=$3,completed_at=now(),sequence=sequence+1
WHERE organization_id=$1 AND id=$2 AND status IN ('scheduled','confirmed') AND sequence=sqlc.arg(expected_sequence)::integer;

-- name: AppointmentLock :one
SELECT * FROM appointments WHERE organization_id=$1 AND id=$2 FOR UPDATE;
