// Package usecase is the public visitor API of the web widget (CF-55): a
// visitor of a customer's site bootstraps a widget instance, gets a signed
// visitor token, optionally proves who they are (identify with an HMAC
// user_hash), starts conversations and writes messages that run through the
// same inbound pipeline as every other channel, reads the replies, marks
// them read, shows typing and rates finished conversations.
//
// Visitors only ever see conversations of their own contact on the same
// widget instance, and only messages the contact wrote or the channel
// delivered (never private notes).
package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	attachmentsmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	convmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	convrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	deptmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments/model"
	formsengine "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/engine"
	formsmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/businesshours"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Errors returned by the service.
var (
	ErrNotFound           = errors.New("widget: not found")
	ErrInvalid            = errors.New("widget: invalid request")
	ErrOriginNotAllowed   = errors.New("widget: this site may not use the widget")
	ErrUnauthorized       = errors.New("widget: visitor token is missing or invalid")
	ErrIdentityDisabled   = errors.New("widget: identity verification is not configured")
	ErrIdentityInvalid    = errors.New("widget: user_hash does not match user_id")
	ErrIdentityRequired   = errors.New("widget: this widget only talks to identified users")
	ErrConversationClosed = errors.New("widget: conversation is closed")
	ErrRatingNotAllowed   = errors.New("widget: only finished conversations can be rated")
	ErrRatingDisabled     = errors.New("widget: ratings are turned off")
	ErrUploadDisabled     = errors.New("widget: file upload is turned off")
	ErrFileTooLarge       = errors.New("widget: file exceeds the size limit")
	ErrRealtimeDisabled   = errors.New("widget: realtime is disabled")
	ErrFileType           = errors.New("widget: this file type is not allowed")
	ErrConsentRequired    = errors.New("widget: consent is required")
)

// MaxUploadBytes caps one visitor file regardless of the organization's
// attachment limit.
const MaxUploadBytes = 10 << 20

// InstanceSource opens a widget instance by its public id
// (channels usecase ForWebhook).
type InstanceSource interface {
	ForWebhook(ctx context.Context, channelType string, id uuid.UUID) (channels.Instance, channels.Adapter, error)
}

// Pipeline is the conversations core the visitor API writes through.
type Pipeline interface {
	Receive(ctx context.Context, inst channels.Instance, res channels.WebhookResult) (convusecase.ReceiveResult, error)
	ApplyStatus(ctx context.Context, inst channels.Instance, st channels.StatusUpdate) (bool, error)
}

// Files signs download links of message attachments and reports the
// organization's per-file limit.
type Files interface {
	SignedURL(ctx context.Context, orgID int64, id uuid.UUID, variant string, ttl time.Duration) (attachmentsmodel.SignedURL, error)
	MaxFileBytes(ctx context.Context, orgID int64) (int64, error)
}

// TokenIssuer mints anonymous Centrifugo tokens (realtime.TokenIssuer).
type TokenIssuer interface {
	Enabled() bool
	WSURL() string
	AnonymousConnectionToken(exp time.Time) (string, error)
	AnonymousSubscriptionToken(channel string, exp time.Time) (string, error)
}

// Service is the visitor API.
type Service struct {
	replayStore  storage.Driver
	replayEnt    entitlement.Engine
	replaySecret []byte
	q            *db.Queries
	store        *convrepository.Store
	instances    InstanceSource
	pipeline     Pipeline
	files        Files
	signer       *widget.Signer
	pub          realtime.Publisher
	issuer       TokenIssuer
	appOrigin    string
	forms        FormSource
	departments  DepartmentSource
	fields       FieldSource
	hours        businesshours.Source
	log          *slog.Logger
	now          func() time.Time
}

// Deps wires the service.
type Deps struct {
	Queries   *db.Queries
	Store     *convrepository.Store
	Instances InstanceSource
	Pipeline  Pipeline
	Files     Files
	Signer    *widget.Signer
	Publisher realtime.Publisher
	Issuer    TokenIssuer
	// AppOrigin is the ClickFix app origin: always allowed (admin preview).
	AppOrigin string
	// Forms offers the widget's request form (CF-52); optional.
	Forms FormSource
	// Hours is the organization's working-hours source (the SLA schedule,
	// CF-198); nil means always open.
	Hours businesshours.Source
	Log   *slog.Logger
}

// FormChannel is the request form channel of the web widget.
const FormChannel = "widget"

// FormSource is the request form use case (CF-52). The widget's pre-chat
// form is one of these forms (CF-198): preferred is the form the widget
// settings pick (uuid.Nil = the first active widget form).
type FormSource interface {
	Offered(ctx context.Context, orgID int64, channel string, preferred uuid.UUID) (*formsmodel.PublicForm, error)
	CheckOffered(ctx context.Context, orgID int64, channel string, preferred uuid.UUID, sub *formsmodel.Submission, optional ...string) (*formsengine.Submission, uuid.UUID, error)
	Apply(ctx context.Context, orgID, conversationID int64, formUUID uuid.UUID, sub formsengine.Submission) error
}

// preChatForm is the request form the widget settings pick (uuid.Nil = the
// first active widget form).
func preChatForm(set widget.Settings) uuid.UUID {
	id, err := uuid.Parse(set.PreChat.FormUUID)
	if err != nil {
		return uuid.Nil
	}
	return id
}

// New builds the service.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		q: d.Queries, store: d.Store, instances: d.Instances, pipeline: d.Pipeline, files: d.Files,
		signer: d.Signer, pub: d.Publisher, issuer: d.Issuer, appOrigin: strings.TrimRight(d.AppOrigin, "/"),
		forms: d.Forms, hours: d.Hours, log: log, now: func() time.Time { return time.Now().UTC() },
	}
}

// hoursState is the organization's working-hours state at now (CF-198). An
// error or a missing source means open: hours never take the widget down.
func (s *Service) hoursState(ctx context.Context, org int64, now time.Time) businesshours.State {
	if s.hours == nil {
		return businesshours.State{Open: true}
	}
	st, err := s.hours.HoursState(ctx, org, now)
	if err != nil {
		s.log.Warn("widget_hours_state_failed", "organization_id", org, "error", err)
		return businesshours.State{Open: true}
	}
	return st
}

// WithClock replaces the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service { s.now = now; return s }

// WithForms sets the request form source (tests).
func (s *Service) WithForms(f FormSource) *Service { s.forms = f; return s }

// WithHours sets the working-hours source (CF-198).
func (s *Service) WithHours(h businesshours.Source) *Service { s.hours = h; return s }

// Widget is an opened widget instance for one request.
type Widget struct {
	Instance channels.Instance
	Settings widget.Settings
	// App is set when the request came from a native app with an app key
	// (CF-201) instead of a page origin.
	App *AppIdentity
}

// Open loads a live widget by public id and checks the page origin against
// its allowlist.
func (s *Service) Open(ctx context.Context, publicID uuid.UUID, origin string) (Widget, error) {
	w, err := s.load(ctx, publicID)
	if err != nil {
		return Widget{}, err
	}
	if !s.originAllowed(w.Settings, origin) {
		return Widget{}, ErrOriginNotAllowed
	}
	return w, nil
}

// load resolves a live widget and its (brand-merged, normalized) settings.
func (s *Service) load(ctx context.Context, publicID uuid.UUID) (Widget, error) {
	inst, _, err := s.instances.ForWebhook(ctx, channels.TypeWidget, publicID)
	if err != nil {
		return Widget{}, ErrNotFound
	}
	if inst.Status != "active" && inst.Status != "error" {
		return Widget{}, ErrNotFound
	}
	if inst.BrandID != nil {
		b, err := s.q.GetBrand(ctx, db.GetBrandParams{OrganizationID: inst.OrganizationID, Uuid: *inst.BrandID})
		if err != nil {
			return Widget{}, err
		}
		defaults := map[string]any{}
		if err := json.Unmarshal(b.WidgetDefaults, &defaults); err != nil {
			return Widget{}, err
		}
		defaults["color"] = b.PrimaryColor
		for k, v := range inst.Settings {
			defaults[k] = v
		}
		inst.Settings = defaults
	}
	return Widget{Instance: inst, Settings: widget.Normalize(inst.Settings)}, nil
}

