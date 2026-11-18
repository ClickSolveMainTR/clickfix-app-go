// Package conversations mounts the inbox routes (CF-43 / CF-44):
// /v1/tenant/conversations and /v1/tenant/tags.
package conversations

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the inbox. A nil resolver (tenancy off) mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantConversationsRead)
	reply := middleware.RequireTenantPermission(rbac.PermTenantConversationsReply)
	manage := middleware.RequireTenantPermission(rbac.PermTenantConversationsManage)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		// h.WithViewer last: after the permission guards it puts the caller's
		// team visibility on the context (CF-44).
		chain := append([]func(http.Handler) http.Handler{authn, org}, guards...)
		mux.Handle(pattern, middleware.Chain(fn, append(chain, h.WithViewer)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/conversations/counts", h.Counts, read)
	route("GET /v1/tenant/conversations/agents", h.Agents, read)
	route("GET /v1/tenant/conversation-views/counts", h.ViewCounts, read)
	route("GET /v1/tenant/conversation-views", h.Views, read)
	route("POST /v1/tenant/conversation-views", h.CreateView, read)
	route("PATCH /v1/tenant/conversation-views/{uuid}", h.UpdateView, read)
	route("DELETE /v1/tenant/conversation-views/{uuid}", h.DeleteView, read)
	route("POST /v1/tenant/conversations/bulk", h.Bulk, read)
	route("POST /v1/tenant/conversations/bulk/undo/{uuid}", h.UndoBulk, read)
	route("DELETE /v1/tenant/conversation-reminders/{uuid}", h.CancelReminder, read)
	route("POST /v1/tenant/conversations", h.Create, read, manage)
	route("GET /v1/tenant/conversations", h.List, read)
	route("GET /v1/tenant/conversations/{uuid}", h.Get, read)
	// PATCH, POST messages: the handler checks manage / assign and reply / note
	// per field (a light agent may note but not reply; an agent may assign).
	route("PATCH /v1/tenant/conversations/{uuid}", h.Update, read)
	route("GET /v1/tenant/conversations/{uuid}/diagnostics", h.Diagnostics, read)
	route("GET /v1/tenant/conversations/{uuid}/messages", h.Messages, read)
	route("POST /v1/tenant/conversations/{uuid}/messages", h.Post, read)
	route("GET /v1/tenant/conversations/{uuid}/closing-texts", h.ClosingTexts, read)
	route("POST /v1/tenant/conversations/{uuid}/messages/{message}/retry", h.Retry, read, reply)
	// CF-248: an agent corrects their own reply; the usecase checks author,
	// kind and the edit window.
	route("PATCH /v1/tenant/conversations/{uuid}/messages/{message}", h.EditMessage, read, reply)
	route("GET /v1/tenant/conversations/{uuid}/messages/{message}/edits", h.MessageEdits, read)
	route("POST /v1/tenant/conversations/{uuid}/read", h.Read, read)
	route("POST /v1/tenant/conversations/{uuid}/unread", h.Unread, read)
	route("POST /v1/tenant/conversations/{uuid}/presence", h.Presence, read)
	route("DELETE /v1/tenant/conversations/{uuid}/presence", h.Leave, read)
	route("GET /v1/tenant/conversations/{uuid}/reminders", h.Reminders, read)
	route("POST /v1/tenant/conversations/{uuid}/reminders", h.AddReminder, read)
	route("POST /v1/tenant/conversations/{uuid}/tags", h.AddTag, read, manage)
	route("DELETE /v1/tenant/conversations/{uuid}/tags/{tag}", h.RemoveTag, read, manage)

	// CF-98: merge / split, links, side conversations.
	route("POST /v1/tenant/conversations/{uuid}/merge", h.Merge, read, manage)
	route("POST /v1/tenant/conversations/{uuid}/split", h.Split, read, manage)
	route("GET /v1/tenant/conversations/{uuid}/links", h.Links, read)
	route("POST /v1/tenant/conversations/{uuid}/links", h.Link, read, manage)
	route("DELETE /v1/tenant/conversations/{uuid}/links/{other}", h.Unlink, read, manage)
	route("GET /v1/tenant/conversations/{uuid}/side-conversations", h.SideConversations, read)
	route("POST /v1/tenant/conversations/{uuid}/side-conversations", h.StartSideConversation, read, reply)
	route("GET /v1/tenant/side-conversations/{uuid}", h.SideConversation, read)
	route("POST /v1/tenant/side-conversations/{uuid}/messages", h.ReplySideConversation, read, reply)
	route("PATCH /v1/tenant/side-conversations/{uuid}", h.UpdateSideConversation, read, reply)

	// CF-249: the organization's own conversation statuses.
	route("GET /v1/tenant/conversation-statuses", h.CustomStatuses, read)
	route("POST /v1/tenant/conversation-statuses", h.CreateCustomStatus, manage)
	route("PATCH /v1/tenant/conversation-statuses/{uuid}", h.UpdateCustomStatus, manage)
	route("DELETE /v1/tenant/conversation-statuses/{uuid}", h.DeleteCustomStatus, manage)

	route("GET /v1/tenant/tags", h.Tags, read)
	route("POST /v1/tenant/tags", h.CreateTag, manage)
	route("PATCH /v1/tenant/tags/{uuid}", h.UpdateTag, manage)
	route("DELETE /v1/tenant/tags/{uuid}", h.DeleteTag, manage)
}
