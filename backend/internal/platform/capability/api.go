package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"
	"github.com/google/uuid"
)

// Response is a /v1 response read for a capability: the data on success,
// else the API error code and message.
type Response struct {
	Status  int
	OK      bool
	Data    json.RawMessage
	Code    string
	Message string
}

// Decode unmarshals the data of a successful response.
func (r Response) Decode(v any) error {
	if !r.OK {
		return r.Err()
	}
	return json.Unmarshal(r.Data, v)
}

// Err is the API error of a failed response.
func (r Response) Err() error {
	if r.OK {
		return nil
	}
	return &APIError{Status: r.Status, Code: r.Code, Message: r.Message}
}

// APIError is a failed /v1 call. Its message is safe to show the user
// (the API's own validation messages; never a secret).
type APIError struct {
	Status        int
	Code, Message string
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

// API calls the panel's /v1 routes as the user.
type API interface {
	Call(ctx context.Context, method, path string, body any) (Response, error)
}

// Bridge runs /v1 in process with the user's identity (apibridge): the
// route checks permissions, entitlements and validation exactly as for
// the panel, and writes its own audit record.
type Bridge struct {
	API  *apibridge.Bridge
	User uuid.UUID
	Org  uuid.UUID
}

// Call implements API.
func (b Bridge) Call(ctx context.Context, method, path string, body any) (Response, error) {
	if !strings.HasPrefix(path, "/v1/tenant/") {
		return Response{}, fmt.Errorf("capability: path outside /v1/tenant: %s", path)
	}
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			return Response{}, err
		}
	}
	out, err := b.API.Do(ctx, apibridge.Principal{UserUUID: b.User, OrganizationUUID: b.Org}, method, path, raw)
	if err != nil {
		return Response{}, err
	}
	return ReadResponse(out.Status, out.Body, out.Truncated), nil
}

// ReadResponse converts a /v1 envelope.
func ReadResponse(status int, body []byte, truncated bool) Response {
	if truncated {
		return Response{Status: status, Code: "RESULT_TOO_LARGE", Message: "the result is too large"}
	}
	if status == http.StatusNoContent {
		return Response{Status: status, OK: true, Data: json.RawMessage(`{}`)}
	}
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details []struct {
				Field   string `json:"field"`
				Message string `json:"message"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return Response{Status: status, Code: "UNREADABLE", Message: http.StatusText(status)}
	}
	if status < 300 && env.Success {
		if len(env.Data) == 0 || string(env.Data) == "null" {
			env.Data = json.RawMessage(`{}`)
		}
		return Response{Status: status, OK: true, Data: env.Data}
	}
	out := Response{Status: status, Code: "ERROR", Message: http.StatusText(status)}
	if status == http.StatusForbidden || status == http.StatusUnauthorized {
		out.Code = "FORBIDDEN"
	}
	if env.Error != nil {
		if env.Error.Code != "" {
			out.Code = env.Error.Code
		}
		if env.Error.Message != "" {
			out.Message = env.Error.Message
		}
		parts := []string{}
		for _, d := range env.Error.Details {
			if d.Field != "" {
				parts = append(parts, d.Field+": "+d.Message)
			} else if d.Message != "" {
				parts = append(parts, d.Message)
			}
		}
		if len(parts) > 0 {
			out.Message += " (" + strings.Join(parts, "; ") + ")"
		}
	}
	return out
}

// ReadOnly wraps an API so only GET goes through: dry runs and read
// capabilities can never change data or trigger a provider call. The
// optional simulations ("POST /v1/tenant/workflows/{uuid}/test") are
// dry-run endpoints that change nothing and may pass too.
func ReadOnly(api API, simulations ...string) API {
	r := readOnly{next: api}
	for _, s := range simulations {
		r.allow = append(r.allow, simulationPattern(s))
	}
	return r
}

func simulationPattern(s string) *regexp.Regexp {
	method, path, _ := strings.Cut(s, " ")
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") {
			parts[i] = `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`
		} else {
			parts[i] = regexp.QuoteMeta(p)
		}
	}
	return regexp.MustCompile("^" + regexp.QuoteMeta(method) + " " + strings.Join(parts, "/") + "$")
}

type readOnly struct {
	next  API
	allow []*regexp.Regexp
}

func (r readOnly) Call(ctx context.Context, method, path string, body any) (Response, error) {
	if method != http.MethodGet && !r.allowed(method, path) {
		return Response{}, ErrReadOnly
	}
	if r.next == nil {
		return Response{}, fmt.Errorf("capability: no API")
	}
	return r.next.Call(ctx, method, path, body)
}

func (r readOnly) allowed(method, path string) bool {
	for _, a := range r.allow {
		if a.MatchString(method + " " + path) {
			return true
		}
	}
	return false
}
