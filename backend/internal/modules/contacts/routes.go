// Package contacts mounts contacts and companies (CF-84).
package contacts

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/ioadapter"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/tenant/contacts and /v1/tenant/companies.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantContactsRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantContactsWrite)
	del := middleware.RequireTenantPermission(rbac.PermTenantContactsDelete)
	route := func(pattern string, fn http.HandlerFunc, guard func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, authn, org, guard))
	}
	route("GET /v1/tenant/contacts", h.ListContacts, read)
	route("POST /v1/tenant/contacts", h.CreateContact, write)
	// CF-195: create or update by the customer's own user id.
	route("PUT /v1/tenant/contacts/by-external-id", h.UpsertContact, write)
	route("GET /v1/tenant/contacts/{uuid}", h.GetContact, read)
	route("PATCH /v1/tenant/contacts/{uuid}", h.UpdateContact, write)
	route("DELETE /v1/tenant/contacts/{uuid}", h.DeleteContact, del)
	route("POST /v1/tenant/contacts/{uuid}/merge", h.MergeContact, write)

	route("GET /v1/tenant/companies", h.ListCompanies, read)
	route("POST /v1/tenant/companies", h.CreateCompany, write)
	route("GET /v1/tenant/companies/{uuid}", h.GetCompany, read)
	route("PATCH /v1/tenant/companies/{uuid}", h.UpdateCompany, write)
	route("DELETE /v1/tenant/companies/{uuid}", h.DeleteCompany, del)
	// CF-86: customer revenue data.
	route("GET /v1/tenant/companies/{uuid}/revenue", h.GetCompanyRevenue, read)
	route("PUT /v1/tenant/companies/{uuid}/revenue", h.PutCompanyRevenue, write)
	route("DELETE /v1/tenant/companies/{uuid}/revenue", h.DeleteCompanyRevenue, write)
	route("PUT /v1/tenant/company-revenue", h.UpsertCompanyRevenue, write)
	// CF-85: account screen and unified timeline.
	route("GET /v1/tenant/companies/{uuid}/overview", h.GetCompanyOverview, read)
	route("GET /v1/tenant/companies/{uuid}/timeline", h.GetCompanyTimeline, read)
	route("POST /v1/tenant/companies/{uuid}/notes", h.CreateCompanyNote, write)
	route("DELETE /v1/tenant/company-notes/{uuid}", h.DeleteCompanyNote, write)
	route("POST /v1/tenant/company-events", h.RecordCompanyEvent, write)
}

// RegisterIORoutes mounts contact / company import and export (CF-84).
// Exports are a bulk read of personal data, so they need the write slug
// like imports do.
func RegisterIORoutes(mux *http.ServeMux, h *handler.IOHandler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	write := middleware.RequireTenantPermission(rbac.PermTenantContactsWrite)
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, middleware.Chain(fn, authn, org, write))
	}
	route("GET /v1/tenant/contacts/export", h.ExportContacts)
	route("GET /v1/tenant/companies/export", h.ExportCompanies)
	route("POST /v1/tenant/contacts/import", h.ImportContacts)
	route("POST /v1/tenant/companies/import", h.ImportCompanies)
	route("GET /v1/tenant/contacts/import/sample", h.ImportSample(ioadapter.ResourceContacts))
	route("GET /v1/tenant/companies/import/sample", h.ImportSample(ioadapter.ResourceCompanies))
	route("GET /v1/tenant/contact-imports", h.ListImports)
	route("GET /v1/tenant/contact-imports/{uuid}", h.GetImport)
	route("PATCH /v1/tenant/contact-imports/{uuid}/mapping", h.UpdateImportMapping)
	route("POST /v1/tenant/contact-imports/{uuid}/preview", h.PreviewImport)
	route("POST /v1/tenant/contact-imports/{uuid}/confirm", h.ConfirmImport)
	route("POST /v1/tenant/contact-imports/{uuid}/rollback", h.RollbackImport)
}
