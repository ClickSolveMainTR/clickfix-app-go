-- name: QACreateScorecard :one
INSERT INTO qa_scorecards(organization_id,name,criteria,threshold,auto_score) VALUES($1,$2,$3,$4,$5) RETURNING *;
-- name: QAGetScorecard :one
SELECT * FROM qa_scorecards WHERE organization_id=$1 AND uuid=$2;
-- name: QAListScorecards :many
SELECT * FROM qa_scorecards WHERE organization_id=$1 ORDER BY id LIMIT 100;
-- name: QAInsertReview :one
INSERT INTO qa_reviews(organization_id,scorecard_id,conversation_id,agent_id,reviewer_id,source,rubric,scores,score,critical_failure,needs_review,ai_run_uuid)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING *;
-- name: QAListReviews :many
SELECT r.uuid,c.uuid AS conversation_uuid,u.uuid AS agent_uuid,r.source,r.score,r.critical_failure,r.needs_review,r.created_at,r.rubric,r.scores
FROM qa_reviews r JOIN conversations c ON c.organization_id=r.organization_id AND c.id=r.conversation_id LEFT JOIN users u ON u.id=r.agent_id
WHERE r.organization_id=$1 AND (NOT sqlc.arg(pending_only)::boolean OR r.needs_review AND r.reviewed_at IS NULL)
ORDER BY r.id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: QAAcknowledge :execrows
UPDATE qa_reviews SET reviewed_at=now(),reviewer_id=$3 WHERE organization_id=$1 AND uuid=$2 AND source='ai' AND reviewed_at IS NULL;
-- name: QAReport :many
SELECT u.uuid AS agent_uuid,count(*)::bigint AS reviews,avg(r.score)::numeric(6,2) AS average_score,
 count(*) FILTER(WHERE r.critical_failure)::bigint AS critical_failures,
 count(*) FILTER(WHERE r.needs_review AND r.reviewed_at IS NULL)::bigint AS pending
FROM qa_reviews r LEFT JOIN users u ON u.id=r.agent_id WHERE r.organization_id=$1 AND r.created_at>=$2 AND r.created_at<$3 GROUP BY u.uuid ORDER BY u.uuid;
-- name: QAAutoCards :many
SELECT * FROM qa_scorecards WHERE organization_id=$1 AND auto_score ORDER BY id;
-- name: QAExistingAuto :one
SELECT uuid FROM qa_reviews WHERE organization_id=$1 AND scorecard_id=$2 AND conversation_id=$3 AND source='ai';
-- name: QALockAutomatic :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(organization_id)::bigint::text || ':qa:' || sqlc.arg(scorecard)::uuid::text || ':' || sqlc.arg(conversation)::uuid::text,0));
-- name: QACountReviews :one
SELECT count(*) FROM qa_reviews WHERE organization_id=$1 AND (NOT sqlc.arg(pending_only)::boolean OR needs_review AND reviewed_at IS NULL);
-- name: QAUpdateScorecard :one
UPDATE qa_scorecards SET name=$3,criteria=$4,threshold=$5,auto_score=$6 WHERE organization_id=$1 AND uuid=$2 RETURNING *;
