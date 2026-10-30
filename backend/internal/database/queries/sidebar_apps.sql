-- name: SidebarAppCreate :one
INSERT INTO sidebar_apps(organization_id,name,url,icon,placement,scopes,enabled,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *;
-- name: SidebarAppUpdate :one
UPDATE sidebar_apps SET name=$3,url=$4,icon=$5,placement=$6,scopes=$7,enabled=$8,revision=revision+1 WHERE organization_id=$1 AND uuid=$2 AND revision=$9 RETURNING *;
-- name: SidebarAppGet :one
SELECT * FROM sidebar_apps WHERE organization_id=$1 AND uuid=$2;
-- name: SidebarAppList :many
SELECT * FROM sidebar_apps WHERE organization_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: SidebarAppCount :one
SELECT count(*) FROM sidebar_apps WHERE organization_id=$1;
-- name: SidebarAppDelete :execrows
DELETE FROM sidebar_apps WHERE organization_id=$1 AND uuid=$2;
