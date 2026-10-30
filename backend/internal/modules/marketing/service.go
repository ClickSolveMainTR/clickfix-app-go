package marketing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
)

// ErrNotFound is returned when there is no share image or attribution.
var ErrNotFound = errors.New("marketing: not found")

// OGImagePath is the API path of the default share image.
const OGImagePath = "/v1/public/marketing/og-image"

// Settings is the admin view of the stored settings.
type Settings struct {
	Config
	OGImageURL *string   `json:"og_image_url"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Public is what the marketing site reads (`GET /v1/public/marketing`): the
// same fields; ids and snippets are public on any page that loads them.
type Public = Settings

// Service reads and writes the settings and attribution rows.
type Service struct {
	q *db.Queries
}

// NewService builds the service.
func NewService(q *db.Queries) *Service { return &Service{q: q} }

func toSettings(row db.MarketingSetting) Settings {
	var cfg Config
	_ = json.Unmarshal(row.Config, &cfg)
	s := Settings{Config: cfg, UpdatedAt: row.UpdatedAt.Time}
	if row.OgImageKey.Valid && row.OgImageKey.String != "" {
		// The version query changes with every upload (cache busting).
		u := fmt.Sprintf("%s?v=%d", OGImagePath, row.UpdatedAt.Time.Unix())
		s.OGImageURL = &u
	}
	return s
}

// Get returns the current settings.
func (s *Service) Get(ctx context.Context) (Settings, error) {
	row, err := s.q.MarketingSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	return toSettings(row), nil
}

// Save stores a normalized, validated config.
func (s *Service) Save(ctx context.Context, cfg Config, actor int64) (Settings, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return Settings{}, err
	}
	row, err := s.q.MarketingSaveConfig(ctx, db.MarketingSaveConfigParams{
		Config: raw, UpdatedBy: pgtype.Int8{Int64: actor, Valid: actor > 0},
	})
	if err != nil {
		return Settings{}, err
	}
	return toSettings(row), nil
}

// SetOGImage stores (key != "") or clears the share image key.
func (s *Service) SetOGImage(ctx context.Context, key string, actor int64) (Settings, error) {
	row, err := s.q.MarketingSetOGImage(ctx, db.MarketingSetOGImageParams{
		OgImageKey: pgtype.Text{String: key, Valid: key != ""},
		UpdatedBy:  pgtype.Int8{Int64: actor, Valid: actor > 0},
	})
	if err != nil {
		return Settings{}, err
	}
	return toSettings(row), nil
}

// OGImageKey returns the stored share image key.
func (s *Service) OGImageKey(ctx context.Context) (string, error) {
	row, err := s.q.MarketingSettings(ctx)
	if err != nil {
		return "", err
	}
	if !row.OgImageKey.Valid || row.OgImageKey.String == "" {
		return "", ErrNotFound
	}
	return row.OgImageKey.String, nil
}

// OrganizationAttribution is the platform admin view of one organization.
type OrganizationAttribution struct {
	Attribution
	Source    string    `json:"source"`
	Medium    string    `json:"medium"`
	Campaign  string    `json:"campaign"`
	CreatedAt time.Time `json:"created_at"`
}

// ForOrganization returns the sign-up attribution of an organization.
func (s *Service) ForOrganization(ctx context.Context, org uuid.UUID) (OrganizationAttribution, error) {
	row, err := s.q.MarketingOrganizationAttribution(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrganizationAttribution{}, ErrNotFound
	}
	if err != nil {
		return OrganizationAttribution{}, err
	}
	out := OrganizationAttribution{
		Source: row.Source, Medium: row.Medium, Campaign: row.Campaign, CreatedAt: row.CreatedAt.Time,
	}
	_ = json.Unmarshal(row.FirstTouch, &out.FirstTouch)
	_ = json.Unmarshal(row.LastTouch, &out.LastTouch)
	return out, nil
}

// SourceRow is one line of the "Kaynaklar" report; empty source means the
// organization has no recorded attribution (direct / unknown).
type SourceRow struct {
	Source        string `json:"source"`
	Medium        string `json:"medium"`
	Campaign      string `json:"campaign"`
	Organizations int64  `json:"organizations"`
}

// SourcesReport is organizations created in [from, to) grouped by source.
type SourcesReport struct {
	From  time.Time   `json:"from"`
	To    time.Time   `json:"to"`
	Total int64       `json:"total"`
	Rows  []SourceRow `json:"rows"`
}

// Sources builds the report.
func (s *Service) Sources(ctx context.Context, from, to time.Time) (SourcesReport, error) {
	rows, err := s.q.MarketingSourcesReport(ctx, db.MarketingSourcesReportParams{
		FromAt: pgtype.Timestamptz{Time: from, Valid: true},
		ToAt:   pgtype.Timestamptz{Time: to, Valid: true},
	})
	if err != nil {
		return SourcesReport{}, err
	}
	out := SourcesReport{From: from, To: to, Rows: make([]SourceRow, 0, len(rows))}
	for _, r := range rows {
		out.Total += r.Organizations
		out.Rows = append(out.Rows, SourceRow{
			Source: strings.TrimSpace(r.Source), Medium: r.Medium, Campaign: r.Campaign, Organizations: r.Organizations,
		})
	}
	return out, nil
}
