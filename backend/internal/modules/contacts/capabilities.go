package contacts

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Capabilities of contacts and companies (CF-211): the assistant and MCP
// clients look people and accounts up, create and edit them through
// /v1/tenant as the member. Deleting a contact is destructive.

const (
	contactsPath  = "/v1/tenant/contacts"
	companiesPath = "/v1/tenant/companies"
)

func contactProps() map[string]any {
	return map[string]any{
		"name": capability.Titled("Ad", capability.Text(255, "")), "email": capability.Titled("E-posta", capability.Text(254, "")),
		"phone": capability.Titled("Telefon", capability.Text(40, "")), "job_title": capability.Text(120, ""),
		"company": capability.UUID("company uuid"), "timezone": capability.Text(64, "IANA"), "locale": capability.Text(10, ""),
		"notes": capability.Text(5000, ""), "external_id": capability.Text(200, ""),
	}
}

func companyProps() map[string]any {
	return map[string]any{
		"name": capability.Titled("Ad", capability.Text(255, "")), "domain": capability.Titled("Alan adı", capability.Text(255, "")),
		"tier": capability.Text(64, ""), "account_owner": capability.UUID("member uuid"), "notes": capability.Text(5000, ""),
		"external_id": capability.Text(200, ""),
	}
}

func needOne(keys ...string) func(capability.Params) error {
	return func(p capability.Params) error {
		for _, k := range keys {
			if s, _ := p[k].(string); s != "" {
				return capability.CheckUUIDs(p, "contact_uuid", "company_uuid", "company", "account_owner")
			}
		}
		return capability.Invalid("one of %v is required: ask the user", keys)
	}
}

