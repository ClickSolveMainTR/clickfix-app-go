package videorooms

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/entitlements"
	notif "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/i18n"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/livekit"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Wallet is the billing engine's metered-unit API (video_minutes).
type Wallet interface {
	MeteredBalance(ctx context.Context, org int64, unit string) (int64, error)
	MeterUsage(ctx context.Context, org int64, unit string, amount int64, key, reason string) (int64, int64, error)
}

// Entitlements resolves the plan snapshot (video.enabled, video.minutes).
type Entitlements interface {
	Resolve(ctx context.Context, org int64) (entitlements.Snapshot, error)
}

type Notifier interface {
	Enqueue(context.Context, notif.EnqueueInput) ([]notif.Notification, error)
}

// RemoteSupportRoomPrefix names the LiveKit rooms of remote support
// sessions (remotesupport.RoomPrefix); their webhook events are handed to
// Service.RemoteSupportEvent.
const RemoteSupportRoomPrefix = "rs_"

type Service struct {
	RecordingCompleted    func(context.Context) error
	RemoteSupportEvent    func(context.Context, livekit.Event) error
	AppointmentOpenBefore time.Duration
	AppointmentGrace      time.Duration
	Q                     *db.Queries
	LiveKit               *livekit.Client
	Box                   *crypto.SecretBox
	Signer                Signer
	Wallet                Wallet
	Entitlements          Entitlements
	// Storage is the platform object store (SeaweedFS); PlatformS3 its
	// credentials, handed to egress per recording.
	Storage          storage.Driver
	PlatformS3       config.ObjectStoreConfig
	EgressS3Endpoint string
	Notifier         Notifier
	FrontendURL      string
	Log              *slog.Logger
	Now              func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func (s *Service) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

// ---- policy ----

func (s *Service) PlatformPolicy(ctx context.Context) (Policy, error) {
	raw, err := s.Q.VideoPlatformPolicy(ctx)
	if err != nil {
		return Policy{}, err
	}
	var p Policy
	return p, json.Unmarshal(raw, &p)
}

func (s *Service) SavePlatformPolicy(ctx context.Context, actor int64, p Policy) (Policy, error) {
	if !p.Valid() {
		return p, ErrInvalid
	}
	if p.E2EERequired {
		p.AllowRecording = false
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	return p, s.Q.VideoSavePlatformPolicy(ctx, db.VideoSavePlatformPolicyParams{Policy: raw, UpdatedBy: pgtype.Int8{Int64: actor, Valid: actor > 0}})
}

type PlanPolicy struct {
	PlanUUID uuid.UUID `json:"plan_uuid"`
	Code     string    `json:"code"`
	Name     string    `json:"name"`
	Policy   Partial   `json:"policy"`
}

func (s *Service) PlanPolicies(ctx context.Context) ([]PlanPolicy, error) {
	rows, err := s.Q.VideoPlanPolicies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PlanPolicy, 0, len(rows))
	for _, r := range rows {
		p := PlanPolicy{PlanUUID: r.PlanUuid, Code: r.Code, Name: r.Name}
		if err = json.Unmarshal(r.Policy, &p.Policy); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Service) SavePlanPolicy(ctx context.Context, plan uuid.UUID, p Partial) error {
	if plan == uuid.Nil || !p.Valid() {
		return ErrInvalid
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.Q.VideoSavePlanPolicy(ctx, db.VideoSavePlanPolicyParams{Uuid: plan, Policy: raw})
}

func (s *Service) DeletePlanPolicy(ctx context.Context, plan uuid.UUID) (bool, error) {
	n, err := s.Q.VideoDeletePlanPolicy(ctx, plan)
	return n > 0, err
}

func (s *Service) orgSettings(ctx context.Context, org int64) (db.VideoOrgSetting, Partial, error) {
	row, err := s.Q.VideoOrgSettings(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.VideoOrgSetting{OrganizationID: org, Enabled: true, StorageMode: "platform", S3PathStyle: true}, Partial{}, nil
	}
	if err != nil {
		return row, Partial{}, err
	}
	var p Partial
	return row, p, json.Unmarshal(row.Policy, &p)
}

// Limits is the platform + plan policy before the organization tightens it:
// the ceiling the organization settings page shows.
func (s *Service) limits(ctx context.Context, org int64) (Policy, error) {
	p, err := s.PlatformPolicy(ctx)
	if err != nil {
		return p, err
	}
	raw, err := s.Q.VideoOrgPlanPolicy(ctx, org)
	if err == nil {
		var o Partial
		if err = json.Unmarshal(raw, &o); err != nil {
			return p, err
		}
		p = p.Override(o)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return p, err
	}
	return p, nil
}

// Availability explains whether rooms can be used and why not.
type Availability struct {
	Configured      bool   `json:"configured"`
	PlatformEnabled bool   `json:"platform_enabled"`
	Entitled        bool   `json:"entitled"`
	OrgEnabled      bool   `json:"org_enabled"`
	Ready           bool   `json:"ready"`
	Unlimited       bool   `json:"unlimited"`
	Balance         int64  `json:"balance"`
	LowBalance      bool   `json:"low_balance"`
	Policy          Policy `json:"policy"`
	Limits          Policy `json:"limits"`
}

func (s *Service) Availability(ctx context.Context, org int64) (Availability, error) {
	var a Availability
	limits, err := s.limits(ctx, org)
	if err != nil {
		return a, err
	}
	row, tighten, err := s.orgSettings(ctx, org)
	if err != nil {
		return a, err
	}
	a.Limits, a.Policy = limits, limits.Tighten(tighten)
	a.Configured = s.LiveKit.Enabled()
	a.PlatformEnabled = limits.Enabled
	a.OrgEnabled = row.Enabled && a.Policy.Enabled
	if s.Entitlements != nil {
		snap, err := s.Entitlements.Resolve(ctx, org)
		if err != nil {
			return a, err
		}
		v := snap.Values["video.enabled"]
		a.Entitled = snap.Writable && v.Kind == "boolean" && v.Enabled
		a.Unlimited = snap.Values["video.minutes"].Unlimited
	}
	if s.Wallet != nil {
		if a.Balance, err = s.Wallet.MeteredBalance(ctx, org, UnitVideoMinutes); err != nil {
			return a, err
		}
	}
	a.LowBalance = !a.Unlimited && a.Balance < a.Policy.LowBalanceMinutes
	a.Ready = a.Configured && a.PlatformEnabled && a.Entitled && a.OrgEnabled
	return a, nil
}

func (s *Service) usable(ctx context.Context, org int64, needMinutes bool) (Availability, error) {
	a, err := s.Availability(ctx, org)
	if err != nil {
		return a, err
	}
	switch {
	case !a.Configured:
		return a, ErrNotConfigured
	case !a.Ready:
		return a, ErrDisabled
	case needMinutes && !a.Unlimited && a.Balance <= 0:
		return a, ErrNoMinutes
	}
	return a, nil
}

// ---- organization settings ----

type OrgSettingsView struct {
	Enabled      bool         `json:"enabled"`
	Policy       Partial      `json:"policy"`
	Storage      StorageView  `json:"storage"`
	Availability Availability `json:"availability"`
}
type StorageView struct {
	Mode         string `json:"mode"`
	Endpoint     string `json:"endpoint"`
	Region       string `json:"region"`
	Bucket       string `json:"bucket"`
	Prefix       string `json:"prefix"`
	PathStyle    bool   `json:"path_style"`
	HasAccessKey bool   `json:"has_access_key"`
	HasSecretKey bool   `json:"has_secret_key"`
}

func (s *Service) OrgSettings(ctx context.Context, org int64) (OrgSettingsView, error) {
	row, p, err := s.orgSettings(ctx, org)
	if err != nil {
		return OrgSettingsView{}, err
	}
	a, err := s.Availability(ctx, org)
	return OrgSettingsView{Enabled: row.Enabled, Policy: p, Availability: a, Storage: StorageView{
		Mode: row.StorageMode, Endpoint: row.S3Endpoint, Region: row.S3Region, Bucket: row.S3Bucket, Prefix: row.S3Prefix,
		PathStyle: row.S3PathStyle, HasAccessKey: row.S3AccessKeyEnc != "", HasSecretKey: row.S3SecretKeyEnc != "",
	}}, err
}

var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.\-]{1,61}[a-z0-9]$`)

func (s *Service) SaveOrgSettings(ctx context.Context, org int64, in OrgSettingsInput) (OrgSettingsView, error) {
	if !in.Policy.Valid() {
		return OrgSettingsView{}, ErrInvalid
	}
	old, _, err := s.orgSettings(ctx, org)
	if err != nil {
		return OrgSettingsView{}, err
	}
	st := in.Storage
	params := db.VideoSaveOrgSettingsParams{OrganizationID: org, Enabled: in.Enabled, StorageMode: "platform", S3PathStyle: true}
	if params.Policy, err = json.Marshal(in.Policy); err != nil {
		return OrgSettingsView{}, err
	}
	switch st.Mode {
	case "", "platform":
	case "custom":
		st.Endpoint, st.Prefix = strings.TrimSpace(st.Endpoint), strings.Trim(strings.TrimSpace(st.Prefix), "/")
		if !bucketPattern.MatchString(st.Bucket) || len(st.Region) > 64 || len(st.Prefix) > 256 || len(st.Endpoint) > 512 ||
			publicEndpoint(st.Endpoint) != nil || strings.Contains(st.Prefix, "..") {
			return OrgSettingsView{}, ErrInvalid
		}
		access, secret := old.S3AccessKeyEnc, old.S3SecretKeyEnc
		if st.AccessKey != "" {
			if access, err = s.seal(st.AccessKey); err != nil {
				return OrgSettingsView{}, err
			}
		}
		if st.SecretKey != "" {
			if secret, err = s.seal(st.SecretKey); err != nil {
				return OrgSettingsView{}, err
			}
		}
		if access == "" || secret == "" {
			return OrgSettingsView{}, ErrInvalid
		}
		params.StorageMode, params.S3Endpoint, params.S3Region, params.S3Bucket, params.S3Prefix, params.S3PathStyle = "custom", st.Endpoint, st.Region, st.Bucket, st.Prefix, st.PathStyle
		params.S3AccessKeyEnc, params.S3SecretKeyEnc = access, secret
	default:
		return OrgSettingsView{}, ErrInvalid
	}
	if _, err = s.Q.VideoSaveOrgSettings(ctx, params); err != nil {
		return OrgSettingsView{}, err
	}
	return s.OrgSettings(ctx, org)
}

func (s *Service) seal(v string) (string, error) {
	if s.Box == nil {
		return "", ErrNotConfigured
	}
	if len(v) > 512 {
		return "", ErrInvalid
	}
	return s.Box.Encrypt(v)
}

func (s *Service) open(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	if s.Box == nil {
		return "", ErrNotConfigured
	}
	return s.Box.Decrypt(v)
}

// ---- rooms ----

// RoomDetail is the tenant projection: the host sees the PIN and the guest
// link so they can share them.
type RoomDetail struct {
	RoomView
	PIN              string     `json:"pin"`
	GuestLink        string     `json:"guest_link"`
	LinksExpireAt    time.Time  `json:"links_expire_at"`
	ConversationUUID *uuid.UUID `json:"conversation_uuid"`
	AppointmentUUID  *uuid.UUID `json:"appointment_uuid"`
	StartedAt        *time.Time `json:"started_at"`
	EndedAt          *time.Time `json:"ended_at"`
	BilledMinutes    int64      `json:"billed_minutes"`
	UnbilledMinutes  int64      `json:"unbilled_minutes"`
	Waiting          int64      `json:"waiting"`
	CreatedAt        time.Time  `json:"created_at"`
}

var pinPattern = regexp.MustCompile(`^[0-9]{4,12}$`)

const joinTokenTTL = 10 * time.Minute

func optTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func (s *Service) view(r db.VideoRoom, orgName string) RoomView {
	return RoomView{
		UUID: r.Uuid, Name: r.Name, OrganizationName: orgName, Status: r.Status, PINRequired: r.PinEnc != "", Lobby: r.Lobby,
		GuestJoin: r.GuestJoin, E2EE: r.E2ee, Recording: r.Recording, AllowCamera: r.AllowCamera, AllowScreenShare: r.AllowScreenShare,
		AllowChat: r.AllowChat, MaxParticipants: r.MaxParticipants, MaxDurationMinutes: r.MaxDurationMinutes,
		ScheduledStartAt: optTime(r.ScheduledStartAt), ScheduledEndAt: optTime(r.ScheduledEndAt),
	}
}

func (s *Service) detail(ctx context.Context, r db.VideoRoom, waiting int64) (RoomDetail, error) {
	d := RoomDetail{RoomView: s.view(r, ""), LinksExpireAt: r.LinksExpireAt.Time.UTC(), StartedAt: optTime(r.StartedAt), EndedAt: optTime(r.EndedAt),
		BilledMinutes: r.BilledMinutes, UnbilledMinutes: r.UnbilledMinutes, Waiting: waiting, CreatedAt: r.CreatedAt.Time.UTC()}
	var err error
	if d.PIN, err = s.open(r.PinEnc); err != nil {
		return d, err
	}
	if r.AppointmentUuid.Valid {
		id := uuid.UUID(r.AppointmentUuid.Bytes)
		d.AppointmentUUID = &id
	}
	if r.ConversationID.Valid {
		id, err := s.Q.VideoConversationUUID(ctx, db.VideoConversationUUIDParams{OrganizationID: r.OrganizationID, ID: r.ConversationID.Int64})
		if err == nil {
			d.ConversationUUID = &id
		}
	}
	if r.GuestJoin && r.Status != "ended" && r.LinksExpireAt.Time.After(s.now()) {
		if code, err := s.Signer.Sign(Link{Room: r.Uuid, Role: LinkGuest, ExpiresAt: r.LinksExpireAt.Time}); err == nil {
			d.GuestLink = s.joinURL(code)
		}
	}
	return d, nil
}

func (s *Service) joinURL(code string) string {
	return strings.TrimRight(s.FrontendURL, "/") + "/r/" + code
}

func (s *Service) CreateRoom(ctx context.Context, org, actor int64, in CreateRoomInput) (RoomDetail, error) {
	a, err := s.usable(ctx, org, true)
	if err != nil {
		return RoomDetail{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 160 || in.PIN != "" && !pinPattern.MatchString(in.PIN) {
		return RoomDetail{}, ErrInvalid
	}
	set, err := a.Policy.Resolve(RoomOptions{Name: in.Name, PIN: in.PIN, Lobby: in.Lobby, GuestJoin: in.GuestJoin, E2EE: in.E2EE, Record: in.Record,
		AllowCamera: in.AllowCamera, AllowScreenShare: in.AllowScreenShare, AllowChat: in.AllowChat, MaxParticipants: in.MaxParticipants, MaxDurationMinutes: in.MaxDurationMinutes})
	if err != nil {
		return RoomDetail{}, err
	}
	now := s.now()
	if in.ScheduledStartAt != nil && in.ScheduledEndAt != nil && !in.ScheduledEndAt.After(*in.ScheduledStartAt) {
		return RoomDetail{}, ErrInvalid
	}
	// Links stay valid a day past the scheduled end (late joiners), otherwise
	// a week from now.
	expires := now.Add(7 * 24 * time.Hour)
	if in.ScheduledEndAt != nil {
		if !in.ScheduledEndAt.After(now) || in.ScheduledEndAt.After(now.Add(366*24*time.Hour)) {
			return RoomDetail{}, ErrInvalid
		}
		expires = in.ScheduledEndAt.Add(24 * time.Hour)
	}
	host := actor
	if in.HostUUID != nil {
		m, err := s.Q.ReportsMember(ctx, db.ReportsMemberParams{OrganizationID: org, Uuid: *in.HostUUID})
		if errors.Is(err, pgx.ErrNoRows) {
			return RoomDetail{}, ErrInvalid
		} else if err != nil {
			return RoomDetail{}, err
		}
		host = m.ID
	}
	params := db.VideoCreateRoomParams{
		OrganizationID: org, Name: in.Name, LivekitName: "cf-" + uuid.NewString(),
		HostUserID: pgtype.Int8{Int64: host, Valid: host > 0}, CreatedBy: pgtype.Int8{Int64: actor, Valid: actor > 0},
		Lobby: set.Lobby, GuestJoin: set.GuestJoin, E2ee: set.E2EE, Recording: set.Recording, AllowCamera: set.AllowCamera,
		AllowScreenShare: set.AllowScreenShare, AllowChat: set.AllowChat, MaxParticipants: set.MaxParticipants, MaxDurationMinutes: set.MaxDurationMinutes,
		LinksExpireAt: pgtype.Timestamptz{Time: expires, Valid: true},
	}
	if in.ScheduledStartAt != nil {
		params.ScheduledStartAt = pgtype.Timestamptz{Time: *in.ScheduledStartAt, Valid: true}
	}
	if in.ScheduledEndAt != nil {
		params.ScheduledEndAt = pgtype.Timestamptz{Time: *in.ScheduledEndAt, Valid: true}
	}
	if in.AppointmentUUID != nil {
		params.AppointmentUuid = pgtype.UUID{Bytes: *in.AppointmentUUID, Valid: true}
	}
	if in.ConversationUUID != nil {
		c, err := s.Q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: *in.ConversationUUID})
		if errors.Is(err, pgx.ErrNoRows) {
			return RoomDetail{}, ErrInvalid
		} else if err != nil {
			return RoomDetail{}, err
		}
		params.ConversationID = pgtype.Int8{Int64: c.ID, Valid: true}
	}
	if in.PIN != "" {
		if params.PinEnc, err = s.seal(in.PIN); err != nil {
			return RoomDetail{}, err
		}
	}
	if set.E2EE {
		key, err := newSecret()
		if err != nil {
			return RoomDetail{}, err
		}
		if params.E2eeKeyEnc, err = s.seal(key); err != nil {
			return RoomDetail{}, err
		}
	}
	room, err := s.Q.VideoCreateRoom(ctx, params)
	if err != nil {
		return RoomDetail{}, err
	}
	return s.detail(ctx, room, 0)
}

func (s *Service) Room(ctx context.Context, org int64, id uuid.UUID) (RoomDetail, error) {
	r, err := s.Q.VideoRoom(ctx, db.VideoRoomParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return RoomDetail{}, err
	}
	ps, err := s.Q.VideoParticipants(ctx, db.VideoParticipantsParams{OrganizationID: org, RoomID: r.ID})
	if err != nil {
		return RoomDetail{}, err
	}
	var waiting int64
	for _, p := range ps {
		if p.Status == StatusWaiting {
			waiting++
		}
	}
	return s.detail(ctx, r, waiting)
}

func (s *Service) Rooms(ctx context.Context, org int64, status *string, limit, offset int32) ([]RoomDetail, int64, error) {
	st := pgtype.Text{}
	if status != nil {
		st = pgtype.Text{String: *status, Valid: true}
	}
	rows, err := s.Q.VideoRooms(ctx, db.VideoRoomsParams{OrganizationID: org, Status: st, RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, 0, err
	}
	out := make([]RoomDetail, 0, len(rows))
	for _, r := range rows {
		room := db.VideoRoom{ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, Name: r.Name, LivekitName: r.LivekitName, Status: r.Status,
			HostUserID: r.HostUserID, CreatedBy: r.CreatedBy, AppointmentUuid: r.AppointmentUuid, PinEnc: r.PinEnc, Lobby: r.Lobby,
			GuestJoin: r.GuestJoin, E2ee: r.E2ee, Recording: r.Recording, AllowCamera: r.AllowCamera, AllowScreenShare: r.AllowScreenShare,
			AllowChat: r.AllowChat, MaxParticipants: r.MaxParticipants, MaxDurationMinutes: r.MaxDurationMinutes, ScheduledStartAt: r.ScheduledStartAt,
			ScheduledEndAt: r.ScheduledEndAt, LinksExpireAt: r.LinksExpireAt, StartedAt: r.StartedAt, EndedAt: r.EndedAt,
			BilledMinutes: r.BilledMinutes, UnbilledMinutes: r.UnbilledMinutes, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
		d, err := s.detail(ctx, room, r.Waiting)
		if err != nil {
			return nil, 0, err
		}
		if r.ConversationUuid.Valid {
			id := uuid.UUID(r.ConversationUuid.Bytes)
			d.ConversationUUID = &id
		}
		out = append(out, d)
	}
	n, err := s.Q.VideoRoomCount(ctx, db.VideoRoomCountParams{OrganizationID: org, Status: st})
	return out, n, err
}

// EndRoom disconnects everyone, stops a running recording and bills the open
// connections.
func (s *Service) EndRoom(ctx context.Context, org int64, id uuid.UUID) (RoomDetail, error) {
	r, err := s.Q.VideoRoom(ctx, db.VideoRoomParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return RoomDetail{}, err
	}
	if err = s.end(ctx, r, "host"); err != nil {
		return RoomDetail{}, err
	}
	return s.Room(ctx, org, id)
}

func (s *Service) end(ctx context.Context, r db.VideoRoom, reason string) error {
	if rec, err := s.Q.VideoActiveRecording(ctx, db.VideoActiveRecordingParams{OrganizationID: r.OrganizationID, RoomID: r.ID}); err == nil {
		if rec.EgressID != "" {
			if err = s.LiveKit.StopRecording(ctx, rec.EgressID); err != nil {
				s.log().Warn("video_stop_recording_failed", "room", r.Uuid, "error", err)
			}
		}
	}
	if s.LiveKit.Enabled() {
		if err := s.LiveKit.DeleteRoom(ctx, r.LivekitName); err != nil {
			s.log().Warn("video_delete_room_failed", "room", r.Uuid, "error", err)
		}
	}
	if _, err := s.Q.VideoEndRoom(ctx, db.VideoEndRoomParams{OrganizationID: r.OrganizationID, ID: r.ID}); err != nil {
		return err
	}
	s.log().Info("video_room_ended", "room", r.Uuid, "organization_id", r.OrganizationID, "reason", reason)
	return s.closeOpenSessions(ctx, r, s.now())
}

// ---- links and invitations ----

type LinkView struct {
	Role      string    `json:"role"`
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Service) Link(ctx context.Context, org int64, id uuid.UUID, in LinkInput) (LinkView, error) {
	r, err := s.Q.VideoRoom(ctx, db.VideoRoomParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return LinkView{}, err
	}
	if r.Status == "ended" {
		return LinkView{}, ErrRoomEnded
	}
	if in.Role == "" {
		in.Role = LinkGuest
	}
	if in.Role == LinkGuest && !r.GuestJoin {
		return LinkView{}, ErrDenied
	}
	exp := r.LinksExpireAt.Time
	if in.TTLMinutes != 0 {
		if in.TTLMinutes < 5 || in.TTLMinutes > 30*24*60 {
			return LinkView{}, ErrInvalid
		}
		exp = min64Time(exp, s.now().Add(time.Duration(in.TTLMinutes)*time.Minute))
	}
	code, err := s.Signer.Sign(Link{Room: r.Uuid, Role: in.Role, ExpiresAt: exp})
	if err != nil {
		return LinkView{}, err
	}
	return LinkView{Role: in.Role, Code: code, URL: s.joinURL(code), ExpiresAt: exp.UTC()}, nil
}

func min64Time(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func validEmail(v string) bool {
	a, err := mail.ParseAddress(v)
	return err == nil && a.Address == v && len(v) <= 254
}

// Invite e-mails a guest join link (the PIN is sent separately by the host).
func (s *Service) Invite(ctx context.Context, org int64, id uuid.UUID, in InviteInput) (LinkView, error) {
	in.Email, in.Name = strings.TrimSpace(in.Email), strings.TrimSpace(in.Name)
	if !validEmail(in.Email) || len([]rune(in.Name)) > 120 {
		return LinkView{}, ErrInvalid
	}
	link, err := s.Link(ctx, org, id, LinkInput{Role: LinkGuest})
	if err != nil {
		return link, err
	}
	r, err := s.Q.VideoRoom(ctx, db.VideoRoomParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return link, err
	}
	o, err := s.Q.GetOrganizationByID(ctx, org)
	if err != nil {
		return link, err
	}
	title, body := inviteText(in.Locale, o.Name, o.Timezone, r, link.URL, in.Name)
	if s.Notifier == nil {
		return link, ErrNotConfigured
	}
	_, err = s.Notifier.Enqueue(ctx, notif.EnqueueInput{TenantID: &org, Channels: []string{notif.ChannelEmail}, Recipient: &in.Email, Language: lang(in.Locale),
		Title: title, Body: body, SourceEvent: "video.invite", Payload: map[string]any{"room_uuid": r.Uuid.String()}})
	return link, err
}

func lang(v string) string {
	if strings.HasPrefix(strings.ToLower(v), "en") {
		return "en"
	}
	return "tr"
}

// inviteText is the guest invitation e-mail. A scheduled time is written in
// the organization's time zone, with the zone's name.
func inviteText(locale, org, timezone string, r db.VideoRoom, url, name string) (string, string) {
	when := ""
	if r.ScheduledStartAt.Valid {
		loc, err := time.LoadLocation(timezone)
		if err != nil || timezone == "" {
			loc = time.UTC
		}
		l := i18n.LocaleTR
		if lang(locale) == "en" {
			l = i18n.LocaleEN
		}
		when = i18n.FormatDateTime(r.ScheduledStartAt.Time, loc, l) + " (" + loc.String() + ")"
	}
	pin := ""
	if lang(locale) == "en" {
		if r.PinEnc != "" {
			pin = "\nThe room is protected with a PIN; your host will share it with you."
		}
		greet := "Hello"
		if name != "" {
			greet += " " + name
		}
		at := ""
		if when != "" {
			at = "\nTime: " + when
		}
		return org + " invites you to a video call", fmt.Sprintf("%s,\n\n%s invites you to the video call \"%s\".%s\n\nJoin from your browser, no account needed:\n%s%s\n\nThe call is encrypted. Camera and microphone permission is requested when you join.", greet, org, r.Name, at, url, pin)
	}
	if r.PinEnc != "" {
		pin = "\nOda PIN ile korunuyor; PIN'i görüşmeyi düzenleyen kişi size ayrıca iletecek."
	}
	greet := "Merhaba"
	if name != "" {
		greet += " " + name
	}
	at := ""
	if when != "" {
		at = "\nTarih ve saat: " + when
	}
	return org + " sizi görüntülü görüşmeye davet ediyor", fmt.Sprintf("%s,\n\n%s sizi \"%s\" görüntülü görüşmesine davet ediyor.%s\n\nHesap gerekmeden tarayıcınızdan katılın:\n%s%s\n\nGörüşme şifrelidir. Katılırken kamera ve mikrofon izni istenir.", greet, org, r.Name, at, url, pin)
}

// RoomForAppointment returns the open room of an appointment or creates one
// (CF-138 calls this when an appointment is booked); the guest is invited
// when an e-mail is given.
func (s *Service) RoomForAppointment(ctx context.Context, org, actor int64, appointment uuid.UUID, in AppointmentRoomInput) (RoomDetail, error) {
	d, err := s.syncAppointmentRoom(ctx, org, actor, appointment, in)
	if err != nil {
		return d, err
	}
	if in.GuestEmail != "" {
		_, err = s.Invite(ctx, org, d.UUID, InviteInput{Email: in.GuestEmail, Name: in.GuestName, Locale: in.Locale})
	}
	return d, err
}

// ---- joining ----

// ParticipantView is the lobby / participant list row.
type ParticipantView struct {
	UUID        uuid.UUID  `json:"uuid"`
	Role        string     `json:"role"`
	DisplayName string     `json:"display_name"`
	Email       string     `json:"email"`
	Status      string     `json:"status"`
	AdmittedAt  *time.Time `json:"admitted_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func participantView(p db.VideoParticipant) ParticipantView {
	return ParticipantView{UUID: p.Uuid, Role: p.Role, DisplayName: p.DisplayName, Email: p.Email, Status: p.Status, AdmittedAt: optTime(p.AdmittedAt), CreatedAt: p.CreatedAt.Time.UTC()}
}

func (s *Service) Participants(ctx context.Context, org int64, id uuid.UUID) ([]ParticipantView, error) {
	r, err := s.Q.VideoRoom(ctx, db.VideoRoomParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return nil, err
	}
	rows, err := s.Q.VideoParticipants(ctx, db.VideoParticipantsParams{OrganizationID: org, RoomID: r.ID})
	out := make([]ParticipantView, 0, len(rows))
	for _, p := range rows {
		out = append(out, participantView(p))
	}
	return out, err
}

// JoinAsMember admits a signed-in organization member straight into the
// room: the host (or a video manager) as host, everyone else as agent.
func (s *Service) JoinAsMember(ctx context.Context, org int64, user db.User, id uuid.UUID, manager bool) (JoinGrant, error) {
	if _, err := s.usable(ctx, org, true); err != nil {
		return JoinGrant{}, err
	}
	r, err := s.Q.VideoRoom(ctx, db.VideoRoomParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return JoinGrant{}, err
	}
	if err := s.checkAppointmentWindow(r); err != nil {
		return JoinGrant{}, err
	}
	if r.Status == "ended" {
		return JoinGrant{}, ErrRoomEnded
	}
	role := RoleAgent
	if manager || r.HostUserID.Valid && r.HostUserID.Int64 == user.ID {
		role = RoleHost
	}
	name := strings.TrimSpace(user.Name + " " + user.Surname)
	if name == "" {
		name = user.Email
	}
	ticket, err := newSecret()
	if err != nil {
		return JoinGrant{}, err
	}
	if err = s.ensureCapacity(ctx, r, "u:"+user.Uuid.String()); err != nil {
		return JoinGrant{}, err
	}
	p, err := s.Q.VideoCreateParticipant(ctx, db.VideoCreateParticipantParams{OrganizationID: org, RoomID: r.ID, Identity: "u:" + user.Uuid.String(), Role: role,
		DisplayName: truncate(name, 120), Email: user.Email, UserID: pgtype.Int8{Int64: user.ID, Valid: true}, Status: StatusAdmitted, TicketHash: hashTicket(ticket),
		AdmittedAt: pgtype.Timestamptz{Time: s.now(), Valid: true}, AdmittedBy: pgtype.Int8{Int64: user.ID, Valid: true}})
	if err != nil {
		return JoinGrant{}, err
	}
	if p.Status == StatusRemoved {
		return JoinGrant{}, ErrDenied
	}
	g, err := s.grant(ctx, r, p)
	g.Ticket = ticket
	return g, err
}

func truncate(v string, n int) string {
	r := []rune(v)
	if len(r) > n {
		return string(r[:n])
	}
	return v
}

func (s *Service) ensureCapacity(ctx context.Context, r db.VideoRoom, identity string) error {
	n, err := s.Q.VideoAdmittedCount(ctx, db.VideoAdmittedCountParams{OrganizationID: r.OrganizationID, RoomID: r.ID})
	if err != nil {
		return err
	}
	if n < int64(r.MaxParticipants) {
		return nil
	}
	// Rejoining with the same identity does not take a new seat.
	ps, err := s.Q.VideoParticipants(ctx, db.VideoParticipantsParams{OrganizationID: r.OrganizationID, RoomID: r.ID})
	if err != nil {
		return err
	}
	for _, p := range ps {
		if p.Identity == identity && p.Status == StatusAdmitted {
			return nil
		}
	}
	return ErrRoomFull
}

// grant issues the LiveKit token (and E2EE key) for an admitted participant.
func (s *Service) grant(ctx context.Context, r db.VideoRoom, p db.VideoParticipant) (JoinGrant, error) {
	g := JoinGrant{Status: p.Status, ParticipantUUID: p.Uuid, Role: p.Role, Room: s.view(r, "")}
	if p.Status != StatusAdmitted {
		return g, nil
	}
	if err := s.checkAppointmentWindow(r); err != nil {
		return g, err
	}
	if r.Status == "ended" {
		return g, ErrRoomEnded
	}
	if err := s.LiveKit.CreateRoom(ctx, livekit.CreateRoomInput{Name: r.LivekitName, MaxParticipants: safeconv.ToUint32(int64(r.MaxParticipants) + 1), EmptyTimeout: 600, DepartureTimeout: 120}); err != nil {
		return g, err
	}
	sources := []string{livekit.SourceMicrophone}
	if r.AllowCamera {
		sources = append(sources, livekit.SourceCamera)
	}
	if r.AllowScreenShare {
		sources = append(sources, livekit.SourceScreenShare, livekit.SourceScreenShareAudio)
	}
	yes, chat := true, r.AllowChat
	meta, _ := json.Marshal(map[string]string{"role": p.Role, "participant_uuid": p.Uuid.String()})
	// Short-lived: LiveKit refreshes the token of a connected participant, so
	// a removed one cannot reconnect with an old token for long.
	ttl := joinTokenTTL
	token, err := s.LiveKit.JoinToken(p.Identity, p.DisplayName, string(meta), livekit.VideoGrant{
		Room: r.LivekitName, CanPublish: &yes, CanSubscribe: &yes, CanPublishData: &chat, CanPublishSources: sources,
	}, ttl)
	if err != nil {
		return g, err
	}
	g.Token, g.ServerURL = token, s.LiveKit.URL()
	if r.E2ee {
		if g.E2EEKey, err = s.open(r.E2eeKeyEnc); err != nil {
			return g, err
		}
	}
	return g, nil
}

// PublicRoom resolves a join code for the guest page.
func (s *Service) PublicRoom(ctx context.Context, code string) (RoomView, db.VideoRoom, Link, error) {
	l, err := s.Signer.Verify(code, s.now())
	if err != nil {
		return RoomView{}, db.VideoRoom{}, l, err
	}
	r, err := s.Q.VideoRoomByUUID(ctx, l.Room)
	if errors.Is(err, pgx.ErrNoRows) {
		return RoomView{}, r, l, ErrLinkInvalid
	} else if err != nil {
		return RoomView{}, r, l, err
	}
	o, err := s.Q.GetOrganizationByID(ctx, r.OrganizationID)
	if err != nil {
		return RoomView{}, r, l, err
	}
	v := s.view(r, o.Name)
	v.LinkRole = l.Role
	return v, r, l, nil
}

// GuestJoin registers a guest; with a lobby the guest waits for the host,
// otherwise the grant is returned at once. The ticket is the guest's only
// credential for polling.
func (s *Service) GuestJoin(ctx context.Context, code string, in GuestJoinInput) (JoinGrant, error) {
	v, r, l, err := s.PublicRoom(ctx, code)
	if err != nil {
		return JoinGrant{}, err
	}
	if l.Role != LinkGuest {
		return JoinGrant{}, ErrLoginRequired
	}
	if err := s.checkAppointmentWindow(r); err != nil {
		return JoinGrant{}, err
	}
	if r.Status == "ended" {
		return JoinGrant{}, ErrRoomEnded
	}
	if !r.GuestJoin {
		return JoinGrant{}, ErrDenied
	}
	in.Name, in.Email = strings.TrimSpace(in.Name), strings.TrimSpace(in.Email)
	if in.Name == "" || len([]rune(in.Name)) > 120 || in.Email != "" && !validEmail(in.Email) {
		return JoinGrant{}, ErrInvalid
	}
	if r.PinEnc != "" {
		pin, err := s.open(r.PinEnc)
		if err != nil {
			return JoinGrant{}, err
		}
		if subtle.ConstantTimeCompare([]byte(pin), []byte(in.PIN)) != 1 {
			return JoinGrant{}, ErrPINInvalid
		}
	}
	if _, err = s.usable(ctx, r.OrganizationID, true); err != nil {
		return JoinGrant{}, err
	}
	ticket, err := newSecret()
	if err != nil {
		return JoinGrant{}, err
	}
	identity := "g:" + uuid.NewString()
	status := StatusWaiting
	params := db.VideoCreateParticipantParams{OrganizationID: r.OrganizationID, RoomID: r.ID, Identity: identity, Role: RoleGuest,
		DisplayName: in.Name, Email: in.Email, TicketHash: hashTicket(ticket)}
	if !r.Lobby {
		if err = s.ensureCapacity(ctx, r, identity); err != nil {
			return JoinGrant{}, err
		}
		status = StatusAdmitted
		params.AdmittedAt = pgtype.Timestamptz{Time: s.now(), Valid: true}
	}
	params.Status = status
	p, err := s.Q.VideoCreateParticipant(ctx, params)
	if err != nil {
		return JoinGrant{}, err
	}
	g, err := s.grant(ctx, r, p)
	g.Ticket, g.Room = ticket, v
	return g, err
}

// Ticket is the guest's lobby poll: the grant appears once admitted.
func (s *Service) Ticket(ctx context.Context, ticket string) (JoinGrant, error) {
	if len(ticket) < 32 || len(ticket) > 64 {
		return JoinGrant{}, ErrNotFound
	}
	p, err := s.Q.VideoParticipantByTicket(ctx, hashTicket(ticket))
	if errors.Is(err, pgx.ErrNoRows) {
		return JoinGrant{}, ErrNotFound
	} else if err != nil {
		return JoinGrant{}, err
	}
	r, err := s.Q.VideoRoomByID(ctx, db.VideoRoomByIDParams{OrganizationID: p.OrganizationID, ID: p.RoomID})
	if err != nil {
		return JoinGrant{}, err
	}
	if err := s.checkAppointmentWindow(r); err != nil {
		return JoinGrant{}, err
	}
	if r.Status == "ended" {
		return JoinGrant{Status: "ended", ParticipantUUID: p.Uuid, Role: p.Role, Room: s.view(r, "")}, nil
	}
	if p.Status == StatusAdmitted {
		if _, err = s.usable(ctx, r.OrganizationID, true); err != nil {
			return JoinGrant{}, err
		}
	}
	o, err := s.Q.GetOrganizationByID(ctx, r.OrganizationID)
	if err != nil {
		return JoinGrant{}, err
	}
	g, err := s.grant(ctx, r, p)
	g.Room.OrganizationName = o.Name
	return g, err
}

// SetParticipant admits, denies or removes a participant.
func (s *Service) SetParticipant(ctx context.Context, org, actor int64, room, participant uuid.UUID, action string) (ParticipantView, error) {
	r, err := s.Q.VideoRoom(ctx, db.VideoRoomParams{OrganizationID: org, Uuid: room})
	if err != nil {
		return ParticipantView{}, err
	}
	p, err := s.Q.VideoParticipant(ctx, db.VideoParticipantParams{OrganizationID: org, RoomID: r.ID, Uuid: participant})
	if err != nil {
		return ParticipantView{}, err
	}
	status := map[string]string{"admit": StatusAdmitted, "deny": StatusDenied, "remove": StatusRemoved}[action]
	if status == "" || p.Role == RoleHost && status != StatusAdmitted {
		return ParticipantView{}, ErrInvalid
	}
	if status == StatusAdmitted {
		if r.Status == "ended" {
			return ParticipantView{}, ErrRoomEnded
		}
		if err = s.ensureCapacity(ctx, r, p.Identity); err != nil {
			return ParticipantView{}, err
		}
	}
	if status == StatusRemoved && s.LiveKit.Enabled() {
		if err = s.LiveKit.RemoveParticipant(ctx, r.LivekitName, p.Identity); err != nil {
			return ParticipantView{}, err
		}
	}
	p, err = s.Q.VideoSetParticipantStatus(ctx, db.VideoSetParticipantStatusParams{OrganizationID: org, RoomID: r.ID, Uuid: participant, Status: status, Actor: pgtype.Int8{Int64: actor, Valid: actor > 0}})
	if err != nil {
		return ParticipantView{}, err
	}
	// A removed guest could reopen the link under a new identity; from now on
	// every guest of this room waits in the lobby.
	if status == StatusRemoved && p.Role == RoleGuest && !r.Lobby {
		err = s.Q.VideoForceLobby(ctx, db.VideoForceLobbyParams{OrganizationID: org, ID: r.ID})
	}
	return participantView(p), err
}

// New builds the service from configuration. Without LIVEKIT_* the module
// stays visible but reports itself as not configured.
func New(q *db.Queries, cfg config.Config, wallet Wallet, ent Entitlements, store storage.Driver, n Notifier, log *slog.Logger) *Service {
	box, err := crypto.NewSecretBox(cfg.Encryption.Key)
	if err != nil {
		log.Warn("video_secretbox_unavailable", "error", err)
		box = nil
	}
	return &Service{
		AppointmentOpenBefore: time.Duration(max(0, min(120, cfg.Video.AppointmentOpenBeforeMinutes))) * time.Minute,
		AppointmentGrace:      time.Duration(max(0, min(120, cfg.Video.AppointmentGraceMinutes))) * time.Minute,
		Q:                     q, Box: box, Signer: NewSigner(cfg.Encryption.Key), Wallet: wallet, Entitlements: ent, Storage: store, Notifier: n,
		LiveKit:    livekit.New(livekit.Config{URL: cfg.Video.LiveKitURL, APIURL: cfg.Video.LiveKitAPIURL, APIKey: cfg.Video.LiveKitAPIKey, APISecret: cfg.Video.LiveKitAPISecret}),
		PlatformS3: cfg.Storage.S3, EgressS3Endpoint: cfg.Video.EgressS3Endpoint, FrontendURL: cfg.Auth.FrontendURL, Log: log,
	}
}
