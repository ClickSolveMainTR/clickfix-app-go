// Package channels mounts the channel instance routes (CF-8): the
// organization's inboxes under /v1/tenant/channels and the provider webhook
// endpoints under /v1/webhooks/channels.
package channels

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the channel routes. A nil resolver (tenancy off)
// mounts nothing: channels always belong to an organization.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantChannelsRead)
	manage := middleware.RequireTenantPermission(rbac.PermTenantChannelsManage)
	route := func(pattern string, fn http.HandlerFunc, guard func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, authn, org, guard))
	}

	route("POST /v1/tenant/webphone/events", h.WebphoneEvents, read)
	route("GET /v1/tenant/webphone/config", h.WebphoneConfig, read)
	route("GET /v1/tenant/webphone/members/{user}", h.MemberSIP, manage)
	route("PUT /v1/tenant/webphone/members/{user}", h.MemberSIP, manage)
	route("GET /v1/tenant/channels/types", h.Types, read)
	route("GET /v1/tenant/channels", h.List, read)
	route("POST /v1/tenant/channels", h.Create, manage)
	route("GET /v1/tenant/channels/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/channels/{uuid}", h.Update, manage)
	route("DELETE /v1/tenant/channels/{uuid}", h.Delete, manage)
	route("POST /v1/tenant/channels/{uuid}/check", h.Check, manage)

	// Connect wizards of provider channels (providers.go).
	route("POST /v1/tenant/channels/email/test", h.EmailTest, manage)
	route("POST /v1/tenant/channels/email/domains", h.EmailDomainCreate, manage)
	route("GET /v1/tenant/channels/email/domains/{domain}", h.EmailDomainGet, read)
	route("POST /v1/tenant/channels/email/domains/{domain}/verify", h.EmailDomainVerify, manage)
	route("GET /v1/tenant/channels/whatsapp/signup", h.WhatsAppSignupConfig, manage)
	route("POST /v1/tenant/channels/whatsapp/signup", h.WhatsAppSignup, manage)
	route("GET /v1/tenant/channels/{uuid}/whatsapp/templates", h.WhatsAppTemplates, read)
	route("POST /v1/tenant/channels/{uuid}/whatsapp/templates", h.WhatsAppTemplateCreate, manage)
	route("DELETE /v1/tenant/channels/{uuid}/whatsapp/templates/{name}", h.WhatsAppTemplateDelete, manage)
	route("GET /v1/tenant/channels/oauth/{type}/start", h.OAuthStart, manage)
	route("POST /v1/tenant/channels/oauth/slack/complete", h.SlackComplete, manage)
	route("GET /v1/tenant/channels/{uuid}/voice/calls", h.VoiceCalls, read)
	route("GET /v1/tenant/channels/{uuid}/slack/conversations", h.SlackConversations, manage)
	route("GET /v1/tenant/channels/{uuid}/google_chat/spaces", h.GoogleChatSpaces, manage)
	route("POST /v1/tenant/channels/oauth/instagram/complete", h.InstagramComplete, manage)
	route("POST /v1/tenant/channels/oauth/messenger/complete", h.MessengerComplete, manage)
	route("POST /v1/tenant/channels/oauth/messenger/pages", h.MessengerSelectPage, manage)
	route("POST /v1/tenant/channels/{uuid}/reconnect", h.Reconnect, manage)

	// Provider webhooks: authenticated by the adapter (signature with the
	// instance's own secret), never by a session.
	mux.Handle("GET /v1/webhooks/channels/{type}/{instance}", http.HandlerFunc(h.Challenge))
	mux.Handle("POST /v1/webhooks/channels/{type}/{instance}", http.HandlerFunc(h.Webhook))
	mux.Handle("GET /v1/webhooks/channels/{type}", http.HandlerFunc(h.SharedChallenge))
	mux.Handle("POST /v1/webhooks/channels/{type}", http.HandlerFunc(h.SharedWebhook))
}
