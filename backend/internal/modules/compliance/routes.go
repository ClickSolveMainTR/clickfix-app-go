// Package compliance mounts privileged organization compliance tools.
package compliance

import (
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/compliance/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"io"
	"net/http"
	"time"
)

func RegisterRoutes(rt supportops.Router, s *usecase.Service) {
	org := func(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }
	route := func(pattern string, fn supportops.Endpoint) {
		rt.Tenant(pattern, "tenant.settings.write", func(r *http.Request) (any, error) {
			v, e := fn(r)
			if errors.Is(e, usecase.ErrInvalid) {
				e = supportops.ErrInvalid
			}
			return v, e
		})
	}
	route("GET /v1/tenant/compliance/retention", func(r *http.Request) (any, error) { return s.GetPolicy(r.Context(), org(r)) })
	route("PUT /v1/tenant/compliance/retention", func(r *http.Request) (any, error) {
		in, e := supportops.Read[usecase.Policy](r)
		if e != nil {
			return nil, e
		}
		return s.SavePolicy(r.Context(), org(r), in)
	})
	route("POST /v1/tenant/compliance/contacts/{uuid}/requests", func(r *http.Request) (any, error) {
		in, e := supportops.Read[struct {
			Kind string `json:"kind"`
		}](r)
		if e != nil {
			return nil, e
		}
		return s.Create(r.Context(), org(r), supportops.Actor(r.Context()), supportops.ID(r), in.Kind)
	})
	route("GET /v1/tenant/compliance/requests", func(r *http.Request) (any, error) {
		p, e := supportops.List(r)
		if e != nil {
			return nil, e
		}
		items, n, e := s.ListRequests(r.Context(), org(r), p.Limit, p.Offset)
		return supportops.Page(items, n, p), e
	})
	route("GET /v1/tenant/compliance/requests/{uuid}", func(r *http.Request) (any, error) {
		v, e := s.Repo.Q.ComplianceGetRequest(r.Context(), db.ComplianceGetRequestParams{OrganizationID: org(r), Uuid: supportops.ID(r)})
		return usecase.View(v), e
	})
	route("GET /v1/tenant/compliance/contacts/{uuid}/consents", func(r *http.Request) (any, error) {
		c, e := s.Repo.Q.ComplianceContact(r.Context(), db.ComplianceContactParams{OrganizationID: org(r), Uuid: supportops.ID(r)})
		if e != nil {
			return nil, e
		}
		p, e := supportops.List(r)
		if e != nil {
			return nil, e
		}
		rows, e := s.Repo.Q.ComplianceConsents(r.Context(), db.ComplianceConsentsParams{OrganizationID: org(r), ContactID: c.ID, Limit: p.Limit, Offset: p.Offset})
		if e != nil {
			return nil, e
		}
		n, e := s.Repo.Q.ComplianceConsentCount(r.Context(), db.ComplianceConsentCountParams{OrganizationID: org(r), ContactID: c.ID})
		views := make([]any, 0, len(rows))
		for _, row := range rows {
			views = append(views, consentView(row))
		}
		return supportops.Page(views, n, p), e
	})
	route("POST /v1/tenant/compliance/contacts/{uuid}/consents", func(r *http.Request) (any, error) {
		in, e := supportops.Read[struct {
			Purpose       string `json:"purpose"`
			Granted       *bool  `json:"granted"`
			NoticeVersion string `json:"notice_version"`
			Source        string `json:"source"`
		}](r)
		if e != nil || in.Granted == nil || in.Purpose == "" || in.NoticeVersion == "" || in.Source == "" {
			return nil, supportops.ErrInvalid
		}
		c, e := s.Repo.Q.ComplianceContact(r.Context(), db.ComplianceContactParams{OrganizationID: org(r), Uuid: supportops.ID(r)})
		if e != nil {
			return nil, e
		}
		if c.ErasedAt.Valid {
			return nil, supportops.ErrConflict
		}
		row, err := s.Repo.Q.ComplianceConsent(r.Context(), db.ComplianceConsentParams{OrganizationID: org(r), ContactID: c.ID, Purpose: in.Purpose, Granted: *in.Granted, NoticeVersion: in.NoticeVersion, Source: in.Source, RecordedBy: pgtype.Int8{Int64: supportops.Actor(r.Context()), Valid: true}})
		return consentView(row), err
	})
	route("GET /v1/tenant/compliance/processing-inventory", func(r *http.Request) (any, error) {
		p, e := s.GetPolicy(r.Context(), org(r))
		return map[string]any{"organization_uuid": orgctx.Must(r.Context()).UUID, "retention": p, "items": []map[string]any{
			{"category": "Kimlik ve iletişim", "purpose": "Müşteri destek hizmeti", "legal_basis": "Sözleşmenin ifası / meşru menfaat", "subjects": []string{"Müşteri", "Müşteri çalışanı"}, "data": []string{"Ad", "E-posta", "Telefon", "Kanal kimlikleri"}},
			{"category": "Müşteri işlem güvenliği", "purpose": "Destek kayıtları ve hizmet kalitesi", "legal_basis": "Sözleşmenin ifası / meşru menfaat", "data": []string{"Konuşmalar", "Mesajlar", "Dosya ekleri", "Yapay zekâ işlem kayıtları"}},
			{"category": "Hukuki işlem", "purpose": "İlgili kişi başvurularının karşılanması ve açık rıza ispatı", "legal_basis": "Hukuki yükümlülük", "data": []string{"Başvuru denetim izi", "Aydınlatma metni sürümü", "Onay ve geri alma kayıtları"}},
		}, "review_required": true, "note": "VERBİS envanteri için taslaktır; veri sorumlusu alıcı gruplarını, yurt dışı aktarımı ve saklama gerekçelerini kendi süreçlerine göre tamamlamalıdır."}, e
	})
	if rt.Resolver != nil {
		rt.Mux.Handle("GET /v1/tenant/compliance/requests/{uuid}/download", middleware.Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			row, e := s.Repo.Q.ComplianceGetRequest(r.Context(), db.ComplianceGetRequestParams{OrganizationID: org(r), Uuid: supportops.ID(r)})
			if e != nil || row.Status != "completed" || row.ObjectKey == "" || !row.ExpiresAt.Valid || !row.ExpiresAt.Time.After(time.Now()) {
				supportops.Fail(w, r, pgx.ErrNoRows)
				return
			}
			body, size, e := s.Store.Download(r.Context(), row.ObjectKey)
			if e != nil {
				supportops.Fail(w, r, e)
				return
			}
			defer func() { _ = body.Close() }()
			// A data-subject export is personal data leaving the system: audit it (CF-153).
			actor := supportops.Actor(r.Context())
			s.Activity.Record(r.Context(), &actor, "compliance.downloaded", "compliance", &row.Uuid, map[string]any{"kind": row.Kind}, r)
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Content-Disposition", `attachment; filename="data-subject.zip"`)
			_ = size
			_, _ = io.Copy(w, body)
		}), middleware.Authenticate(rt.Tokens, rt.Loader), middleware.RequireOrganization(rt.Resolver), middleware.RequireTenantPermission("tenant.settings.write")))
	}
}

func consentView(row db.ContactConsent) any {
	return map[string]any{"uuid": row.Uuid, "purpose": row.Purpose, "granted": row.Granted, "notice_version": row.NoticeVersion, "source": row.Source, "created_at": row.CreatedAt}
}
