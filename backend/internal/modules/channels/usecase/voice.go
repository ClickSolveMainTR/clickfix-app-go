package usecase

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
)

// VoiceCalls reads only the organization's connected voice instance.
func (s *Service) VoiceCalls(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]db.ListChannelVoiceCallsRow, int64, error) {
	inst, _, err := s.Open(ctx, orgID, id)
	if err != nil {
		return nil, 0, err
	}
	if inst.Type != channels.TypeVoice {
		return nil, 0, ErrNotFound
	}
	rows, err := s.q.ListChannelVoiceCalls(ctx, db.ListChannelVoiceCallsParams{OrganizationID: orgID, ChannelInstanceID: inst.ID, LimitCount: limit, OffsetCount: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountChannelVoiceCalls(ctx, db.CountChannelVoiceCallsParams{OrganizationID: orgID, ChannelInstanceID: inst.ID})
	return rows, total, err
}
