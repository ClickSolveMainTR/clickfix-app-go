-- name: StatusSavePage :one
INSERT INTO status_pages(organization_id,slug,title,custom_domain,published) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(organization_id) DO UPDATE SET slug=excluded.slug,title=excluded.title,custom_domain=excluded.custom_domain,published=excluded.published RETURNING *;
-- name: StatusPage :one
SELECT * FROM status_pages WHERE organization_id=$1;
-- name: StatusPublicPage :one
-- Public slug lookup only exposes published pages of active organizations; returned organization_id scopes all subsequent reads.
SELECT p.* FROM status_pages p JOIN organizations o ON o.id=p.organization_id
WHERE p.slug=$1 AND p.published AND o.deleted_at IS NULL AND o.status='active';
-- name: StatusCreateComponent :one
INSERT INTO status_components(organization_id,name,group_name,description,sort_order) VALUES($1,$2,$3,$4,$5) RETURNING *;
-- name: StatusUpdateComponent :one
UPDATE status_components SET name=$3,group_name=$4,description=$5,sort_order=$6 WHERE organization_id=$1 AND uuid=$2 RETURNING *;
-- name: StatusComponents :one
SELECT COALESCE(jsonb_agg(jsonb_build_object('uuid',c.uuid,'name',c.name,'group_name',c.group_name,'description',c.description,'sort_order',c.sort_order,
 'status',COALESCE((SELECT i.severity FROM status_incident_components ic JOIN status_incidents i ON i.organization_id=ic.organization_id AND i.id=ic.incident_id
 WHERE ic.organization_id=c.organization_id AND ic.component_id=c.id AND i.status<>'resolved' AND i.public AND i.started_at<=now()
 AND (i.kind<>'maintenance' OR i.ends_at>now()) ORDER BY CASE i.severity WHEN 'major_outage' THEN 4 WHEN 'partial_outage' THEN 3 WHEN 'degraded' THEN 2 ELSE 1 END DESC LIMIT 1),'operational')) ORDER BY c.sort_order,c.id),'[]'::jsonb)::jsonb AS result
FROM status_components c WHERE c.organization_id=$1;
-- name: StatusCreateIncident :one
INSERT INTO status_incidents(organization_id,title,kind,severity,status,public,started_at,ends_at,notification_body,notification_template,postmortem)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING *;
-- name: StatusIncident :one
SELECT * FROM status_incidents WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: StatusSetIncidentState :one
UPDATE status_incidents SET status=$3,resolved_at=CASE WHEN $3='resolved' THEN COALESCE(resolved_at,now()) ELSE resolved_at END,
 postmortem=CASE WHEN sqlc.arg(postmortem)::text<>'' THEN sqlc.arg(postmortem)::text ELSE postmortem END
WHERE organization_id=$1 AND uuid=$2 RETURNING *;
-- name: StatusLinkComponent :execrows
INSERT INTO status_incident_components(organization_id,incident_id,component_id)
SELECT c.organization_id,sqlc.arg(incident_id),c.id FROM status_components c WHERE c.organization_id=sqlc.arg(organization_id) AND c.uuid=sqlc.arg(component_uuid)
ON CONFLICT DO NOTHING;
-- name: StatusAddUpdate :one
INSERT INTO status_incident_updates(organization_id,incident_id,status,body) VALUES($1,$2,$3,$4) RETURNING uuid;
-- name: StatusIncidents :one
SELECT COALESCE(jsonb_agg(jsonb_build_object('uuid',i.uuid,'title',i.title,'kind',i.kind,'severity',i.severity,'status',i.status,'public',i.public,
 'started_at',i.started_at,'ends_at',i.ends_at,'resolved_at',i.resolved_at,
 'postmortem',CASE WHEN NOT sqlc.arg(public_only)::bool THEN i.postmortem ELSE NULL END,
 'components',COALESCE((SELECT jsonb_agg(c.uuid) FROM status_incident_components ic JOIN status_components c ON c.organization_id=ic.organization_id AND c.id=ic.component_id WHERE ic.organization_id=i.organization_id AND ic.incident_id=i.id),'[]'::jsonb),
 'updates',COALESCE((SELECT jsonb_agg(jsonb_build_object('uuid',u.uuid,'status',u.status,'body',u.body,'created_at',u.created_at) ORDER BY u.id) FROM status_incident_updates u WHERE u.organization_id=i.organization_id AND u.incident_id=i.id),'[]'::jsonb)) ORDER BY i.started_at DESC),'[]'::jsonb)::jsonb AS result
