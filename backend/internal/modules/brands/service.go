// Package brands manages organization brands and their public help identities.
package brands

import (
	"context"
	"encoding/json"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	attachmentmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const FeatureBrands = "brands"

type Input struct {
	Name             string               `json:"name"`
	LogoAttachmentID *uuid.UUID           `json:"logo_attachment_id"`
	PrimaryColor     string               `json:"primary_color"`
	SecondaryColor   string               `json:"secondary_color"`
	SupportEmail     string               `json:"support_email"`
	SenderName       string               `json:"sender_name"`
	ReplyTo          string               `json:"reply_to"`
	HelpSubdomain    string               `json:"help_subdomain"`
	HelpPath         string               `json:"help_path"`
	WidgetDefaults   map[string]any       `json:"widget_defaults"`
	Signature        string               `json:"signature"`
	BusinessHours    widget.BusinessHours `json:"business_hours"`
	SLAPolicyID      *uuid.UUID           `json:"sla_policy_id"`
	AIPersona        string               `json:"ai_persona"`
}
type Brand struct {
	UUID uuid.UUID `json:"uuid"`
	Input
	LogoURL   *string   `json:"logo_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Files interface {
	Get(context.Context, int64, uuid.UUID) (attachmentmodel.Attachment, error)
	SignedURL(context.Context, int64, uuid.UUID, string, time.Duration) (attachmentmodel.SignedURL, error)
}
type Service struct {
	Repo         *repository.Repository
	Entitlements entitlement.Engine
	Files        Files
}

var colorRE = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var subdomainRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var pathRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`)

func (in *Input) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.HelpSubdomain = strings.ToLower(strings.TrimSpace(in.HelpSubdomain))
	if in.PrimaryColor == "" {
		in.PrimaryColor = "#2563eb"
	}
	if in.SecondaryColor == "" {
		in.SecondaryColor = "#ffffff"
	}
	if in.Name == "" || len([]rune(in.Name)) > 120 || !colorRE.MatchString(in.PrimaryColor) || !colorRE.MatchString(in.SecondaryColor) || len(in.Signature) > 10000 || len(in.AIPersona) > 10000 || len(in.SenderName) > 120 || strings.ContainsAny(in.SenderName, "\r\n") {
		return supportops.ErrInvalid
	}
	for _, v := range []string{in.SupportEmail, in.ReplyTo} {
		if v == "" {
			continue
		}
		a, err := mail.ParseAddress(v)
		if err != nil || a.Address != v || a.Name != "" || len(v) > 254 || strings.ContainsAny(v, "\r\n") {
			return supportops.ErrInvalid
		}
	}
	if !validHelpHost(in.HelpSubdomain) || in.HelpPath != "" && !pathRE.MatchString(in.HelpPath) {
		return supportops.ErrInvalid
	}
	if in.WidgetDefaults == nil {
		in.WidgetDefaults = map[string]any{}
	}
	if in.BusinessHours.Timezone == "" {
		in.BusinessHours.Timezone = widget.DefaultTimezone
	}
	if in.BusinessHours.Days == nil {
		in.BusinessHours.Days = map[string][]widget.Period{}
	}
	if in.BusinessHours.Enabled {
		if _, err := time.LoadLocation(in.BusinessHours.Timezone); err != nil {
			return supportops.ErrInvalid
		}
		clock := regexp.MustCompile(`^(?:[01][0-9]|2[0-3]):[0-5][0-9]$`)
		for day, periods := range in.BusinessHours.Days {
			if !strings.Contains("|mon|tue|wed|thu|fri|sat|sun|", "|"+day+"|") {
				return supportops.ErrInvalid
			}
			for _, p := range periods {
				if !clock.MatchString(p.Start) || !clock.MatchString(p.End) || p.Start >= p.End {
					return supportops.ErrInvalid
				}
			}
		}
	}
	return nil
}
func Optional(v *uuid.UUID) pgtype.UUID {
	if v == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *v, Valid: true}
}
func UUIDPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := uuid.UUID(v.Bytes)
	return &id
}
func (s *Service) model(ctx context.Context, b db.Brand) (Brand, error) {
	out := Brand{UUID: b.Uuid, CreatedAt: b.CreatedAt.Time, UpdatedAt: b.UpdatedAt.Time, Input: Input{Name: b.Name, LogoAttachmentID: UUIDPtr(b.LogoAttachmentID), PrimaryColor: b.PrimaryColor, SecondaryColor: b.SecondaryColor, SupportEmail: b.SupportEmail, SenderName: b.SenderName, ReplyTo: b.ReplyTo, HelpSubdomain: b.HelpSubdomain, HelpPath: b.HelpPath, Signature: b.Signature, SLAPolicyID: UUIDPtr(b.SlaPolicyID), AIPersona: b.AiPersona}}
	if err := json.Unmarshal(b.WidgetDefaults, &out.WidgetDefaults); err != nil {
		return out, err
	}
	if err := json.Unmarshal(b.BusinessHours, &out.BusinessHours); err != nil {
		return out, err
	}
	if out.LogoAttachmentID != nil && s.Files != nil {
		link, err := s.Files.SignedURL(ctx, b.OrganizationID, *out.LogoAttachmentID, "original", time.Hour)
		if err != nil {
			return out, err
		}
		out.LogoURL = &link.URL
	}
	return out, nil
}
func (s *Service) List(ctx context.Context, org int64) ([]Brand, error) {
	rows, err := s.Repo.Q.ListBrands(ctx, org)
	if err != nil {
		return nil, err
	}
	out := make([]Brand, 0, len(rows))
	for _, b := range rows {
		v, err := s.model(ctx, b)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) Get(ctx context.Context, org int64, id uuid.UUID) (Brand, error) {
	b, err := s.Repo.Q.GetBrand(ctx, db.GetBrandParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return Brand{}, err
	}
	return s.model(ctx, b)
}
func (s *Service) Save(ctx context.Context, org int64, id uuid.UUID, in Input) (Brand, error) {
	if err := in.validate(); err != nil {
		return Brand{}, err
	}
	if in.LogoAttachmentID != nil {
		if s.Files == nil {
			return Brand{}, supportops.ErrInvalid
		}
		f, err := s.Files.Get(ctx, org, *in.LogoAttachmentID)
		if err != nil {
			return Brand{}, supportops.ErrInvalid
		}
		if f.OwnerType != nil {
			return Brand{}, supportops.ErrInvalid
		}
		if f.ContentType != "image/png" && f.ContentType != "image/jpeg" && f.ContentType != "image/webp" {
			return Brand{}, supportops.ErrInvalid
		}
	}
	wd, marshalErr := json.Marshal(in.WidgetDefaults)
	if marshalErr != nil {
		return Brand{}, supportops.ErrInvalid
	}
	hours, _ := json.Marshal(in.BusinessHours)
	var b db.Brand
	err := s.Repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		if s.Entitlements == nil {
			return &entitlement.Error{Code: entitlement.CodeFeatureRequired, Feature: FeatureBrands}
		}
		if err := s.Entitlements.RequireWritable(ctx, org); err != nil {
			return err
		}
		var err error
		if id == uuid.Nil {
			used, e := r.Q.CountBrands(ctx, org)
			if e != nil {
				return e
			}
			if _, e = s.Entitlements.CheckLimit(ctx, org, FeatureBrands, used, 1); e != nil {
				return e
			}
			b, err = r.Q.CreateBrand(ctx, db.CreateBrandParams{OrganizationID: org, Name: in.Name, LogoAttachmentID: Optional(in.LogoAttachmentID), PrimaryColor: in.PrimaryColor, SecondaryColor: in.SecondaryColor, SupportEmail: in.SupportEmail, SenderName: in.SenderName, ReplyTo: in.ReplyTo, HelpSubdomain: in.HelpSubdomain, HelpPath: in.HelpPath, WidgetDefaults: wd, Signature: in.Signature, BusinessHours: hours, SlaPolicyID: Optional(in.SLAPolicyID), AiPersona: in.AIPersona})
		} else {
			b, err = r.Q.UpdateBrand(ctx, db.UpdateBrandParams{OrganizationID: org, Uuid: id, Name: in.Name, LogoAttachmentID: Optional(in.LogoAttachmentID), PrimaryColor: in.PrimaryColor, SecondaryColor: in.SecondaryColor, SupportEmail: in.SupportEmail, SenderName: in.SenderName, ReplyTo: in.ReplyTo, HelpSubdomain: in.HelpSubdomain, HelpPath: in.HelpPath, WidgetDefaults: wd, Signature: in.Signature, BusinessHours: hours, SlaPolicyID: Optional(in.SLAPolicyID), AiPersona: in.AIPersona})
		}
		if err != nil {
			return err
		}
		return r.Emit(ctx, events.New("brands.updated").WithOrganization(org).WithEntity("brand", &b.ID, &b.Uuid))
	})
	if err != nil {
		return Brand{}, err
	}
	return s.model(ctx, b)
}