func (s *Service) originAllowed(set widget.Settings, origin string) bool {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	if s.appOrigin != "" && origin != "" && strings.EqualFold(origin, s.appOrigin) {
		return true
	}
	return set.OriginAllowed(origin)
}

// Authenticate verifies a visitor token for a widget.
func (s *Service) Authenticate(w Widget, token string) (widget.Claims, error) {
	c, err := s.signer.Verify(token, w.Instance.UUID)
	if err != nil {
		return widget.Claims{}, ErrUnauthorized
	}
	return c, nil
}

// ---------------------------------------------------------------------------
// Bootstrap

// TeamMember is one face on the widget header.
type TeamMember struct {
	Name      string  `json:"name"`
	Initials  string  `json:"initials"`
	AvatarURL *string `json:"avatar_url"`
}

// PublicSettings is what a visitor's browser may know of the settings: no
// allowlist, member ids, attribute targets or handoff keywords.
type PublicSettings struct {
	SchemaVersion   int                     `json:"schema_version"`
	Color           string                  `json:"color"`
	Position        string                  `json:"position"`
	Locale          string                  `json:"locale"`
	GreetingTitle   string                  `json:"greeting_title"`
	GreetingText    string                  `json:"greeting_text"`
	ShowTeam        bool                    `json:"show_team"`
	PreChat         widget.PreChat          `json:"prechat"`
	OfflineMessage  string                  `json:"offline_message"`
	RequireIdentity bool                    `json:"require_identity"`
	FileUpload      bool                    `json:"file_upload"`
	CSAT            bool                    `json:"csat"`
	Appearance      widget.Appearance       `json:"appearance"`
	Texts           map[string]widget.Texts `json:"texts"`
	ReplyTime       widget.ReplyTimeSetting `json:"reply_time"`
	Home            widget.Home             `json:"home"`
	QuickReplies    []widget.Localized      `json:"quick_replies"`
	CSATOptions     widget.CSATOptions      `json:"csat_options"`
	Offline         widget.Offline          `json:"offline"`
	AwayMode        bool                    `json:"away_mode"`
	Channels        widget.ChannelLinks     `json:"channels"`
	AI              PublicAI                `json:"ai"`
	Attachments     widget.Attachments      `json:"attachments"`
	Features        widget.Features         `json:"features"`
	Triggers        []widget.Trigger        `json:"triggers"`
	Targeting       widget.Targeting        `json:"targeting"`
	Privacy         widget.Privacy          `json:"privacy"`
	Mobile          widget.Mobile           `json:"mobile"`
	// AttributeKeys are the attributes the SDK may set (setAttributes);
	// anonymous ones work before identify.
	AttributeKeys []PublicAttribute `json:"attribute_keys"`
}

// PublicAI is the visitor side of the AI agent switch.
type PublicAI struct {
	Enabled     bool `json:"enabled"`
	HumanButton bool `json:"human_button"`
}

// PublicAttribute is one settable attribute key.
type PublicAttribute struct {
	Key       string `json:"key"`
	Anonymous bool   `json:"anonymous"`
}

// Public returns the visitor-safe view of settings.
func Public(set widget.Settings) PublicSettings {
	triggers := []widget.Trigger{}
	for _, t := range set.Triggers {
		if t.Enabled {
			triggers = append(triggers, t)
		}
	}
	attrs := make([]PublicAttribute, 0, len(set.AttributeMapping))
	for _, m := range set.AttributeMapping {
		attrs = append(attrs, PublicAttribute{Key: m.Key, Anonymous: m.Anonymous})
	}
	return PublicSettings{
		SchemaVersion: set.SchemaVersion, Color: set.Color, Position: set.Position, Locale: set.Locale,
		GreetingTitle: set.GreetingTitle, GreetingText: set.GreetingText, ShowTeam: set.ShowTeam, PreChat: set.PreChat,
		OfflineMessage: set.OfflineMessage, RequireIdentity: set.RequireIdentity,
		FileUpload: set.FileUpload, CSAT: set.CSAT, Appearance: set.Appearance, Texts: set.Texts, ReplyTime: set.ReplyTime,
		Home: set.Home, QuickReplies: set.QuickReplies, CSATOptions: set.CSATOptions, Offline: set.Offline,
		AwayMode: set.AwayMode, Channels: set.Channels, AI: PublicAI{Enabled: set.AI.Enabled, HumanButton: set.AI.HumanButton},
		Attachments: set.Attachments, Features: set.Features, Triggers: triggers, Targeting: set.Targeting,
		Privacy: set.Privacy, Mobile: set.Mobile, AttributeKeys: attrs,
	}
}

// Visitor is the visitor session of a bootstrap or identify.
type Visitor struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Verified  bool      `json:"verified"`
	// CompanyVerified is true when the visitor's contact proved its company
	// with a signed company id (CF-206) and still belongs to it.
	CompanyVerified bool            `json:"company_verified"`
	Contact         *VisitorContact `json:"contact"`
}

// VisitorContact is what the visitor told us about themselves.
type VisitorContact struct {
	Name  string  `json:"name"`
	Email *string `json:"email"`
	Phone *string `json:"phone"`
}

// Bootstrap is the single snapshot a widget starts from (docs/widget/02:
// the backend is the source of truth, the browser only caches).
type Bootstrap struct {
	Widget struct {
		BrandID  *uuid.UUID     `json:"brand_id"`
		ID       uuid.UUID      `json:"id"`
		Name     string         `json:"name"`
		Settings PublicSettings `json:"settings"`
		Online   bool           `json:"online"`
		Team     []TeamMember   `json:"team"`
		// Reply is the expected reply time line (CF-150).
		Reply widget.ReplyEstimate `json:"reply"`
	} `json:"widget"`
	// Context is the server's view of this page view (CF-150): whether the
	// widget should show at all, and which triggers' page-level conditions
	// hold. The SDK re-checks targeting and triggers on SPA navigation.
	Context  BootstrapContext `json:"context"`
	Visitor  Visitor          `json:"visitor"`
	Realtime struct {
		Enabled bool `json:"enabled"`
	} `json:"realtime"`
	Limits struct {
		MaxFileBytes  int64 `json:"max_file_bytes"`
		MaxTextLength int   `json:"max_text_length"`
	} `json:"limits"`
	Conversations []ConversationSummary `json:"conversations"`
	// RequestForm is the start form of a new conversation (CF-52); null
	// when the organization offers none on the widget.
	RequestForm *formsmodel.PublicForm `json:"request_form"`
	// Departments the visitor may pick for a new conversation (CF-215);
	// empty hides the picker and the AI routes the first message.
	Departments []deptmodel.WidgetDepartment `json:"departments"`
}

// DepartmentSource is the departments use case (CF-215): the widget's
// picker and the visitor's pick on a new conversation.
type DepartmentSource interface {
	Widget(ctx context.Context, org int64, brand pgtype.UUID) ([]deptmodel.WidgetDepartment, error)
	PickFromWidget(ctx context.Context, org, conversationID int64, department uuid.UUID) error
}

// WithDepartments adds the department picker (CF-215).
func (s *Service) WithDepartments(d DepartmentSource) *Service { s.departments = d; return s }

// brandOf is the widget's brand as a query argument.
func brandOf(w Widget) pgtype.UUID {
	if w.Instance.BrandID == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *w.Instance.BrandID, Valid: true}
}

