package usecase

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	convrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const MaxReplayChunk = 512 << 10
const MaxReplayExpanded = 2 << 20
const SessionTimeout = 15 * time.Minute

// ReplayPolicy requires separate organization opt-in, plan grants and visitor consent.
type ReplayPolicy struct {
	Replay         bool     `json:"replay_enabled"`
	Cobrowse       bool     `json:"cobrowse_enabled"`
	Control        bool     `json:"control_enabled"`
	BlockSelectors []string `json:"block_selectors"`
	RetentionDays  int      `json:"retention_days"`
}

func Policy(w Widget) ReplayPolicy {
	p := ReplayPolicy{RetentionDays: 7, BlockSelectors: []string{}}
	raw, _ := json.Marshal(w.Instance.Settings["replay"])
	if json.Unmarshal(raw, &p) != nil {
		return ReplayPolicy{RetentionDays: 7, BlockSelectors: []string{}}
	}
	if p.RetentionDays < 1 || p.RetentionDays > 30 {
		p.RetentionDays = 7
	}
	if p.BlockSelectors == nil {
		p.BlockSelectors = []string{}
	}
	if len(p.BlockSelectors) > 20 {
		// Never discard a blocking rule while leaving capture enabled.
		p.Replay = false
		p.Cobrowse = false
		p.BlockSelectors = []string{}
	}
	for _, v := range p.BlockSelectors {
		if len(v) > 256 {
			p.Replay = false
			p.Cobrowse = false
		}
	}
	return p
}
func (s *Service) WithReplay(store storage.Driver, ent entitlement.Engine, secret string) *Service {
	s.replayStore = store
	s.replayEnt = ent
	s.replaySecret = []byte(secret)
	return s
}
func (s *Service) replayAllowed(ctx context.Context, w Widget, kind string) error {
	p := Policy(w)
	feature := "replay.enabled"
	enabled := p.Replay
	if kind == "cobrowse" {
		feature = "cobrowse.enabled"
		enabled = p.Cobrowse
	}
	if !enabled {
		return ErrUploadDisabled
	}
	if kind != "replay" && kind != "cobrowse" {
		return ErrInvalid
	}
	return entitlement.Require(ctx, s.replayEnt, w.Instance.OrganizationID, feature)
}

