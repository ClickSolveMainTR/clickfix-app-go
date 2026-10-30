package quality

import (
	"encoding/json"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
)

// CardView preserves structured rubric JSON at the HTTP boundary (sqlc stores JSONB as bytes).
func CardView(c db.QaScorecard) map[string]any {
	return map[string]any{"uuid": c.Uuid, "name": c.Name, "criteria": json.RawMessage(c.Criteria), "threshold": c.Threshold, "auto_score": c.AutoScore, "created_at": c.CreatedAt}
}
func ReviewView(r db.QaReview) map[string]any {
	return map[string]any{"uuid": r.Uuid, "source": r.Source, "score": r.Score, "critical_failure": r.CriticalFailure, "needs_review": r.NeedsReview, "rubric": json.RawMessage(r.Rubric), "scores": json.RawMessage(r.Scores), "created_at": r.CreatedAt, "reviewed_at": r.ReviewedAt, "ai_run_uuid": r.AiRunUuid}
}
func reviewListView(rows []db.QAListReviewsRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{"uuid": r.Uuid, "conversation_uuid": r.ConversationUuid, "agent_uuid": r.AgentUuid, "source": r.Source, "score": r.Score, "critical_failure": r.CriticalFailure, "needs_review": r.NeedsReview, "rubric": json.RawMessage(r.Rubric), "scores": json.RawMessage(r.Scores), "created_at": r.CreatedAt})
	}
	return out
}
