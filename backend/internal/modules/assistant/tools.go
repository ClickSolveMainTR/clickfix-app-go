package assistant

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
)

// Tool names.
const (
	ToolSearchDocs     = "search_docs"
	ToolSetupState     = "get_setup_state"
	ToolProposeAction  = "propose_action"
	ToolEscalate       = "escalate_to_support"
	maxToolResultBytes = 3800
)

func (s *Service) tools() []ai.Tool {
	str := map[string]any{"type": "string"}
	kinds := sortedKeys(s.Actions)
	tools := []ai.Tool{
		{
			Name:        ToolSearchDocs,
			Description: "Search ClickFix's module guides and help center articles. Use for how-to questions.",
			Schema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"query"},
				"properties": map[string]any{"query": str}},
		},
		{
			Name:        ToolSetupState,
			Description: "Read what this organization has configured (channels, members, business hours, SLA, AI, widget, help center, mobile app) and the missing set-up steps.",
			Schema:      map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}},
		},
		{
			Name: ToolProposeAction,
			Description: "Propose ONE change for the user to confirm. It is shown as a confirmation card and runs only after the user " +
				"confirms (with their own permissions). params must follow the action's description.",
			Schema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"kind", "title", "summary", "params"},
				"properties": map[string]any{
					"kind": map[string]any{"type": "string", "enum": kinds}, "title": str, "summary": str,
					"params": map[string]any{"type": "object"},
				}},
		},
	}
	if _, ok := s.Actions[KindEscalate]; ok {
		tools = append(tools, ai.Tool{
			Name:        ToolEscalate,
			Description: "Offer to hand this conversation to ClickFix's human support team (shown as a confirmation card).",
			Schema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"note"},
				"properties": map[string]any{"note": str}},
		})
	}
	return tools
}

// toolRun executes the tools of one Send. Nothing here changes data:
// proposals are collected and only run on the confirm endpoint.
type toolRun struct {
	svc         *Service
	caller      Caller
	settings    Settings
	locale      string
	actions     []ProposedAction
	suggestions []string
	surface     string
	thread      uuid.UUID
	threadID    int64
	// caps are the capabilities offered in this reply, by tool name.
	caps map[string]*capability.Capability
}

func toolJSON(v any) ai.ToolResult {
	raw, _ := json.Marshal(v)
	return ai.ToolResult{Content: string(raw)}
}

func toolError(msg string) ai.ToolResult {
	return ai.ToolResult{Content: `{"error":` + quoteJSON(msg) + `}`, IsError: true}
}

func quoteJSON(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

func (r *toolRun) run(ctx context.Context, call ai.ToolCall) (ai.ToolResult, error) {
	switch call.Name {
	case ToolSearchDocs:
		var in struct {
			Query string `json:"query"`
		}
		if json.Unmarshal(call.Input, &in) != nil || strings.TrimSpace(in.Query) == "" || utf8.RuneCountInString(in.Query) > 200 {
			return toolError("query is required"), nil
		}
		return r.searchDocs(ctx, in.Query), nil
	case ToolSetupState:
		st, err := r.svc.setupState(ctx, r.caller.Scope.ID)
		if err != nil {
			return toolError("setup state unavailable"), nil
		}
		return toolJSON(st), nil
	case ToolProposeAction:
		var in ProposedAction
		if json.Unmarshal(call.Input, &in) != nil {
			return toolError("invalid proposal"), nil
		}
		return r.propose(in), nil
	case ToolEscalate:
		var in struct {
			Note string `json:"note"`
		}
		_ = json.Unmarshal(call.Input, &in)
		if len(r.actions) >= maxActions {
			return toolError("too many proposals in one reply"), nil
		}
		a := r.svc.escalateAction(r.locale, clipRunes(strings.TrimSpace(in.Note), 2000))
		r.actions = append(r.actions, a)
		return toolJSON(map[string]any{"status": "proposed", "id": a.ID, "note": "Shown to the user as a confirmation card."}), nil
	}
	if call.Name == ToolReadDocument {
		return r.readDocument(ctx, call.Input), nil
	}
	if cp, ok := r.caps[call.Name]; ok {
		return r.runCapability(ctx, cp, call.Input), nil
	}
	return toolError("unknown tool"), nil
}

func (r *toolRun) propose(in ProposedAction) ai.ToolResult {
	if len(r.actions) >= maxActions {
		return toolError("too many proposals in one reply")
	}
	ex, ok := r.svc.Actions[in.Kind]
	if !ok {
		return toolError("unknown action kind")
	}
	in.Title = clipRunes(strings.TrimSpace(in.Title), 120)
	in.Summary = clipRunes(strings.TrimSpace(in.Summary), 500)
	if in.Title == "" {
		return toolError("title is required")
	}
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	raw, _ := json.Marshal(in.Params)
	if len(raw) > 2048 {
		return toolError("params too large")
	}
	// Placeholders of the user's message (masked e-mails) are restored
	// after the loop; check the shape with them replaced by a sample.
	check := map[string]any{}
	_ = json.Unmarshal([]byte(placeholderSample(string(raw))), &check)
	if err := ex.Validate(check); err != nil {
		return toolError("params do not fit this action: " + ex.Describe)
	}
	a := ProposedAction{ID: newActionID(), Kind: in.Kind, Title: in.Title, Summary: in.Summary, Params: in.Params, Status: ActionProposed, Route: ex.Route}
	r.actions = append(r.actions, a)
	return toolJSON(map[string]any{"status": "proposed", "id": a.ID, "note": "Shown to the user as a confirmation card; wait for their decision."})
}

// placeholderSample swaps masked e-mail placeholders for a syntactically
// valid address so Validate can check the proposal before unmasking.
func placeholderSample(raw string) string {
	out := raw
	for {
		i := strings.Index(out, "[PII_EMAIL_")
		if i < 0 {
			return out
		}
		j := strings.Index(out[i:], "]")
		if j < 0 {
			return out
		}
		out = out[:i] + "placeholder@example.com" + out[i+j+1:]
	}
}

func (r *toolRun) searchDocs(ctx context.Context, query string) ai.ToolResult {
	result := map[string]any{}
	if hits, err := r.svc.Guides.Search(ctx, query, r.locale); err == nil {
		result["guides"] = hits
	}
	if r.svc.Help != nil && r.settings.SupportOrgUUID != nil {
		if articles, err := r.svc.Help.SearchHelp(ctx, *r.settings.SupportOrgUUID, query, r.locale); err == nil {
			result["help_articles"] = articles
		}
	}
	// The tool loop rejects results over 4 KiB: drop items from the end.
	for {
		raw, _ := json.Marshal(result)
		if len(raw) <= maxToolResultBytes {
			return ai.ToolResult{Content: string(raw)}
		}
		trimmed := false
		for _, key := range []string{"help_articles", "guides"} {
			if !trimmed {
				trimmed = trimLast(result, key)
			}
		}
		if !trimmed {
			return toolJSON(map[string]any{"guides": []any{}})
		}
	}
}

func trimLast(m map[string]any, key string) bool {
	switch v := m[key].(type) {
	case []map[string]any:
		if len(v) > 0 {
			m[key] = v[:len(v)-1]
			return true
		}
	case []any:
		if len(v) > 0 {
			m[key] = v[:len(v)-1]
			return true
		}
	default:
		raw, _ := json.Marshal(v)
		var items []any
		if json.Unmarshal(raw, &items) == nil && len(items) > 0 {
			m[key] = items[:len(items)-1]
			return true
		}
	}
	return false
}
