package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MeteredTranscriber reserves one AI credit per started audio minute. Seconds
// occupy the existing quota units so audio cannot bypass organization AI limits.
// Unknown outcomes retain their reservation exactly like generation/embeddings.
type MeteredTranscriber struct {
	Raw            ai.Transcriber
	Repo           *repository.Store
	Model          string
	PricePerMinute int64
}

func (t *MeteredTranscriber) Enabled() bool {
	return t != nil && t.Raw != nil && t.Model != "" && t.PricePerMinute > 0 && t.PricePerMinute <= 1000000000
}
func (t *MeteredTranscriber) Transcribe(ctx context.Context, org, conversation int64, id, recording uuid.UUID, in ai.AudioRequest) (ai.Transcript, error) {
	if !t.Enabled() {
		return ai.Transcript{}, ai.ErrUnavailable
	}
	if in.DurationSeconds < 1 || in.DurationSeconds > 86400 || len(in.Audio) == 0 || len(in.Audio) > ai.MaxAudioBytes {
		return ai.Transcript{}, ai.ErrInvalidOutput
	}
	minutes := (in.DurationSeconds + 59) / 60
	cost := minutes * t.PricePerMinute
	in.Model = t.Model
	_, _, err := t.Repo.Reserve(ctx, db.AIInsertRunParams{OrganizationID: org, ConversationID: pgtype.Int8{Int64: conversation, Valid: conversation > 0}, Uuid: id, Task: "transcribe", RecordingUuid: pgtype.UUID{Bytes: recording, Valid: recording != uuid.Nil}, Provider: ai.ProviderOpenAI, Model: t.Model, Status: "running", Input: fmt.Sprintf("Recording: %d seconds", in.DurationSeconds), RetrievedChunks: []byte(`[]`), ReservedTokens: in.DurationSeconds, ReservedCredits: minutes, ReservedCostMicros: cost})
	if err != nil {
		return ai.Transcript{}, err
	}
	start := time.Now()
	out, err := t.Raw.Transcribe(ctx, in)
	status, code := "completed", ""
	charged, remaining, seconds := minutes, int64(0), in.DurationSeconds
	if err != nil {
		status, code = "unknown", "AI_TRANSCRIBE_OUTCOME_UNKNOWN"
		remaining, cost = cost, 0
	}
	if ai.NotAccepted(err) {
		status, code = "failed", "AI_PROVIDER_REJECTED"
		charged, remaining, cost, seconds = 0, 0, 0, 0
	}
	masker := ai.NewMasker()
	out.Text = masker.Mask(out.Text)
	for i := range out.Segments {
		out.Segments[i].Text = masker.Mask(out.Segments[i].Text)
		out.Segments[i].Speaker = masker.Mask(out.Segments[i].Speaker)
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	finishErr := t.Repo.Finish(finishCtx, db.AIFinishRunParams{OrganizationID: org, Uuid: id, Provider: ai.ProviderOpenAI, Model: t.Model, Status: status, Output: out.Text, ToolCalls: []byte(`[]`), InputTokens: seconds, CostMicros: cost, ReservedCostMicros: remaining, ChargedCredits: charged, LatencyMs: time.Since(start).Milliseconds(), ErrorCode: code})
	if finishErr != nil {
		return ai.Transcript{}, finishErr
	}
	return out, err
}
