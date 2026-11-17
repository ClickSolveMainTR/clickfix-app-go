package remotesupport

import (
	"context"
	"errors"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	convmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// sweepReason is why an open session must end now ("" = keep it).
func (s *Service) sweepReason(row db.RemoteSupportSession, conversationStatus string, set db.RemoteSupportSetting) string {
	now := s.now()
	switch {
	case set.PlatformState != PlatformActive || !set.Enabled:
		return "platform"
	case conversationStatus == convmodel.StatusClosed:
		return "conversation_closed"
	case row.Status == StatusPending && !row.CodeExpiresAt.Time.After(now):
		return "code_expired"
	case row.Status == StatusJoined && !row.ViewConsentAt.Valid && row.ResolvedAt.Valid && !row.ResolvedAt.Time.Add(CodeTTL).After(now):
		return "timeout"
	}
	if end := endsAt(row); end != nil && !end.After(now) {
		return "max_duration"
	}
	return ""
}

// Sweep runs every minute (queue.TaskRemoteSupportSweep): it closes sessions
// whose code expired, that waited too long for consent, ran past their time
// limit, whose conversation closed, or whose organization lost remote support.
func (s *Service) Sweep(ctx context.Context) error {
	rows, err := s.Q.RemoteSupportOpenSessions(ctx)
	if err != nil {
		return err
	}
	settings := map[int64]db.RemoteSupportSetting{}
	var errs []error
	for _, r := range rows {
		row := r.RemoteSupportSession
		set, ok := settings[row.OrganizationID]
		if !ok {
			if set, err = s.settings(ctx, row.OrganizationID); err != nil {
				errs = append(errs, err)
				continue
			}
			settings[row.OrganizationID] = set
		}
		reason := s.sweepReason(row, r.ConversationStatus, set)
		if reason == "" {
			continue
		}
		by := BySystem
		if reason == "platform" {
			by = ByPlatform
		}
		if _, err = s.end(ctx, row, reason, by, 0); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// PurgeRecordings deletes platform-stored recordings past their retention.
// It runs inside the video recording retention job (same schedule).
func (s *Service) PurgeRecordings(ctx context.Context) error {
	rows, err := s.Q.RemoteSupportExpiredRecordings(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, r := range rows {
		if r.RecordingKey != "" && s.Storage != nil {
			if err := s.Storage.Delete(ctx, r.RecordingKey); err != nil {
				s.log().Warn("remote_support_recording_purge_failed", "session", r.Uuid, "error", err)
				errs = append(errs, err)
				continue
			}
		}
		if err := s.Q.RemoteSupportMarkRecordingDeleted(ctx, db.RemoteSupportMarkRecordingDeletedParams{OrganizationID: r.OrganizationID, ID: r.ID}); err != nil {
			errs = append(errs, err)
			continue
		}
		s.audit(ctx, r.OrganizationID, nil, "remote_support.recording_deleted", r.Uuid, map[string]any{"reason": "retention"})
	}
	return errors.Join(errs...)
}

// endAll ends every open session of an organization (kill switch,
// suspension).
func (s *Service) endAll(ctx context.Context, org int64, reason, by string, actor int64) error {
	rows, err := s.Q.RemoteSupportOpenSessionsForOrg(ctx, org)
	if err != nil {
		return err
	}
	var errs []error
	for _, r := range rows {
		if _, err := s.end(ctx, r, reason, by, actor); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// auditOrg records an organization-level step (resource: the organization).
func (s *Service) auditOrg(ctx context.Context, org int64, actor *int64, action string, orgUUID uuid.UUID, payload map[string]any) {
	s.audit(ctx, org, actor, action, orgUUID, payload)
}

// ---- platform ----

// SetPlatformState is the platform admin's kill switch (disabled) and the
// way back (active) after a review of an automatic suspension.
func (s *Service) SetPlatformState(ctx context.Context, actor int64, orgUUID uuid.UUID, in PlatformStateInput) (Settings, error) {
	in.Reason = clip(in.Reason, 500)
	if in.State != PlatformActive && in.State != PlatformDisabled {
		return Settings{}, ErrInvalid
	}
	if in.State == PlatformDisabled && in.Reason == "" {
		return Settings{}, ErrInvalid
	}
	o, err := s.Q.GetOrganizationByUUID(ctx, orgUUID)
	if err != nil {
		return Settings{}, err
	}
	if _, err = s.Q.RemoteSupportSetPlatformState(ctx, db.RemoteSupportSetPlatformStateParams{OrganizationID: o.ID, PlatformState: in.State,
		PlatformReason: in.Reason, PlatformChangedBy: pgtype.Int8{Int64: actor, Valid: actor > 0}}); err != nil {
		return Settings{}, err
	}
	s.auditOrg(ctx, o.ID, actorPtr(actor), "remote_support.platform_state_changed", o.Uuid, map[string]any{"state": in.State, "reason": in.Reason})
	if in.State == PlatformDisabled {
		if err = s.endAll(ctx, o.ID, "platform", ByPlatform, actor); err != nil {
			return Settings{}, err
		}
	}
	return s.Settings(ctx, o.ID)
}

func (s *Service) PlatformOrganizations(ctx context.Context, state string, limit, offset int32) ([]PlatformOrganization, error) {
	state = strings.TrimSpace(state)
	if state != "" && state != PlatformActive && state != PlatformDisabled && state != PlatformSuspended {
		return nil, ErrInvalid
	}
	rows, err := s.Q.RemoteSupportPlatformOrganizations(ctx, db.RemoteSupportPlatformOrganizationsParams{State: pgtype.Text{String: state, Valid: state != ""}, RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, err
	}
	out := make([]PlatformOrganization, 0, len(rows))
	for _, r := range rows {
		out = append(out, PlatformOrganization{UUID: r.Uuid, Name: r.Name, Slug: r.Slug, Enabled: r.Enabled, PlatformState: r.PlatformState,
			PlatformReason: r.PlatformReason, PlatformChangedAt: opt(r.PlatformChangedAt), Reports30d: r.Reports30d, Sessions30d: r.Sessions30d})
	}
	return out, nil
}

func (s *Service) PlatformReports(ctx context.Context, limit, offset int32) ([]PlatformReport, error) {
	rows, err := s.Q.RemoteSupportPlatformReports(ctx, db.RemoteSupportPlatformReportsParams{RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, err
	}
	out := make([]PlatformReport, 0, len(rows))
	for _, r := range rows {
		out = append(out, PlatformReport{SessionUUID: r.Uuid, Status: r.Status, ModeRequested: r.ModeRequested, ReportedAt: r.ReportedAt.Time.UTC(),
			ReportReason: r.ReportReason, EndedReason: r.EndedReason, CreatedAt: r.CreatedAt.Time.UTC(), OrganizationUUID: r.OrganizationUuid,
			OrganizationName: r.OrganizationName, AgentName: strings.TrimSpace(r.AgentName)})
	}
	return out, nil
}
