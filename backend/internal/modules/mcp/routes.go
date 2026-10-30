package mcp

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/model"
)

// RegisterRoutes mounts the three MCP endpoints outside /v1 (the frontend
// proxies /mcp/*). POST carries the JSON-RPC messages; GET and DELETE are
// answered by the stateless SDK handler after the same Bearer check.
func RegisterRoutes(mux *http.ServeMux, s *Server) {
	for _, path := range model.Resources {
		for _, method := range []string{http.MethodPost, http.MethodGet, http.MethodDelete, http.MethodOptions} {
			mux.Handle(method+" "+path, s)
		}
	}
}