// Bootstrap starts or resumes a visitor session. A valid (or recently
// expired but authentic) token of this widget resumes the same visitor;
// anything else starts a new anonymous visitor.
func (s *Service) Bootstrap(ctx context.Context, w Widget, token string, in BootstrapInput) (Bootstrap, error) {
	var claims widget.Claims
	if token != "" {
		c, err := s.signer.Verify(token, w.Instance.UUID)
		switch {
		case err == nil:
			claims = c
		case errors.Is(err, widget.ErrTokenExpired) && s.signer.Renewable(c):
			claims = c
		}
	}
	returning := claims.Visitor != ""
	if claims.Visitor == "" {
		claims = widget.Claims{Instance: w.Instance.UUID, Visitor: uuid.NewString()}
	}
	var out Bootstrap
	set := w.Settings
	now := s.now()
	out.Widget.ID, out.Widget.Name = w.Instance.UUID, w.Instance.Name
	out.Widget.BrandID = w.Instance.BrandID
	out.Widget.Settings = Public(set)
	hours := s.hoursState(ctx, w.Instance.OrganizationID, now)
	out.Widget.Online = set.Online(hours.Open)
	out.Widget.Reply = set.EstimateReply(hours, s.medianFirstResponse(ctx, w))
	s.touchInstall(ctx, w, in)
	out.Widget.Team = []TeamMember{}
	if set.ShowTeam {
		team, err := s.team(ctx, w)
		if err != nil {
			return Bootstrap{}, err
		}
		out.Widget.Team = team
	}
	// The pre-chat form is a request form (CF-198); off means no form.
	if s.forms != nil && set.PreChat.Enabled {
		form, err := s.forms.Offered(ctx, w.Instance.OrganizationID, FormChannel, preChatForm(set))
		if err != nil {
			return Bootstrap{}, err
		}
		out.RequestForm = form
	}
	out.Departments = []deptmodel.WidgetDepartment{}
	if s.departments != nil {
		list, err := s.departments.Widget(ctx, w.Instance.OrganizationID, brandOf(w))
		if err != nil {
			return Bootstrap{}, err
		}
		out.Departments = list
	}
	out.Realtime.Enabled = s.issuer != nil && s.issuer.Enabled()
	out.Limits.MaxTextLength = widget.MaxTextLength
	out.Limits.MaxFileBytes = s.maxFileBytes(ctx, w.Instance.OrganizationID)
	v, err := s.visitor(ctx, w, claims)
	if err != nil {
		return Bootstrap{}, err
	}
	out.Visitor = v
	out.Conversations, err = s.ListConversations(ctx, w, claims)
	if err != nil {
		return Bootstrap{}, err
	}
	page := widget.PageContext{
		URL: clip(in.PageURL, 2000), Device: widget.DeviceOf(in.UserAgent), Country: strings.ToUpper(in.Country),
		Language: strings.ToLower(clip(in.Language, 2)), Returning: returning || len(out.Conversations) > 0, Identified: claims.Verified(),
	}
	out.Context = BootstrapContext{Device: page.Device, Country: page.Country, Returning: page.Returning, EligibleTriggers: set.EligibleTriggers(page)}
	out.Context.Visible, out.Context.HiddenReason = set.Targeting.Visible(page)
	if out.Context.Visible && !out.Widget.Online && set.Offline.Mode == "hide" && len(out.Conversations) == 0 {
		out.Context.Visible, out.Context.HiddenReason = false, "offline"
	}
	return out, nil
}

// BootstrapInput describes the page view (all optional; older SDKs send
// nothing). UserAgent, Country and Origin come from the request.
type BootstrapInput struct {
	PageURL    string `json:"page_url"`
	Referrer   string `json:"referrer"`
	Language   string `json:"language"`
	SDKVersion string `json:"sdk_version"`
	UserAgent  string `json:"-"`
	Country    string `json:"-"`
	Origin     string `json:"-"`
}

// BootstrapContext is the server's evaluation of the page view.
type BootstrapContext struct {
	Visible          bool     `json:"visible"`
	HiddenReason     string   `json:"hidden_reason"`
	Device           string   `json:"device"`
	Country          string   `json:"country"`
	Returning        bool     `json:"returning"`
	EligibleTriggers []string `json:"eligible_triggers"`
}

// medianFirstResponse is the recent median first response of the widget
// (nil without data or on error: the estimate falls back to online state).
func (s *Service) medianFirstResponse(ctx context.Context, w Widget) *time.Duration {
	if w.Settings.ReplyTime.Mode != "auto" {
		return nil
	}
	sec, err := s.q.WidgetMedianFirstResponse(ctx, db.WidgetMedianFirstResponseParams{
		OrganizationID: w.Instance.OrganizationID, ChannelInstanceID: pgtype.Int8{Int64: w.Instance.ID, Valid: true},
	})
	if err != nil || sec < 0 {
		return nil
	}
	d := time.Duration(sec * float64(time.Second))
	return &d
}

// touchInstall records the page origin the widget runs on (install
// checker). The app's own origin (editor preview) is not an install.
func (s *Service) touchInstall(ctx context.Context, w Widget, in BootstrapInput) {
	origin := strings.TrimRight(strings.TrimSpace(in.Origin), "/")
	if origin == "" || origin == "null" || strings.EqualFold(origin, s.appOrigin) || len(origin) > 300 {
		return
	}
	page := in.PageURL
	if !strings.HasPrefix(strings.ToLower(page), strings.ToLower(origin)) {
		page = ""
	}
	if i := strings.IndexAny(page, "?#"); i >= 0 {
		// Query strings may carry personal data; keep the path only.
		page = page[:i]
	}
	if err := s.q.TouchWidgetInstall(ctx, db.TouchWidgetInstallParams{
		OrganizationID: w.Instance.OrganizationID, ChannelInstanceID: w.Instance.ID, Origin: strings.ToLower(origin),
		SdkVersion: clip(in.SDKVersion, 40), LastPageUrl: clip(page, 500),
	}); err != nil {
		s.log.Warn("widget_install_touch_failed", "instance", w.Instance.UUID, "error", err)
	}
}

func (s *Service) visitor(ctx context.Context, w Widget, claims widget.Claims) (Visitor, error) {
	token, exp := s.signer.Issue(claims)
	v := Visitor{ID: claims.Visitor, Token: token, ExpiresAt: exp, Verified: claims.Verified()}
	contact, ok, err := s.contactOf(ctx, w, claims)
	if err != nil {
		return Visitor{}, err
	}
	if ok {
		v.Contact = &VisitorContact{Name: contact.Name, Email: textPtr(contact.Email), Phone: textPtr(contact.Phone)}
		// The proof is the contact's own (a signed identify of this user),
		// never the company's: a contact attached to a verified company
		// by a domain or name hint is not reported as proved.
		v.CompanyVerified = contact.CompanyID.Valid && contact.CompanyVerifiedAt.Valid
	}
	return v, nil
}

func (s *Service) team(ctx context.Context, w Widget) ([]TeamMember, error) {
	ids := []uuid.UUID{}
	for _, raw := range w.Settings.TeamMemberUUIDs {
		if id, err := uuid.Parse(raw); err == nil {
			ids = append(ids, id)
		}
	}
	rows, err := s.q.ListWidgetTeam(ctx, db.ListWidgetTeamParams{OrganizationID: w.Instance.OrganizationID, UserUuids: ids, RowLimit: 5})
	if err != nil {
		return nil, err
	}
	out := make([]TeamMember, 0, len(rows))
	for _, r := range rows {
		out = append(out, TeamMember{Name: strings.TrimSpace(r.Name), Initials: initials(r.Name, r.Surname)})
	}
	return out, nil
}

func (s *Service) maxFileBytes(ctx context.Context, orgID int64) int64 {
	limit := int64(MaxUploadBytes)
	if s.files != nil {
		if n, err := s.files.MaxFileBytes(ctx, orgID); err == nil && n > 0 && n < limit {
			limit = n
		}
	}
	return limit
}

// ---------------------------------------------------------------------------
// Identity

// Company is the B2B company an identified user belongs to. ID is the
// company's id in the customer's system (companies.external_id); it binds
// the contact to the company only when user_hash covers it (CF-206).
type Company struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

// IdentifyInput proves the visitor is one of the customer's users.
type IdentifyInput struct {
	UserID   string   `json:"user_id"`
	UserHash string   `json:"user_hash"`
	Name     string   `json:"name"`
	Email    string   `json:"email"`
	Phone    string   `json:"phone"`
	Company  *Company `json:"company"`
}

