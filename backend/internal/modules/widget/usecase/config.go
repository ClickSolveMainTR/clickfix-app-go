package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	channelsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	convrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Errors of the editor API.
var (
	// ErrVersionConflict: someone published after the editor loaded.
	ErrVersionConflict = errors.New("widget: a newer version was published")
)

// SettingsError carries the issues that block a publish.
type SettingsError struct{ Issues []widget.Issue }

func (e *SettingsError) Error() string {
	if len(e.Issues) == 0 {
		return "widget: invalid settings"
	}
	return fmt.Sprintf("widget: %s: %s", e.Issues[0].Path, e.Issues[0].Message)
}

// InstanceCreator creates channel instances (channels usecase), used to
// duplicate a widget with the plan's channel limit applied.
type InstanceCreator interface {
	Create(ctx context.Context, in channelsusecase.CreateInput) (channelsusecase.InstanceView, error)
}

// Configs is the editor side of the widget (CF-150): an autosaved draft per
// widget, explicit publishing into channel_instances.settings with a
// version history and rollback, duplication and install detection. The
// visitor API keeps reading the published settings only.
type Configs struct {
	q        *db.Queries
	store    *convrepository.Store
	creator  InstanceCreator
	now      func() time.Time
	maxDraft int
}

// NewConfigs builds the editor service.
func NewConfigs(q *db.Queries, store *convrepository.Store, creator InstanceCreator) *Configs {
	return &Configs{q: q, store: store, creator: creator, now: func() time.Time { return time.Now().UTC() }, maxDraft: 256 << 10}
}