FROM (SELECT * FROM status_incidents si WHERE si.organization_id=sqlc.arg(organization_id) AND (NOT sqlc.arg(public_only)::bool OR si.public) ORDER BY si.started_at DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset)) i;
-- name: StatusUptime :one
-- Calendar-day UTC availability; union outage ranges prevents double counting overlapping incidents.
WITH days AS (SELECT generate_series(date_trunc('day',sqlc.arg(from_at)::timestamptz AT TIME ZONE 'UTC'),date_trunc('day',(sqlc.arg(to_at)::timestamptz-interval '1 microsecond') AT TIME ZONE 'UTC'),interval '1 day') AT TIME ZONE 'UTC' AS day),
 ranges AS (
 SELECT c.uuid,c.name,d.day,range_agg(tstzrange(GREATEST(i.started_at,d.day),LEAST(COALESCE(i.resolved_at,now()),d.day+interval '24 hours',sqlc.arg(to_at)::timestamptz),'[)')) FILTER(WHERE i.id IS NOT NULL) AS outages
 FROM status_components c CROSS JOIN days d
 LEFT JOIN status_incident_components ic ON ic.organization_id=c.organization_id AND ic.component_id=c.id
 LEFT JOIN status_incidents i ON i.organization_id=ic.organization_id AND i.id=ic.incident_id AND i.public AND i.kind='incident' AND i.severity IN ('partial_outage','major_outage')
 AND i.started_at<LEAST(d.day+interval '24 hours',sqlc.arg(to_at)::timestamptz) AND COALESCE(i.resolved_at,now())>d.day
 WHERE c.organization_id=sqlc.arg(organization_id) GROUP BY c.uuid,c.name,d.day
), history AS (
 SELECT uuid,name,to_char(day AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') AS day,100.0*(1-COALESCE((SELECT sum(extract(epoch FROM upper(r)-lower(r))) FROM unnest(outages) r),0)/extract(epoch FROM LEAST(day+interval '24 hours',sqlc.arg(to_at)::timestamptz)-day)) AS uptime_percent FROM ranges
)
SELECT COALESCE(jsonb_agg(to_jsonb(history) ORDER BY day,uuid),'[]'::jsonb)::jsonb AS result FROM history;
-- name: StatusLinkConversation :execrows
INSERT INTO status_incident_conversations(organization_id,incident_id,conversation_id)
SELECT c.organization_id,sqlc.arg(incident_id),c.id FROM conversations c WHERE c.organization_id=sqlc.arg(organization_id) AND c.uuid=sqlc.arg(conversation_uuid) AND c.deleted_at IS NULL
ON CONFLICT(organization_id,incident_id,conversation_id) DO UPDATE SET conversation_id=excluded.conversation_id;
-- name: StatusPrepareDeliveries :exec
INSERT INTO status_incident_deliveries(organization_id,incident_id,contact_id,conversation_id)
SELECT DISTINCT ON(c.contact_id) c.organization_id,ic.incident_id,c.contact_id,c.id
FROM status_incident_conversations ic JOIN conversations c ON c.organization_id=ic.organization_id AND c.id=ic.conversation_id
JOIN contacts ct ON ct.organization_id=c.organization_id AND ct.id=c.contact_id AND ct.deleted_at IS NULL
WHERE ic.organization_id=$1 AND ic.incident_id=$2 AND c.deleted_at IS NULL AND c.channel_instance_id IS NOT NULL
ORDER BY c.contact_id,c.last_message_at DESC,c.id DESC
ON CONFLICT(organization_id,incident_id,contact_id) DO NOTHING;

-- name: StatusNotificationRecipients :many
SELECT c.uuid,c.contact_id,c.channel_type
FROM status_incident_deliveries d JOIN conversations c ON c.organization_id=d.organization_id AND c.id=d.conversation_id
JOIN contacts ct ON ct.organization_id=c.organization_id AND ct.id=c.contact_id AND ct.deleted_at IS NULL
WHERE d.organization_id=$1 AND d.incident_id=$2 AND c.deleted_at IS NULL AND c.channel_instance_id IS NOT NULL AND c.contact_id>sqlc.arg(after_contact)::bigint
ORDER BY c.contact_id LIMIT 500;
-- name: StatusSubscribe :one
INSERT INTO status_subscriptions(organization_id,email) VALUES($1,$2)
ON CONFLICT(organization_id,email) DO UPDATE SET expires_at=CASE WHEN status_subscriptions.expires_at<now() THEN now()+interval '1 day' ELSE status_subscriptions.expires_at END
RETURNING *;
-- name: StatusSubscription :one
SELECT * FROM status_subscriptions WHERE organization_id=$1 AND uuid=$2;
-- name: StatusConfirmSubscription :execrows
UPDATE status_subscriptions SET confirmed_at=now(),unsubscribed_at=NULL WHERE organization_id=$1 AND uuid=$2 AND expires_at>now();
-- name: StatusUnsubscribe :exec
UPDATE status_subscriptions SET unsubscribed_at=now() WHERE organization_id=$1 AND uuid=$2;
-- name: StatusSubscribers :many
SELECT id,uuid,email FROM status_subscriptions WHERE organization_id=$1 AND confirmed_at IS NOT NULL AND unsubscribed_at IS NULL AND id>sqlc.arg(after_id)::bigint ORDER BY id LIMIT 500;

-- name: StatusIncidentCount :one
SELECT count(*) FROM status_incidents WHERE organization_id=$1;

-- name: StatusNotificationPreview :one
SELECT jsonb_build_object('contacts',COALESCE(sum(n),0),'channels',COALESCE(jsonb_object_agg(channel_type,n),'{}'::jsonb))::jsonb AS result
FROM (SELECT channel_type,count(*) AS n FROM (
SELECT DISTINCT ON(c.contact_id) c.contact_id,c.channel_type
FROM status_incident_conversations ic JOIN conversations c ON c.organization_id=ic.organization_id AND c.id=ic.conversation_id
JOIN contacts ct ON ct.organization_id=c.organization_id AND ct.id=c.contact_id AND ct.deleted_at IS NULL
WHERE ic.organization_id=$1 AND ic.incident_id=$2 AND c.deleted_at IS NULL AND c.channel_instance_id IS NOT NULL
ORDER BY c.contact_id,c.last_message_at DESC,c.id DESC) recipients GROUP BY channel_type) counts;
