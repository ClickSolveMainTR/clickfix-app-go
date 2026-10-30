// Package mcp is the MCP Streamable HTTP server of the three layers
// (docs/design/mcp.md): /mcp/platform for platform staff, /mcp/business for
// a member in one business and /mcp/customer for a customer panel person.
// It runs the official Go SDK in stateless mode.
//
// Every HTTP request is checked on its own:
//
//	Bearer token (mcpauth, audience = endpoint; 401 + WWW-Authenticate with
//	the RFC 9728 metadata link) → principal with what the subject holds now
//	(403 when the permission, membership or business was lost) → request
//	limits per connection and per business (429 + JSON-RPC error) →
//	tools/list = Catalog.Available(principal) → tools/call checks the same
//	again, validates the arguments and runs the /v1 route in process
//	(Bridge), so the panel's permission, branch, module and validation rules
//	and side effects apply unchanged.
//
// Every tools/call is written to the activity log with the tool, duration
// and outcome; personal data in the arguments is masked.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/model"
	mcpauthusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/usecase"
)

// Request limits.
const (
	// TokenRequestsPerMinute caps one connection (token family).
	TokenRequestsPerMinute = 60
	// BusinessRequestsPerHour caps every connection of one business.
	BusinessRequestsPerHour = 1000
	maxBodyBytes            = 1 << 20
)

// JSON-RPC error codes of the HTTP gate (implementation-defined server
// error range).
const (
	CodeUnauthorized = -32001
	CodeForbidden    = -32003
	CodeRateLimited  = -32029
	CodeInternal     = -32603
)

// TokenValidator resolves Bearer tokens (*mcpauth usecase.Service).
type TokenValidator interface {
	ValidateAccessToken(ctx context.Context, raw, resource string) (model.AccessToken, error)
	ResourceMetadataURL(resource string) string
}

// PrincipalResolver builds the principal of a token (Resolver).
type PrincipalResolver interface {
	Resolve(ctx context.Context, tok model.AccessToken) (Principal, error)
}

// Limiter is a fixed window limiter (*ratelimit.Limiter).
type Limiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// ActivityRecorder records audit events (*activity.Recorder).
type ActivityRecorder interface {
	Record(ctx context.Context, actorID *int64, action, resource string, resourceUUID *uuid.UUID, payload map[string]any, req *http.Request)
}

// Config wires the server. Limiter and Activity may be nil (tests).
type Config struct {
	Tokens     TokenValidator
	Principals PrincipalResolver
	Catalog    *Catalog
	Bridge     *Bridge
	Limiter    Limiter
	Activity   ActivityRecorder
	Version    string
	Log        *slog.Logger
}

// Server serves the three MCP endpoints.
type Server struct {
	cfg Config
	sdk *mcpsdk.StreamableHTTPHandler
	now func() time.Time
	// sdkLog keeps the SDK's per-request session lines out of the log.
	sdkLog *slog.Logger
}

// warnOnly passes warnings and errors of the SDK through.
type warnOnly struct{ slog.Handler }

func (h warnOnly) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= slog.LevelWarn && h.Handler.Enabled(ctx, l)
}

func (h warnOnly) WithAttrs(a []slog.Attr) slog.Handler { return warnOnly{h.Handler.WithAttrs(a)} }

func (h warnOnly) WithGroup(n string) slog.Handler { return warnOnly{h.Handler.WithGroup(n)} }

// New builds the server.
func New(cfg Config) *Server {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.Version == "" {
		cfg.Version = "1"
	}
	s := &Server{cfg: cfg, now: time.Now, sdkLog: slog.New(warnOnly{cfg.Log.Handler()})}
	s.sdk = mcpsdk.NewStreamableHTTPHandler(func(r *http.Request) *mcpsdk.Server {
		sess, ok := r.Context().Value(sessionKey{}).(*session)
		if !ok {
			return nil
		}
		return s.build(sess)
	}, &mcpsdk.StreamableHTTPOptions{
		// Every request carries its own Bearer token and gets a fresh tool
		// list; there is no server side session to resume.
		Stateless:    true,
		JSONResponse: true,
		Logger:       s.sdkLog,
		// The frontend proxies /mcp/* (only it is public), so the Host
		// header is the public one while the connection may be local;
		// Bearer auth (no cookies) makes DNS rebinding moot.
		DisableLocalhostProtection: true,
		MaxRequestBodyBytes:        maxBodyBytes,
	})
	return s
}

// SetClock replaces the clock (tests).
func (s *Server) SetClock(now func() time.Time) { s.now = now }

