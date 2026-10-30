// Ported from takwim-app-go modules/mcp/bridge.go, organization staff only.
package mcp

import (
	"context"
	"encoding/json"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"
	"net/http"
	"strings"
)

type Bridge struct{ API *apibridge.Bridge }
type Response struct {
	Status    int
	Body      []byte
	Truncated bool
}

func (b *Bridge) Do(ctx context.Context, p Principal, c Call, _ *http.Request) (Response, error) {
	path := c.Path
	if len(c.Query) > 0 {
		path += "?" + c.Query.Encode()
	}
	out, err := b.API.JSON(ctx, apibridge.Principal{UserUUID: p.UserUUID, OrganizationUUID: p.Scope.UUID}, c.Method, path, c.Body)
	return Response{Status: out.Status, Body: out.Body, Truncated: out.Truncated}, err
}

// envelope is the /v1 response shape (pkg/response).
type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details []struct {
			Field   string `json:"field"`
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"details"`
	} `json:"error"`
}

// Outcome is a bridged response read for the client: the data on success,
// else a code and a message the model can act on.
type Outcome struct {
	OK      bool
	Data    json.RawMessage
	Code    string
	Message string
}

// Read converts a bridged response.
func (r Response) Read() Outcome {
	if r.Truncated {
		return Outcome{Code: "RESULT_TOO_LARGE", Message: "The result is too large. Ask for fewer rows (limit) or narrow the filters."}
	}
	if r.Status == http.StatusNoContent {
		return Outcome{OK: true, Data: json.RawMessage(`{"ok":true}`)}
	}
	var env envelope
	if err := json.Unmarshal(r.Body, &env); err != nil {
		if r.Status < 300 {
			return Outcome{Code: "TOOL_FAILED", Message: "The server returned an unreadable response."}
		}
		return Outcome{Code: statusCode(r.Status), Message: http.StatusText(r.Status)}
	}
	if r.Status < 300 && env.Success {
		data := env.Data
		if len(data) == 0 || string(data) == "null" {
			data = json.RawMessage(`{"ok":true}`)
		}
		return Outcome{OK: true, Data: data}
	}
	out := Outcome{Code: statusCode(r.Status), Message: http.StatusText(r.Status)}
	if env.Error != nil {
		if env.Error.Code != "" {
			out.Code = env.Error.Code
		}
		if env.Error.Message != "" {
			out.Message = env.Error.Message
		}
		var parts []string
		for _, d := range env.Error.Details {
			if d.Field != "" {
				parts = append(parts, d.Field+": "+d.Message)
			} else {
				parts = append(parts, d.Message)
			}
		}
		if len(parts) > 0 {
			out.Message += " (" + strings.Join(parts, "; ") + ")"
		}
	}
	return out
}

func statusCode(status int) string {
	switch {
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return "FORBIDDEN"
	case status == http.StatusNotFound:
		return "NOT_FOUND"
	case status == http.StatusConflict:
		return "CONFLICT"
	case status == http.StatusTooManyRequests:
		return "RATE_LIMITED"
	case status >= 500:
		return "TOOL_FAILED"
	}
	return "INVALID_REQUEST"
}