type Session struct {
	UUID           uuid.UUID  `json:"uuid"`
	Kind           string     `json:"kind"`
	Status         string     `json:"status"`
	Control        bool       `json:"control_enabled"`
	ConsentVersion string     `json:"consent_version"`
	AcceptedAt     *time.Time `json:"accepted_at"`
	TimeoutAt      time.Time  `json:"timeout_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
}

func sessionModel(v db.WidgetSession) Session {
	var accepted *time.Time
	if v.AcceptedAt.Valid {
		t := v.AcceptedAt.Time
		accepted = &t
	}
	status := v.Status
	if (status == "active" || status == "requested") && !v.TimeoutAt.Time.After(time.Now()) {
		status = "ended"
	}
	return Session{UUID: v.Uuid, Kind: v.Kind, Status: status, Control: v.ControlEnabled, ConsentVersion: v.ConsentVersion, AcceptedAt: accepted, TimeoutAt: v.TimeoutAt.Time, ExpiresAt: v.ExpiresAt.Time}
}
func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: !t.IsZero()} }
func (s *Service) auditSession(ctx context.Context, q *db.Queries, row db.WidgetSession, action string) error {
	ev := events.New("widget.session_"+action).WithOrganization(row.OrganizationID).WithEntity("widget_session", &row.ID, &row.Uuid).WithPayload(map[string]any{"kind": row.Kind, "status": row.Status, "control_enabled": row.ControlEnabled, "consent_version": row.ConsentVersion})
	if p, ok := authctx.PrincipalFrom(ctx); ok {
		ev = ev.WithActor(p.UserInternal)
	}
	if err := outbox.EnqueueQueries(ctx, q, ev); err != nil {
		return err
	}
	var actor *int64
	if p, ok := authctx.PrincipalFrom(ctx); ok {
		id := p.UserInternal
		actor = &id
	}
	activity.NewRecorder(q, s.log).Record(orgctx.WithOrganizationID(ctx, row.OrganizationID), actor, "widget.session_"+action, "widget_session", &row.Uuid, map[string]any{"kind": row.Kind, "status": row.Status, "control_enabled": row.ControlEnabled}, nil)
	return nil
}
func recordSessionConsent(ctx context.Context, q *db.Queries, conv db.Conversation, row db.WidgetSession, granted bool) error {
	_, err := q.ComplianceConsent(ctx, db.ComplianceConsentParams{OrganizationID: row.OrganizationID, ContactID: conv.ContactID, Purpose: "widget_" + row.Kind, Granted: granted, NoticeVersion: row.ConsentVersion, Source: "widget"})
	return err
}

func (s *Service) StartReplay(ctx context.Context, w Widget, c widget.Claims, id uuid.UUID, consent bool) (Session, error) {
	if !consent {
		return Session{}, ErrInvalid
	}
	conv, err := s.conversation(ctx, w, c, id)
	if err != nil {
		return Session{}, err
	}
	return s.startSession(ctx, w, conv, c.ExternalID(), "replay", false)
}
func (s *Service) startSession(ctx context.Context, w Widget, conv db.Conversation, visitor, kind string, control bool) (Session, error) {
	if err := s.replayAllowed(ctx, w, kind); err != nil {
		return Session{}, err
	}
	if control && !Policy(w).Control {
		return Session{}, ErrInvalid
	}
	if kind == "cobrowse" && (s.issuer == nil || !s.issuer.Enabled() || s.pub == nil) {
		return Session{}, ErrRealtimeDisabled
	}
	var row db.WidgetSession
	err := s.store.InTx(ctx, func(ctx context.Context, tx *convrepo.Tx) error {
		// Compliance locks contact before conversation. Match that order before
		// inserting the consent's contact foreign key to avoid an erasure deadlock.
		contact, err := tx.Q.ComplianceSubjectByID(ctx, db.ComplianceSubjectByIDParams{OrganizationID: conv.OrganizationID, ID: conv.ContactID})
		if err != nil {
			return err
		}
		if contact.DeletedAt.Valid || contact.ErasedAt.Valid {
			return ErrNotFound
		}
		if err := tx.Q.LockWidgetDiagnosticQuota(ctx, conv.OrganizationID); err != nil {
			return err
		}
		locked, err := tx.Q.LockConversation(ctx, db.LockConversationParams{OrganizationID: conv.OrganizationID, ID: conv.ID})
		if err != nil {
			return err
		}
		if locked.ContactID != conv.ContactID {
			return ErrNotFound
		}
		if locked.AnonymizedAt.Valid || locked.DeletedAt.Valid || locked.Status == "closed" {
			return ErrConversationClosed
		}
		n, err := tx.Q.WidgetDailySessionCount(ctx, db.WidgetDailySessionCountParams{OrganizationID: conv.OrganizationID, Kind: kind})
		if err != nil {
			return err
		}
		if n >= 1000 {
			return ErrDiagnosticQuota
		}
		if kind == "cobrowse" {
			if _, err = s.replayEnt.CheckLimit(ctx, conv.OrganizationID, "cobrowse.sessions", n, 1); err != nil {
				return err
			}
		}
		days := Policy(w).RetentionDays
		if _, err = s.replayEnt.CheckLimit(ctx, conv.OrganizationID, "replay.retention_days", 0, int64(days)); err != nil {
			return err
		}
		status := "active"
		accepted := ts(s.now())
		var actor pgtype.Int8
		if kind == "cobrowse" {
			status = "requested"
			accepted = pgtype.Timestamptz{}
			p := authctx.MustPrincipal(ctx)
			actor = pgtype.Int8{Int64: p.UserInternal, Valid: true}
		}
		row, err = tx.Q.InsertWidgetSession(ctx, db.InsertWidgetSessionParams{OrganizationID: conv.OrganizationID, ConversationID: conv.ID, VisitorID: visitor, Kind: kind, Status: status, ControlEnabled: control, RequestedBy: actor, AcceptedAt: accepted, TimeoutAt: ts(s.now().Add(SessionTimeout)), ExpiresAt: ts(s.now().Add(time.Duration(days) * 24 * time.Hour))})
		if err != nil {
			return err
		}
		if kind == "replay" {
			if err := recordSessionConsent(ctx, tx.Q, conv, row, true); err != nil {
				return err
			}
		}
		return s.auditSession(ctx, tx.Q, row, "created")
	})
	if err != nil {
		return Session{}, err
	}
	if kind == "cobrowse" {
		if err = s.pub.Publish(ctx, widget.VisitorChannel(w.Instance.UUID, visitor), map[string]any{"type": "cobrowse.requested", "conversation_uuid": conv.Uuid, "session": sessionModel(row)}); err != nil {
			_, _ = s.q.UpdateWidgetSession(ctx, db.UpdateWidgetSessionParams{OrganizationID: conv.OrganizationID, Uuid: row.Uuid, Status: "declined", EndedAt: ts(s.now())})
			return Session{}, err
		}
	}
	return sessionModel(row), nil
}

// AgentConversation applies the inbox team rule and requires an installed, active widget.
func (s *Service) AgentConversation(ctx context.Context, id uuid.UUID) (db.Conversation, Widget, error) {
	scope := orgctx.Must(ctx)
	p := authctx.MustPrincipal(ctx)
	conv, err := s.q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: scope.ID, Uuid: id})
	if err != nil {
		return conv, Widget{}, ErrNotFound
	}
	v, err := access.For(ctx, s.q, scope, p.UserInternal)
	if err != nil {
		return conv, Widget{}, err
	}
	ok, err := v.CanSee(ctx, s.q, scope.ID, conv.ID)
	if err != nil {
		return conv, Widget{}, err
	}
	if !ok || !access.CanRead(scope) || !conv.ChannelInstanceID.Valid || conv.AnonymizedAt.Valid || conv.DeletedAt.Valid {
		return conv, Widget{}, ErrNotFound
	}
	inst, err := s.q.GetChannelInstanceByID(ctx, db.GetChannelInstanceByIDParams{OrganizationID: scope.ID, ID: conv.ChannelInstanceID.Int64})
	if err != nil {
		return conv, Widget{}, ErrNotFound
	}
	opened, _, err := s.instances.ForWebhook(ctx, "widget", inst.Uuid)
	if err != nil || opened.Type != "widget" || opened.Status != "active" {
		return conv, Widget{}, ErrNotFound
	}
	return conv, Widget{Instance: opened, Settings: widget.Normalize(opened.Settings)}, nil
}
func (s *Service) RequestCobrowse(ctx context.Context, id uuid.UUID, control bool) (Session, error) {
	conv, w, err := s.AgentConversation(ctx, id)
	if err != nil {
		return Session{}, err
	}
	if control && !orgctx.Must(ctx).Has(rbac.PermTenantCobrowseControl) {
		return Session{}, ErrUnauthorized
	}
	identities, err := s.q.ListContactIdentities(ctx, db.ListContactIdentitiesParams{OrganizationID: conv.OrganizationID, ContactID: conv.ContactID})
	if err != nil {
		return Session{}, err
	}
	for _, ident := range identities {
		if ident.ChannelInstanceID.Valid && ident.ChannelInstanceID.Int64 == w.Instance.ID {
			return s.startSession(ctx, w, conv, ident.ExternalID, "cobrowse", control)
		}
	}
	return Session{}, ErrNotFound
}
func (s *Service) VisitorSession(ctx context.Context, w Widget, c widget.Claims, convID, id uuid.UUID) (db.WidgetSession, error) {
	conv, err := s.conversation(ctx, w, c, convID)
	if err != nil {
		return db.WidgetSession{}, err
	}
	row, err := s.q.GetWidgetSession(ctx, db.GetWidgetSessionParams{OrganizationID: conv.OrganizationID, Uuid: id})
	if err != nil || row.ConversationID != conv.ID || row.VisitorID != c.ExternalID() || conv.AnonymizedAt.Valid {
		return row, ErrNotFound
	}
	return row, nil
}
func (s *Service) ChangeSession(ctx context.Context, w Widget, row db.WidgetSession, action string) (Session, error) {
	if action != "accept" && action != "decline" && action != "stop" && action != "revoke" {
		return Session{}, ErrInvalid
	}
	before, err := s.q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: row.OrganizationID, ID: row.ConversationID})
	if err != nil {
		return Session{}, ErrNotFound
	}
	var out db.WidgetSession
	err = s.store.InTx(ctx, func(ctx context.Context, tx *convrepo.Tx) error {
		// Keep the same contact -> quota -> conversation order as erasure/start.
		contact, err := tx.Q.ComplianceSubjectByID(ctx, db.ComplianceSubjectByIDParams{OrganizationID: row.OrganizationID, ID: before.ContactID})
		if err != nil {
			return err
		}
		if contact.DeletedAt.Valid || contact.ErasedAt.Valid {
			return ErrNotFound
		}
		if err := tx.Q.LockWidgetDiagnosticQuota(ctx, row.OrganizationID); err != nil {
			return err
		}
		conv, err := tx.Q.LockConversation(ctx, db.LockConversationParams{OrganizationID: row.OrganizationID, ID: row.ConversationID})
		if err != nil {
			return err
		}
		locked, err := tx.Q.LockWidgetSession(ctx, db.LockWidgetSessionParams{OrganizationID: row.OrganizationID, Uuid: row.Uuid})
		if err != nil {
			return err
		}
		if conv.ContactID != before.ContactID {
			return ErrNotFound
		}
		if conv.AnonymizedAt.Valid || conv.DeletedAt.Valid {
			return ErrNotFound
		}
		status := "ended"
		accepted := pgtype.Timestamptz{}
		ended := ts(s.now())
		switch action {
		case "accept":
			if err := s.replayAllowed(ctx, w, locked.Kind); err != nil {
				return err
			}
			if locked.Status != "requested" || !locked.TimeoutAt.Time.After(s.now()) || conv.Status == "closed" {
				return ErrInvalid
			}
			status = "active"
			accepted = ts(s.now())
			ended = pgtype.Timestamptz{}
		case "decline":
			if locked.Status != "requested" {
				return ErrInvalid
			}
			status = "declined"
		case "revoke":
			status = "revoked"
		case "stop":
			if locked.Status != "active" && locked.Status != "requested" {
				return ErrInvalid
			}
		}
		out, err = tx.Q.UpdateWidgetSession(ctx, db.UpdateWidgetSessionParams{OrganizationID: row.OrganizationID, Uuid: row.Uuid, Status: status, AcceptedAt: accepted, EndedAt: ended})
		if err != nil {
			return err
		}
		if action == "accept" || action == "decline" || action == "revoke" {
			if err := recordSessionConsent(ctx, tx.Q, conv, out, action == "accept"); err != nil {
				return err
			}
		}
		return s.auditSession(ctx, tx.Q, out, action)
	})
	if err == nil && s.pub != nil {
		_ = s.pub.Publish(ctx, liveChannel(out.Uuid), map[string]any{"type": "cobrowse.status", "session": sessionModel(out)})
	}
	return sessionModel(out), err
}

// ValidateReplay bounds decompression before decoding JSON.
func ValidateReplay(data []byte) (json.RawMessage, error) {
	if len(data) == 0 || len(data) > MaxReplayChunk {
		return nil, ErrFileTooLarge
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, ErrInvalid
	}
	defer func() { _ = gz.Close() }()
	raw, err := io.ReadAll(io.LimitReader(gz, MaxReplayExpanded+1))
	if err != nil || len(raw) > MaxReplayExpanded {
		return nil, ErrInvalid
	}
	var events []json.RawMessage
	if json.Unmarshal(raw, &events) != nil || len(events) == 0 || len(events) > 5000 {
		return nil, ErrInvalid
	}
	for _, ev := range events {
		var v struct {
			Type      *int  `json:"type"`
			Timestamp int64 `json:"timestamp"`
		}
		if json.Unmarshal(ev, &v) != nil || v.Type == nil || *v.Type < 0 || *v.Type > 6 || v.Timestamp <= 0 {
			return nil, ErrInvalid
		}
	}
	var tree any
	if json.Unmarshal(raw, &tree) != nil {
		return nil, ErrInvalid
	}
	if !maskReplay(tree, 0) {
		return nil, ErrInvalid
	}
	clean, err := json.Marshal(tree)
	if err != nil {
		return nil, ErrInvalid
	}
	return clean, nil
}

var replayDimension = regexp.MustCompile(`^[0-9]{1,6}(px)?$`)
var replayTag = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,40}$`)

