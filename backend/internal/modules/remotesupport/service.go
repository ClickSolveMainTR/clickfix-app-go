package remotesupport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	convmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/livekit"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Entitlements is the plan engine slice remote support uses.
type Entitlements interface {
	Require(ctx context.Context, org int64, key string) error
	CheckLimit(ctx context.Context, org int64, key string, used, delta int64) (entitlement.Decision, error)
	Consume(ctx context.Context, org int64, key string, n int64, eventKey string) (entitlement.Decision, error)
}

// Poster posts the customer-visible message (download link + code) on the
// conversation through its channel.
type Poster interface {
	PostAuto(ctx context.Context, org, conversationID int64, kind, content string) (uuid.UUID, error)
}

// Timeline writes a system event into the conversation timeline. The
// conversations usecase (the Poster in production) implements it; the
// session summary is written there when a session ends (CF-237).
type Timeline interface {
	SystemEvent(ctx context.Context, orgID, convID int64, actor convusecase.Actor, attrs map[string]any) error
}

type Service struct {
	Q            *db.Queries
	LiveKit      *livekit.Client
	Entitlements Entitlements
	Poster       Poster
	Publisher    realtime.Publisher
	Activity     *activity.Recorder
	Hasher       Hasher
	// Storage / PlatformS3: recordings go to platform storage and are deleted
	// by the video recording retention job after recording_retention_days.
	Storage          storage.Driver
	PlatformS3       config.ObjectStoreConfig
	EgressS3Endpoint string
	FrontendURL      string
	Log              *slog.Logger
	Now              func() time.Time
}

// New wires the service from configuration.
func New(q *db.Queries, cfg config.Config, ent Entitlements, poster Poster, pub realtime.Publisher, rec *activity.Recorder, store storage.Driver, log *slog.Logger) *Service {
	return &Service{
		Q: q, Entitlements: ent, Poster: poster, Publisher: pub, Activity: rec, Hasher: NewHasher(cfg.Encryption.Key), Storage: store,
		LiveKit:    livekit.New(livekit.Config{URL: cfg.Video.LiveKitURL, APIURL: cfg.Video.LiveKitAPIURL, APIKey: cfg.Video.LiveKitAPIKey, APISecret: cfg.Video.LiveKitAPISecret}),
		PlatformS3: cfg.Storage.S3, EgressS3Endpoint: cfg.Video.EgressS3Endpoint, FrontendURL: cfg.Auth.FrontendURL, Log: log,
	}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func opt(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

// ---- audit and realtime ----

type requestKey struct{}

// WithRequest lets the audit rows carry the caller's address and user agent.
func WithRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, requestKey{}, r)
}

// audit writes one row to the organization's audit log (fail-soft).
func (s *Service) audit(ctx context.Context, org int64, actor *int64, action string, id uuid.UUID, payload map[string]any) {
	if s.Activity == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["organization_id"] = org
	r, _ := ctx.Value(requestKey{}).(*http.Request)
	s.Activity.Record(orgctx.WithOrganizationID(ctx, org), actor, action, "remote_support_session", &id, payload, r)
}

func actorPtr(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}

// publish tells the agent screen about a state change (best effort).
func (s *Service) publish(ctx context.Context, row db.RemoteSupportSession, event string) {
	if s.Publisher == nil {
		return
	}
	view, err := s.view(ctx, row)
	if err != nil {
		s.log().Warn("remote_support_publish_view_failed", "session", row.Uuid, "error", err)
		return
	}
	msg := realtime.Message{Type: "remote_support." + event, Data: map[string]any{"session": view}}
	if err := s.Publisher.Publish(ctx, Channel(row.Uuid), msg); err != nil {
		s.log().Warn("remote_support_publish_failed", "session", row.Uuid, "error", err)
	}
}

// ---- projections ----

func controlActive(r db.RemoteSupportSession) bool {
	return r.ModeRequested == ModeControl && r.ControlConsentAt.Valid && !r.ControlRevokedAt.Valid &&
		(r.Status == StatusJoined || r.Status == StatusActive)
}

