-- CF-217: history pipeline (archive, runs, clusters, decision memory).

-- name: HistorySettingsGet :one
SELECT * FROM history_settings WHERE organization_id=$1;

-- name: HistorySettingsSave :one
INSERT INTO history_settings(organization_id,evidence_threshold,cluster_similarity,resolution_similarity,rejection_similarity,
 raw_retention_hours,archive_retention_days,nightly_enabled,nightly_consent_analysis,nightly_consent_training,updated_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT(organization_id) DO UPDATE SET evidence_threshold=EXCLUDED.evidence_threshold,cluster_similarity=EXCLUDED.cluster_similarity,
 resolution_similarity=EXCLUDED.resolution_similarity,rejection_similarity=EXCLUDED.rejection_similarity,
 raw_retention_hours=EXCLUDED.raw_retention_hours,archive_retention_days=EXCLUDED.archive_retention_days,
 nightly_enabled=EXCLUDED.nightly_enabled,nightly_consent_analysis=EXCLUDED.nightly_consent_analysis,
 nightly_consent_training=EXCLUDED.nightly_consent_training,updated_by=EXCLUDED.updated_by
RETURNING *;

-- name: HistorySettingsWatermark :exec
UPDATE history_settings SET nightly_watermark=$2 WHERE organization_id=$1;

-- name: HistoryNightlyOrganizations :many
SELECT s.* FROM history_settings s JOIN organizations o ON o.id=s.organization_id
WHERE s.nightly_enabled AND s.nightly_consent_analysis AND o.deleted_at IS NULL ORDER BY s.organization_id;

-- name: HistoryImportCreate :one
INSERT INTO history_imports(organization_id,brand_id,department_id,source,connector,file_name,consent_analysis,consent_training,
 consent_by,raw_key,raw_retain_until,window_start,window_end,created_by,total_records)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING *;

-- name: HistoryImportGet :one
SELECT * FROM history_imports WHERE organization_id=$1 AND uuid=$2;

-- name: HistoryImportList :many
SELECT * FROM history_imports WHERE organization_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3;

-- name: HistoryImportCount :one
SELECT count(*) FROM history_imports WHERE organization_id=$1;

-- name: HistoryImportActive :one
SELECT count(*) FROM history_imports WHERE organization_id=$1 AND status IN ('pending','running');

-- name: HistoryImportDue :many
SELECT organization_id,uuid FROM history_imports
WHERE (status='pending' OR (status='running' AND lease_until<now())) ORDER BY id LIMIT $1;

-- name: HistoryImportClaim :one
UPDATE history_imports SET status='running',lease_until=now()+interval '10 minutes',started_at=coalesce(started_at,now()),stage='reading'
WHERE organization_id=$1 AND uuid=$2 AND (status='pending' OR (status='running' AND lease_until<now())) RETURNING *;

-- name: HistoryImportProgress :one
UPDATE history_imports SET stage=$3,total_records=$4,processed_records=$5,skipped_records=$6,clusters_touched=$7,
 suggestions_created=$8,suggestions_suppressed=$9,lease_until=now()+interval '10 minutes'
WHERE organization_id=$1 AND id=$2 RETURNING cancel_requested;

-- name: HistoryImportFinish :exec
UPDATE history_imports SET status=$3,stage=$4,error=$5,finished_at=now(),lease_until=NULL WHERE organization_id=$1 AND id=$2;

-- name: HistoryImportRequestCancel :one
UPDATE history_imports SET cancel_requested=true,
 status=CASE WHEN status='pending' THEN 'cancelled' ELSE status END,
 finished_at=CASE WHEN status='pending' THEN now() ELSE finished_at END
WHERE organization_id=$1 AND uuid=$2 AND status IN ('pending','running') RETURNING *;

-- name: HistoryImportLock :one
SELECT * FROM history_imports WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;

-- name: HistoryImportRolledBack :one
UPDATE history_imports SET status='rolled_back',finished_at=now() WHERE organization_id=$1 AND id=$2 RETURNING *;

-- name: HistoryImportRawDeleted :exec
UPDATE history_imports SET raw_key='',raw_deleted_at=now() WHERE organization_id=$1 AND id=$2;

-- name: HistoryImportRawExpired :many
SELECT organization_id,id,raw_key FROM history_imports
WHERE raw_key<>'' AND raw_deleted_at IS NULL AND status IN ('completed','failed','cancelled','rolled_back')
ORDER BY id LIMIT $1;

-- name: HistoryImportExpireStale :execrows
UPDATE history_imports SET status='failed',finished_at=now(),lease_until=NULL,
 error='the raw file passed its retention before the run finished; upload it again'
WHERE raw_key<>'' AND raw_retain_until<now()
 AND (status='pending' OR (status='running' AND lease_until<now()));

-- name: HistoryCaseInsert :one
INSERT INTO history_cases(organization_id,import_id,brand_id,department_id,source,external_ref,conversation_id,subject,problem,resolution,
 message_count,locale,training_allowed,category,embedding,resolution_embedding,embedding_model,opened_at,closed_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
ON CONFLICT(organization_id,external_ref) DO NOTHING RETURNING id;

-- name: HistoryCaseAssign :exec
UPDATE history_cases SET cluster_id=$3 WHERE organization_id=$1 AND id=$2;

-- name: HistoryCaseList :many
SELECT c.uuid,c.source,c.subject,c.problem,c.resolution,c.message_count,c.locale,c.category,c.training_allowed,c.brand_id,c.department_id,
 c.opened_at,c.closed_at,c.created_at,i.uuid AS import_uuid,cl.uuid AS cluster_uuid
FROM history_cases c JOIN history_imports i ON i.organization_id=c.organization_id AND i.id=c.import_id
LEFT JOIN history_clusters cl ON cl.organization_id=c.organization_id AND cl.id=c.cluster_id
WHERE c.organization_id=$1 AND (sqlc.narg(import_uuid)::uuid IS NULL OR i.uuid=sqlc.narg(import_uuid))
 AND (sqlc.narg(cluster_uuid)::uuid IS NULL OR cl.uuid=sqlc.narg(cluster_uuid))
ORDER BY c.id DESC LIMIT $2 OFFSET $3;

-- name: HistoryCaseCount :one
SELECT count(*) FROM history_cases c JOIN history_imports i ON i.organization_id=c.organization_id AND i.id=c.import_id
LEFT JOIN history_clusters cl ON cl.organization_id=c.organization_id AND cl.id=c.cluster_id
WHERE c.organization_id=$1 AND (sqlc.narg(import_uuid)::uuid IS NULL OR i.uuid=sqlc.narg(import_uuid))
 AND (sqlc.narg(cluster_uuid)::uuid IS NULL OR cl.uuid=sqlc.narg(cluster_uuid));

-- name: HistoryClusterCases :many
SELECT id,subject,problem,resolution,resolution_embedding,locale FROM history_cases
WHERE organization_id=$1 AND cluster_id=$2 ORDER BY id DESC LIMIT 200;

-- name: HistoryCaseDeleteByImport :execrows
DELETE FROM history_cases WHERE organization_id=$1 AND import_id=$2;

-- name: HistoryCaseExpire :execrows
DELETE FROM history_cases c USING history_settings s
WHERE s.organization_id=c.organization_id AND s.archive_retention_days>0
 AND c.created_at < now() - make_interval(days => s.archive_retention_days);

-- name: HistoryClusterCandidates :many
SELECT id,centroid,size FROM history_clusters
WHERE organization_id=$1 AND category=$2 AND embedding_model=$3
 AND brand_id IS NOT DISTINCT FROM sqlc.narg(brand_id)::uuid AND department_id IS NOT DISTINCT FROM sqlc.narg(department_id)::bigint
ORDER BY id LIMIT 5000;

-- name: HistoryClusterCreate :one
INSERT INTO history_clusters(organization_id,brand_id,department_id,category,label,centroid,embedding_model,size)
VALUES($1,$2,$3,$4,$5,$6,$7,1) RETURNING id;

-- name: HistoryClusterGrow :exec
UPDATE history_clusters SET centroid=$3,size=$4 WHERE organization_id=$1 AND id=$2;

-- name: HistoryClusterEvidence :exec
UPDATE history_clusters SET evidence=$3 WHERE organization_id=$1 AND id=$2;

-- name: HistoryClusterGet :one
SELECT * FROM history_clusters WHERE organization_id=$1 AND id=$2;

-- name: HistoryClusterList :many
SELECT cl.uuid,cl.category,cl.label,cl.size,cl.evidence,cl.brand_id,cl.department_id,cl.created_at,cl.updated_at,
 coalesce(ls.uuid::text,'')::text AS suggestion_uuid,coalesce(ls.status,'')::text AS suggestion_status
FROM history_clusters cl
LEFT JOIN LATERAL (SELECT s.uuid,s.status FROM knowledge_suggestions s WHERE s.organization_id=cl.organization_id AND s.cluster_id=cl.id ORDER BY s.id DESC LIMIT 1) ls ON true
WHERE cl.organization_id=$1 AND cl.size>0 ORDER BY cl.evidence DESC,cl.size DESC,cl.id LIMIT $2 OFFSET $3;

-- name: HistoryClusterCount :one
SELECT count(*) FROM history_clusters WHERE organization_id=$1 AND size>0;

-- name: HistoryClustersOfImport :many
SELECT DISTINCT cluster_id::bigint AS id FROM history_cases WHERE organization_id=$1 AND import_id=$2 AND cluster_id IS NOT NULL;

-- name: HistoryClusterRecount :exec
UPDATE history_clusters cl SET size=(SELECT count(*) FROM history_cases c WHERE c.organization_id=cl.organization_id AND c.cluster_id=cl.id)
WHERE cl.organization_id=$1 AND cl.id=ANY(sqlc.arg(ids)::bigint[]);

-- name: HistoryClusterDeleteEmpty :exec
DELETE FROM history_clusters cl WHERE cl.organization_id=$1 AND cl.size=0
 AND NOT EXISTS (SELECT 1 FROM knowledge_suggestions s WHERE s.organization_id=cl.organization_id AND s.cluster_id=cl.id AND s.status<>'pending')
 AND NOT EXISTS (SELECT 1 FROM history_decisions d WHERE d.organization_id=cl.organization_id AND d.cluster_id=cl.id);

-- name: HistoryClusterSuggestionState :one
SELECT count(*) FILTER (WHERE status='pending')::bigint AS pending, count(*) FILTER (WHERE status='approved')::bigint AS approved
FROM knowledge_suggestions WHERE organization_id=$1 AND cluster_id=$2;

-- name: HistorySuggestionCreate :one
INSERT INTO knowledge_suggestions(organization_id,topic,title,body,source,cluster_id,import_id,department_id,evidence_count,conflict_article_id,conflict_score)
VALUES($1,$2,$3,$4,'history',$5,$6,$7,$8,$9,$10) RETURNING *;

-- name: HistorySuggestionList :many
SELECT s.*,cl.uuid AS cluster_uuid,cl.size AS cluster_size,a.uuid AS conflict_article_uuid,a.title AS conflict_article_title
FROM knowledge_suggestions s
LEFT JOIN history_clusters cl ON cl.organization_id=s.organization_id AND cl.id=s.cluster_id
LEFT JOIN knowledge_articles a ON a.organization_id=s.organization_id AND a.id=s.conflict_article_id
WHERE s.organization_id=$1 AND s.source='history' AND (sqlc.narg(status)::text IS NULL OR s.status=sqlc.narg(status))
ORDER BY s.id DESC LIMIT $2 OFFSET $3;

-- name: HistorySuggestionCount :one
SELECT count(*) FROM knowledge_suggestions WHERE organization_id=$1 AND source='history' AND (sqlc.narg(status)::text IS NULL OR status=sqlc.narg(status));

-- name: HistorySuggestionReview :one
UPDATE knowledge_suggestions SET status=$3,reviewed_by=$4,article_id=$5,title=$6,body=$7
WHERE organization_id=$1 AND id=$2 AND status='pending' AND source='history' RETURNING *;

-- name: HistorySuggestionDeletePendingByImport :execrows
DELETE FROM knowledge_suggestions s WHERE s.organization_id=sqlc.arg(organization_id)::bigint AND s.import_id=sqlc.arg(import_id)::bigint AND s.status='pending'
 AND NOT EXISTS (SELECT 1 FROM history_cases c WHERE c.organization_id=s.organization_id
  AND c.cluster_id=s.cluster_id AND c.import_id<>sqlc.arg(import_id)::bigint);

-- name: HistoryDecisionCreate :one
INSERT INTO history_decisions(organization_id,suggestion_id,cluster_id,department_id,decision,reason,title,body,correction,embedding,embedding_model,decided_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING *;

-- name: HistoryNearestRejection :one
SELECT uuid,reason,(1-(embedding <=> sqlc.arg(embedding)::vector))::double precision AS score
FROM history_decisions WHERE organization_id=$1 AND decision='rejected' AND embedding_model=$2
 AND department_id IS NOT DISTINCT FROM sqlc.narg(department_id)::bigint
ORDER BY score DESC,id LIMIT 1;

-- name: HistoryDecisionList :many
SELECT d.uuid,d.decision,d.reason,d.title,d.correction,d.department_id,d.created_at,s.uuid AS suggestion_uuid,u.name AS decided_by_name
FROM history_decisions d LEFT JOIN knowledge_suggestions s ON s.organization_id=d.organization_id AND s.id=d.suggestion_id LEFT JOIN users u ON u.id=d.decided_by
WHERE d.organization_id=$1 ORDER BY d.id DESC LIMIT $2 OFFSET $3;

-- name: HistoryDecisionCount :one
SELECT count(*) FROM history_decisions WHERE organization_id=$1;

-- name: HistoryNearestArticle :one
SELECT a.id,a.uuid,v.title,(1-(c.embedding <=> sqlc.arg(embedding)::vector))::double precision AS score
FROM knowledge_chunks c JOIN knowledge_articles a ON a.organization_id=c.organization_id AND a.id=c.article_id
JOIN knowledge_article_versions v ON v.organization_id=c.organization_id AND v.article_id=c.article_id AND v.version=c.version
WHERE c.organization_id=$1 AND a.deleted_at IS NULL AND a.published_version=c.version AND c.embedding_model=$2
ORDER BY score DESC,c.id LIMIT 1;

-- name: HistoryClosedConversations :many
SELECT c.id,c.uuid,c.subject,c.brand_id,c.created_at,coalesce(c.closed_at,c.resolved_at,c.updated_at)::timestamptz AS ended_at,
 ct.name AS contact_name,coalesce(ct.email,'')::text AS contact_email,coalesce(ct.phone,'')::text AS contact_phone,
 coalesce(co.name,'')::text AS company_name
FROM conversations c JOIN contacts ct ON ct.organization_id=c.organization_id AND ct.id=c.contact_id
LEFT JOIN companies co ON co.organization_id=ct.organization_id AND co.id=ct.company_id
WHERE c.organization_id=$1 AND c.status IN ('closed','resolved') AND c.deleted_at IS NULL
 AND coalesce(c.closed_at,c.resolved_at,c.updated_at) > sqlc.arg(since)::timestamptz
 AND coalesce(c.closed_at,c.resolved_at,c.updated_at) <= sqlc.arg(until)::timestamptz
ORDER BY coalesce(c.closed_at,c.resolved_at,c.updated_at),c.id LIMIT $2;

-- name: HistoryConversationMessages :many
SELECT sender_type,content FROM messages
WHERE organization_id=$1 AND conversation_id=$2 AND NOT private AND deleted_at IS NULL AND kind='text' AND content<>''
ORDER BY created_at,id LIMIT 200;

-- name: HistoryCaseRefsExisting :many
SELECT external_ref FROM history_cases WHERE organization_id=$1 AND external_ref=ANY(sqlc.arg(refs)::text[]);

-- name: HistoryClusterRejected :one
SELECT count(*) FROM history_decisions WHERE organization_id=$1 AND cluster_id=$2 AND decision='rejected';

-- name: HistorySuggestionLock :one
SELECT * FROM knowledge_suggestions WHERE organization_id=$1 AND uuid=$2 AND source='history' FOR UPDATE;

-- name: HistorySuggestionGet :one
SELECT * FROM knowledge_suggestions WHERE organization_id=$1 AND uuid=$2 AND source='history';