// Re-mask on the server so hand-crafted clients cannot persist DOM attributes or URLs.
func maskReplay(value any, depth int) bool {
	if depth > 64 {
		return false
	}
	switch v := value.(type) {
	case []any:
		for _, x := range v {
			if !maskReplay(x, depth+1) {
				return false
			}
		}
	case map[string]any:
		for k, x := range v {
			if k == "attributes" {
				safe := map[string]any{}
				if attrs, ok := x.(map[string]any); ok {
					for _, name := range []string{"width", "height", "rr_width", "rr_height", "rr_scrollLeft", "rr_scrollTop"} {
						switch dim := attrs[name].(type) {
						case string:
							if replayDimension.MatchString(dim) {
								safe[name] = dim
							}
						case float64:
							if dim >= 0 && dim <= 100000 {
								safe[name] = dim
							}
						}
					}
				}
				v[k] = safe
				continue
			}
			if text, ok := x.(string); ok {
				switch k {
				case "href":
					v[k] = "about:blank"
				case "tagName":
					if !replayTag.MatchString(text) {
						return false
					}
				case "textContent", "text":
					v[k] = strings.Map(func(r rune) rune {
						if r == ' ' || r == '\n' || r == '\t' {
							return r
						}
						return '*'
					}, text)
				default:
					v[k] = ""
				}
			} else if !maskReplay(x, depth+1) {
				return false
			}
		}
	}
	return true
}