type sessionKey struct{}

// session is one authenticated HTTP request.
type session struct {
	resource  string
	token     model.AccessToken
	principal Principal
	tools     []*Tool
	req       *http.Request
}

// ServeHTTP implements http.Handler for the endpoint at r.URL.Path.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	resource := r.URL.Path
	if !model.IsResource(resource) {
		http.NotFound(w, r)
		return
	}
	cors(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ctx := r.Context()
	raw := bearer(r.Header.Get("Authorization"))
	if raw == "" {
		s.unauthorized(w, r, resource, "")
		return
	}
	tok, err := s.cfg.Tokens.ValidateAccessToken(ctx, raw, resource)
	switch {
	case errors.Is(err, mcpauthusecase.ErrUnauthorized):
		s.unauthorized(w, r, resource, "the access token is invalid, expired or not issued for this endpoint")
		return
	case err != nil:
		s.internal(w, r, "token", err)
		return
	}
	p, err := s.cfg.Principals.Resolve(ctx, tok)
	if errors.Is(err, ErrForbidden) {
		s.reject(w, r, http.StatusForbidden, CodeForbidden, "this connection is no longer allowed to use this endpoint", 0)
		return
	}
	if err != nil {
		s.internal(w, r, "principal", err)
		return
	}
	if r.Method == http.MethodPost {
		if retry, limited := s.limited(ctx, tok); limited {
			s.reject(w, r, http.StatusTooManyRequests, CodeRateLimited, "MCP request limit exceeded; retry later", retry)
			return
		}
	}
	sess := &session{resource: resource, token: tok, principal: p, tools: s.cfg.Catalog.Available(p), req: r}
	s.sdk.ServeHTTP(w, r.WithContext(context.WithValue(ctx, sessionKey{}, sess)))
}

// limited applies the connection limit first, then the business limit, so
// a connection over its own limit does not use up the business's.
func (s *Server) limited(ctx context.Context, tok model.AccessToken) (time.Duration, bool) {
	if s.cfg.Limiter == nil {
		return 0, false
	}
	if ok, retry := s.cfg.Limiter.Allow(ctx, "mcp_token", tok.Family.String(), TokenRequestsPerMinute, time.Minute); !ok {
		return retry, true
	}
	if tok.Subject.OrganizationID > 0 {
		org := strconv.FormatInt(tok.Subject.OrganizationID, 10)
		if ok, retry := s.cfg.Limiter.Allow(ctx, "mcp_org", org, BusinessRequestsPerHour, time.Hour); !ok {
			return retry, true
		}
	}
	return 0, false
}

func cors(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Mcp-Session-Id, MCP-Protocol-Version, Last-Event-ID")
	h.Set("Access-Control-Expose-Headers", "WWW-Authenticate, Mcp-Session-Id")
	h.Set("Access-Control-Max-Age", "86400")
}

// unauthorized answers 401 with the RFC 9728 challenge (RFC 6750 §3).
func (s *Server) unauthorized(w http.ResponseWriter, r *http.Request, resource, desc string) {
	params := []string{}
	if desc != "" {
		params = append(params, `error="invalid_token"`, fmt.Sprintf("error_description=%q", desc))
	}
	params = append(params, fmt.Sprintf("resource_metadata=%q", s.cfg.Tokens.ResourceMetadataURL(resource)))
	w.Header().Set("WWW-Authenticate", "Bearer "+strings.Join(params, ", "))
	msg := "a Bearer access token is required"
	if desc != "" {
		msg = desc
	}
	s.reject(w, r, http.StatusUnauthorized, CodeUnauthorized, msg, 0)
}

func (s *Server) internal(w http.ResponseWriter, r *http.Request, step string, err error) {
	s.cfg.Log.ErrorContext(r.Context(), "mcp_request_failed", "step", step, "error", err)
	s.reject(w, r, http.StatusInternalServerError, CodeInternal, "internal error", 0)
}

// reject writes an HTTP error whose body is a JSON-RPC error response for
// the request id (null when the body is not a single request).
func (s *Server) reject(w http.ResponseWriter, r *http.Request, status, code int, msg string, retry time.Duration) {
	id := json.RawMessage("null")
	if r.Method == http.MethodPost && r.Body != nil {
		raw, _ := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		if json.Unmarshal(raw, &req) == nil && len(req.ID) > 0 {
			id = req.ID
		}
	}
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int((retry+time.Second-1)/time.Second)))
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": code, "message": msg},
	})
}

func bearer(header string) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}
