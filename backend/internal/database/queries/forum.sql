-- name: ForumCategories :many
SELECT uuid,slug,name,created_at,updated_at FROM forum_categories WHERE organization_id=$1 ORDER BY name,id;
-- name: ForumCategory :one
SELECT * FROM forum_categories WHERE organization_id=$1 AND slug=$2;
-- name: ForumCreateCategory :one
INSERT INTO forum_categories(organization_id,slug,name) VALUES($1,$2,$3) RETURNING uuid,slug,name,created_at,updated_at;
-- name: ForumPost :one
SELECT * FROM forum_posts WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: ForumTopicBySlug :one
SELECT * FROM forum_posts WHERE organization_id=$1 AND slug=$2 AND topic_id IS NULL AND status='published';
-- name: ForumTopics :many
SELECT p.uuid,p.slug,p.title,p.body,p.created_at,p.updated_at,c.slug AS category_slug,
 (SELECT count(*) FROM forum_votes v WHERE v.organization_id=p.organization_id AND v.post_id=p.id)::bigint AS votes,
 a.uuid AS accepted_reply_uuid
FROM forum_posts p JOIN forum_categories c ON c.organization_id=p.organization_id AND c.id=p.category_id
LEFT JOIN forum_posts a ON a.organization_id=p.organization_id AND a.id=p.accepted_reply_id AND a.status='published'
WHERE p.organization_id=$1 AND p.topic_id IS NULL AND p.status='published'
AND (sqlc.arg(category)::text='' OR c.slug=sqlc.arg(category))
AND (sqlc.arg(q)::text='' OR p.title ILIKE '%'||sqlc.arg(q)||'%')
ORDER BY p.created_at DESC,p.id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: ForumTopicCount :one
SELECT count(*) FROM forum_posts p JOIN forum_categories c ON c.organization_id=p.organization_id AND c.id=p.category_id
WHERE p.organization_id=$1 AND p.topic_id IS NULL AND p.status='published'
AND (sqlc.arg(category)::text='' OR c.slug=sqlc.arg(category)) AND (sqlc.arg(q)::text='' OR p.title ILIKE '%'||sqlc.arg(q)||'%');
-- name: ForumReplies :many
SELECT p.uuid,p.body,p.created_at,p.updated_at,
 (SELECT count(*) FROM forum_votes v WHERE v.organization_id=p.organization_id AND v.post_id=p.id)::bigint AS votes
FROM forum_posts p WHERE p.organization_id=$1 AND p.topic_id=$2 AND p.status='published'
ORDER BY p.created_at,p.id LIMIT $3 OFFSET $4;
-- name: ForumReplyCount :one
SELECT count(*) FROM forum_posts WHERE organization_id=$1 AND topic_id=$2 AND status='published';
-- name: ForumCreatePost :one
INSERT INTO forum_posts(organization_id,category_id,topic_id,contact_id,slug,title,body,flag_reason,conversation_id)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING *;
-- name: ForumModeration :many
SELECT * FROM forum_posts WHERE organization_id=$1 AND status='pending' ORDER BY created_at,id LIMIT $2 OFFSET $3;
-- name: ForumModerationCount :one
SELECT count(*) FROM forum_posts WHERE organization_id=$1 AND status='pending';
-- name: ForumModerate :one
UPDATE forum_posts SET status=$3 WHERE organization_id=$1 AND uuid=$2 AND status='pending' RETURNING *;
-- name: ForumFlag :exec
UPDATE forum_posts SET flag_reason=$3 WHERE organization_id=$1 AND uuid=$2 AND status='pending';
-- name: ForumAccept :exec
UPDATE forum_posts SET accepted_reply_id=$3 WHERE organization_id=$1 AND id=$2 AND topic_id IS NULL;
-- name: ForumVote :exec
INSERT INTO forum_votes(organization_id,post_id,contact_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING;
-- name: ForumUnvote :exec
DELETE FROM forum_votes WHERE organization_id=$1 AND post_id=$2 AND contact_id=$3;
-- name: ForumSubscribe :exec
INSERT INTO forum_subscriptions(organization_id,topic_id,contact_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING;
-- name: ForumUnsubscribe :exec
DELETE FROM forum_subscriptions WHERE organization_id=$1 AND topic_id=$2 AND contact_id=$3;
-- name: ForumNotify :exec
INSERT INTO forum_notifications(organization_id,post_id,contact_id)
SELECT p.organization_id,p.id,s.contact_id FROM forum_posts p
JOIN forum_subscriptions s ON s.organization_id=p.organization_id AND s.topic_id=p.topic_id
WHERE p.organization_id=$1 AND p.id=$2 AND p.status='published' AND s.contact_id<>p.contact_id
ON CONFLICT DO NOTHING;
-- name: ForumNotifications :many
SELECT n.uuid,n.created_at,n.read_at,p.uuid AS reply_uuid,t.slug AS topic_slug,t.title
FROM forum_notifications n JOIN forum_posts p ON p.organization_id=n.organization_id AND p.id=n.post_id AND p.status='published'
JOIN forum_posts t ON t.organization_id=p.organization_id AND t.id=p.topic_id AND t.status='published'
WHERE n.organization_id=$1 AND n.contact_id=$2 ORDER BY n.created_at DESC,n.id DESC LIMIT $3 OFFSET $4;
-- name: ForumReadNotification :execrows
UPDATE forum_notifications SET read_at=now() WHERE organization_id=$1 AND contact_id=$2 AND uuid=$3;
-- name: ForumLinkConversation :exec
UPDATE forum_posts SET conversation_id=$3 WHERE organization_id=$1 AND id=$2 AND topic_id IS NULL;
-- name: ForumCreateConversation :one
INSERT INTO conversations(organization_id,number,channel_type,channel_instance_id,contact_id,subject,source,assignee_user_id,last_message_preview,message_count)
SELECT p.organization_id,sqlc.arg(number)::bigint,i.channel_type,i.id,p.contact_id,p.title,'inbound',sqlc.arg(actor)::bigint,left(p.body,500),1
FROM forum_posts p JOIN channel_instances i ON i.organization_id=p.organization_id AND i.uuid=sqlc.arg(inbox_uuid)::uuid AND i.status='active' AND i.deleted_at IS NULL
WHERE p.organization_id=$1 AND p.id=$2 AND p.topic_id IS NULL RETURNING *;
-- name: ForumCopyMessage :exec
INSERT INTO messages(organization_id,conversation_id,direction,sender_type,sender_contact_id,kind,content,status)
SELECT p.organization_id,sqlc.arg(conversation_id)::bigint,'in','contact',p.contact_id,'text',p.body,'received'
FROM forum_posts p WHERE p.organization_id=$1 AND p.id=$2;
-- name: ForumConversationTopic :one
SELECT * FROM forum_posts WHERE organization_id=$1 AND conversation_id=$2 AND topic_id IS NULL;
-- name: ForumTopicByID :one
SELECT * FROM forum_posts WHERE organization_id=$1 AND id=$2 AND topic_id IS NULL;
-- name: ForumPublicAnswer :one
SELECT uuid,body,created_at,updated_at FROM forum_posts WHERE organization_id=$1 AND id=$2 AND topic_id IS NOT NULL AND status='published';
-- name: ForumSitemap :many
SELECT slug,updated_at FROM forum_posts WHERE organization_id=$1 AND topic_id IS NULL AND status='published'
ORDER BY updated_at DESC,id DESC LIMIT $2 OFFSET $3;
