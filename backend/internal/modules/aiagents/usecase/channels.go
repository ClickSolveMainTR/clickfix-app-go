package usecase

import (
	"context"
	"log/slog"
)

// ChannelConnected puts a newly connected channel type in the agent's scope
// (CF-243): an organization that already runs the agent (config saved, mode
// not off) expects it on the new channel too. Idempotent; a failure is only
// logged so it never fails the connection itself. Implements the channels
// module's ConnectHook.
func (s *Service) ChannelConnected(ctx context.Context, org int64, channelType string) {
	changed, err := s.Repo.EnableChannel(ctx, org, channelType)
	if err != nil {
		slog.WarnContext(ctx, "ai_agent_channel_enable_failed", "organization_id", org, "channel_type", channelType, "error", err)
		return
	}
	if changed {
		slog.InfoContext(ctx, "ai_agent_channel_enabled", "organization_id", org, "channel_type", channelType)
	}
}
