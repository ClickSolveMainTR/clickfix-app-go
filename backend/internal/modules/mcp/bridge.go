// Ported from takwim-app-go modules/mcp/bridge.go, organization staff only.
package mcp

import "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"

// Bridge runs the panel's /v1 routes in process as the principal; tools
// reach it through capability.Bridge, so the routes' own permission and
// validation rules apply (CF-192).
type Bridge struct{ API *apibridge.Bridge }