// endsAt is when the session's time limit is reached (from the start of
// screen sharing, or the viewing consent before that).
func endsAt(r db.RemoteSupportSession) *time.Time {
	base := r.StartedAt
	if !base.Valid {
		base = r.ViewConsentAt
	}
	if !base.Valid {
		return nil
	}
	t := base.Time.UTC().Add(time.Duration(r.MaxDurationMinutes) * time.Minute)
	return &t
}

func boolPtr(b pgtype.Bool) *bool {
	if !b.Valid {
		return nil
	}
	v := b.Bool
	return &v
}

func (s *Service) person(ctx context.Context, id pgtype.Int8) (*Person, error) {
	if !id.Valid {
		return nil, nil
	}
	u, err := s.Q.GetUserByID(ctx, id.Int64)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Person{UUID: u.Uuid, Name: strings.TrimSpace(u.Name + " " + u.Surname)}, nil
}

func (s *Service) view(ctx context.Context, r db.RemoteSupportSession) (SessionView, error) {
	conv, err := s.Q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: r.OrganizationID, ID: r.ConversationID})
	if err != nil {
		return SessionView{}, err
	}
	agent, err := s.person(ctx, r.AgentUserID)
	if err != nil {
		return SessionView{}, err
	}
	return SessionView{
		UUID: r.Uuid, ConversationUUID: conv.Uuid, Status: r.Status, ModeRequested: r.ModeRequested, Agent: agent,
		CodeExpiresAt: r.CodeExpiresAt.Time.UTC(), ResolvedAt: opt(r.ResolvedAt), ViewConsentAt: opt(r.ViewConsentAt),
		ControlConsentAt: opt(r.ControlConsentAt), ControlRevokedAt: opt(r.ControlRevokedAt), ControlActive: controlActive(r),
		RecordingConsent: boolPtr(r.RecordingConsent), RecordingStatus: r.RecordingStatus, ClientOS: r.ClientOs, ClientVersion: r.ClientVersion,
		MaxDurationMinutes: r.MaxDurationMinutes, StartedAt: opt(r.StartedAt), EndsAt: endsAt(r), EndedAt: opt(r.EndedAt),
		EndedReason: r.EndedReason, EndedBy: r.EndedBy, ReportedAt: opt(r.ReportedAt), BilledMinutes: r.BilledMinutes,
		Channel: Channel(r.Uuid), CreatedAt: r.CreatedAt.Time.UTC(),
	}, nil
}

// ---- settings and availability ----

func defaultSettings(org int64) db.RemoteSupportSetting {
	return db.RemoteSupportSetting{OrganizationID: org, AllowControl: true, Recording: "off", MaxDurationMinutes: DefaultMaxDuration,
		RecordingRetentionDays: 30, PlatformState: PlatformActive}
}

func (s *Service) settings(ctx context.Context, org int64) (db.RemoteSupportSetting, error) {
	row, err := s.Q.RemoteSupportSettings(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultSettings(org), nil
	}
	return row, err
}

// Settings explains whether remote support can be used and why not.
func (s *Service) Settings(ctx context.Context, org int64) (Settings, error) {
	row, err := s.settings(ctx, org)
	if err != nil {
		return Settings{}, err
	}
	out := Settings{Enabled: row.Enabled, AllowControl: row.AllowControl, Recording: row.Recording, MaxDurationMinutes: row.MaxDurationMinutes,
		RecordingRetentionDays: row.RecordingRetentionDays, PlatformState: row.PlatformState, PlatformReason: row.PlatformReason,
		PlatformChangedAt: opt(row.PlatformChangedAt), Configured: s.LiveKit.Enabled(), Entitled: true}
	if s.Entitlements != nil {
		if err = s.Entitlements.Require(ctx, org, FeatureEnabled); err != nil {
			var ent *entitlement.Error
			if !errors.As(err, &ent) {
				return out, err
			}
			out.Entitled = false
		}
	}
	out.Ready = out.Configured && out.Entitled && out.Enabled && out.PlatformState == PlatformActive
	return out, nil
}

