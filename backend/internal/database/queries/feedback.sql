-- name: FeedbackSettings :one
SELECT COALESCE((SELECT public_enabled FROM feedback_settings WHERE organization_id=$1),false)::boolean AS public_enabled;
-- name: FeedbackSaveSettings :exec
INSERT INTO feedback_settings(organization_id,public_enabled) VALUES($1,$2) ON CONFLICT(organization_id) DO UPDATE SET public_enabled=excluded.public_enabled;
-- name: FeedbackPublicOrg :one
SELECT o.id,o.uuid,o.name FROM organizations o JOIN feedback_settings s ON s.organization_id=o.id AND s.public_enabled WHERE o.uuid=$1 AND o.status='active' AND o.deleted_at IS NULL;
-- name: FeedbackCreate :one
INSERT INTO feature_requests(organization_id,title,body,is_public,created_by) VALUES($1,$2,$3,$4,$5) RETURNING *;
-- name: FeedbackLock :one
SELECT * FROM feature_requests WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: FeedbackGet :one
SELECT * FROM feature_requests WHERE organization_id=$1 AND uuid=$2;
-- name: FeedbackStatus :one
UPDATE feature_requests SET status=$3,is_public=$4 WHERE organization_id=$1 AND id=$2 AND merged_into_id IS NULL RETURNING *;
-- name: FeedbackMerge :exec
UPDATE feature_requests SET status='merged',is_public=false,merged_into_id=$3 WHERE organization_id=$1 AND id=$2;
-- name: FeedbackMergeVotes :exec
INSERT INTO feature_votes(organization_id,request_id,contact_id)
SELECT v.organization_id,$3,v.contact_id FROM feature_votes v WHERE v.organization_id=$1 AND v.request_id=$2 ON CONFLICT DO NOTHING;
-- name: FeedbackDeleteVotes :exec
DELETE FROM feature_votes WHERE organization_id=$1 AND request_id=$2;
-- name: FeedbackMergeSources :exec
INSERT INTO feature_sources(organization_id,request_id,conversation_id)
SELECT v.organization_id,$3,v.conversation_id FROM feature_sources v WHERE v.organization_id=$1 AND v.request_id=$2 ON CONFLICT DO NOTHING;
-- name: FeedbackDeleteSources :exec
DELETE FROM feature_sources WHERE organization_id=$1 AND request_id=$2;
-- name: FeedbackMergeComments :exec
UPDATE feedback_comments SET request_id=$3 WHERE organization_id=$1 AND request_id=$2;
-- name: FeedbackVote :exec
INSERT INTO feature_votes(organization_id,request_id,contact_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING;
-- name: FeedbackUnvote :exec
DELETE FROM feature_votes WHERE organization_id=$1 AND request_id=$2 AND contact_id=$3;
-- name: FeedbackSource :exec
INSERT INTO feature_sources(organization_id,request_id,conversation_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING;
-- name: FeedbackComment :one
INSERT INTO feedback_comments(organization_id,request_id,contact_id,body) VALUES($1,$2,$3,$4) RETURNING uuid;
-- name: FeedbackModerate :execrows
UPDATE feedback_comments SET approved=$3 WHERE organization_id=$1 AND uuid=$2;
-- name: FeedbackComments :many
SELECT uuid,body,created_at,approved FROM feedback_comments WHERE organization_id=$1 AND request_id=$2 AND (NOT sqlc.arg(public_only)::boolean OR approved) ORDER BY id LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: FeedbackList :many
SELECT f.uuid,f.title,f.body,f.status,f.is_public,f.created_at,f.updated_at,
 (SELECT count(*) FROM feature_votes v WHERE v.organization_id=f.organization_id AND v.request_id=f.id)::bigint AS votes,
 (SELECT count(*) FROM feature_requests c WHERE c.organization_id=f.organization_id AND c.status<>'merged' AND (sqlc.arg(q)::text='' OR c.title ILIKE '%'||sqlc.arg(q)||'%') AND (NOT sqlc.arg(public_only)::boolean OR c.is_public))::bigint AS total
FROM feature_requests f WHERE f.organization_id=sqlc.arg(organization_id) AND f.status<>'merged'
 AND (NOT sqlc.arg(public_only)::boolean OR f.is_public) AND (sqlc.arg(q)::text='' OR f.title ILIKE '%'||sqlc.arg(q)||'%')
ORDER BY CASE WHEN sqlc.arg(sort)::text='-revenue' THEN (
 SELECT COALESCE(sum(r.mrr_minor),0) FROM company_revenue r WHERE r.organization_id=f.organization_id AND r.currency=sqlc.arg(currency)::text
 AND EXISTS(SELECT 1 FROM feature_votes v JOIN contacts c ON c.organization_id=v.organization_id AND c.id=v.contact_id WHERE v.organization_id=f.organization_id AND v.request_id=f.id AND c.company_id=r.company_id)
 ) END DESC, CASE WHEN sqlc.arg(sort)::text='-votes' THEN (SELECT count(*) FROM feature_votes v WHERE v.organization_id=f.organization_id AND v.request_id=f.id) END DESC,f.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: FeedbackRevenue :many
SELECT r.currency, sum(r.mrr_minor)::bigint AS mrr_minor FROM company_revenue r
WHERE r.organization_id=$1 AND EXISTS(SELECT 1 FROM feature_votes v JOIN contacts c ON c.organization_id=v.organization_id AND c.id=v.contact_id WHERE v.organization_id=r.organization_id AND v.request_id=$2 AND c.company_id=r.company_id)
GROUP BY r.currency ORDER BY r.currency;
-- name: FeedbackVoters :many
SELECT c.id,c.email FROM feature_votes v JOIN contacts c ON c.organization_id=v.organization_id AND c.id=v.contact_id WHERE v.organization_id=$1 AND v.request_id=$2 AND c.deleted_at IS NULL ORDER BY c.id;
-- name: FeedbackContact :one
SELECT c.id FROM contact_identities i JOIN contacts c ON c.organization_id=i.organization_id AND c.id=i.contact_id WHERE i.organization_id=$1 AND i.channel_instance_id=$2 AND i.external_id=$3 AND c.deleted_at IS NULL;
-- name: ChangelogCreate :one
INSERT INTO changelog_entries(organization_id,title,body,tags,request_id,published_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING *;
-- name: ChangelogList :many
SELECT e.uuid,e.title,e.body,e.tags,e.published_at,e.created_at,
 (SELECT count(*) FROM changelog_entries c WHERE c.organization_id=e.organization_id AND (NOT sqlc.arg(public_only)::boolean OR c.published_at<=now()))::bigint AS total
FROM changelog_entries e WHERE e.organization_id=sqlc.arg(organization_id) AND (NOT sqlc.arg(public_only)::boolean OR e.published_at<=now()) ORDER BY e.published_at DESC NULLS LAST,e.id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: ChangelogPublish :one
UPDATE changelog_entries SET published_at=now() WHERE organization_id=$1 AND uuid=$2 AND published_at IS NULL RETURNING *;
-- name: FeedbackVoterConversation :one
SELECT uuid FROM conversations WHERE organization_id=$1 AND contact_id=$2 AND channel_instance_id IS NOT NULL AND deleted_at IS NULL ORDER BY id DESC LIMIT 1;
-- name: FeedbackCount :one
SELECT count(*) FROM feature_requests f WHERE f.organization_id=$1 AND f.status<>'merged' AND (NOT sqlc.arg(public_only)::boolean OR f.is_public) AND (sqlc.arg(q)::text='' OR f.title ILIKE '%'||sqlc.arg(q)||'%');
-- name: ChangelogCount :one
SELECT count(*) FROM changelog_entries WHERE organization_id=$1 AND (NOT sqlc.arg(public_only)::boolean OR published_at<=now());
-- name: FeedbackCommentCount :one
SELECT count(*) FROM feedback_comments WHERE organization_id=$1 AND request_id=$2 AND (NOT sqlc.arg(public_only)::boolean OR approved);
-- name: RoadmapCreate :one
INSERT INTO roadmap_entries(organization_id,title,body,status,source_issue_url,is_public) VALUES($1,$2,$3,$4,$5,$6) RETURNING *;
-- name: RoadmapUpdate :one
UPDATE roadmap_entries SET title=$3,body=$4,status=$5,source_issue_url=$6,is_public=$7 WHERE organization_id=$1 AND uuid=$2 RETURNING *;
-- name: RoadmapList :many
SELECT f.uuid,f.title,f.body,f.status,f.created_at,f.updated_at,'feature_request'::text AS source_type,''::text AS source_issue_url
FROM feature_requests f WHERE f.organization_id=sqlc.arg(organization_id) AND f.is_public AND f.status IN ('planned','in_progress','released')
UNION ALL
SELECT r.uuid,r.title,r.body,r.status,r.created_at,r.updated_at,'issue'::text AS source_type,r.source_issue_url
FROM roadmap_entries r WHERE r.organization_id=sqlc.arg(organization_id) AND (NOT sqlc.arg(public_only)::boolean OR r.is_public)
ORDER BY created_at DESC,uuid LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: RoadmapCount :one
SELECT ((SELECT count(*) FROM feature_requests f WHERE f.organization_id=$1 AND f.is_public AND f.status IN ('planned','in_progress','released'))+(SELECT count(*) FROM roadmap_entries r WHERE r.organization_id=$1 AND (NOT sqlc.arg(public_only)::boolean OR r.is_public)))::bigint AS total;