// companyID is the external company id of an identify call, as it is
// signed and stored (trimmed, at most 200 characters, like user_id).
func (in IdentifyInput) companyID() string {
	if in.Company == nil {
		return ""
	}
	return clip(strings.TrimSpace(in.Company.ID), 200)
}

// Identify verifies user_hash with the widget's identity secret, links the
// visitor to the user's contact (moving the anonymous conversations over,
// without creating a second contact) and returns a token for the user.
//
// user_hash signs either user_id alone (the original formula) or user_id
// and company.id together (CF-206: HMAC over user_id + "\n" + company_id).
// With a company id in the call the signed formula is tried first; when it
// matches, the contact is bound to that company by the proved id. When only
// the user-only formula matches, the user is verified but the company id
// is a claim, not proof: it binds no external id and is not stored, so the
// company's name and domain are only hints (as a call without an id). A
// hash that matches neither is rejected, including one that signs another
// company id.
func (s *Service) Identify(ctx context.Context, w Widget, claims widget.Claims, in IdentifyInput) (Visitor, error) {
	in.UserID = strings.TrimSpace(in.UserID)
	if in.UserID == "" || len(in.UserID) > 200 {
		return Visitor{}, fmt.Errorf("%w: user_id must be 1-200 characters", ErrInvalid)
	}
	companyID := in.companyID()
	if strings.ContainsAny(in.UserID+companyID, "\n\r") {
		// The newline separates the two ids inside the signed bytes.
		return Visitor{}, fmt.Errorf("%w: user_id and company.id must not contain line breaks", ErrInvalid)
	}
	secret := w.Instance.Credentials[widget.IdentitySecretField]
	if secret == "" {
		s.noteIdentityRejection(ctx, w, RejectionIdentityDisabled)
		return Visitor{}, ErrIdentityDisabled
	}
	companySigned := companyID != "" && widget.VerifyUserCompanyHash(secret, in.UserID, companyID, in.UserHash)
	if !companySigned && !widget.VerifyUserHash(secret, in.UserID, in.UserHash) {
		s.noteIdentityRejection(ctx, w, RejectionIdentityInvalid)
		return Visitor{}, ErrIdentityInvalid
	}
	email, phone := cleanEmail(in.Email), clip(strings.TrimSpace(in.Phone), 40)
	next := claims
	next.User = in.UserID
	org, inst := w.Instance.OrganizationID, w.Instance.ID
	err := s.store.InTx(ctx, func(ctx context.Context, tx *convrepository.Tx) error {
		// CF-195: a contact the customer created through the contacts API
		// with this user id is the user's contact; bind the widget
		// identity to it before resolving, so no second contact appears.
		matchEmail, err := s.bindExternalContact(ctx, tx, org, inst, in.UserID, next.ExternalID(), in.Name, email)
		if err != nil {
			return err
		}
		matchPhone := phone
		if !matchEmail {
			// The address belongs to another user's contact: never land on it.
			email, matchPhone = "", ""
		}
		rc, err := tx.ResolveContact(ctx, convrepository.IdentityInput{
			OrganizationID: org, ChannelType: channels.TypeWidget, InstanceID: inst,
			ExternalID: next.ExternalID(), DisplayName: clip(strings.TrimSpace(in.Name), 255), Email: email, Phone: matchPhone,
			Meta: []byte(`{"verified":true}`), SeenAt: s.now(),
		})
		if err != nil {
			return err
		}
		// The user id is the contact's external id from now on, and the
		// contact is verified (the badge in the panel).
		if err := s.markVerified(ctx, tx, org, rc.Contact.ID, in.UserID); err != nil {
			return err
		}
		if in.Name = strings.TrimSpace(in.Name); in.Name != "" {
			if err := tx.Q.UpdateWidgetContactProfile(ctx, db.UpdateWidgetContactProfileParams{
				Name: clip(in.Name, 255), OrganizationID: org, ID: rc.Contact.ID,
			}); err != nil {
				return err
			}
		}
		if in.Company != nil {
			company := *in.Company
			if !companySigned {
				// An unsigned company id never binds: name and domain stay hints.
				company.ID = ""
			}
			if err := s.linkCompany(ctx, tx, org, rc.Contact.ID, company, companySigned); err != nil {
				return err
			}
		}
		// Move the anonymous visitor's contact into the user's, unless that
		// contact already belongs to another identified user (shared device).
		anon := widget.Claims{Instance: claims.Instance, Visitor: claims.Visitor}
		visitorContact, ok, err := s.contactWith(ctx, tx.Q, org, inst, anon.ExternalID())
		if err != nil || !ok || visitorContact.ID == rc.Contact.ID {
			return err
		}
		ids, err := tx.Q.ListContactIdentities(ctx, db.ListContactIdentitiesParams{OrganizationID: org, ContactID: visitorContact.ID})
		if err != nil {
			return err
		}
		for _, id := range ids {
			if strings.HasPrefix(id.ExternalID, "user:") {
				return nil
			}
		}
		_, err = tx.MergeContacts(ctx, org, visitorContact.ID, rc.Contact.ID)
		return err
	})
	if err != nil {
		return Visitor{}, err
	}
	// A verified user proves the integration works: drop the editor warning.
	if err := s.q.ClearWidgetIdentityRejection(ctx, db.ClearWidgetIdentityRejectionParams{
		OrganizationID: org, ChannelInstanceID: inst,
	}); err != nil {
		s.log.Warn("widget_identity_rejection_clear_failed", "instance", w.Instance.UUID, "error", err)
	}
	return s.visitor(ctx, w, next)
}