func (s *Service) UploadReplay(ctx context.Context, w Widget, row db.WidgetSession, sequence int, data []byte) error {
	if sequence < 0 || sequence >= 300 {
		return ErrInvalid
	}
	raw, err := ValidateReplay(data)
	if err != nil {
		return err
	}
	// Store only the server-sanitized payload, never the supplied gzip bytes.
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err = gz.Write(raw); err != nil {
		return err
	}
	if err = gz.Close(); err != nil {
		return err
	}
	data = compressed.Bytes()
	if len(data) > MaxReplayChunk {
		return ErrFileTooLarge
	}
	if err = s.replayAllowed(ctx, w, row.Kind); err != nil {
		return err
	}
	if s.replayStore == nil {
		return ErrUploadDisabled
	}
	var key string
	err = s.store.InTx(ctx, func(ctx context.Context, tx *convrepo.Tx) error {
		if err := tx.Q.LockWidgetDiagnosticQuota(ctx, row.OrganizationID); err != nil {
			return err
		}
		conv, err := tx.Q.LockConversation(ctx, db.LockConversationParams{OrganizationID: row.OrganizationID, ID: row.ConversationID})
		if err != nil {
			return err
		}
		locked, err := tx.Q.LockWidgetSession(ctx, db.LockWidgetSessionParams{OrganizationID: row.OrganizationID, Uuid: row.Uuid})
		if err != nil {
			return err
		}
		if locked.Status != "active" || !locked.TimeoutAt.Time.After(s.now()) || !locked.ExpiresAt.Time.After(s.now()) || conv.AnonymizedAt.Valid || conv.DeletedAt.Valid || conv.Status == "closed" {
			return ErrConversationClosed
		}
		if row.Kind == "cobrowse" {
			return s.pub.Publish(ctx, liveChannel(row.Uuid), map[string]any{"type": "cobrowse.dom", "events": raw})
		}
		chunks, err := tx.Q.ListWidgetReplayChunks(ctx, db.ListWidgetReplayChunksParams{OrganizationID: row.OrganizationID, SessionID: row.ID})
		if err != nil {
			return err
		}
		if sequence != len(chunks) {
			return ErrInvalid
		}
		used, err := tx.Q.WidgetReplayUsage(ctx, row.OrganizationID)
		if err != nil {
			return err
		}
		if _, err = s.replayEnt.CheckLimit(ctx, row.OrganizationID, "replay.bytes", used, int64(len(data))); err != nil {
			return err
		}
		org, err := tx.Q.GetOrganizationByID(ctx, row.OrganizationID)
		if err != nil {
			return err
		}
		key = storage.ReplayChunkKey(org.Uuid.String(), row.Uuid.String(), uuid.NewString())
		if err = s.replayStore.Upload(ctx, storage.File{Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: "application/gzip"}, key); err != nil {
			return err
		}
		_, err = tx.Q.InsertWidgetReplayChunk(ctx, db.InsertWidgetReplayChunkParams{OrganizationID: row.OrganizationID, SessionID: row.ID, Sequence: safeconv.ToInt32(sequence), ObjectKey: key, SizeBytes: int64(len(data))})
		return err
	})
	if err != nil && key != "" {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = s.replayStore.Delete(cleanup, key)
	}
	return err
}
func liveChannel(id uuid.UUID) string { return "widget:live." + id.String() }
func (s *Service) LiveTokens(row db.WidgetSession) (map[string]any, error) {
	if row.Kind != "cobrowse" || row.Status != "active" || !row.TimeoutAt.Time.After(s.now()) {
		return nil, ErrInvalid
	}
	if s.issuer == nil || !s.issuer.Enabled() {
		return nil, ErrRealtimeDisabled
	}
	exp := row.TimeoutAt.Time
	channel := liveChannel(row.Uuid)
	conn, err := s.issuer.AnonymousConnectionToken(exp)
	if err != nil {
		return nil, err
	}
	sub, err := s.issuer.AnonymousSubscriptionToken(channel, exp)
	if err != nil {
		return nil, err
	}
	return map[string]any{"connection_token": conn, "subscription_token": sub, "channel": channel, "ws_url": s.issuer.WSURL(), "expires_at": exp}, nil
}

