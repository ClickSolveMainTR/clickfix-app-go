-- name: AIProcedureCreate :one
INSERT INTO ai_procedures(organization_id,intent,locale,title,draft) VALUES($1,$2,$3,$4,$5) RETURNING *;
-- name: AIProcedureUpdate :one
UPDATE ai_procedures SET title=$3,draft=$4,revision=revision+1 WHERE organization_id=$1 AND uuid=$2 AND revision=$5 AND intent=$6 AND locale=$7 RETURNING *;
-- name: AIProcedurePublish :one
UPDATE ai_procedures SET published=CASE WHEN sqlc.arg(publish)::boolean THEN draft ELSE NULL END,
published_revision=CASE WHEN sqlc.arg(publish)::boolean THEN revision ELSE NULL END
WHERE organization_id=$1 AND uuid=$2 AND revision=sqlc.arg(expected_revision) RETURNING *;
-- name: AIProcedures :many
SELECT * FROM ai_procedures WHERE organization_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: AIProcedureCount :one
SELECT count(*) FROM ai_procedures WHERE organization_id=$1;
-- name: AIProcedurePublished :one
SELECT * FROM ai_procedures WHERE organization_id=$1 AND intent=$2 AND locale=$3 AND published IS NOT NULL;
