// Package widget mounts the public web widget API (CF-55):
// /v1/widget/{public_id}/... called by the embeddable SDK from customers'
// sites (CORS, no session; a signed visitor token instead).
package widget

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/handler"
)

// RegisterRoutes mounts the widget routes.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler) {
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, handler.CORS(fn))
	}
	route("OPTIONS /v1/widget/{public_id}/{rest...}", h.Preflight)
	route("POST /v1/widget/{public_id}/bootstrap", h.Bootstrap)
	route("POST /v1/widget/{public_id}/identify", h.Identify)
	route("PATCH /v1/widget/{public_id}/profile", h.UpdateProfile)
	route("PATCH /v1/widget/{public_id}/attributes", h.SetAttributes)
	route("POST /v1/widget/{public_id}/realtime-token", h.RealtimeToken)
	route("GET /v1/widget/{public_id}/conversations", h.ListConversations)
	route("POST /v1/widget/{public_id}/conversations", h.CreateConversation)
	route("GET /v1/widget/{public_id}/conversations/{conversation}/messages", h.ListMessages)
	route("POST /v1/widget/{public_id}/conversations/{conversation}/messages", h.SendMessage)
	route("POST /v1/widget/{public_id}/conversations/{conversation}/read", h.MarkRead)
	route("POST /v1/widget/{public_id}/conversations/{conversation}/typing", h.Typing)
	route("POST /v1/widget/{public_id}/conversations/{conversation}/rating", h.Rate)
	route("POST /v1/widget/{public_id}/conversations/{conversation}/diagnostics", h.Capture)
	route("GET /v1/widget/{public_id}/replay-policy", h.ReplayPolicy)
	route("POST /v1/widget/{public_id}/conversations/{conversation}/sessions", h.StartReplay)
	route("POST /v1/widget/{public_id}/conversations/{conversation}/sessions/{session}/consent", h.SessionConsent)
	route("POST /v1/widget/{public_id}/conversations/{conversation}/sessions/{session}/chunks", h.ReplayChunk)
	route("POST /v1/widget/{public_id}/conversations/{conversation}/sessions/{session}/live-token", h.VisitorLiveToken)
	route("GET /v1/widget/{public_id}/help/search", h.HelpSearch)
	route("GET /v1/widget/{public_id}/booking", h.Booking)
}