type Pointer struct {
	Type string  `json:"type"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

func (s *Service) AgentPointer(ctx context.Context, w Widget, row db.WidgetSession, in Pointer) error {
	return s.store.InTx(ctx, func(ctx context.Context, tx *convrepo.Tx) error {
		if err := tx.Q.LockWidgetDiagnosticQuota(ctx, row.OrganizationID); err != nil {
			return err
		}
		conv, err := tx.Q.LockConversation(ctx, db.LockConversationParams{OrganizationID: row.OrganizationID, ID: row.ConversationID})
		if err != nil {
			return err
		}
		if conv.AnonymizedAt.Valid || conv.DeletedAt.Valid || conv.Status == "closed" {
			return ErrNotFound
		}
		current, err := tx.Q.LockWidgetSession(ctx, db.LockWidgetSessionParams{OrganizationID: row.OrganizationID, Uuid: row.Uuid})
		if err != nil {
			return err
		}
		return s.agentPointer(ctx, tx.Q, w, current, in)
	})
}
func (s *Service) agentPointer(ctx context.Context, q *db.Queries, w Widget, row db.WidgetSession, in Pointer) error {
	if err := s.replayAllowed(ctx, w, "cobrowse"); err != nil {
		return err
	}
	if row.Kind != "cobrowse" || row.Status != "active" || !row.TimeoutAt.Time.After(s.now()) || s.pub == nil {
		return ErrInvalid
	}
	if in.Type != "pointer" && in.Type != "highlight" && in.Type != "control_click" {
		return ErrInvalid
	}
	if in.X < 0 || in.X > 100000 || in.Y < 0 || in.Y > 100000 {
		return ErrInvalid
	}
	if in.Type == "control_click" && (!row.ControlEnabled || !Policy(w).Control || !orgctx.Must(ctx).Has(rbac.PermTenantCobrowseControl)) {
		return ErrUnauthorized
	}
	if in.Type == "control_click" {
		if err := s.auditSession(ctx, q, row, "control_click"); err != nil {
			return err
		}
	}
	return s.pub.Publish(ctx, liveChannel(row.Uuid), map[string]any{"type": "cobrowse.pointer", "event": in})
}
func (s *Service) AgentSessions(ctx context.Context, id uuid.UUID) ([]Session, error) {
	conv, _, err := s.AgentConversation(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListWidgetSessions(ctx, db.ListWidgetSessionsParams{OrganizationID: conv.OrganizationID, ConversationID: conv.ID})
	if err != nil {
		return nil, err
	}
	out := []Session{}
	for _, row := range rows {
		out = append(out, sessionModel(row))
	}
	return out, nil
}
func (s *Service) AgentSession(ctx context.Context, convID, id uuid.UUID) (Widget, db.WidgetSession, error) {
	conv, w, err := s.AgentConversation(ctx, convID)
	if err != nil {
		return w, db.WidgetSession{}, err
	}
	row, err := s.q.GetWidgetSession(ctx, db.GetWidgetSessionParams{OrganizationID: conv.OrganizationID, Uuid: id})
	if err != nil || row.ConversationID != conv.ID || !row.ExpiresAt.Time.After(s.now()) {
		return w, row, ErrNotFound
	}
	return w, row, nil
}
