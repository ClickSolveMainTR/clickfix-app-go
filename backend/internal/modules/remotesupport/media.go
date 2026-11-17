package remotesupport

import (
	"context"
	"errors"
	"path"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/livekit"
	"github.com/jackc/pgx/v5"
)

// ensureRoom pre-creates the LiveKit room (auto_create is off on the
// server) for two participants plus the hidden recorder. Creating an
// existing room is a no-op in LiveKit.
func (s *Service) ensureRoom(ctx context.Context, row db.RemoteSupportSession) error {
	if !s.LiveKit.Enabled() {
		return ErrNotConfigured
	}
	return s.LiveKit.CreateRoom(ctx, livekit.CreateRoomInput{Name: row.RoomName, MaxParticipants: 3, EmptyTimeout: 120, DepartureTimeout: 60,
		Metadata: `{"kind":"remote_support"}`})
}

// applyControl pushes the current control state to the connected agent:
// LiveKit replaces its permissions, so data publishing (input events) stops
// the moment the customer withdraws control.
func (s *Service) applyControl(ctx context.Context, row db.RemoteSupportSession, set db.RemoteSupportSetting) {
	if !s.LiveKit.Enabled() {
		return
	}
	data := controlActive(row) && set.AllowControl
	if err := s.LiveKit.UpdatePermission(ctx, row.RoomName, agentIdentity(row), livekit.Permission{CanSubscribe: true, CanPublishData: data}); err != nil {
		s.log().Warn("remote_support_update_permission_failed", "session", row.Uuid, "error", err)
	}
}

// startRecording starts a composite egress of the room into platform
// storage; it expires after the organization's retention days and is
// deleted by the video recording retention job.
func (s *Service) startRecording(ctx context.Context, row db.RemoteSupportSession, set db.RemoteSupportSetting) {
	if set.Recording != "optional" || !row.RecordingConsent.Valid || !row.RecordingConsent.Bool || row.RecordingStatus != "none" || !s.LiveKit.Enabled() {
		return
	}
	o, err := s.Q.GetOrganizationByID(ctx, row.OrganizationID)
	if err != nil {
		s.log().Warn("remote_support_recording_org_failed", "session", row.Uuid, "error", err)
		return
	}
	key := path.Join("remote-support-recordings", o.Uuid.String(), row.Uuid.String()+".mp4")
	expires := s.now().Add(time.Duration(row.MaxDurationMinutes)*time.Minute + time.Duration(set.RecordingRetentionDays)*24*time.Hour)
	if _, err = s.Q.RemoteSupportStartRecording(ctx, db.RemoteSupportStartRecordingParams{OrganizationID: row.OrganizationID, ID: row.ID, RecordingKey: key, RecordingExpiresAt: ts(expires)}); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			s.log().Warn("remote_support_recording_row_failed", "session", row.Uuid, "error", err)
		}
		return
	}
	out := livekit.S3Output{AccessKey: s.PlatformS3.AccessKey, Secret: s.PlatformS3.SecretKey, Region: s.PlatformS3.Region,
		Endpoint: s.PlatformS3.Endpoint, Bucket: s.PlatformS3.Bucket, ForcePathStyle: s.PlatformS3.UsePathStyle}
	if s.EgressS3Endpoint != "" {
		out.Endpoint = s.EgressS3Endpoint
	}
	egress, err := s.LiveKit.StartRecording(ctx, row.RoomName, key, out)
	if err != nil {
		s.log().Warn("remote_support_egress_start_failed", "session", row.Uuid, "error", err)
		_ = s.Q.RemoteSupportSetRecordingStatus(ctx, db.RemoteSupportSetRecordingStatusParams{OrganizationID: row.OrganizationID, ID: row.ID, RecordingStatus: "failed"})
		return
	}
	if err = s.Q.RemoteSupportSetEgress(ctx, db.RemoteSupportSetEgressParams{OrganizationID: row.OrganizationID, ID: row.ID, EgressID: egress}); err != nil {
		s.log().Warn("remote_support_egress_row_failed", "session", row.Uuid, "error", err)
		return
	}
	s.audit(ctx, row.OrganizationID, nil, "remote_support.recording_started", row.Uuid, nil)
}

