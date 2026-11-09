// Package usecase manages the channel instances (inboxes) of an
// organization (CF-8): connecting an account of a channel type with sealed
// credentials, listing and editing them, and opening an instance with its
// decrypted credentials for the inbox core and the webhook endpoint.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/email"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Errors returned by the service.
var (
	ErrNotFound       = errors.New("channels: instance not found")
	ErrInvalid        = errors.New("channels: invalid request")
	ErrUnknownType    = channels.ErrUnknownType
	ErrAlreadyLinked  = errors.New("channels: this account is already connected")
	ErrConnectFailed  = errors.New("channels: the provider rejected the credentials")
	ErrInstanceClosed = errors.New("channels: instance is not receiving")
)

// Instance statuses.
const (
	StatusActive       = "active"
	StatusPaused       = "paused"
	StatusError        = "error"
	StatusDisconnected = "disconnected"
)

// TypeInfo describes a channel type for the connect screen.
type TypeInfo struct {
	Type         string                `json:"type"`
	Capabilities channels.Capabilities `json:"capabilities"`
	Fields       []channels.Field      `json:"fields"`
}

// InstanceView is the API shape of an instance; credentials only as hints.
type InstanceView struct {
	BrandID         *uuid.UUID            `json:"brand_id"`
	UUID            uuid.UUID             `json:"uuid"`
	Type            string                `json:"type"`
	Name            string                `json:"name"`
	Status          string                `json:"status"`
	ExternalID      *string               `json:"external_id"`
	CredentialHints map[string]string     `json:"credential_hints"`
	Settings        map[string]any        `json:"settings"`
	Capabilities    channels.Capabilities `json:"capabilities"`
	WebhookPath     string                `json:"webhook_path"`
	// SharedWebhookPath is the type-wide URL of Router channels (one Meta
	// app or Resend webhook for every organization); empty otherwise.
	SharedWebhookPath string `json:"shared_webhook_path"`
	// WebhookURL is the absolute URL to paste into the provider console
	// (PUBLIC_API_URL + the shared or own path); empty when it is unset.
	WebhookURL     string     `json:"webhook_url"`
	Health         Health     `json:"health"`
	LastInboundAt  *time.Time `json:"last_inbound_at"`
	LastOutboundAt *time.Time `json:"last_outbound_at"`
	LastError      string     `json:"last_error"`
	LastErrorAt    *time.Time `json:"last_error_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// CreateInput connects a new instance.
type CreateInput struct {
	BrandID        *uuid.UUID
	OrganizationID int64
	ActorID        int64
	Type           string
	Name           string
	Credentials    channels.Credentials
	Settings       map[string]any
}

// UpdateInput edits an instance. Nil fields stay; Credentials merges (an
// empty value keeps the stored secret, so the form never re-asks).
type UpdateInput struct {
	BrandID     *uuid.UUID
	BrandSet    bool
	Name        *string
	Status      *string
	Credentials channels.Credentials
	Settings    map[string]any
}

// Service manages instances.
type Service struct {
	netgsmStore  NetgsmStore
	imapLocker   IMAPLocker
	q            *db.Queries
	registry     *channels.Registry
	vault        *channels.Vault
	entitlements entitlement.Engine
	guard        CredentialGuard
	log          *slog.Logger
	now          func() time.Time
	publicAPIURL string
}

// New builds the service.
func New(q *db.Queries, registry *channels.Registry, vault *channels.Vault) *Service {
	return &Service{q: q, registry: registry, vault: vault, log: slog.Default(), now: time.Now}
}

// Registry returns the adapter registry.
func (s *Service) Registry() *channels.Registry { return s.registry }

// Types lists the registered channel types.
func (s *Service) Types() []TypeInfo {
	out := []TypeInfo{}
	for _, t := range s.registry.Types() {
		a, _ := s.registry.Get(t)
		out = append(out, TypeInfo{Type: t, Capabilities: a.Capabilities(), Fields: a.Fields()})
	}
	return out
}

// List returns the organization's instances.
func (s *Service) List(ctx context.Context, orgID int64, channelType string) ([]InstanceView, error) {
	rows, err := s.q.ListChannelInstances(ctx, db.ListChannelInstancesParams{OrganizationID: orgID, ChannelType: text(channelType)})
	if err != nil {
		return nil, err
	}
	stats, err := s.stats(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]InstanceView, 0, len(rows))
	for _, r := range rows {
		v, err := s.view(r, stats)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// Get returns one instance.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (InstanceView, error) {
	row, err := s.q.GetChannelInstanceByUUID(ctx, db.GetChannelInstanceByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return InstanceView{}, notFound(err)
	}
	stats, err := s.stats(ctx, orgID)
	if err != nil {
		return InstanceView{}, err
	}
	return s.view(row, stats)
}

// Create validates the credentials with the provider and stores the instance.
func (s *Service) Create(ctx context.Context, in CreateInput) (InstanceView, error) {
	adapter, ok := s.registry.Get(in.Type)
	if !ok {
		return InstanceView{}, ErrUnknownType
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > 120 {
		return InstanceView{}, fmt.Errorf("%w: name must be 1-120 characters", ErrInvalid)
	}
	creds, missing := channels.ValidateCredentials(adapter, in.Credentials)
	if len(missing) > 0 {
		return InstanceView{}, fmt.Errorf("%w: missing %s", ErrInvalid, strings.Join(missing, ", "))
	}
	if in.BrandID != nil {
		if _, err := s.q.GetBrand(ctx, db.GetBrandParams{OrganizationID: in.OrganizationID, Uuid: *in.BrandID}); err != nil {
			return InstanceView{}, fmt.Errorf("%w: brand_id", ErrInvalid)
		}
	}
	settings, err := json.Marshal(nonNil(in.Settings))
	if err != nil {
		return InstanceView{}, fmt.Errorf("%w: settings", ErrInvalid)
	}
	if err := s.checkLimit(ctx, in.OrganizationID); err != nil {
		return InstanceView{}, err
	}
	if err := s.checkCredentials(ctx, in.OrganizationID, in.Type, creds); err != nil {
		return InstanceView{}, err
	}
	id := uuid.New()
	inst := channels.Instance{UUID: id, OrganizationID: in.OrganizationID, Type: in.Type, Name: name, Status: StatusActive, Credentials: creds, Settings: nonNil(in.Settings)}
	externalID, err := adapter.Connect(ctx, inst)
	if err != nil {
		return InstanceView{}, fmt.Errorf("%w: %v", ErrConnectFailed, err)
	}
	sealed, err := s.vault.Seal(in.OrganizationID, id, creds)
	if err != nil {
		return InstanceView{}, err
	}
	row, err := s.q.InsertChannelInstance(ctx, db.InsertChannelInstanceParams{
		BrandID: brandUUID(in.BrandID), Uuid: id, OrganizationID: in.OrganizationID, ChannelType: in.Type, Name: name, Status: StatusActive,
		ExternalID: text(externalID), CredentialsEnc: sealed, Settings: settings, CreatedBy: optInt8(in.ActorID),
	})
	if err != nil {
		return InstanceView{}, uniqueErr(err)
	}
	return s.view(row, nil)
}

// Update edits an instance; changed credentials are validated with the
// provider again.
func (s *Service) Update(ctx context.Context, orgID int64, id uuid.UUID, in UpdateInput) (InstanceView, error) {
	row, err := s.q.GetChannelInstanceByUUID(ctx, db.GetChannelInstanceByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return InstanceView{}, notFound(err)
	}
	adapter, ok := s.registry.Get(row.ChannelType)
	if !ok {
		return InstanceView{}, ErrUnknownType
	}
	name, status := row.Name, row.Status
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		if name == "" || len([]rune(name)) > 120 {
			return InstanceView{}, fmt.Errorf("%w: name must be 1-120 characters", ErrInvalid)
		}
	}
	if in.Status != nil {
		if *in.Status != StatusActive && *in.Status != StatusPaused {
			return InstanceView{}, fmt.Errorf("%w: status must be active or paused", ErrInvalid)
		}
		status = *in.Status
	}
	creds, err := s.vault.Open(orgID, row.Uuid, row.CredentialsEnc)
	if err != nil {
		return InstanceView{}, err
	}
	settings := map[string]any{}
	_ = json.Unmarshal(row.Settings, &settings)
	if in.Settings != nil {
		settings = in.Settings
	}
	externalID := row.ExternalID
	if len(in.Credentials) > 0 {
		for k, v := range in.Credentials {
			if strings.TrimSpace(v) != "" {
				creds[k] = v
			}
		}
		var missing []string
		creds, missing = channels.ValidateCredentials(adapter, creds)
		if len(missing) > 0 {
			return InstanceView{}, fmt.Errorf("%w: missing %s", ErrInvalid, strings.Join(missing, ", "))
		}
		if err := s.checkCredentials(ctx, orgID, row.ChannelType, creds); err != nil {
			return InstanceView{}, err
		}
		ext, err := adapter.Connect(ctx, channels.Instance{ID: row.ID, UUID: row.Uuid, OrganizationID: orgID, Type: row.ChannelType, Name: name, Status: status, Credentials: creds, Settings: settings})
		if err != nil {
			return InstanceView{}, fmt.Errorf("%w: %v", ErrConnectFailed, err)
		}
		externalID = text(ext)
	}
	if row.ChannelType == channels.TypeEmail && email.IsIMAP(creds) {
		if _, err := email.IMAPConfiguration(channels.Instance{Credentials: creds, Settings: settings}); err != nil {
			return InstanceView{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
	}
	sealed, err := s.vault.Seal(orgID, row.Uuid, creds)
	if err != nil {
		return InstanceView{}, err
	}
	rawSettings, err := json.Marshal(settings)
	if err != nil {
		return InstanceView{}, fmt.Errorf("%w: settings", ErrInvalid)
	}
	brandID := row.BrandID
	if in.BrandSet {
		brandID = brandUUID(in.BrandID)
		if in.BrandID != nil {
			if _, err := s.q.GetBrand(ctx, db.GetBrandParams{OrganizationID: orgID, Uuid: *in.BrandID}); err != nil {
				return InstanceView{}, fmt.Errorf("%w: brand_id", ErrInvalid)
			}
		}
	}
	updated, err := s.q.UpdateChannelInstance(ctx, db.UpdateChannelInstanceParams{
		BrandID: brandID,
		Name:    name, Status: status, ExternalID: externalID, CredentialsEnc: sealed, Settings: rawSettings,
		OrganizationID: orgID, ID: row.ID,
	})
	if err != nil {
		return InstanceView{}, uniqueErr(err)
	}
	if updated.ChannelType == channels.TypeEmail && email.IsIMAP(creds) && (in.Credentials != nil || in.Settings != nil) {
		if err := s.resetIMAPBackoff(ctx, orgID, row.ID); err != nil {
			return InstanceView{}, err
		}
	}
	return s.view(updated, nil)
}

// Delete disconnects an instance (its conversations stay, without a channel).
func (s *Service) Delete(ctx context.Context, orgID int64, id uuid.UUID) (InstanceView, error) {
	row, err := s.q.GetChannelInstanceByUUID(ctx, db.GetChannelInstanceByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return InstanceView{}, notFound(err)
	}
	if inst, adapter, err := s.open(row); err == nil {
		if d, ok := adapter.(Disconnecter); ok {
			if err := d.Disconnect(ctx, inst); err != nil {
				s.log.Warn("channel_disconnect_failed", "instance", row.Uuid, "type", row.ChannelType, "error", err)
			}
		}
	}
	deleted, err := s.q.SoftDeleteChannelInstance(ctx, db.SoftDeleteChannelInstanceParams{OrganizationID: orgID, ID: row.ID})
	if err != nil {
		return InstanceView{}, notFound(err)
	}
	return s.view(deleted, nil)
}

// Instance opens an instance by id with its decrypted credentials
// (conversations usecase.InstanceSource).
func (s *Service) Instance(ctx context.Context, orgID, id int64) (channels.Instance, channels.Adapter, error) {
	row, err := s.q.GetChannelInstanceByID(ctx, db.GetChannelInstanceByIDParams{OrganizationID: orgID, ID: id})
	if err != nil {
		return channels.Instance{}, nil, notFound(err)
	}
	if row.DeletedAt.Valid {
		return channels.Instance{}, nil, ErrNotFound
	}
	return s.open(row)
}

// ForWebhook opens the live instance a webhook URL addresses.
func (s *Service) ForWebhook(ctx context.Context, channelType string, id uuid.UUID) (channels.Instance, channels.Adapter, error) {
	row, err := s.q.GetChannelInstanceByPublicUUID(ctx, id)
	if err != nil {
		return channels.Instance{}, nil, notFound(err)
	}
	if row.ChannelType != channelType {
		return channels.Instance{}, nil, ErrNotFound
	}
	return s.open(row)
}

// ForExternal opens the live instance of a provider account id (shared
// webhook URLs of Router channels).
func (s *Service) ForExternal(ctx context.Context, channelType, externalID string) (channels.Instance, channels.Adapter, error) {
	row, err := s.q.GetChannelInstanceByExternal(ctx, db.GetChannelInstanceByExternalParams{ChannelType: channelType, ExternalID: text(externalID)})
	if err != nil {
		return channels.Instance{}, nil, notFound(err)
	}
	return s.open(row)
}

func (s *Service) open(row db.ChannelInstance) (channels.Instance, channels.Adapter, error) {
	adapter, ok := s.registry.Get(row.ChannelType)
	if !ok {
		return channels.Instance{}, nil, ErrUnknownType
	}
	creds, err := s.vault.Open(row.OrganizationID, row.Uuid, row.CredentialsEnc)
	if err != nil {
		return channels.Instance{}, nil, err
	}
	settings := map[string]any{}
	_ = json.Unmarshal(row.Settings, &settings)
	return channels.Instance{
		BrandID: brandPtr(row.BrandID), ID: row.ID, UUID: row.Uuid, OrganizationID: row.OrganizationID, Type: row.ChannelType, Name: row.Name,
		Status: row.Status, ExternalID: row.ExternalID.String, Credentials: creds, Settings: settings,
	}, adapter, nil
}

// WebhookPath is the API path a provider posts the instance's webhooks to.
func WebhookPath(channelType string, id uuid.UUID) string {
	return "/v1/webhooks/channels/" + channelType + "/" + id.String()
}

func (s *Service) view(row db.ChannelInstance, stats map[int64]outboundStats) (InstanceView, error) {
	v := InstanceView{
		BrandID: brandPtr(row.BrandID), UUID: row.Uuid, Type: row.ChannelType, Name: row.Name, Status: row.Status,
		CredentialHints: map[string]string{}, Settings: map[string]any{},
		WebhookPath: WebhookPath(row.ChannelType, row.Uuid), LastError: row.LastError,
		LastInboundAt: timePtr(row.LastInboundAt), LastOutboundAt: timePtr(row.LastOutboundAt),
		LastErrorAt: timePtr(row.LastErrorAt), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	v.Health = s.health(row, stats[row.ID])
	if row.ExternalID.Valid {
		ext := row.ExternalID.String
		v.ExternalID = &ext
	}
	_ = json.Unmarshal(row.Settings, &v.Settings)
	if a, ok := s.registry.Get(row.ChannelType); ok {
		v.Capabilities = a.Capabilities()
		if _, shared := a.(channels.Router); shared {
			v.SharedWebhookPath = "/v1/webhooks/channels/" + row.ChannelType
		}
		if creds, err := s.vault.Open(row.OrganizationID, row.Uuid, row.CredentialsEnc); err == nil {
			v.CredentialHints = channels.Hint(a, creds)
			if row.ChannelType == channels.TypeWidget && stats != nil {
				widgetIdentityHealth(&v.Health, creds[widget.IdentitySecretField] != "", stats[row.ID].identityRejection)
			}
		}
	}
	if s.publicAPIURL != "" {
		v.WebhookURL = s.publicAPIURL + v.WebhookPath
		if v.SharedWebhookPath != "" {
			v.WebhookURL = s.publicAPIURL + v.SharedWebhookPath
		}
	}
	return v, nil
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func uniqueErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrAlreadyLinked
	}
	return err
}

func text(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func optInt8(v int64) pgtype.Int8 {
	if v <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func nonNil(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// Open returns an organization's instance by uuid with its decrypted
// credentials (provider specific endpoints: WhatsApp templates).
func (s *Service) Open(ctx context.Context, orgID int64, id uuid.UUID) (channels.Instance, channels.Adapter, error) {
	row, err := s.q.GetChannelInstanceByUUID(ctx, db.GetChannelInstanceByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return channels.Instance{}, nil, notFound(err)
	}
	return s.open(row)
}

func brandUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}
func brandPtr(id pgtype.UUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	v := uuid.UUID(id.Bytes)
	return &v
}