// Capabilities are the contact and company capabilities.
func Capabilities() []*capability.Capability {
	s := capability.ReadSurfaces
	cRoutes, coRoutes := []string{"/contacts"}, []string{"/companies"}
	contact, company := capability.UUID("contact uuid"), capability.UUID("company uuid")
	update := contactProps()
	update["contact_uuid"] = contact
	coUpdate := companyProps()
	coUpdate["company_uuid"] = company
	return []*capability.Capability{
		{
			Name: "contacts.list", MCPName: "list_contacts", Module: "contacts", Kind: capability.Read, Permission: rbac.PermTenantContactsRead,
			Surfaces: s, Routes: cRoutes, Title: capability.T("Kişileri ara", "Search contacts"),
			Description: "Search contacts by name, e-mail or phone (q); filter by company uuid.",
			Input: capability.Object(nil, capability.Paged(map[string]any{"q": capability.Text(200, ""), "company": company, "external_id": capability.Text(200, ""),
				"sort": capability.Enum("-created_at", "name", "-last_seen_at")})),
			Run: capability.Get(contactsPath, "q", "company", "external_id", "sort", "limit", "offset"),
		},
		{
			Name: "contacts.get", Module: "contacts", Kind: capability.Read, Permission: rbac.PermTenantContactsRead,
			Surfaces: s, Routes: cRoutes, Title: capability.T("Kişiyi getir", "Get a contact"), FullText: true,
			Description: "One contact with its channels and conversation count.",
			Input:       capability.Object([]string{"contact_uuid"}, map[string]any{"contact_uuid": contact}),
			Run:         capability.Get(contactsPath + "/{contact_uuid}"),
		},
		{
			Name: "contacts.create", Module: "contacts", Kind: capability.Write, Permission: rbac.PermTenantContactsWrite,
			Surfaces: s, Routes: cRoutes, Title: capability.T("Kişi oluştur", "Create a contact"),
			Description: "Create a contact (name, email or phone required); company is matched from the e-mail domain when omitted.",
			Input:       capability.Object(nil, contactProps()), Validate: needOne("name", "email", "phone"),
			Apply: capability.Endpoint{Method: http.MethodPost, Path: contactsPath, Route: "/contacts"}.Do(),
		},
		{
			Name: "contacts.update", Module: "contacts", Kind: capability.Write, Permission: rbac.PermTenantContactsWrite,
			Surfaces: s, Routes: cRoutes, Title: capability.T("Kişiyi güncelle", "Update a contact"),
			Description: "Change contact fields; omitted fields stay.",
			Input:       capability.Object([]string{"contact_uuid"}, update),
			Validate:    func(p capability.Params) error { return capability.CheckUUIDs(p, "contact_uuid", "company") },
			Apply:       capability.Endpoint{Method: http.MethodPatch, Path: contactsPath + "/{contact_uuid}", Route: "/contacts"}.Do(),
		},
		{
			Name: "contacts.delete", Module: "contacts", Kind: capability.Destructive, Permission: rbac.PermTenantContactsDelete,
			Surfaces: capability.DefaultSurfaces, Routes: cRoutes, Title: capability.T("Kişiyi sil", "Delete a contact"),
			Description: "Delete a contact; its conversations stay.",
			Input:       capability.Object([]string{"contact_uuid"}, map[string]any{"contact_uuid": contact}),
			Apply:       capability.Endpoint{Method: http.MethodDelete, Path: contactsPath + "/{contact_uuid}", Route: "/contacts"}.Do(),
		},
		{
			Name: "companies.list", Module: "companies", Kind: capability.Read, Permission: rbac.PermTenantContactsRead,
			Surfaces: s, Routes: coRoutes, Title: capability.T("Firmaları ara", "Search companies"),
			Description: "Search companies (name/domain q) with contact and open-conversation counts.",
			Input:       capability.Object(nil, capability.Paged(map[string]any{"q": capability.Text(200, ""), "external_id": capability.Text(200, "")})),
			Run:         capability.Get(companiesPath, "q", "external_id", "limit", "offset"),
		},
		{
			Name: "companies.get", Module: "companies", Kind: capability.Read, Permission: rbac.PermTenantContactsRead,
			Surfaces: s, Routes: coRoutes, Title: capability.T("Firma özeti", "Company overview"), FullText: true,
			Description: "Company overview: stats, CSAT, open and recently closed conversations.",
			Input:       capability.Object([]string{"company_uuid"}, map[string]any{"company_uuid": company}),
			Run:         capability.Get(companiesPath + "/{company_uuid}/overview"),
		},
		{
			Name: "companies.create", Module: "companies", Kind: capability.Write, Permission: rbac.PermTenantContactsWrite,
			Surfaces: s, Routes: coRoutes, Title: capability.T("Firma oluştur", "Create a company"),
			Description: "Create a company; contacts with its e-mail domain are linked.",
			Input:       capability.Object([]string{"name"}, companyProps()),
			Validate:    func(p capability.Params) error { return capability.CheckUUIDs(p, "account_owner") },
			Apply:       capability.Endpoint{Method: http.MethodPost, Path: companiesPath, Route: "/companies"}.Do(),
		},
		{
			Name: "companies.update", Module: "companies", Kind: capability.Write, Permission: rbac.PermTenantContactsWrite,
			Surfaces: s, Routes: coRoutes, Title: capability.T("Firmayı güncelle", "Update a company"),
			Description: "Change company fields; omitted fields stay.",
			Input:       capability.Object([]string{"company_uuid"}, coUpdate),
			Validate:    func(p capability.Params) error { return capability.CheckUUIDs(p, "company_uuid", "account_owner") },
			Apply:       capability.Endpoint{Method: http.MethodPatch, Path: companiesPath + "/{company_uuid}", Route: "/companies/{company_uuid}"}.Do(),
		},
		{
			Name: "companies.add_note", Module: "companies", Kind: capability.Write, Permission: rbac.PermTenantContactsWrite,
			Surfaces: s, Routes: coRoutes, Title: capability.T("Firmaya not ekle", "Add a company note"),
			Description: "Add a note to a company's timeline.",
			Input:       capability.Object([]string{"company_uuid", "body"}, map[string]any{"company_uuid": company, "body": capability.Titled("Not", capability.Text(10000, ""))}),
			Apply:       capability.Endpoint{Method: http.MethodPost, Path: companiesPath + "/{company_uuid}/notes", Body: []string{"body"}, Route: "/companies/{company_uuid}"}.Do(),
		},
	}
}
