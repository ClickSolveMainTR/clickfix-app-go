// Package aimemory registers the CF-216 knowledge layer APIs: knowledge
// units, the answer cache, case memory and unanswered questions.
package aimemory

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aimemory/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil || h == nil || h.Service == nil {
		return
	}
	for _, route := range []struct {
		pattern    string
		fn         http.HandlerFunc
		permission string
	}{
		{"POST /v1/tenant/ai/memory/search", h.Search, rbac.PermTenantConversationsReply},
		{"GET /v1/tenant/ai/case-memories", h.Cases, rbac.PermTenantSettingsRead},
		{"POST /v1/tenant/ai/case-memories/{uuid}/review", h.ReviewCase, rbac.PermTenantSettingsWrite},
		{"GET /v1/tenant/ai/knowledge-units", h.Units, rbac.PermTenantSettingsRead},
		{"GET /v1/tenant/ai/answer-cache/stats", h.Stats, rbac.PermTenantSettingsRead},
		{"GET /v1/tenant/ai/knowledge-gaps/questions", h.GapQuestions, rbac.PermTenantSettingsRead},
	} {
		mux.Handle(route.pattern, middleware.Chain(route.fn, middleware.Authenticate(tokens, loader), middleware.RequireOrganization(resolver), middleware.RequireTenantPermission(route.permission)))
	}
}