func (s *Service) SaveSettings(ctx context.Context, org int64, in SettingsInput) (Settings, error) {
	if in.Recording == "" {
		in.Recording = "off"
	}
	if in.MaxDurationMinutes == 0 {
		in.MaxDurationMinutes = DefaultMaxDuration
	}
	if in.RecordingRetentionDays == 0 {
		in.RecordingRetentionDays = 30
	}
	if (in.Recording != "off" && in.Recording != "optional") || in.MaxDurationMinutes < 5 || in.MaxDurationMinutes > 240 ||
		in.RecordingRetentionDays < 1 || in.RecordingRetentionDays > 365 {
		return Settings{}, ErrInvalid
	}
	if _, err := s.Q.RemoteSupportSaveSettings(ctx, db.RemoteSupportSaveSettingsParams{OrganizationID: org, Enabled: in.Enabled, AllowControl: in.AllowControl,
		Recording: in.Recording, MaxDurationMinutes: in.MaxDurationMinutes, RecordingRetentionDays: in.RecordingRetentionDays}); err != nil {
		return Settings{}, err
	}
	return s.Settings(ctx, org)
}

// usable is the gate for every step that starts or continues a session.
func (s *Service) usable(ctx context.Context, org int64) (db.RemoteSupportSetting, error) {
	row, err := s.settings(ctx, org)
	if err != nil {
		return row, err
	}
	switch {
	case !s.LiveKit.Enabled():
		return row, ErrNotConfigured
	case row.PlatformState != PlatformActive:
		return row, ErrSuspended
	case !row.Enabled:
		return row, ErrDisabled
	}
	if s.Entitlements != nil {
		if err = s.Entitlements.Require(ctx, org, FeatureEnabled); err != nil {
			return row, err
		}
	}
	return row, nil
}

// monthStart is the metering month (TRT, like the billing usage periods).
func monthStart(t time.Time) time.Time {
	loc := time.FixedZone("TRT", 3*60*60)
	at := t.In(loc)
	return time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, loc)
}

func (s *Service) checkMinutes(ctx context.Context, org int64) error {
	if s.Entitlements == nil {
		return nil
	}
	used, err := s.Q.RemoteSupportMinutesSince(ctx, db.RemoteSupportMinutesSinceParams{OrganizationID: org, Since: ts(monthStart(s.now()))})
	if err != nil {
		return err
	}
	d, err := s.Entitlements.CheckLimit(ctx, org, FeatureMinutes, used, 1)
	var ent *entitlement.Error
	if errors.As(err, &ent) && ent.Code == entitlement.CodeLimitExceeded {
		return ErrNoMinutes
	}
	if err != nil {
		return err
	}
	if !d.Allowed && !d.Unlimited {
		return ErrNoMinutes
	}
	return nil
}

// ---- agent side ----

// Agent is the signed-in member calling a tenant route.
type Agent struct {
	ID         int64
	CanControl bool
	// Manager holds tenant.remote_support.settings: may see and end any
	// session of the organization.
	Manager bool
}

