-- name: InsertWidgetDiagnostic :one
INSERT INTO widget_diagnostics(organization_id,conversation_id,kind,attachment_id,duration_ms,context,expires_at)
VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING *;
-- name: ListWidgetDiagnostics :many
SELECT * FROM widget_diagnostics WHERE organization_id=$1 AND conversation_id=$2 AND expires_at>now() ORDER BY id DESC LIMIT 100;
-- name: CountWidgetDiagnostics :one
SELECT count(*) FROM widget_diagnostics WHERE organization_id=$1 AND conversation_id=$2 AND expires_at>now();
-- name: SetDiagnosticAttachmentExpiry :exec
UPDATE attachments SET expires_at=$3 WHERE organization_id=$1 AND uuid=$2;
-- name: CountOrganizationDiagnosticsToday :one
SELECT count(*) FROM widget_diagnostics WHERE organization_id=$1 AND created_at>=date_trunc('day',now());
-- retention worker enumerates expired rows; all deletes and object cleanup use row organization_id
-- name: ListExpiredWidgetDiagnostics :many
SELECT d.organization_id,d.uuid,d.attachment_id FROM widget_diagnostics d WHERE d.expires_at<=now() ORDER BY d.expires_at LIMIT 100;
-- name: DeleteExpiredWidgetDiagnostic :exec
DELETE FROM widget_diagnostics WHERE organization_id=$1 AND uuid=$2 AND expires_at<=now();

-- name: LockWidgetDiagnosticQuota :exec
SELECT pg_advisory_xact_lock(sqlc.arg(organization_id)::bigint # 4847566674759057408::bigint);
-- retention worker enumerates expired attachment keys; object deletion and metadata updates use each row organization_id
-- name: ListExpiredDiagnosticAttachments :many
SELECT organization_id,uuid,object_key,thumb_key FROM attachments WHERE expires_at<=now() AND deleted_at IS NULL ORDER BY expires_at LIMIT 100;
-- retention worker deletes expired context only, after matching attachment removal in the same organization
-- name: PurgeExpiredWidgetDiagnosticContext :exec
DELETE FROM widget_diagnostics WHERE expires_at<=now() AND (attachment_id IS NULL OR EXISTS(
 SELECT 1 FROM attachments a WHERE a.organization_id=widget_diagnostics.organization_id AND a.uuid=widget_diagnostics.attachment_id AND a.deleted_at IS NOT NULL));