// markVerified stores the proved user id on the contact (CF-195). It runs
// in a savepoint: when a concurrent call gave the id to another contact
// first (unique violation), the contact simply stays unverified instead of
// failing the identify call.
func (s *Service) markVerified(ctx context.Context, tx *convrepository.Tx, org, contactID int64, userID string) error {
	sp, err := tx.Raw.Begin(ctx)
	if err != nil {
		return err
	}
	_, err = db.New(sp).MarkContactVerified(ctx, db.MarkContactVerifiedParams{
		VerifiedAt: pgtype.Timestamptz{Time: s.now(), Valid: true}, ExternalID: userID,
		OrganizationID: org, ID: contactID,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return sp.Rollback(ctx)
	}
	if err != nil {
		_ = sp.Rollback(ctx)
		return err
	}
	return sp.Commit(ctx)
}

// bindExternalContact attaches the widget identity of a verified user to
// the live contact that already has the user id as its external id, when
// the identity does not exist yet (CF-195). It reports whether resolving
// may still match a contact by the user's e-mail: not when that contact
// carries another user's external id.
func (s *Service) bindExternalContact(ctx context.Context, tx *convrepository.Tx, org, inst int64, userID, identityID, name, email string) (bool, error) {
	instance := pgtype.Int8{Int64: inst, Valid: true}
	_, err := tx.Q.FindContactIdentity(ctx, db.FindContactIdentityParams{
		OrganizationID: org, ChannelType: channels.TypeWidget, ChannelInstanceID: instance, ExternalID: identityID,
	})
	if err == nil || !errors.Is(err, pgx.ErrNoRows) {
		return true, err
	}
	c, err := tx.Q.GetContactByExternalID(ctx, db.GetContactByExternalIDParams{OrganizationID: org, ExternalID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		if email == "" {
			return true, nil
		}
		other, err := tx.Q.FindContactByEmail(ctx, db.FindContactByEmailParams{OrganizationID: org, Email: email})
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return !other.ExternalID.Valid || other.ExternalID.String == userID, nil
	}
	if err != nil {
		return false, err
	}
	_, err = tx.Q.UpsertContactIdentity(ctx, db.UpsertContactIdentityParams{
		OrganizationID: org, ContactID: c.ID, ChannelType: channels.TypeWidget, ChannelInstanceID: instance,
		ExternalID: identityID, DisplayName: clip(strings.TrimSpace(name), 255), Meta: []byte(`{"verified":true}`),
		LastSeenAt: pgtype.Timestamptz{Time: s.now(), Valid: true},
	})
	return true, err
}

// Error codes of a rejected identify call, as the visitor API returns them
// and as the editor reads them back (CF-176).
const (
	RejectionIdentityDisabled = "WIDGET_IDENTITY_DISABLED"
	RejectionIdentityInvalid  = "WIDGET_IDENTITY_INVALID"
)

// noteIdentityRejection remembers why the widget's last identify call was
// rejected so the editor can show it to the integrator. The caller is not a
// verified user, so nothing of the request is stored (no user_id, user_hash
// or secret): only the fixed code and the time, rewritten at most once a
// minute by the query.
func (s *Service) noteIdentityRejection(ctx context.Context, w Widget, code string) {
	if err := s.q.TouchWidgetIdentityRejection(ctx, db.TouchWidgetIdentityRejectionParams{
		OrganizationID: w.Instance.OrganizationID, ChannelInstanceID: w.Instance.ID, Code: code,
	}); err != nil {
		s.log.Warn("widget_identity_rejection_touch_failed", "instance", w.Instance.UUID, "error", err)
	}
}

// linkCompany attaches the contact to the company of an identify call: by
// external id first, then by domain, else a new company. With signed set
// (CF-206: user_hash covered c.ID) the id is proof: a company matched by
// domain takes the id when it has none, the company is marked verified and
// the contact moves to it even when it had another company. Unsigned, only
// a contact without a company is attached (and c.ID is already empty).
func (s *Service) linkCompany(ctx context.Context, tx *convrepository.Tx, org, contactID int64, c Company, signed bool) error {
	name := clip(strings.TrimSpace(c.Name), 255)
	extID := clip(strings.TrimSpace(c.ID), 200)
	domain := strings.ToLower(clip(strings.TrimSpace(c.Domain), 255))
	if name == "" && extID == "" && domain == "" {
		return nil
	}
	var company db.Company
	err := error(pgx.ErrNoRows)
	if extID != "" {
		company, err = tx.Q.GetCompanyByExternalID(ctx, db.GetCompanyByExternalIDParams{OrganizationID: org, ExternalID: extID})
	}
	if errors.Is(err, pgx.ErrNoRows) && domain != "" {
		company, err = tx.Q.GetCompanyByDomain(ctx, db.GetCompanyByDomainParams{OrganizationID: org, Domain: domain})
		switch {
		case err != nil || !signed || extID == "":
		case company.ExternalID.Valid && company.ExternalID.String != extID:
			// The domain is unsigned: it must not bind the user to a company
			// that proved (or was given) another id. The proved id gets its
			// own company, without the other company's domain.
			company, err, domain = db.Company{}, pgx.ErrNoRows, ""
		default:
			err = tx.Q.SetCompanyExternalIDIfEmpty(ctx, db.SetCompanyExternalIDIfEmptyParams{ExternalID: extID, OrganizationID: org, ID: company.ID})
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		if name == "" {
			name = firstNonEmpty(domain, extID)
		}
		company, err = tx.Q.InsertCompany(ctx, db.InsertCompanyParams{
			OrganizationID: org, Name: name, Domain: optText(domain), ExternalID: optText(extID), CustomFields: []byte("{}"),
		})
	}
	if err != nil {
		return err
	}
	companyID := pgtype.Int8{Int64: company.ID, Valid: true}
	if !signed || extID == "" {
		return tx.Q.SetContactCompanyIfEmpty(ctx, db.SetContactCompanyIfEmptyParams{CompanyID: companyID, OrganizationID: org, ID: contactID})
	}
	if err := tx.Q.MarkCompanyVerified(ctx, db.MarkCompanyVerifiedParams{
		VerifiedAt: pgtype.Timestamptz{Time: s.now(), Valid: true}, OrganizationID: org, ID: company.ID,
	}); err != nil {
		return err
	}
	return tx.Q.SetContactCompany(ctx, db.SetContactCompanyParams{
		CompanyID: companyID, VerifiedAt: pgtype.Timestamptz{Time: s.now(), Valid: true}, OrganizationID: org, ID: contactID,
	})
}

// ProfileInput updates what the visitor told us.
type ProfileInput struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

// UpdateProfile updates the visitor's own contact (never creating one,
// docs/widget/05). Anonymous visitors can only set an e-mail address or
// phone that no other contact uses, so nobody can attach themselves to a
// customer's history by typing their address.
func (s *Service) UpdateProfile(ctx context.Context, w Widget, claims widget.Claims, in ProfileInput) (Visitor, error) {
	contact, ok, err := s.contactOf(ctx, w, claims)
	if err != nil {
		return Visitor{}, err
	}
	if ok {
		if err := s.applyProfile(ctx, w.Instance.OrganizationID, contact, claims.Verified(), in); err != nil {
			return Visitor{}, err
		}
	}
	return s.visitor(ctx, w, claims)
}

func (s *Service) applyProfile(ctx context.Context, org int64, contact db.Contact, verified bool, in ProfileInput) error {
	email := cleanEmail(in.Email)
	if email != "" && !verified {
		other, err := s.q.FindContactByEmail(ctx, db.FindContactByEmailParams{OrganizationID: org, Email: email})
		switch {
		case err == nil && other.ID != contact.ID:
			email = ""
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return err
		}
	}
	return s.q.UpdateWidgetContactProfile(ctx, db.UpdateWidgetContactProfileParams{
		Name: clip(strings.TrimSpace(in.Name), 255), Email: optText(email),
		Phone: optText(clip(strings.TrimSpace(in.Phone), 40)), OrganizationID: org, ID: contact.ID,
	})
}

// contactOf finds the visitor's contact (following merges).
func (s *Service) contactOf(ctx context.Context, w Widget, claims widget.Claims) (db.Contact, bool, error) {
	return s.contactWith(ctx, s.q, w.Instance.OrganizationID, w.Instance.ID, claims.ExternalID())
}

func (s *Service) contactWith(ctx context.Context, q *db.Queries, org, inst int64, externalID string) (db.Contact, bool, error) {
	id, err := q.FindContactIdentity(ctx, db.FindContactIdentityParams{
		OrganizationID: org, ChannelType: channels.TypeWidget, ChannelInstanceID: pgtype.Int8{Int64: inst, Valid: true}, ExternalID: externalID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Contact{}, false, nil
	}
	if err != nil {
		return db.Contact{}, false, err
	}
	c, err := q.GetContactByID(ctx, db.GetContactByIDParams{OrganizationID: org, ID: id.ContactID})
	for hops := 0; err == nil && c.MergedIntoID.Valid && hops < 8; hops++ {
		c, err = q.GetContactByID(ctx, db.GetContactByIDParams{OrganizationID: org, ID: c.MergedIntoID.Int64})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Contact{}, false, nil
	}
	if err != nil {
		return db.Contact{}, false, err
	}
	return c, true, nil
}

// ---------------------------------------------------------------------------
// Conversations and messages

// ConversationSummary is one conversation in the visitor's list.
type ConversationSummary struct {
	UUID                 uuid.UUID  `json:"uuid"`
	Number               int64      `json:"number"`
	Status               string     `json:"status"`
	Open                 bool       `json:"open"`
	LastMessagePreview   string     `json:"last_message_preview"`
	LastMessageDirection string     `json:"last_message_direction"`
	LastMessageAt        time.Time  `json:"last_message_at"`
	UnreadCount          int64      `json:"unread_count"`
	CanRate              bool       `json:"can_rate"`
	Rating               *int       `json:"rating"`
	CreatedAt            time.Time  `json:"created_at"`
	ResolvedAt           *time.Time `json:"resolved_at"`
}

// ListConversations returns the visitor's conversations on this widget.
func (s *Service) ListConversations(ctx context.Context, w Widget, claims widget.Claims) ([]ConversationSummary, error) {
	contact, ok, err := s.contactOf(ctx, w, claims)
	if err != nil || !ok {
		return []ConversationSummary{}, err
	}
	rows, err := s.q.ListWidgetConversations(ctx, db.ListWidgetConversationsParams{
		OrganizationID: w.Instance.OrganizationID, ContactID: contact.ID,
		ChannelInstanceID: pgtype.Int8{Int64: w.Instance.ID, Valid: true}, RowLimit: 50,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ConversationSummary, 0, len(rows))
	for _, r := range rows {
		c := ConversationSummary{
			UUID: r.Uuid, Number: r.Number, Status: r.Status, Open: openStatus(r.Status),
			LastMessagePreview: r.LastMessagePreview, LastMessageDirection: r.LastMessageDirection,
			LastMessageAt: r.LastMessageAt.Time.UTC(), UnreadCount: r.UnreadCount, CreatedAt: r.CreatedAt.Time.UTC(),
			ResolvedAt: timePtr(r.ResolvedAt),
		}
		if r.RatingScore.Valid {
			v := int(r.RatingScore.Int16)
			c.Rating = &v
		}
		c.CanRate = w.Settings.CSAT && !c.Open && c.Rating == nil
		out = append(out, c)
	}
	return out, nil
}

func openStatus(st string) bool {
	return st != convmodel.StatusResolved && st != convmodel.StatusClosed
}

// Upload is a file a visitor attaches to a message.
type Upload struct {
	Filename    string
	ContentType string
	Data        []byte
}

// SendInput is one visitor message. ConversationUUID nil starts a new
// conversation.
type SendInput struct {
	ConversationUUID *uuid.UUID
	Text             string
	ClientID         uuid.UUID
	File             *Upload
	// Profile carries the pre-chat form (name, e-mail) of a first message.
	Profile ProfileInput
	// Form is the request form (CF-52) of a first message.
	Form    *formsmodel.Submission
	PageURL string
	// Events are the visitor's recent ClickFix('track', ...) events, kept on
	// the message for the agent (newest last, at most MaxTrackedEvents).
	Events []TrackedEvent
	// Consent is the GDPR/KVKK checkbox of the pre-chat form.
	Consent *bool
	// Intent: "" (chat), "human" (talk to a person), "callback" or "video".
	Intent string
	// Attributes are SDK attributes set before the contact existed.
	Attributes map[string]any
	// DepartmentUUID is the department the visitor picked for a new
	// conversation (CF-215); nil lets the AI route the first message.
	DepartmentUUID *uuid.UUID
}

// Intents of a visitor message.
var intents = []string{"", "human", "callback", "video"}

// MaxTrackedEvents bounds the events kept on one message.
const MaxTrackedEvents = 20

// TrackedEvent is one SDK track call.
type TrackedEvent struct {
	Name       string         `json:"name"`
	Properties map[string]any `json:"properties,omitempty"`
	At         time.Time      `json:"at"`
}

// Sent is the stored visitor message.
type Sent struct {
	ConversationUUID uuid.UUID `json:"conversation_uuid"`
	Message          Message   `json:"message"`
	Created          bool      `json:"created"`
}

// Send stores a visitor message through the inbound pipeline. Every
// widget conversation is a thread of its own, so a new conversation is a
// new thread id and a reply joins its conversation's thread.
func (s *Service) Send(ctx context.Context, w Widget, claims widget.Claims, in SendInput) (Sent, error) {
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" && in.File == nil {
		return Sent{}, fmt.Errorf("%w: message is empty", ErrInvalid)
	}
	if utf8.RuneCountInString(in.Text) > widget.MaxTextLength {
		return Sent{}, fmt.Errorf("%w: message is too long", ErrInvalid)
	}
	if w.Settings.RequireIdentity && !claims.Verified() {
		return Sent{}, ErrIdentityRequired
	}
	if in.File != nil {
		if !w.Settings.FileUpload {
			return Sent{}, ErrUploadDisabled
		}
		if int64(len(in.File.Data)) > min(s.maxFileBytes(ctx, w.Instance.OrganizationID), w.Settings.MaxUploadBytes()) {
			return Sent{}, ErrFileTooLarge
		}
		if !w.Settings.AttachmentAllowed(in.File.ContentType, in.File.Filename) {
			return Sent{}, ErrFileType
		}
	}
	if !slices.Contains(intents, in.Intent) {
		return Sent{}, fmt.Errorf("%w: unknown intent", ErrInvalid)
	}
	if in.ConversationUUID == nil {
		if err := checkPreChat(w.Settings, in); err != nil {
			return Sent{}, err
		}
	}
	org, inst := w.Instance.OrganizationID, w.Instance
	// A new conversation answers the widget's pre-chat form first (CF-52,
	// CF-198: a request form); the composer text is its message.
	var form *formsengine.Submission
	var formUUID uuid.UUID
	if in.ConversationUUID == nil && s.forms != nil && w.Settings.PreChat.Enabled {
		var err error
		if form, formUUID, err = s.forms.CheckOffered(ctx, org, FormChannel, preChatForm(w.Settings), in.Form, "message"); err != nil {
			return Sent{}, err
		}
		if form != nil {
			if in.Profile.Name == "" {
				in.Profile.Name = form.Name
			}
			if in.Profile.Email == "" {
				in.Profile.Email = form.Email
			}
			if in.Profile.Phone == "" {
				in.Profile.Phone = form.Phone
			}
		}
	}
	threadID := "widget:" + uuid.NewString()
	created := true
	if in.ConversationUUID != nil {
		conv, err := s.conversation(ctx, w, claims, *in.ConversationUUID)
		if err != nil {
			return Sent{}, err
		}
		if conv.Status == convmodel.StatusClosed || !conv.ExternalThreadID.Valid {
			return Sent{}, ErrConversationClosed
		}
		threadID, created = conv.ExternalThreadID.String, false
	}
	if in.ClientID == uuid.Nil {
		in.ClientID = uuid.New()
	}
	externalID := "visitor:" + in.ClientID.String()
	sender := channels.Sender{
		ExternalID: claims.ExternalID(), InstanceScoped: true, Name: clip(strings.TrimSpace(in.Profile.Name), 255),
		Meta: map[string]any{"visitor_id": claims.Visitor, "verified": claims.Verified()},
	}
	if email := cleanEmail(in.Profile.Email); email != "" {
		// Unverified addresses never match an existing contact; they are
		// only kept on the message and filled into the visitor's own
		// contact below when no one else uses them.
		sender.Meta["email"] = email
	}
	msg := channels.InboundMessage{
		ExternalID: externalID, From: sender, Kind: convmodel.KindText, Text: in.Text, ThreadID: threadID,
		Attributes: map[string]any{"source": "widget"}, At: s.now(),
	}
	if in.PageURL != "" {
		msg.Attributes["page_url"] = clip(in.PageURL, 500)
	}
	if in.Consent != nil && *in.Consent {
		msg.Attributes["consent"] = map[string]any{"given": true, "at": s.now()}
	}
	if in.Intent != "" {
		msg.Attributes["intent"] = in.Intent
	}
	if !w.Settings.AI.Enabled {
		msg.Attributes["ai_disabled"] = true
	}
	if in.Intent == "human" || handoffKeyword(w.Settings.AI.HandoffKeywords, in.Text) {
		msg.Attributes["handoff_requested"] = true
	}
	if n := len(in.Events); n > 0 {
		events := in.Events[max(0, n-MaxTrackedEvents):]
		for i := range events {
			events[i].Name = clip(strings.TrimSpace(events[i].Name), 100)
		}
		msg.Attributes["events"] = events
	}
	if in.File != nil {
		data := in.File.Data
		msg.Attachments = []channels.InboundAttachment{{
			Filename: clip(in.File.Filename, 200), ContentType: in.File.ContentType, Size: int64(len(data)),
			Open: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil },
		}}
		if in.Text == "" {
			msg.Kind = convmodel.KindMedia
		}
	}
	if _, err := s.pipeline.Receive(ctx, inst, channels.WebhookResult{Messages: []channels.InboundMessage{msg}}); err != nil {
		return Sent{}, err
	}
	stored, err := s.q.GetMessageByExternal(ctx, db.GetMessageByExternalParams{
		OrganizationID: org, ChannelInstanceID: pgtype.Int8{Int64: inst.ID, Valid: true},
		ExternalID: pgtype.Text{String: externalID, Valid: true},
	})
	if err != nil {
		return Sent{}, err
	}
	conv, err := s.q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: org, ID: stored.ConversationID})
	if err != nil {
		return Sent{}, err
	}
	if form != nil && created && conv.MessageCount <= 1 {
		if err := s.forms.Apply(ctx, org, conv.ID, formUUID, *form); err != nil {
			s.log.Warn("widget_request_form_apply_failed", "instance", inst.UUID, "error", err)
		}
	}
	if in.DepartmentUUID != nil && created && conv.MessageCount <= 1 && s.departments != nil {
		if err := s.departments.PickFromWidget(ctx, org, conv.ID, *in.DepartmentUUID); err != nil {
			s.log.Warn("widget_department_pick_failed", "instance", inst.UUID, "error", err)
		}
	}
	if in.Profile != (ProfileInput{}) {
		if contact, ok, err := s.contactOf(ctx, w, claims); err == nil && ok {
			if err := s.applyProfile(ctx, org, contact, claims.Verified(), in.Profile); err != nil {
				s.log.Warn("widget_profile_update_failed", "instance", inst.UUID, "error", err)
			}
		}
	}
	// SDK attributes follow each mapping's anonymous flag (the pre-chat
	// form's answers went onto the conversation and contact through the
	// forms module above).
	if len(in.Attributes) > 0 {
		if _, err := s.applyAttributes(ctx, w, claims, in.Attributes, false); err != nil {
			s.log.Warn("widget_attributes_failed", "instance", inst.UUID, "error", err)
		}
	}
	m, err := s.messages(ctx, org, []db.ListWidgetMessagesRow{{
		ID: stored.ID, Uuid: stored.Uuid, Direction: stored.Direction, SenderType: stored.SenderType, Kind: stored.Kind,
		Content: stored.Content, Status: stored.Status, ClientID: stored.ClientID, CreatedAt: stored.CreatedAt, ReadAt: stored.ReadAt,
	}})
	if err != nil {
		return Sent{}, err
	}
	m[0].ClientID = &in.ClientID
	return Sent{ConversationUUID: conv.Uuid, Message: m[0], Created: created && conv.MessageCount <= 1}, nil
}

// conversation loads one of the visitor's conversations on this widget.
func (s *Service) conversation(ctx context.Context, w Widget, claims widget.Claims, id uuid.UUID) (db.Conversation, error) {
	contact, ok, err := s.contactOf(ctx, w, claims)
	if err != nil {
		return db.Conversation{}, err
	}
	if !ok {
		return db.Conversation{}, ErrNotFound
	}
	conv, err := s.q.GetWidgetConversation(ctx, db.GetWidgetConversationParams{
		OrganizationID: w.Instance.OrganizationID, Uuid: id, ContactID: contact.ID,
		ChannelInstanceID: pgtype.Int8{Int64: w.Instance.ID, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Conversation{}, ErrNotFound
	}
	return conv, err
}

// Attachment is a file of a widget message with a short-lived download link.
type Attachment struct {
	UUID        uuid.UUID `json:"uuid"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	URL         string    `json:"url"`
}

// Message is one message as the visitor sees it.
type Message struct {
	UUID        uuid.UUID    `json:"uuid"`
	Direction   string       `json:"direction"`
	SenderType  string       `json:"sender_type"`
	SenderName  string       `json:"sender_name"`
	Kind        string       `json:"kind"`
	Content     string       `json:"content"`
	Status      string       `json:"status"`
	ClientID    *uuid.UUID   `json:"client_id"`
	Attachments []Attachment `json:"attachments"`
	CreatedAt   time.Time    `json:"created_at"`
	ReadAt      *time.Time   `json:"read_at"`
	// EditedAt is set when an agent corrected the reply (CF-248); Content
	// is the corrected text.
	EditedAt *time.Time `json:"edited_at"`
}

// MessagePage is a page of messages, oldest first.
type MessagePage struct {
	Conversation ConversationSummary `json:"conversation"`
	Items        []Message           `json:"items"`
	HasMore      bool                `json:"has_more"`
}

// ListMessages returns up to 50 messages of a conversation: the newest
// ones, those after a message (polling) or before one (history).
func (s *Service) ListMessages(ctx context.Context, w Widget, claims widget.Claims, id uuid.UUID, after, before uuid.UUID) (MessagePage, error) {
	conv, err := s.conversation(ctx, w, claims, id)
	if err != nil {
		return MessagePage{}, err
	}
	org := w.Instance.OrganizationID
	cursor := func(ref uuid.UUID) (int64, error) {
		if ref == uuid.Nil {
			return 0, nil
		}
		n, err := s.q.GetWidgetMessageID(ctx, db.GetWidgetMessageIDParams{OrganizationID: org, ConversationID: conv.ID, Uuid: ref})
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("%w: unknown message cursor", ErrInvalid)
		}
		return n, err
	}
	afterID, err := cursor(after)
	if err != nil {
		return MessagePage{}, err
	}
	beforeID, err := cursor(before)
	if err != nil {
		return MessagePage{}, err
	}
	const limit = 50
	rows, err := s.q.ListWidgetMessages(ctx, db.ListWidgetMessagesParams{
		OrganizationID: org, ConversationID: conv.ID, AfterID: afterID, BeforeID: beforeID, RowLimit: limit + 1,
	})
	if err != nil {
		return MessagePage{}, err
	}
	page := MessagePage{HasMore: len(rows) > limit}
	if page.HasMore {
		rows = rows[:limit]
	}
	// Oldest first for rendering.
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	if page.Items, err = s.messages(ctx, org, rows); err != nil {
		return MessagePage{}, err
	}
	list, err := s.ListConversations(ctx, w, claims)
	if err != nil {
		return MessagePage{}, err
	}
	for _, c := range list {
		if c.UUID == conv.Uuid {
			page.Conversation = c
		}
	}
	return page, nil
}

func (s *Service) messages(ctx context.Context, org int64, rows []db.ListWidgetMessagesRow) ([]Message, error) {
	out := make([]Message, 0, len(rows))
	owners := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		m := Message{
			UUID: r.Uuid, Direction: r.Direction, SenderType: r.SenderType, Kind: r.Kind, Content: r.Content,
			Status: r.Status, Attachments: []Attachment{}, CreatedAt: r.CreatedAt.Time.UTC(), ReadAt: timePtr(r.ReadAt),
			EditedAt: timePtr(r.EditedAt),
		}
		if r.ClientID.Valid {
			id := uuid.UUID(r.ClientID.Bytes)
			m.ClientID = &id
		}
		if r.Direction == convmodel.DirectionOut && r.SenderUserName.Valid {
			m.SenderName = r.SenderUserName.String
		}
		out = append(out, m)
		owners = append(owners, r.Uuid)
	}
	if len(owners) == 0 || s.files == nil {
		return out, nil
	}
	files, err := s.q.ListMessageAttachmentsByOwners(ctx, db.ListMessageAttachmentsByOwnersParams{OrganizationID: org, OwnerIds: owners})
	if err != nil {
		return nil, err
	}
	index := map[uuid.UUID]int{}
	for i, m := range out {
		index[m.UUID] = i
	}
	for _, f := range files {
		if !f.OwnerID.Valid {
			continue
		}
		i, ok := index[uuid.UUID(f.OwnerID.Bytes)]
		if !ok {
			continue
		}
		link, err := s.files.SignedURL(ctx, org, f.Uuid, attachmentsmodel.VariantOriginal, time.Hour)
		if err != nil {
			continue
		}
		out[i].Attachments = append(out[i].Attachments, Attachment{
			UUID: f.Uuid, Filename: f.Filename, ContentType: f.ContentType, SizeBytes: f.SizeBytes, URL: link.URL,
		})
	}
	return out, nil
}

// MarkRead marks the agent replies of a conversation read (read receipts
// for the agents) and returns how many changed.
func (s *Service) MarkRead(ctx context.Context, w Widget, claims widget.Claims, id uuid.UUID) (int, error) {
	conv, err := s.conversation(ctx, w, claims, id)
	if err != nil {
		return 0, err
	}
	ids, err := s.q.ListUnreadOutboundExternalIDs(ctx, db.ListUnreadOutboundExternalIDsParams{OrganizationID: w.Instance.OrganizationID, ConversationID: conv.ID})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, ext := range ids {
		changed, err := s.pipeline.ApplyStatus(ctx, w.Instance, channels.StatusUpdate{ExternalID: ext, Status: channels.StatusRead, At: s.now()})
		if err != nil {
			return n, err
		}
		if changed {
			n++
		}
	}
	return n, nil
}

// Typing tells the organization's agents that the visitor is typing.
func (s *Service) Typing(ctx context.Context, w Widget, claims widget.Claims, id uuid.UUID, on bool) error {
	conv, err := s.conversation(ctx, w, claims, id)
	if err != nil {
		return err
	}
	s.notifyAgents(ctx, w.Instance.OrganizationID, map[string]any{
		"type": "contact.typing", "conversation_uuid": conv.Uuid, "typing": on, "occurred_at": s.now(),
	})
	return nil
}

func (s *Service) notifyAgents(ctx context.Context, orgID int64, payload map[string]any) {
	if s.pub == nil {
		return
	}
	org, err := s.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return
	}
	if err := s.pub.Publish(ctx, realtime.InboxChannel(org.Uuid), payload); err != nil {
		s.log.Warn("widget_agent_notify_failed", "type", payload["type"], "error", err)
	}
	// Members without read_all listen on their own inbox channel (CF-44).
	convUUID, _ := payload["conversation_uuid"].(uuid.UUID)
	if convUUID == uuid.Nil {
		return
	}
	users, err := access.Audience(ctx, s.q, orgID, convUUID)
	if err != nil {
		s.log.Warn("widget_agent_audience_failed", "type", payload["type"], "error", err)
		return
	}
	for _, u := range users {
		if err := s.pub.Publish(ctx, realtime.InboxUserChannel(org.Uuid, u), payload); err != nil {
			s.log.Warn("widget_agent_notify_failed", "type", payload["type"], "error", err)
		}
	}
}

// Rating is a stored CSAT rating.
type Rating struct {
	Score     int       `json:"score"`
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
	// Existing: the conversation was rated before; the first rating stays.
	Existing bool `json:"existing"`
}

// Rate stores the visitor's CSAT rating of a finished (resolved or closed)
// conversation. A conversation is rated once: a repeated submit returns the
// stored rating (docs/widget/07).
func (s *Service) Rate(ctx context.Context, w Widget, claims widget.Claims, id uuid.UUID, score int, comment string) (Rating, error) {
	if !w.Settings.CSAT {
		return Rating{}, ErrRatingDisabled
	}
	if score < 1 || score > 5 {
		return Rating{}, fmt.Errorf("%w: score must be 1-5", ErrInvalid)
	}
	conv, err := s.conversation(ctx, w, claims, id)
	if err != nil {
		return Rating{}, err
	}
	org := w.Instance.OrganizationID
	if existing, err := s.q.GetConversationRating(ctx, db.GetConversationRatingParams{OrganizationID: org, ConversationID: conv.ID}); err == nil {
		return Rating{Score: int(existing.Score), Comment: existing.Comment, CreatedAt: existing.CreatedAt.Time.UTC(), Existing: true}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Rating{}, err
	}
	if openStatus(conv.Status) {
		return Rating{}, ErrRatingNotAllowed
	}
	var row db.ConversationRating
	err = s.store.InTx(ctx, func(ctx context.Context, tx *convrepository.Tx) error {
		row, err = tx.Q.InsertConversationRating(ctx, db.InsertConversationRatingParams{
			OrganizationID: org, ConversationID: conv.ID, ContactID: pgtype.Int8{Int64: conv.ContactID, Valid: true},
			ChannelInstanceID: pgtype.Int8{Int64: w.Instance.ID, Valid: true}, Score: safeconv.ToInt16(score), Comment: clip(strings.TrimSpace(comment), 2000),
		})
		if err != nil {
			return err
		}
		return outbox.EnqueueQueries(ctx, tx.Q, events.New("surveys.responded").WithOrganization(org).WithEntity("conversation", &conv.ID, &conv.Uuid).WithPayload(map[string]any{"kind": "csat", "score": score}).WithIdempotencyKey("widget-rating:"+conv.Uuid.String()))
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// A concurrent submit won.
		row, err = s.q.GetConversationRating(ctx, db.GetConversationRatingParams{OrganizationID: org, ConversationID: conv.ID})
		if err != nil {
			return Rating{}, err
		}
		return Rating{Score: int(row.Score), Comment: row.Comment, CreatedAt: row.CreatedAt.Time.UTC(), Existing: true}, nil
	}
	if err != nil {
		return Rating{}, err
	}
	s.notifyAgents(ctx, org, map[string]any{"type": "conversation.rated", "conversation_uuid": conv.Uuid, "score": score, "occurred_at": s.now()})
	return Rating{Score: int(row.Score), Comment: row.Comment, CreatedAt: row.CreatedAt.Time.UTC()}, nil
}

// RealtimeToken grants the visitor's browser its private channel.
type RealtimeToken struct {
	WSURL             string    `json:"ws_url"`
	ConnectionToken   string    `json:"connection_token"`
	SubscriptionToken string    `json:"subscription_token"`
	Channel           string    `json:"channel"`
	ExpiresAt         time.Time `json:"expires_at"`
}

// Realtime issues an anonymous Centrifugo connection token and a
// subscription token for the visitor's channel only (docs/widget/04).
func (s *Service) Realtime(w Widget, claims widget.Claims) (RealtimeToken, error) {
	if s.issuer == nil || !s.issuer.Enabled() {
		return RealtimeToken{}, ErrRealtimeDisabled
	}
	exp := s.now().Add(time.Hour)
	channel := widget.VisitorChannel(w.Instance.UUID, claims.ExternalID())
	conn, err := s.issuer.AnonymousConnectionToken(exp)
	if err != nil {
		return RealtimeToken{}, err
	}
	sub, err := s.issuer.AnonymousSubscriptionToken(channel, exp)
	if err != nil {
		return RealtimeToken{}, err
	}
	return RealtimeToken{WSURL: s.issuer.WSURL(), ConnectionToken: conn, SubscriptionToken: sub, Channel: channel, ExpiresAt: exp}, nil
}

// ---------------------------------------------------------------------------

func initials(name, surname string) string {
	out := []rune{}
	for _, part := range []string{name, surname} {
		if r := []rune(strings.TrimSpace(part)); len(r) > 0 {
			out = append(out, r[0])
		}
	}
	return strings.ToUpper(string(out))
}

func cleanEmail(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	at := strings.LastIndex(v, "@")
	if len(v) > 254 || at < 1 || at == len(v)-1 || !strings.Contains(v[at:], ".") || strings.ContainsAny(v, " <>,;") {
		return ""
	}
	return v
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func optText(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

// BrandHelp only reads the current widget brand's published public articles.
func (s *Service) BrandHelp(ctx context.Context, w Widget, q, locale string) ([]db.BrandPublicArticlesRow, string, error) {
	if w.Instance.BrandID == nil || len(q) > 200 {
		return nil, "", ErrInvalid
	}
	if locale != "en" {
		locale = "tr"
	}
	org, err := s.q.GetOrganizationByID(ctx, w.Instance.OrganizationID)
	if err != nil {
		return nil, "", err
	}
	rows, err := s.q.BrandPublicArticles(ctx, db.BrandPublicArticlesParams{OrganizationID: org.ID, BrandID: pgtype.UUID{Bytes: *w.Instance.BrandID, Valid: true}, Locale: locale, Q: strings.TrimSpace(q)})
	return rows, org.Slug, err
}