// Create starts a session on a conversation the agent can see (the caller
// checks visibility) and posts the code to the customer.
func (s *Service) Create(ctx context.Context, org int64, agent Agent, conv db.Conversation, in CreateInput) (Created, error) {
	if in.Mode == "" {
		in.Mode = ModeView
	}
	if in.Mode != ModeView && in.Mode != ModeControl {
		return Created{}, ErrInvalid
	}
	set, err := s.usable(ctx, org)
	if err != nil {
		return Created{}, err
	}
	if in.Mode == ModeControl && (!agent.CanControl || !set.AllowControl) {
		return Created{}, ErrDenied
	}
	if conv.Status == convmodel.StatusClosed {
		return Created{}, ErrConflict
	}
	if err = s.checkMinutes(ctx, org); err != nil {
		return Created{}, err
	}
	open, err := s.Q.RemoteSupportOpenForConversation(ctx, db.RemoteSupportOpenForConversationParams{OrganizationID: org, ConversationID: conv.ID})
	if err != nil {
		return Created{}, err
	}
	if open > 0 {
		return Created{}, ErrConflict
	}
	token, err := NewSecret()
	if err != nil {
		return Created{}, err
	}
	params := db.RemoteSupportCreateSessionParams{
		OrganizationID: org, ConversationID: conv.ID, AgentUserID: pgtype.Int8{Int64: agent.ID, Valid: true},
		ContactID: pgtype.Int8{Int64: conv.ContactID, Valid: conv.ContactID > 0}, CodeExpiresAt: ts(s.now().Add(CodeTTL)),
		JoinTokenHash: s.Hasher.JoinToken(token), ModeRequested: in.Mode, MaxDurationMinutes: set.MaxDurationMinutes,
	}
	if set.Recording == "optional" {
		// Asked separately; the customer may say no and continue unrecorded.
		params.RecordingConsent = pgtype.Bool{Bool: false, Valid: true}
	}
	var row db.RemoteSupportSession
	var code string
	for attempt := 0; ; attempt++ {
		if code, err = NewCode(); err != nil {
			return Created{}, err
		}
		id := uuid.New()
		params.CodeLookup, params.CodeHash, params.RoomName = s.Hasher.CodeLookup(code), s.Hasher.Code(code), RoomPrefix+id.String()
		row, err = s.Q.RemoteSupportCreateSession(ctx, params)
		var pe *pgconn.PgError
		// Another pending session holds the same 5-digit prefix: draw again.
		if errors.As(err, &pe) && pe.Code == "23505" && attempt < 8 {
			continue
		}
		break
	}
	if err != nil {
		return Created{}, err
	}
	out := Created{Code: code, JoinURL: "clickfix-support://join?t=" + token, DownloadURL: s.downloadURL()}
	if out.Session, err = s.view(ctx, row); err != nil {
		return Created{}, err
	}
	out.MessagePosted = s.postInvite(ctx, org, conv.ID, code)
	s.audit(ctx, org, actorPtr(agent.ID), "remote_support.created", row.Uuid, map[string]any{"mode": in.Mode, "conversation_uuid": conv.Uuid.String(), "message_posted": out.MessagePosted})
	return out, nil
}

func (s *Service) downloadURL() string {
	return strings.TrimRight(s.FrontendURL, "/") + "/download/support"
}

// postInvite drops the customer-visible message. A channel whose reply
// window is closed cannot carry it; the agent then reads the code out.
func (s *Service) postInvite(ctx context.Context, org, conversation int64, code string) bool {
	if s.Poster == nil {
		return false
	}
	locale := "tr"
	if o, err := s.Q.GetOrganizationByID(ctx, org); err == nil && strings.HasPrefix(o.Locale, "en") {
		locale = "en"
	}
	text := fmt.Sprintf("Uzaktan destek için ClickFix Destek uygulamasını indirin: %s\nUygulamayı açıp şu kodu girin: %s\nKod 15 dakika geçerlidir ve yalnız bir kez kullanılabilir. Ekranınızı paylaşmadan önce onayınız istenir.\nClickFix ve işletmeler sizden asla para transferi, banka girişi veya kart bilgisi istemez.",
		s.downloadURL(), FormatCode(code))
	if locale == "en" {
		text = fmt.Sprintf("For remote support, download the ClickFix Support app: %s\nOpen it and enter this code: %s\nThe code is valid for 15 minutes and can be used once. You will be asked to approve before your screen is shared.\nClickFix and businesses will never ask you for a money transfer, bank login or card details.",
			s.downloadURL(), FormatCode(code))
	}
	if _, err := s.Poster.PostAuto(ctx, org, conversation, "remote_support", text); err != nil {
		s.log().Warn("remote_support_invite_not_posted", "organization_id", org, "error", err)
		return false
	}
	return true
}

