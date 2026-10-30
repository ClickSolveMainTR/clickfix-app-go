package brands

import (
	"context"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type EmailTemplateInput struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (s *Service) Delete(ctx context.Context, org int64, id uuid.UUID) (bool, error) {
	n, err := s.Repo.Q.DeleteBrand(ctx, db.DeleteBrandParams{OrganizationID: org, Uuid: id})
	return n > 0, err
}
func (s *Service) Templates(ctx context.Context, org int64, id uuid.UUID) ([]db.BrandEmailTemplate, error) {
	if _, err := s.Repo.Q.GetBrand(ctx, db.GetBrandParams{OrganizationID: org, Uuid: id}); err != nil {
		return nil, err
	}
	return s.Repo.Q.ListBrandEmailTemplates(ctx, db.ListBrandEmailTemplatesParams{OrganizationID: org, BrandID: id})
}
func (s *Service) SaveTemplate(ctx context.Context, org int64, id uuid.UUID, code string, in EmailTemplateInput) (db.BrandEmailTemplate, error) {
	if len(code) < 1 || len(code) > 80 || len(in.Body) > 20000 || code == "reply" && !strings.Contains(in.Body, "{{content}}") || len(in.Subject) > 998 || strings.ContainsAny(in.Subject, "\r\n") {
		return db.BrandEmailTemplate{}, supportops.ErrInvalid
	}
	return s.Repo.Q.SaveBrandEmailTemplate(ctx, db.SaveBrandEmailTemplateParams{OrganizationID: org, BrandID: id, Code: code, Subject: in.Subject, Body: in.Body})
}
func (s *Service) AssignArticle(ctx context.Context, org int64, id, article uuid.UUID) (bool, error) {
	if _, err := s.Repo.Q.GetBrand(ctx, db.GetBrandParams{OrganizationID: org, Uuid: id}); err != nil {
		return false, err
	}
	n, err := s.Repo.Q.AssignArticleBrand(ctx, db.AssignArticleBrandParams{OrganizationID: org, Uuid: article, BrandID: OptionalUUID(id)})
	if err == nil && n == 0 {
		return false, pgx.ErrNoRows
	}
	return n > 0, err
}

// PublicHelp resolves a public help identity and only returns published content and public styling.
func (s *Service) PublicHelp(ctx context.Context, orgSlug, brandID, subdomain, path, locale, article, query string) (any, error) {
	var b db.Brand
	var err error
	if brandID != "" {
		id, e := uuid.Parse(brandID)
		if e != nil {
			return nil, pgx.ErrNoRows
		}
		b, err = s.Repo.Q.ResolvePublicBrand(ctx, db.ResolvePublicBrandParams{Slug: orgSlug, Uuid: id})
	} else {
		b, err = s.Repo.Q.ResolvePublicBrandIdentity(ctx, db.ResolvePublicBrandIdentityParams{Subdomain: normalizeHelpHost(subdomain), OrganizationSlug: orgSlug, HelpPath: path})
	}
	if err != nil {
		return nil, err
	}
	if locale != "en" {
		locale = "tr"
	}
	if article != "" {
		return s.Repo.Q.BrandPublicArticle(ctx, db.BrandPublicArticleParams{OrganizationID: b.OrganizationID, BrandID: OptionalUUID(b.Uuid), Locale: locale, Slug: article})
	}
	query = strings.TrimSpace(query)
	if len(query) > 200 {
		return nil, supportops.ErrInvalid
	}
	articles, err := s.Repo.Q.BrandPublicArticles(ctx, db.BrandPublicArticlesParams{OrganizationID: b.OrganizationID, BrandID: OptionalUUID(b.Uuid), Locale: locale, Q: query})
	if err != nil {
		return nil, err
	}
	brand, err := s.model(ctx, b)
	if err != nil {
		return nil, err
	}
	return map[string]any{"brand_id": brand.UUID, "name": brand.Name, "logo_url": brand.LogoURL, "primary_color": brand.PrimaryColor, "secondary_color": brand.SecondaryColor, "items": articles}, nil
}