// Draft is the editor's unpublished copy.
type Draft struct {
	Settings    map[string]any `json:"settings"`
	BaseVersion int            `json:"base_version"`
	Issues      []widget.Issue `json:"issues"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// Config is everything the editor opens with.
type Config struct {
	WidgetUUID    uuid.UUID      `json:"widget_uuid"`
	SchemaVersion int            `json:"schema_version"`
	Version       int            `json:"version"`
	Published     map[string]any `json:"published"`
	Draft         *Draft         `json:"draft"`
	Defaults      map[string]any `json:"defaults"`
}

// VersionSummary is one published version.
type VersionSummary struct {
	UUID          uuid.UUID `json:"uuid"`
	Version       int       `json:"version"`
	Note          string    `json:"note"`
	RollbackOf    *int      `json:"rollback_of"`
	CreatedByName string    `json:"created_by_name"`
	CreatedAt     time.Time `json:"created_at"`
}

// VersionDetail is a version with its settings.
type VersionDetail struct {
	VersionSummary
	Settings map[string]any `json:"settings"`
}

// Install is a page origin where the widget ran.
type Install struct {
	Origin      string    `json:"origin"`
	SDKVersion  string    `json:"sdk_version"`
	LastPageURL string    `json:"last_page_url"`
	Hits        int64     `json:"hits"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

// IdentityRejection is the widget's last rejected identify call (CF-176): a
// fixed error code and when it happened, nothing of the request itself.
type IdentityRejection struct {
	Code       string    `json:"code"`
	RejectedAt time.Time `json:"rejected_at"`
}

func (c *Configs) instance(ctx context.Context, org int64, id uuid.UUID) (db.ChannelInstance, error) {
	row, err := c.q.GetChannelInstanceByUUID(ctx, db.GetChannelInstanceByUUIDParams{OrganizationID: org, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (row.ChannelType != channels.TypeWidget || row.DeletedAt.Valid)) {
		return db.ChannelInstance{}, ErrNotFound
	}
	return row, err
}

func object(raw []byte) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

// Config loads the published settings, the draft and the defaults.
func (c *Configs) Config(ctx context.Context, org int64, id uuid.UUID) (Config, error) {
	row, err := c.instance(ctx, org, id)
	if err != nil {
		return Config{}, err
	}
	version, err := c.q.LatestWidgetVersion(ctx, db.LatestWidgetVersionParams{OrganizationID: org, ChannelInstanceID: row.ID})
	if err != nil {
		return Config{}, err
	}
	published, err := widget.Stored(object(row.Settings))
	if err != nil {
		return Config{}, err
	}
	defaults, err := widget.Stored(nil)
	if err != nil {
		return Config{}, err
	}
	out := Config{WidgetUUID: row.Uuid, SchemaVersion: widget.SchemaVersion, Version: int(version), Published: published, Defaults: defaults}
	d, err := c.q.GetWidgetDraft(ctx, db.GetWidgetDraftParams{OrganizationID: org, ChannelInstanceID: row.ID})
	switch {
	case err == nil:
		out.Draft = draftOf(d)
	case !errors.Is(err, pgx.ErrNoRows):
		return Config{}, err
	}
	return out, nil
}

func draftOf(d db.WidgetDraft) *Draft {
	s := object(d.Settings)
	issues := widget.Validate(s)
	if issues == nil {
		issues = []widget.Issue{}
	}
	return &Draft{Settings: s, BaseVersion: int(d.BaseVersion), Issues: issues, UpdatedAt: d.UpdatedAt.Time.UTC()}
}

// SaveDraft autosaves the editor state as is (invalid values included, so
// nothing the user typed is lost) and returns its issues.
func (c *Configs) SaveDraft(ctx context.Context, org, actor int64, id uuid.UUID, settings map[string]any, baseVersion int) (Draft, error) {
	row, err := c.instance(ctx, org, id)
	if err != nil {
		return Draft{}, err
	}
	if settings == nil {
		return Draft{}, fmt.Errorf("%w: settings are required", ErrInvalid)
	}
	raw, err := json.Marshal(settings)
	if err != nil || len(raw) > c.maxDraft {
		return Draft{}, fmt.Errorf("%w: settings are too large", ErrInvalid)
	}
	d, err := c.q.UpsertWidgetDraft(ctx, db.UpsertWidgetDraftParams{
		OrganizationID: org, ChannelInstanceID: row.ID, Settings: raw, BaseVersion: safeconv.ToInt32(max(baseVersion, 0)), UpdatedBy: optID(actor),
	})
	if err != nil {
		return Draft{}, err
	}
	return *draftOf(d), nil
}

// DiscardDraft drops the draft.
func (c *Configs) DiscardDraft(ctx context.Context, org int64, id uuid.UUID) error {
	row, err := c.instance(ctx, org, id)
	if err != nil {
		return err
	}
	return c.q.DeleteWidgetDraft(ctx, db.DeleteWidgetDraftParams{OrganizationID: org, ChannelInstanceID: row.ID})
}

// PublishInput publishes the given settings (or the draft when nil).
// BaseVersion must be the version the editor started from.
type PublishInput struct {
	Settings    map[string]any `json:"settings"`
	BaseVersion int            `json:"base_version"`
	Note        string         `json:"note"`
}

// Publish validates strictly and makes the settings live as a new version.
func (c *Configs) Publish(ctx context.Context, org, actor int64, id uuid.UUID, in PublishInput) (VersionSummary, error) {
	row, err := c.instance(ctx, org, id)
	if err != nil {
		return VersionSummary{}, err
	}
	settings := in.Settings
	if settings == nil {
		d, err := c.q.GetWidgetDraft(ctx, db.GetWidgetDraftParams{OrganizationID: org, ChannelInstanceID: row.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return VersionSummary{}, fmt.Errorf("%w: nothing to publish", ErrInvalid)
		}
		if err != nil {
			return VersionSummary{}, err
		}
		settings = object(d.Settings)
	}
	return c.publish(ctx, org, actor, row, settings, in.BaseVersion, in.Note, nil)
}

func (c *Configs) publish(ctx context.Context, org, actor int64, row db.ChannelInstance, settings map[string]any, base int, note string, rollbackOf *int) (VersionSummary, error) {
	if issues := widget.Validate(settings); len(issues) > 0 {
		return VersionSummary{}, &SettingsError{Issues: issues}
	}
	stored, err := widget.Stored(settings)
	if err != nil {
		return VersionSummary{}, err
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return VersionSummary{}, err
	}
	note = clip(strings.TrimSpace(note), 200)
	var v db.WidgetVersion
	err = c.store.InTx(ctx, func(ctx context.Context, tx *convrepository.Tx) error {
		latest, err := tx.Q.LatestWidgetVersion(ctx, db.LatestWidgetVersionParams{OrganizationID: org, ChannelInstanceID: row.ID})
		if err != nil {
			return err
		}
		if int(latest) != base {
			return ErrVersionConflict
		}
		if _, err := tx.Q.SetWidgetInstanceSettings(ctx, db.SetWidgetInstanceSettingsParams{Settings: raw, OrganizationID: org, ID: row.ID}); err != nil {
			return err
		}
		rb := pgtype.Int4{}
		if rollbackOf != nil {
			rb = pgtype.Int4{Int32: safeconv.ToInt32(*rollbackOf), Valid: true}
		}
		v, err = tx.Q.InsertWidgetVersion(ctx, db.InsertWidgetVersionParams{
			OrganizationID: org, ChannelInstanceID: row.ID, Version: latest + 1, Settings: raw, Note: note,
			RollbackOf: rb, CreatedBy: optID(actor),
		})
		if err != nil {
			return err
		}
		return tx.Q.DeleteWidgetDraft(ctx, db.DeleteWidgetDraftParams{OrganizationID: org, ChannelInstanceID: row.ID})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return VersionSummary{}, ErrVersionConflict
	}
	if err != nil {
		return VersionSummary{}, err
	}
	return VersionSummary{UUID: v.Uuid, Version: int(v.Version), Note: v.Note, RollbackOf: rollbackOf, CreatedAt: v.CreatedAt.Time.UTC()}, nil
}

// Versions lists the published versions, newest first.
func (c *Configs) Versions(ctx context.Context, org int64, id uuid.UUID) ([]VersionSummary, error) {
	row, err := c.instance(ctx, org, id)
	if err != nil {
		return nil, err
	}
	rows, err := c.q.ListWidgetVersions(ctx, db.ListWidgetVersionsParams{OrganizationID: org, ChannelInstanceID: row.ID, RowLimit: 100})
	if err != nil {
		return nil, err
	}
	out := make([]VersionSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, VersionSummary{
			UUID: r.Uuid, Version: int(r.Version), Note: r.Note, RollbackOf: int4Ptr(r.RollbackOf),
			CreatedByName: r.CreatedByName, CreatedAt: r.CreatedAt.Time.UTC(),
		})
	}
	return out, nil
}

// Version returns one version with its settings.
func (c *Configs) Version(ctx context.Context, org int64, id uuid.UUID, version int) (VersionDetail, error) {
	row, err := c.instance(ctx, org, id)
	if err != nil {
		return VersionDetail{}, err
	}
	v, err := c.q.GetWidgetVersion(ctx, db.GetWidgetVersionParams{OrganizationID: org, ChannelInstanceID: row.ID, Version: safeconv.ToInt32(version)})
	if errors.Is(err, pgx.ErrNoRows) {
		return VersionDetail{}, ErrNotFound
	}
	if err != nil {
		return VersionDetail{}, err
	}
	return VersionDetail{
		VersionSummary: VersionSummary{UUID: v.Uuid, Version: int(v.Version), Note: v.Note, RollbackOf: int4Ptr(v.RollbackOf), CreatedAt: v.CreatedAt.Time.UTC()},
		Settings:       object(v.Settings),
	}, nil
}

// Rollback publishes an older version again as the newest one.
func (c *Configs) Rollback(ctx context.Context, org, actor int64, id uuid.UUID, version, base int) (VersionSummary, error) {
	old, err := c.Version(ctx, org, id, version)
	if err != nil {
		return VersionSummary{}, err
	}
	row, err := c.instance(ctx, org, id)
	if err != nil {
		return VersionSummary{}, err
	}
	return c.publish(ctx, org, actor, row, old.Settings, base, fmt.Sprintf("Rollback to v%d", version), &version)
}

// Duplicate copies a widget (published settings and brand; never the
// identity secret) into a new, active widget.
func (c *Configs) Duplicate(ctx context.Context, org, actor int64, id uuid.UUID, name string) (channelsusecase.InstanceView, error) {
	row, err := c.instance(ctx, org, id)
	if err != nil {
		return channelsusecase.InstanceView{}, err
	}
	if c.creator == nil {
		return channelsusecase.InstanceView{}, ErrInvalid
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = clip(row.Name, 110) + " (2)"
	}
	settings, err := widget.Stored(object(row.Settings))
	if err != nil {
		return channelsusecase.InstanceView{}, err
	}
	var brand *uuid.UUID
	if row.BrandID.Valid {
		b := uuid.UUID(row.BrandID.Bytes)
		brand = &b
	}
	return c.creator.Create(ctx, channelsusecase.CreateInput{
		BrandID: brand, OrganizationID: org, ActorID: actor, Type: channels.TypeWidget, Name: name, Settings: settings,
	})
}

// Installs lists where the widget was seen running.
func (c *Configs) Installs(ctx context.Context, org int64, id uuid.UUID) ([]Install, error) {
	row, err := c.instance(ctx, org, id)
	if err != nil {
		return nil, err
	}
	rows, err := c.q.ListWidgetInstalls(ctx, db.ListWidgetInstallsParams{OrganizationID: org, ChannelInstanceID: row.ID})
	if err != nil {
		return nil, err
	}
	out := make([]Install, 0, len(rows))
	for _, r := range rows {
		out = append(out, Install{
			Origin: r.Origin, SDKVersion: r.SdkVersion, LastPageURL: r.LastPageUrl, Hits: r.Hits,
			FirstSeenAt: r.FirstSeenAt.Time.UTC(), LastSeenAt: r.LastSeenAt.Time.UTC(),
		})
	}
	return out, nil
}

// IdentityRejection returns the last rejected identify call of the widget,
// or nil when there is none (never rejected, or a user was verified since).
func (c *Configs) IdentityRejection(ctx context.Context, org int64, id uuid.UUID) (*IdentityRejection, error) {
	row, err := c.instance(ctx, org, id)
	if err != nil {
		return nil, err
	}
	r, err := c.q.GetWidgetIdentityRejection(ctx, db.GetWidgetIdentityRejectionParams{OrganizationID: org, ChannelInstanceID: row.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &IdentityRejection{Code: r.Code, RejectedAt: r.RejectedAt.Time.UTC()}, nil
}

func optID(id int64) pgtype.Int8 {
	return pgtype.Int8{Int64: id, Valid: id > 0}
}

func int4Ptr(v pgtype.Int4) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int32)
	return &n
}
