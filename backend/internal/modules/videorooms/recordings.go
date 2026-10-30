package videorooms

import (
	"context"
	"errors"
	"io"
	"path"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/livekit"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/netguard"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Recording and encryption: LiveKit egress joins the room as a hidden
// participant and composes the media server-side, so it cannot record
// end-to-end encrypted tracks. A room is therefore either E2EE or recordable,
// chosen when it is created (policy: allow_recording, e2ee_required).
// Recordings go to the organization's own S3 bucket when configured (kept
// there, never deleted by us) or to platform storage, where the retention
// sweep deletes them after recording_retention_hours (default 24).

type RecordingView struct {
	UUID            uuid.UUID  `json:"uuid"`
	RoomUUID        uuid.UUID  `json:"room_uuid"`
	RoomName        string     `json:"room_name"`
	Status          string     `json:"status"`
	Storage         string     `json:"storage"`
	SizeBytes       int64      `json:"size_bytes"`
	DurationSeconds int64      `json:"duration_seconds"`
	Error           string     `json:"error"`
	StartedAt       time.Time  `json:"started_at"`
	EndedAt         *time.Time `json:"ended_at"`
	ExpiresAt       *time.Time `json:"expires_at"`
}

func recordingView(r db.VideoRecording, room uuid.UUID, name string) RecordingView {
	return RecordingView{UUID: r.Uuid, RoomUUID: room, RoomName: name, Status: r.Status, Storage: r.Storage, SizeBytes: r.SizeBytes,
		DurationSeconds: r.DurationSeconds, Error: r.Error, StartedAt: r.StartedAt.Time.UTC(), EndedAt: optTime(r.EndedAt), ExpiresAt: optTime(r.ExpiresAt)}
}

func (s *Service) StartRecording(ctx context.Context, org, actor int64, id uuid.UUID) (RecordingView, error) {
	a, err := s.usable(ctx, org, true)
	if err != nil {
		return RecordingView{}, err
	}
	r, err := s.Q.VideoRoom(ctx, db.VideoRoomParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return RecordingView{}, err
	}
	if !a.Policy.AllowRecording || r.E2ee || !r.Recording {
		return RecordingView{}, ErrRecordingNotAllowed
	}
	if r.Status != "live" {
		return RecordingView{}, ErrConflict
	}
	if _, err = s.Q.VideoActiveRecording(ctx, db.VideoActiveRecordingParams{OrganizationID: org, RoomID: r.ID}); err == nil {
		return RecordingView{}, ErrConflict
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return RecordingView{}, err
	}
	running, err := s.Q.VideoActiveRecordingCount(ctx, org)
	if err != nil {
		return RecordingView{}, err
	}
	if running >= int64(a.Policy.MaxConcurrentRecordings) {
		return RecordingView{}, ErrRateLimited
	}
	settings, _, err := s.orgSettings(ctx, org)
	if err != nil {
		return RecordingView{}, err
	}
	o, err := s.Q.GetOrganizationByID(ctx, org)
	if err != nil {
		return RecordingView{}, err
	}
	recID := uuid.New()
	key := path.Join("video-recordings", o.Uuid.String(), r.Uuid.String(), recID.String()+".mp4")
	out := livekit.S3Output{AccessKey: s.PlatformS3.AccessKey, Secret: s.PlatformS3.SecretKey, Region: s.PlatformS3.Region,
		Endpoint: s.PlatformS3.Endpoint, Bucket: s.PlatformS3.Bucket, ForcePathStyle: s.PlatformS3.UsePathStyle}
	if s.EgressS3Endpoint != "" {
		out.Endpoint = s.EgressS3Endpoint
	}
	params := db.VideoCreateRecordingParams{OrganizationID: org, RoomID: r.ID, Storage: "platform", StartedBy: pgtype.Int8{Int64: actor, Valid: actor > 0},
		ExpiresAt: pgtype.Timestamptz{Time: s.now().Add(time.Duration(a.Policy.MaxRecordingMinutes)*time.Minute + time.Duration(a.Policy.RecordingRetentionHours)*time.Hour), Valid: true}}
	if settings.StorageMode == "custom" {
		cfg, err := s.customStore(settings)
		if err != nil {
			return RecordingView{}, err
		}
		if settings.S3Prefix != "" {
			key = path.Join(settings.S3Prefix, key)
		}
		out = livekit.S3Output{AccessKey: cfg.AccessKey, Secret: cfg.SecretKey, Region: cfg.Region, Endpoint: cfg.Endpoint, Bucket: cfg.Bucket, ForcePathStyle: cfg.UsePathStyle}
		params.Storage, params.ExpiresAt = "custom", pgtype.Timestamptz{}
	}
	params.ObjectKey = key
	rec, err := s.Q.VideoCreateRecording(ctx, params)
	if err != nil {
		return RecordingView{}, err
	}
	egress, err := s.LiveKit.StartRecording(ctx, r.LivekitName, key, out)
	if err != nil {
		_ = s.Q.VideoFailRecording(ctx, db.VideoFailRecordingParams{OrganizationID: org, ID: rec.ID, Error: "egress start failed"})
		if errors.Is(err, livekit.ErrNotFound) {
			// No egress worker registered (or the room closed meanwhile).
			return RecordingView{}, ErrRecorderUnavailable
		}
		return RecordingView{}, err
	}
	if err = s.Q.VideoSetRecordingEgress(ctx, db.VideoSetRecordingEgressParams{OrganizationID: org, ID: rec.ID, EgressID: egress}); err != nil {
		return RecordingView{}, err
	}
	rec.EgressID, rec.Status = egress, "active"
	return recordingView(rec, r.Uuid, r.Name), nil
}

func (s *Service) StopRecording(ctx context.Context, org int64, id uuid.UUID) error {
	r, err := s.Q.VideoRoom(ctx, db.VideoRoomParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return err
	}
	rec, err := s.Q.VideoActiveRecording(ctx, db.VideoActiveRecordingParams{OrganizationID: org, RoomID: r.ID})
	if err != nil {
		return err
	}
	if rec.EgressID == "" {
		return s.Q.VideoFailRecording(ctx, db.VideoFailRecordingParams{OrganizationID: org, ID: rec.ID, Error: "stopped before start"})
	}
	return s.LiveKit.StopRecording(ctx, rec.EgressID)
}

func (s *Service) egressEvent(ctx context.Context, info livekit.EgressInfo) error {
	if info.EgressID == "" {
		return nil
	}
	rec, err := s.Q.VideoRecordingByEgress(ctx, info.EgressID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	var status string
	switch info.Status {
	case "EGRESS_COMPLETE", "EGRESS_LIMIT_REACHED":
		status = "complete"
	case "EGRESS_FAILED", "EGRESS_ABORTED":
		status = "failed"
	default:
		return nil
	}
	var size, secs int64
	for _, f := range info.FileResults {
		size += int64(f.Size)
		secs += int64(f.Duration) / int64(time.Second)
	}
	a, err := s.limits(ctx, rec.OrganizationID)
	if err != nil {
		return err
	}
	_, tighten, err := s.orgSettings(ctx, rec.OrganizationID)
	if err != nil {
		return err
	}
	err = s.Q.VideoFinishRecording(ctx, db.VideoFinishRecordingParams{OrganizationID: rec.OrganizationID, ID: rec.ID, Status: status, SizeBytes: size,
		DurationSeconds: secs, Error: truncate(info.Error, 500), RetentionHours: a.Tighten(tighten).RecordingRetentionHours})
	if err != nil {
		return err
	}
	if status == "complete" && s.RecordingCompleted != nil {
		if err = s.RecordingCompleted(ctx); err != nil {
			s.log().Warn("transcript_enqueue_deferred", "error", err)
		}
	}
	return nil
}

func (s *Service) Recordings(ctx context.Context, org int64, room *uuid.UUID, limit, offset int32) ([]RecordingView, int64, error) {
	ru := pgtype.UUID{}
	if room != nil {
		ru = pgtype.UUID{Bytes: *room, Valid: true}
	}
	rows, err := s.Q.VideoRecordings(ctx, db.VideoRecordingsParams{OrganizationID: org, RoomUuid: ru, RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, 0, err
	}
	out := make([]RecordingView, 0, len(rows))
	for _, r := range rows {
		out = append(out, recordingView(db.VideoRecording{Uuid: r.Uuid, Status: r.Status, Storage: r.Storage, SizeBytes: r.SizeBytes, DurationSeconds: r.DurationSeconds,
			Error: r.Error, StartedAt: r.StartedAt, EndedAt: r.EndedAt, ExpiresAt: r.ExpiresAt}, r.RoomUuid, r.RoomName))
	}
	n, err := s.Q.VideoRecordingCount(ctx, db.VideoRecordingCountParams{OrganizationID: org, RoomUuid: ru})
	return out, n, err
}

// store returns the driver that holds a recording.
func (s *Service) store(ctx context.Context, rec db.VideoRecording) (storage.Driver, error) {
	if rec.Storage == "platform" {
		if s.Storage == nil {
			return nil, ErrNotConfigured
		}
		return s.Storage, nil
	}
	settings, _, err := s.orgSettings(ctx, rec.OrganizationID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.customStore(settings)
	if err != nil {
		return nil, err
	}
	return storage.NewS3Untrusted(ctx, cfg)
}

func (s *Service) customStore(row db.VideoOrgSetting) (config.ObjectStoreConfig, error) {
	if row.StorageMode != "custom" {
		return config.ObjectStoreConfig{}, ErrConflict
	}
	access, err := s.open(row.S3AccessKeyEnc)
	if err != nil {
		return config.ObjectStoreConfig{}, err
	}
	secret, err := s.open(row.S3SecretKeyEnc)
	if err != nil {
		return config.ObjectStoreConfig{}, err
	}
	if err = publicEndpoint(row.S3Endpoint); err != nil {
		return config.ObjectStoreConfig{}, err
	}
	return config.ObjectStoreConfig{Endpoint: row.S3Endpoint, Region: row.S3Region, Bucket: row.S3Bucket, AccessKey: access, SecretKey: secret, UsePathStyle: row.S3PathStyle}, nil
}

// publicEndpoint keeps an organization's S3 endpoint off the private network
// (the API and egress would otherwise call internal services for it). The
// API side re-checks the resolved address at dial time (NewS3Untrusted).
func publicEndpoint(raw string) error {
	if netguard.ValidateURL(raw, false) != nil {
		return ErrInvalid
	}
	return nil
}

// OpenRecording streams a finished recording through the API (media URLs are
// API-owned; buckets are never exposed).
func (s *Service) OpenRecording(ctx context.Context, org int64, id uuid.UUID) (io.ReadCloser, int64, string, error) {
	rec, err := s.Q.VideoRecording(ctx, db.VideoRecordingParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return nil, 0, "", err
	}
	if rec.Status != "complete" {
		return nil, 0, "", ErrConflict
	}
	st, err := s.store(ctx, rec)
	if err != nil {
		return nil, 0, "", err
	}
	body, size, err := st.Download(ctx, rec.ObjectKey)
	return body, size, "recording-" + rec.Uuid.String() + ".mp4", err
}

func (s *Service) DeleteRecording(ctx context.Context, org int64, id uuid.UUID) error {
	rec, err := s.Q.VideoRecording(ctx, db.VideoRecordingParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return err
	}
	if rec.Status == "starting" || rec.Status == "active" {
		return ErrConflict
	}
	return s.purge(ctx, rec)
}

func (s *Service) purge(ctx context.Context, rec db.VideoRecording) error {
	if rec.Status == "complete" {
		st, err := s.store(ctx, rec)
		if err != nil {
			return err
		}
		if err = st.Delete(ctx, rec.ObjectKey); err != nil {
			return err
		}
	}
	return s.Q.VideoMarkRecordingDeleted(ctx, db.VideoMarkRecordingDeletedParams{OrganizationID: rec.OrganizationID, ID: rec.ID})
}

// PurgeExpired is the retention job (platform storage only).
func (s *Service) PurgeExpired(ctx context.Context) error {
	rows, err := s.Q.VideoExpiredRecordings(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, rec := range rows {
		if err = s.purge(ctx, rec); err != nil {
			s.log().Warn("video_recording_purge_failed", "recording", rec.Uuid, "error", err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
