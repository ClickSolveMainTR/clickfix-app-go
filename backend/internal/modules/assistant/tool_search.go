package assistant

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
)

// Meta tools of intent-based loading (CF-211). Only part of the registry
// is loaded as tools in a turn; find_tools lists the other capabilities
// the user may use (name, kind, one line, params) and use_tool runs one of
// them exactly like a loaded tool: reads run, writes become cards.
const (
	ToolFindTools = "find_tools"
	ToolUseTool   = "use_tool"
	// maxFound bounds one find_tools answer.
	maxFound = 8
)

func metaTools() []ai.Tool {
	return []ai.Tool{
		{
			Name:        ToolFindTools,
			Description: "Find more tools by topic when none of the loaded tools fits (e.g. \"invoices\", \"randevu\", \"kişi sil\"). Returns names and params for use_tool.",
			Schema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"query"},
				"properties": map[string]any{"query": map[string]any{"type": "string", "maxLength": 200}}},
		},
		{
			Name:        ToolUseTool,
			Description: "Run a tool returned by find_tools. Reads return data; PROPOSES tools only create a confirmation card (give summary).",
			Schema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name", "params"},
				"properties": map[string]any{
					"name":    map[string]any{"type": "string", "maxLength": 64},
					"params":  map[string]any{"type": "object"},
					"summary": map[string]any{"type": "string", "maxLength": 500},
				}},
		},
	}
}

// moduleIndex is the stable list of tool groups for the instructions: the
// same text on every page, so it stays in the cached prefix.
func moduleIndex() string {
	mods := make([]string, 0, len(capability.ModuleKeywords))
	for m := range capability.ModuleKeywords {
		mods = append(mods, m)
	}
	sort.Strings(mods)
	return strings.Join(mods, ", ")
}

func (r *toolRun) findTools(input json.RawMessage) ai.ToolResult {
	var in struct {
		Query string `json:"query"`
	}
	if json.Unmarshal(input, &in) != nil || strings.TrimSpace(in.Query) == "" || utf8.RuneCountInString(in.Query) > 200 {
		return toolError("query is required")
	}
	type found struct {
		Name   string `json:"name"`
		Kind   string `json:"kind"`
		Desc   string `json:"desc"`
		Params string `json:"params,omitempty"`
	}
	out := []found{}
	for _, cp := range capability.Search(r.other, in.Query, maxFound) {
		out = append(out, found{Name: cp.ToolName(), Kind: string(cp.Kind), Desc: cp.Description, Params: cp.Signature()})
	}
	if len(out) == 0 {
		return toolJSON(map[string]any{"tools": out, "note": "Nothing else matches for this user; answer with the loaded tools, search_docs or navigation_open_route. * = required."})
	}
	return toolJSON(map[string]any{"tools": out, "note": "Call use_tool with name and params; * = required."})
}

func (r *toolRun) useTool(ctx context.Context, input json.RawMessage) ai.ToolResult {
	var in struct {
		Name    string          `json:"name"`
		Params  json.RawMessage `json:"params"`
		Summary string          `json:"summary"`
	}
	if json.Unmarshal(input, &in) != nil || in.Name == "" {
		return toolError("name is required")
	}
	cp, ok := r.reachable[in.Name]
	if !ok {
		if c, found := r.svc.Caps.Get(in.Name); found {
			cp, ok = r.reachable[c.ToolName()]
		}
	}
	if !ok {
		return toolError("unknown tool for this user: call find_tools first")
	}
	params := in.Params
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	if cp.Writes() {
		raw, _ := json.Marshal(map[string]any{"summary": in.Summary, "params": params})
		return r.runCapability(ctx, cp, raw)
	}
	return r.runCapability(ctx, cp, params)
}

// ActionToolCalled is the activity action of a read tool the assistant ran
// for the user (writes are recorded on confirm as assistant.action_*).
const ActionToolCalled = "assistant.tool_called"

// recordRead writes one read tool call to the organization's audit log
// (fail-soft): the capability, its param NAMES (values may hold personal
// data) and the outcome.
func (r *toolRun) recordRead(ctx context.Context, cp *capability.Capability, p capability.Params, start time.Time, failed bool) {
	if r.svc.Activity == nil {
		return
	}
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	outcome := "ok"
	if failed {
		outcome = "error"
	}
	actor := r.caller.UserID
	thread := r.thread
	ctx = orgctx.WithOrganizationID(context.WithoutCancel(ctx), r.caller.Scope.ID)
	r.svc.Activity.Record(ctx, &actor, ActionToolCalled, "assistant_thread", &thread, map[string]any{
		"capability": cp.Name, "surface": r.surface, "params": keys, "result": outcome,
		"duration_ms": time.Since(start).Milliseconds(),
	}, nil)
}
