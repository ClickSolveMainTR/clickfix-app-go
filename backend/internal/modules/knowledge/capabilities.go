package knowledge

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Capabilities of the knowledge base (CF-211): search, read, draft, edit
// and publish articles through /v1/tenant/knowledge as the member. Saves
// create a new draft version; publishing is a separate confirmed step.

const articlesPath = "/v1/tenant/knowledge/articles"

func articleProps() map[string]any {
	return map[string]any{
		"title":           capability.Titled("Başlık", capability.Text(300, "")),
		"slug":            capability.Titled("Kısa ad", map[string]any{"type": "string", "maxLength": 200, "pattern": "^[a-z0-9]+(-[a-z0-9]+)*$"}),
		"locale":          capability.Enum("tr", "en"),
		"visibility":      capability.Enum("public", "internal"),
		"body":            capability.Titled("İçerik", capability.Text(200000, "markdown")),
		"tags":            map[string]any{"type": "array", "maxItems": 30, "items": capability.Text(100, "")},
		"collection_uuid": capability.UUID(""),
	}
}

// Capabilities are the knowledge base capabilities.
func Capabilities() []*capability.Capability {
	s, routes := capability.ReadSurfaces, []string{"/settings/knowledge"}
	id := capability.UUID("article uuid")
	upd := articleProps()
	upd["article_uuid"] = id
	upd["version"] = map[string]any{"type": "integer", "minimum": 1, "description": "current version from knowledge_articles_get"}
	return []*capability.Capability{
		{
			Name: "knowledge.search", Module: "knowledge", Kind: capability.Read, Permission: rbac.PermTenantSettingsRead, Surfaces: s, Routes: routes,
			Title: capability.T("Bilgi bankasında ara", "Search the knowledge base"), Description: "Search published articles of this organization (top passages).",
			Input: capability.Object([]string{"q"}, map[string]any{"q": capability.Text(500, ""), "locale": capability.Enum("tr", "en"), "public_only": capability.Bool("")}),
			Run:   capability.Get("/v1/tenant/knowledge/search", "q", "locale", "public_only"),
		},
		{
			Name: "knowledge.articles.list", Module: "knowledge", Kind: capability.Read, Permission: rbac.PermTenantSettingsRead, Surfaces: s, Routes: routes,
			Title: capability.T("Makaleler", "Articles"), Description: "List articles (title q, state draft|published|changed).",
			Input: capability.Object(nil, capability.Paged(map[string]any{"q": capability.Text(200, ""), "locale": capability.Enum("tr", "en"),
				"visibility": capability.Enum("public", "internal"), "state": capability.Enum("draft", "published", "changed"), "collection_uuid": capability.UUID("")})),
			Run: capability.Get(articlesPath, "q", "locale", "visibility", "state", "collection_uuid", "limit", "offset"),
		},
		{
			Name: "knowledge.articles.get", Module: "knowledge", Kind: capability.Read, Permission: rbac.PermTenantSettingsRead, Surfaces: s, Routes: routes,
			Title: capability.T("Makaleyi getir", "Get an article"), FullText: true, Description: "One article with its body and current version.",
			Input: capability.Object([]string{"article_uuid"}, map[string]any{"article_uuid": id}),
			Run:   capability.Get(articlesPath + "/{article_uuid}"),
		},
		{
			Name: "knowledge.collections.list", Module: "knowledge", Kind: capability.Read, Permission: rbac.PermTenantSettingsRead, Surfaces: s, Routes: routes,
			Title: capability.T("Koleksiyonlar", "Collections"), Description: "Knowledge collections and categories.",
			Input: capability.Object(nil, capability.Paged(map[string]any{})),
			Run:   capability.Get("/v1/tenant/knowledge/collections", "limit", "offset"),
		},
		{
			Name: "knowledge.reviews.list", Module: "knowledge", Kind: capability.Read, Permission: rbac.PermTenantSettingsRead, Surfaces: s, Routes: routes,
			Title: capability.T("İnceleme bekleyen makaleler", "Articles to review"), Description: "Articles flagged for review and why.",
			Input: capability.Object(nil, capability.Paged(map[string]any{})),
			Run:   capability.Get("/v1/tenant/knowledge/reviews", "limit", "offset"),
		},
		{
			Name: "knowledge.articles.create", Module: "knowledge", Kind: capability.Write, Permission: rbac.PermTenantSettingsWrite, Surfaces: s, Routes: routes,
			Title: capability.T("Makale taslağı oluştur", "Draft an article"), Description: "Create an article draft (not published).",
			Input: capability.Object([]string{"title", "slug", "locale", "visibility"}, articleProps()),
			Apply: capability.Endpoint{Method: http.MethodPost, Path: articlesPath, Route: "/settings/knowledge"}.Do(),
		},
		{
			Name: "knowledge.articles.update", Module: "knowledge", Kind: capability.Write, Permission: rbac.PermTenantSettingsWrite, Surfaces: s, Routes: routes,
			Title: capability.T("Makaleyi düzenle", "Edit an article"), Description: "Save a new draft version: a full replace, so send every field, the whole body and the current version (read it with knowledge_articles_get first).",
			Input: capability.Object([]string{"article_uuid", "version", "title", "slug", "locale", "visibility", "body"}, upd),
			Apply: capability.Endpoint{Method: http.MethodPut, Path: articlesPath + "/{article_uuid}", Route: "/settings/knowledge/{article_uuid}"}.Do(),
		},
		{
			Name: "knowledge.articles.publish", Module: "knowledge", Kind: capability.Write, Permission: rbac.PermTenantSettingsWrite, Surfaces: s, Routes: routes,
			Title: capability.T("Makaleyi yayınla / kaldır", "Publish or unpublish"), Description: "Publish (publish=true) or unpublish the current version.",
			Input: capability.Object([]string{"article_uuid", "version", "publish"}, map[string]any{"article_uuid": id,
				"version": map[string]any{"type": "integer", "minimum": 1}, "publish": capability.Bool("")}),
			Apply: capability.Endpoint{Method: http.MethodPost, Path: articlesPath + "/{article_uuid}/publish", Body: []string{"version", "publish"}, Route: "/settings/knowledge/{article_uuid}"}.Do(),
		},
	}
}