// Session returns one session of the organization.
func (s *Service) Session(ctx context.Context, org int64, id uuid.UUID) (db.RemoteSupportSession, error) {
	row, err := s.Q.RemoteSupportSession(ctx, db.RemoteSupportSessionParams{OrganizationID: org, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	return row, err
}

func (s *Service) View(ctx context.Context, row db.RemoteSupportSession) (SessionView, error) {
	return s.view(ctx, row)
}

func (s *Service) ForConversation(ctx context.Context, org, conversation int64) ([]SessionView, error) {
	rows, err := s.Q.RemoteSupportSessionsForConversation(ctx, db.RemoteSupportSessionsForConversationParams{OrganizationID: org, ConversationID: conversation})
	if err != nil {
		return nil, err
	}
	out := make([]SessionView, 0, len(rows))
	for _, r := range rows {
		v, err := s.view(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func open(r db.RemoteSupportSession) bool {
	return r.Status == StatusPending || r.Status == StatusJoined || r.Status == StatusActive
}

func agentIdentity(r db.RemoteSupportSession) string {
	return identityAgent + r.Uuid.String()
}

func customerIdentity(r db.RemoteSupportSession) string {
	return identityCustomer + r.Uuid.String()
}

// tokenTTL keeps a LiveKit token no longer than the session may last.
func (s *Service) tokenTTL(r db.RemoteSupportSession) time.Duration {
	end := endsAt(r)
	if end == nil {
		return time.Duration(r.MaxDurationMinutes) * time.Minute
	}
	ttl := end.Sub(s.now())
	if ttl < time.Minute {
		ttl = time.Minute
	}
	return ttl
}

// AgentToken: the agent subscribes to the screen and never publishes media.
// It may publish data (input events) only while the customer's control
// consent is in force, the organization allows control and the agent holds
// tenant.remote_support.control. Revoking control updates the connected
// participant at once and the next token is issued without data.
func (s *Service) AgentToken(ctx context.Context, org int64, agent Agent, id uuid.UUID) (Grant, error) {
	row, err := s.Session(ctx, org, id)
	if err != nil {
		return Grant{}, err
	}
	if !row.AgentUserID.Valid || row.AgentUserID.Int64 != agent.ID {
		return Grant{}, ErrDenied
	}
	if !open(row) {
		return Grant{}, ErrEnded
	}
	if !row.ViewConsentAt.Valid {
		return Grant{}, ErrConsent
	}
	set, err := s.usable(ctx, org)
	if err != nil {
		return Grant{}, err
	}
	if err = s.ensureRoom(ctx, row); err != nil {
		return Grant{}, err
	}
	data := controlActive(row) && set.AllowControl && agent.CanControl
	no := false
	grant := livekit.VideoGrant{Room: row.RoomName, CanPublish: &no, CanSubscribe: ptr(true), CanPublishData: &data, CanUpdateOwnMetadata: &no}
	name := ""
	if p, _ := s.person(ctx, row.AgentUserID); p != nil {
		name = p.Name
	}
	ttl := s.tokenTTL(row)
	tok, err := s.LiveKit.JoinToken(agentIdentity(row), name, `{"role":"agent"}`, grant, ttl)
	if err != nil {
		return Grant{}, err
	}
	v, err := s.view(ctx, row)
	if err != nil {
		return Grant{}, err
	}
	s.audit(ctx, org, actorPtr(agent.ID), "remote_support.agent_token_issued", row.Uuid, map[string]any{"can_publish_data": data})
	return Grant{ServerURL: s.LiveKit.URL(), Token: tok, Identity: agentIdentity(row), Room: row.RoomName, CanPublishData: data, ExpiresAt: s.now().Add(ttl), Session: v}, nil
}

func ptr[T any](v T) *T { return &v }

// EndByAgent stops the session from the agent side.
func (s *Service) EndByAgent(ctx context.Context, org int64, agent Agent, id uuid.UUID) (SessionView, error) {
	row, err := s.Session(ctx, org, id)
	if err != nil {
		return SessionView{}, err
	}
	if !agent.Manager && (!row.AgentUserID.Valid || row.AgentUserID.Int64 != agent.ID) {
		return SessionView{}, ErrDenied
	}
	if open(row) {
		if row, err = s.end(ctx, row, "agent", ByAgent, agent.ID); err != nil {
			return SessionView{}, err
		}
	}
	return s.view(ctx, row)
}

// end closes the session: final status, LiveKit room deleted (that also
// stops a running egress), minutes metered, agent notified, audit row.
func (s *Service) end(ctx context.Context, row db.RemoteSupportSession, reason, by string, byUser int64) (db.RemoteSupportSession, error) {
	status := StatusEnded
	switch {
	case !row.StartedAt.Valid && (reason == "code_expired" || reason == "timeout" || reason == "attempts"):
		status = StatusExpired
	case !row.StartedAt.Valid:
		status = StatusCancelled
	}
	ended, err := s.Q.RemoteSupportEnd(ctx, db.RemoteSupportEndParams{OrganizationID: row.OrganizationID, ID: row.ID, Status: status, EndedAt: ts(s.now()),
		EndedReason: reason, EndedBy: by, EndedByUserID: pgtype.Int8{Int64: byUser, Valid: byUser > 0}})
	if errors.Is(err, pgx.ErrNoRows) {
		// Ended concurrently: report the stored state.
		return s.Q.RemoteSupportSession(ctx, db.RemoteSupportSessionParams{OrganizationID: row.OrganizationID, Uuid: row.Uuid})
	}
	if err != nil {
		return row, err
	}
	if row.Status != StatusPending && s.LiveKit.Enabled() {
		if err := s.LiveKit.DeleteRoom(ctx, row.RoomName); err != nil {
			s.log().Warn("remote_support_delete_room_failed", "session", row.Uuid, "error", err)
		}
	}
	ended = s.meter(ctx, ended)
	action := "remote_support.ended"
	if status == StatusExpired {
		action = "remote_support.expired"
	}
	s.audit(ctx, row.OrganizationID, actorPtr(byUser), action, row.Uuid, map[string]any{"reason": reason, "by": by, "status": status, "billed_minutes": ended.BilledMinutes})
	s.timeline(ctx, ended, byUser)
	s.publish(ctx, ended, "ended")
	return ended, nil
}

// timeline leaves the session summary on the conversation (best effort):
// the agent and later readers see how long it lasted and what the customer
// allowed, without opening the audit log.
func (s *Service) timeline(ctx context.Context, row db.RemoteSupportSession, byUser int64) {
	tl, ok := s.Poster.(Timeline)
	if !ok || tl == nil {
		return
	}
	var seconds int64
	if row.StartedAt.Valid && row.EndedAt.Valid {
		seconds = int64(row.EndedAt.Time.Sub(row.StartedAt.Time) / time.Second)
	}
	attrs := map[string]any{
		"event": "remote_support_ended", "session_uuid": row.Uuid.String(), "status": row.Status, "mode": row.ModeRequested,
		"reason": row.EndedReason, "by": row.EndedBy, "duration_seconds": seconds, "billed_minutes": row.BilledMinutes,
		"view_consent": row.ViewConsentAt.Valid, "control_consent": row.ControlConsentAt.Valid,
		"recording": row.RecordingConsent.Valid && row.RecordingConsent.Bool,
	}
	if err := tl.SystemEvent(ctx, row.OrganizationID, row.ConversationID, convusecase.Actor{ID: byUser}, attrs); err != nil {
		s.log().Warn("remote_support_timeline_failed", "session", row.Uuid, "error", err)
	}
}

// meter charges the shared-screen time (whole minutes, rounded up) against
// remote_support.minutes once per session.
func (s *Service) meter(ctx context.Context, row db.RemoteSupportSession) db.RemoteSupportSession {
	if !row.StartedAt.Valid || !row.EndedAt.Valid {
		return row
	}
	d := row.EndedAt.Time.Sub(row.StartedAt.Time)
	minutes := int64((d + time.Minute - time.Second) / time.Minute)
	if minutes < 1 {
		minutes = 1
	}
	if s.Entitlements != nil {
		if _, err := s.Entitlements.Consume(ctx, row.OrganizationID, FeatureMinutes, minutes, "remote_support:"+row.Uuid.String()); err != nil {
			// The session already happened; the overage is logged, not lost silently.
			s.log().Warn("remote_support_meter_failed", "session", row.Uuid, "minutes", minutes, "error", err)
		}
	}
	if err := s.Q.RemoteSupportSetBilled(ctx, db.RemoteSupportSetBilledParams{OrganizationID: row.OrganizationID, ID: row.ID, BilledMinutes: minutes}); err != nil {
		s.log().Warn("remote_support_billed_update_failed", "session", row.Uuid, "error", err)
		return row
	}
	row.BilledMinutes = minutes
	return row
}