func (s *Service) stopRecording(ctx context.Context, row db.RemoteSupportSession) {
	if row.EgressID == "" || (row.RecordingStatus != "active" && row.RecordingStatus != "starting") || !s.LiveKit.Enabled() {
		return
	}
	if err := s.LiveKit.StopRecording(ctx, row.EgressID); err != nil {
		s.log().Warn("remote_support_egress_stop_failed", "session", row.Uuid, "error", err)
	}
}

// IsRoom tells the video webhook which events belong to this module.
func IsRoom(name string) bool { return strings.HasPrefix(name, RoomPrefix) }

// HandleEvent is the rs_ branch of the LiveKit webhook (the video module
// verifies the signature and drops duplicates first).
func (s *Service) HandleEvent(ctx context.Context, ev livekit.Event) error {
	at := s.now()
	if ev.CreatedAt > 0 {
		at = time.Unix(int64(ev.CreatedAt), 0).UTC()
	}
	if ev.EgressInfo != nil && strings.HasPrefix(ev.Event, "egress_") {
		return s.egressEvent(ctx, *ev.EgressInfo)
	}
	if ev.Room == nil || !IsRoom(ev.Room.Name) {
		return nil
	}
	row, err := s.Q.RemoteSupportSessionByRoom(ctx, ev.Room.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	switch ev.Event {
	case "participant_joined":
		if ev.Participant == nil || ev.Participant.Identity != customerIdentity(row) {
			if ev.Participant != nil && ev.Participant.Identity == agentIdentity(row) {
				s.publish(ctx, row, "agent_joined")
			}
			return nil
		}
		if !row.ViewConsentAt.Valid || (row.Status != StatusJoined && row.Status != StatusActive) {
			return nil
		}
		joined := at
		if ev.Participant.JoinedAt > 0 {
			joined = time.Unix(int64(ev.Participant.JoinedAt), 0).UTC()
		}
		first := row.Status == StatusJoined
		if row, err = s.Q.RemoteSupportActivate(ctx, db.RemoteSupportActivateParams{OrganizationID: row.OrganizationID, ID: row.ID, StartedAt: ts(joined)}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		if first {
			s.audit(ctx, row.OrganizationID, nil, "remote_support.started", row.Uuid, nil)
		}
		set, err := s.settings(ctx, row.OrganizationID)
		if err != nil {
			return err
		}
		s.startRecording(ctx, row, set)
		s.publish(ctx, row, "active")
	case "participant_left":
		if ev.Participant != nil && ev.Participant.Identity == customerIdentity(row) && open(row) {
			s.publish(ctx, row, "customer_left")
		}
	case "room_finished":
		// Everyone left (the app quit or lost the network for good).
		if open(row) && row.Status != StatusPending {
			_, err = s.end(ctx, row, "customer", BySystem, 0)
			return err
		}
	}
	return nil
}

func (s *Service) egressEvent(ctx context.Context, info livekit.EgressInfo) error {
	if info.EgressID == "" || !IsRoom(info.RoomName) {
		return nil
	}
	row, err := s.Q.RemoteSupportSessionByEgress(ctx, info.EgressID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	status := ""
	var size int64
	switch info.Status {
	case "EGRESS_COMPLETE":
		status = "complete"
		for _, f := range info.FileResults {
			size += int64(f.Size)
		}
	case "EGRESS_FAILED", "EGRESS_ABORTED", "EGRESS_LIMIT_REACHED":
		status = "failed"
	}
	if status == "" {
		return nil
	}
	if err = s.Q.RemoteSupportSetRecordingStatus(ctx, db.RemoteSupportSetRecordingStatusParams{OrganizationID: row.OrganizationID, ID: row.ID,
		RecordingStatus: status, RecordingSizeBytes: size}); err != nil {
		return err
	}
	s.audit(ctx, row.OrganizationID, nil, "remote_support.recording_"+status, row.Uuid, map[string]any{"size_bytes": size})
	return nil
}
