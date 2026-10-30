-- name: SurveyCreateTemplate :one
INSERT INTO survey_templates(organization_id,name,kind,question,on_resolve,enabled) VALUES($1,$2,$3,$4,$5,$6) RETURNING *;
-- name: SurveyUpdateTemplate :one
UPDATE survey_templates SET name=$3,question=$4,on_resolve=$5,enabled=$6 WHERE organization_id=$1 AND uuid=$2 RETURNING *;
-- name: SurveyTemplates :many
SELECT uuid,name,kind,question,on_resolve,enabled,created_at,updated_at FROM survey_templates WHERE organization_id=$1 ORDER BY id DESC LIMIT 100;
-- name: SurveyResolveTemplates :many
SELECT uuid FROM survey_templates WHERE organization_id=$1 AND on_resolve AND enabled ORDER BY id;
-- name: SurveyInvite :one
INSERT INTO survey_invitations(organization_id,template_id,conversation_id,kind,question,period_key)
SELECT t.organization_id,t.id,c.id,t.kind,t.question,sqlc.arg(period_key) FROM survey_templates t
JOIN conversations c ON c.organization_id=t.organization_id AND c.uuid=sqlc.arg(conversation_uuid) AND c.deleted_at IS NULL
WHERE t.organization_id=sqlc.arg(organization_id) AND t.uuid=sqlc.arg(template_uuid) AND t.enabled
AND (t.kind<>'csat' OR c.status IN ('resolved','closed'))
ON CONFLICT(organization_id,template_id,conversation_id,period_key) DO UPDATE SET period_key=excluded.period_key
RETURNING *;
-- name: SurveyInvitation :one
SELECT * FROM survey_invitations WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: SurveyAnswer :one
INSERT INTO survey_responses(organization_id,invitation_id,kind,score,comment) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(organization_id,invitation_id) DO UPDATE SET invitation_id=excluded.invitation_id RETURNING score,comment;
-- name: SurveyMarkAnswered :exec
UPDATE survey_invitations SET answered_at=COALESCE(answered_at,now()) WHERE organization_id=$1 AND id=$2;
-- name: SurveyResults :one
WITH answers AS (
 SELECT 'csat'::text AS kind,cr.score,cr.comment,cr.created_at,c.assignee_user_id,c.company_id,c.channel_type
 FROM conversation_ratings cr JOIN conversations c ON c.organization_id=cr.organization_id AND c.id=cr.conversation_id
 WHERE cr.organization_id=sqlc.arg(organization_id)
 UNION ALL
 SELECT sr.kind,sr.score,sr.comment,sr.created_at,c.assignee_user_id,c.company_id,c.channel_type
 FROM survey_responses sr JOIN survey_invitations i ON i.organization_id=sr.organization_id AND i.id=sr.invitation_id
 JOIN conversations c ON c.organization_id=i.organization_id AND c.id=i.conversation_id
 WHERE sr.organization_id=sqlc.arg(organization_id)
), summary AS (
 SELECT kind,count(*) AS responses,avg(score) AS average,
 CASE WHEN kind='nps' THEN 100.0*(count(*) FILTER(WHERE score>=9)-count(*) FILTER(WHERE score<=6))/count(*) END AS nps,
 CASE WHEN kind='csat' THEN 100.0*count(*) FILTER(WHERE score>=4)/count(*) END AS satisfaction_percent
 FROM answers WHERE created_at>=sqlc.arg(from_at)::timestamptz AND created_at<sqlc.arg(to_at)::timestamptz
 GROUP BY kind
)
SELECT COALESCE(jsonb_agg(to_jsonb(summary)),'[]'::jsonb)::jsonb AS result FROM summary;
