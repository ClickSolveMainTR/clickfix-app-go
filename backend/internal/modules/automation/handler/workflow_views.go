package handler

import (
	"encoding/json"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
)

func workflowView(r db.Workflow) map[string]any {
	var published *int32
	if r.PublishedVersion.Valid {
		v := r.PublishedVersion.Int32
		published = &v
	}
	return map[string]any{"uuid": r.Uuid, "name": r.Name, "draft": json.RawMessage(r.Draft), "revision": r.Revision, "published_version": published, "enabled": r.Enabled, "created_at": r.CreatedAt, "updated_at": r.UpdatedAt}
}
func workflowViews(rows []db.Workflow) []any {
	out := []any{}
	for _, r := range rows {
		out = append(out, workflowView(r))
	}
	return out
}
func versionViews(rows []db.WorkflowVersion) []any {
	out := []any{}
	for _, r := range rows {
		out = append(out, map[string]any{"uuid": r.Uuid, "version": r.Version, "graph": json.RawMessage(r.Graph), "trigger_event": r.TriggerEvent, "schedule_minutes": r.ScheduleMinutes, "created_at": r.CreatedAt})
	}
	return out
}
func runView(r db.WorkflowRun) map[string]any {
	return map[string]any{"uuid": r.Uuid, "conversation_uuid": r.ConversationUuid, "event_id": r.EventID, "status": r.Status, "node_id": r.NodeID, "step_index": r.StepIndex, "error": r.Error, "due_at": r.DueAt, "trace": json.RawMessage(r.Trace), "created_at": r.CreatedAt}
}
func runViews(rows []db.WorkflowRun) []any {
	out := []any{}
	for _, r := range rows {
		out = append(out, runView(r))
	}
	return out
}
func stepViews(rows []db.WorkflowStep) []any {
	out := []any{}
	for _, r := range rows {
		out = append(out, map[string]any{"uuid": r.Uuid, "step_index": r.StepIndex, "node_id": r.NodeID, "status": r.Status, "output": json.RawMessage(r.Output), "error": r.Error, "created_at": r.CreatedAt, "updated_at": r.UpdatedAt})
	}
	return out
}
