package mcp

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/model"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
)

// ActionToolCalled is the activity action of every tools/call.
const ActionToolCalled = "mcp.tool_called"

// record writes one tools/call to the activity log (fail-soft): business
// calls in the business's log, the others in the platform log.
func (s *Server) record(ctx context.Context, sess *session, name string, args json.RawMessage, start time.Time, outcome, code string) {
	if s.cfg.Activity == nil {
		return
	}
	p := sess.principal
	payload := map[string]any{
		"tool":        name,
		"layer":       p.Layer,
		"client_id":   sess.token.ClientID,
		"grant_uuid":  sess.token.GrantUUID.String(),
		"duration_ms": s.now().Sub(start).Milliseconds(),
		"result":      outcome,
		"arguments":   MaskArguments(args),
	}
	if code != "" {
		payload["code"] = code
	}
	ctx = context.WithoutCancel(ctx)
	var actor *int64
	switch p.Layer {
	case model.LayerPlatform, model.LayerBusiness:
		id := p.UserID
		actor = &id
		if p.Scope != nil {
			ctx = orgctx.WithOrganizationID(ctx, p.Scope.ID)
		}

	}
	grant := sess.token.GrantUUID
	s.cfg.Activity.Record(ctx, actor, ActionToolCalled, "mcp_oauth_grants", &grant, payload, sess.req)
}

// Masked replaces a personal value in logged arguments.
const Masked = "***"

// personalKeys are argument names (or name parts) whose values are
// personal data or free text that may hold it.
var personalKeys = []string{
	"name", "phone", "email", "mail", "birth", "note", "reason", "tag", "address", "message", "text",
	"comment", "description", "content", "tckn", "identity_number", "iban", "query", "search",
}

var (
	emailRe = regexp.MustCompile(`[^\s@]+@[^\s@]+\.[^\s@]+`)
	phoneRe = regexp.MustCompile(`\+?[0-9][0-9 ()\-]{8,}[0-9]`)
)

// MaskArguments returns the tool arguments for the activity log: values
// under personal keys (and q) are masked, e-mail addresses and phone
// numbers anywhere else too, long strings are cut. Invalid JSON is not
// logged.
func MaskArguments(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return Masked
	}
	return maskValue("", v)
}

func maskValue(key string, v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = maskValue(k, val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = maskValue(key, val)
		}
		return out
	case nil, bool:
		return t
	}
	if personal(key) {
		return Masked
	}
	if s, ok := v.(string); ok {
		if _, err := uuid.Parse(s); err == nil {
			return s
		}
		if emailRe.MatchString(s) || hasPhone(s) {
			return Masked
		}
		if r := []rune(s); len(r) > 64 {
			return string(r[:64]) + "…"
		}
	}
	return v
}

// hasPhone reports a phone-like run of at least 10 digits (dates and
// short numbers stay readable).
func hasPhone(s string) bool {
	for _, m := range phoneRe.FindAllString(s, -1) {
		digits := 0
		for _, r := range m {
			if r >= '0' && r <= '9' {
				digits++
			}
		}
		if digits >= 10 {
			return true
		}
	}
	return false
}

func personal(key string) bool {
	k := strings.ToLower(key)
	if k == "q" {
		return true
	}
	for _, p := range personalKeys {
		if strings.Contains(k, p) {
			return true
		}
	}
	return false
}
