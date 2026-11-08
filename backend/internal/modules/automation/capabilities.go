package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/usecase"
	convmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Assistant capabilities of workflows (CF-186, scenario 2 of
// docs/design/asistan-yetenekleri.md): from "when a new message arrives
// and X, do Y" the assistant drafts a linear workflow (trigger, optional
// condition, actions), the server builds and validates the graph, the
// user confirms, it is saved (and published) and simulated on the latest
// conversation. Every call goes through /v1/tenant/workflows as the user.

const workflowsPath = "/v1/tenant/workflows"

// DraftActions are the action types a draft may use, with what they need.
var DraftActions = map[string]string{
	"set_status":        "value: open|pending|resolved|closed",
	"set_priority":      "value: low|normal|high|urgent",
	"add_note":          "content: internal note text",
	"send_canned_reply": "content: reply text sent to the customer",
	"add_tag":           "target_uuid: tag uuid (workflows_reference.tags)",
	"remove_tag":        "target_uuid: tag uuid (workflows_reference.tags)",
	"assign_user":       "target_uuid: member user uuid (workflows_reference.members)",
	"notify":            "target_uuid: member user uuid; content: notification text",
}

func draftActionTypes() []string {
	out := make([]string, 0, len(DraftActions))
	for k := range DraftActions {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func str(p capability.Params, k string) string {
	s, _ := p[k].(string)
	return strings.TrimSpace(s)
}

func t(e capability.Exec, tr, en string, a ...any) string {
	if e.En() {
		return fmt.Sprintf(en, a...)
	}
	return fmt.Sprintf(tr, a...)
}

// draft is the linear form the model writes.
type draft struct {
	Name            string           `json:"name"`
	Trigger         string           `json:"trigger"`
	ScheduleMinutes int32            `json:"schedule_minutes"`
	Condition       *model.Condition `json:"condition"`
	Actions         []model.Action   `json:"actions"`
	Enable          bool             `json:"enable"`
}

func readDraft(p capability.Params) (draft, error) {
	var d draft
	raw, _ := json.Marshal(p)
	if err := json.Unmarshal(raw, &d); err != nil {
		return d, capability.Invalid("the draft does not fit: %s", err.Error())
	}
	d.Name = strings.TrimSpace(d.Name)
	return d, nil
}

// BuildGraph turns a draft into the editor graph: trigger → [condition]
// → action 1 … n → end; a false condition goes to the end.
func BuildGraph(d draft) model.Graph {
	g := model.Graph{SchemaVersion: 1}
	pos := func(i int) map[string]float64 { return map[string]float64{"x": float64(80 + 260*i), "y": 120} }
	g.Nodes = append(g.Nodes, model.Node{ID: "trigger", Type: "trigger", Trigger: d.Trigger, ScheduleMinutes: d.ScheduleMinutes, Position: pos(0)})
	prev, port := "trigger", "next"
	col := 1
	if d.Condition != nil {
		g.Nodes = append(g.Nodes, model.Node{ID: "condition", Type: "condition", Condition: d.Condition, Position: pos(col)})
		g.Edges = append(g.Edges, model.Edge{From: prev, Port: port, To: "condition", Input: "in"})
		prev, port = "condition", "true"
		col++
	}
	for i := range d.Actions {
		a := d.Actions[i]
		id := fmt.Sprintf("action_%d", i+1)
		g.Nodes = append(g.Nodes, model.Node{ID: id, Type: "action", Action: &a, Position: pos(col)})
		g.Edges = append(g.Edges, model.Edge{From: prev, Port: port, To: id, Input: "in"})
		prev, port = id, "next"
		col++
	}
	g.Nodes = append(g.Nodes, model.Node{ID: "end", Type: "end", Position: pos(col)})
	g.Edges = append(g.Edges, model.Edge{From: prev, Port: port, To: "end", Input: "in"})
	if d.Condition != nil {
		g.Edges = append(g.Edges, model.Edge{From: "condition", Port: "false", To: "end", Input: "in"})
	}
	return g
}

func validateDraft(d draft) error {
	if d.Name == "" || len(d.Name) > 200 {
		return capability.Invalid("name is required (max 200 characters): ask the user")
	}
	if len(d.Actions) == 0 {
		return capability.Invalid("at least one action is required: ask the user what should happen")
	}
	for _, a := range d.Actions {
		if _, ok := DraftActions[a.Type]; !ok {
			return capability.Invalid("unsupported action %q; use one of %s", a.Type, strings.Join(draftActionTypes(), ", "))
		}
		if a.Type == "notify" && strings.TrimSpace(a.Content) == "" {
			return capability.Invalid("notify needs content")
		}
	}
	if err := usecase.ValidateGraph(BuildGraph(d)); err != nil {
		msg := strings.TrimPrefix(err.Error(), usecase.ErrInvalid.Error())
		return capability.Invalid("the workflow is not valid%s; check the trigger, the condition fields/operators and each action's required value (workflows_reference)", msg)
	}
	return nil
}

// describe renders a draft for the card.
func describe(e capability.Exec, d draft) []capability.Detail {
	trigger := d.Trigger
	if d.Trigger == "scheduled" {
		trigger = t(e, "zamanlanmış (her %d dakikada)", "scheduled (every %d minutes)", d.ScheduleMinutes)
	}
	out := []capability.Detail{{Label: t(e, "Tetikleyici", "Trigger"), Value: trigger}}
	if d.Condition != nil {
		out = append(out, capability.Detail{Label: t(e, "Koşul", "Condition"), Value: conditionText(*d.Condition)})
	}
	for i, a := range d.Actions {
		v := a.Type
		switch {
		case a.Value != "":
			v += ": " + a.Value
		case a.Content != "":
			v += ": " + clip(a.Content, 120)
		case a.TargetUUID != uuid.Nil:
			v += ": " + a.TargetUUID.String()
		}
		out = append(out, capability.Detail{Label: t(e, "Eylem %d", "Action %d", i+1), Value: v})
	}
	return out
}

func conditionText(c model.Condition) string {
	join := func(list []model.Condition, sep string) string {
		parts := []string{}
		for _, x := range list {
			parts = append(parts, conditionText(x))
		}
		return "(" + strings.Join(parts, sep) + ")"
	}
	switch {
	case c.All != nil:
		return join(c.All, " AND ")
	case c.Any != nil:
		return join(c.Any, " OR ")
	}
	if c.Value == nil {
		return c.Field + " " + c.Operator
	}
	raw, _ := json.Marshal(c.Value)
	return c.Field + " " + c.Operator + " " + string(raw)
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

type workflowView struct {
	UUID             uuid.UUID `json:"uuid"`
	Name             string    `json:"name"`
	Revision         int32     `json:"revision"`
	PublishedVersion *int32    `json:"published_version"`
	Enabled          bool      `json:"enabled"`
}

func getWorkflow(ctx context.Context, api capability.API, id string) (workflowView, error) {
	var w workflowView
	if _, err := uuid.Parse(id); err != nil {
		return w, capability.Invalid("workflow_uuid must be a UUID")
	}
	r, err := api.Call(ctx, http.MethodGet, workflowsPath+"/"+id, nil)
	if err != nil {
		return w, err
	}
	return w, r.Decode(&w)
}

// latestConversation is the conversation a saved workflow is simulated on.
func latestConversation(ctx context.Context, api capability.API) string {
	r, err := api.Call(ctx, http.MethodGet, "/v1/tenant/conversations?limit=1", nil)
	if err != nil {
		return ""
	}
	var page struct {
		Items []struct {
			UUID string `json:"uuid"`
		} `json:"items"`
	}
	if r.Decode(&page) != nil || len(page.Items) == 0 {
		return ""
	}
	return page.Items[0].UUID
}

type traceStep struct {
	NodeID string `json:"node_id"`
	Type   string `json:"type"`
	Port   string `json:"port"`
	Status string `json:"status"`
}

// simulate runs the saved workflow's dry run on a conversation.
func simulate(ctx context.Context, e capability.Exec, id, conversation string) ([]traceStep, string, error) {
	if conversation == "" {
		conversation = latestConversation(ctx, e.API)
	}
	if conversation == "" {
		return nil, "", errNoConversation
	}
	r, err := e.API.Call(ctx, http.MethodPost, workflowsPath+"/"+id+"/test", map[string]any{"conversation_uuid": conversation})
	if err != nil {
		return nil, conversation, err
	}
	var out struct {
		Trace []traceStep `json:"trace"`
	}
	return out.Trace, conversation, r.Decode(&out)
}

var errNoConversation = errors.New("no conversation to simulate on yet")

func traceSummary(e capability.Exec, trace []traceStep) string {
	would := 0
	stopped := false
	for _, s := range trace {
		if s.Status == "would_execute" {
			would++
		}
		if s.Type == "condition" && s.Port == "false" {
			stopped = true
		}
	}
	if stopped {
		return t(e, "Koşul bu konuşmada sağlanmadı; eylem çalışmazdı.", "The condition is not met on this conversation; no action would run.")
	}
	return t(e, "%d eylem çalışırdı.", "%d actions would run.", would)
}

var conditionDoc = "Condition tree: {\"all\":[...]} | {\"any\":[...]} | {\"field\",\"operator\",\"value\"}. " +
	"Fields and operators: workflows_reference. Omit for no condition."

// Capabilities are the workflow capabilities.
func Capabilities() []*capability.Capability {
	routes := []string{"/workflows", "/automation"}
	statuses := slices.DeleteFunc(slices.Clone(convmodel.Statuses), func(s string) bool { return s == convmodel.StatusSnoozed })
	actionItem := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"type"},
		"properties": map[string]any{
			"type":        map[string]any{"type": "string", "enum": draftActionTypes()},
			"value":       map[string]any{"type": "string", "maxLength": 200},
			"content":     map[string]any{"type": "string", "maxLength": 10000},
			"target_uuid": map[string]any{"type": "string", "pattern": "^[0-9a-fA-F-]{36}$"},
		}}
	return []*capability.Capability{
		{
			Name: "workflows.list", Module: "automation", Kind: capability.Read, Permission: rbac.PermTenantAutomationRead, Routes: routes,
			Title:       map[string]string{"tr": "İş akışlarını listele", "en": "List workflows"},
			Description: "List the organization's workflows: uuid, name, revision, published version and whether it is enabled.",
			Run: func(ctx context.Context, e capability.Exec, _ capability.Params) (capability.Result, error) {
				r, err := e.API.Call(ctx, http.MethodGet, workflowsPath+"?limit=30", nil)
				if err != nil {
					return capability.Result{}, err
				}
				var page struct {
					Items []workflowView `json:"items"`
					Total int64          `json:"total"`
				}
				if err := r.Decode(&page); err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: fmt.Sprintf("%d workflows", page.Total), Data: page}, nil
			},
		},
		{
			Name: "workflows.reference", Module: "automation", Kind: capability.Read, Permission: rbac.PermTenantAutomationRead, Routes: routes,
			Title:       map[string]string{"tr": "İş akışı başvurusu", "en": "Workflow reference"},
			Description: "What a workflow draft may use: triggers, condition fields and operators, action types with their required value, conversation statuses and priorities, and the organization's tags and members (uuid + name) for target_uuid. Call it before drafting.",
			Run: func(ctx context.Context, e capability.Exec, _ capability.Params) (capability.Result, error) {
				data := map[string]any{
					"triggers": usecase.WorkflowTriggers(), "condition_fields": usecase.ConditionFields(),
					"operators": usecase.ConditionOperators(), "actions": DraftActions,
					"statuses": statuses, "priorities": convmodel.Priorities,
					"condition_shape": conditionDoc,
				}
				if r, err := e.API.Call(ctx, http.MethodGet, "/v1/tenant/tags", nil); err == nil {
					var page struct {
						Items []struct {
							UUID string `json:"uuid"`
							Name string `json:"name"`
						} `json:"items"`
					}
					if r.Decode(&page) == nil {
						data["tags"] = page.Items[:min(len(page.Items), 25)]
					}
				}
				if r, err := e.API.Call(ctx, http.MethodGet, "/v1/tenant/members?limit=25", nil); err == nil {
					var page struct {
						Items []struct {
							User struct {
								UUID    string `json:"uuid"`
								Name    string `json:"name"`
								Surname string `json:"surname"`
							} `json:"user"`
						} `json:"items"`
					}
					if r.Decode(&page) == nil {
						members := []map[string]string{}
						for _, m := range page.Items {
							members = append(members, map[string]string{"uuid": m.User.UUID, "name": strings.TrimSpace(m.User.Name + " " + m.User.Surname)})
						}
						data["members"] = members
					}
				}
				return capability.Result{Summary: "workflow reference", Data: data}, nil
			},
		},
		{
			Name: "workflows.create", Module: "automation", Kind: capability.Write, Permission: rbac.PermTenantAutomationWrite, Routes: routes,
			Title: map[string]string{"tr": "İş akışı oluştur", "en": "Create a workflow"},
			Description: "Draft a workflow from the user's sentence (\"when a new message arrives and X, do Y\"): trigger, optional condition, " +
				"ordered actions. The server builds and validates the graph. enable=true publishes and turns it on after saving. " +
				"Use workflows_reference for valid values; ask the user when a value (a tag, a person, a text) is unclear.",
			Undo: "/workflows",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name", "trigger", "actions"},
				"properties": map[string]any{
					"name":             map[string]any{"type": "string", "title": "Name", "minLength": 1, "maxLength": 200},
					"trigger":          map[string]any{"type": "string", "title": "Trigger", "enum": usecase.WorkflowTriggers()},
					"schedule_minutes": map[string]any{"type": "integer", "title": "Every (minutes)", "minimum": 1, "maximum": 525600, "description": "Only for the scheduled trigger"},
					"condition":        map[string]any{"type": "object", "title": "Condition", "description": conditionDoc},
					"actions":          map[string]any{"type": "array", "title": "Actions", "minItems": 1, "maxItems": 10, "items": actionItem},
					"enable":           map[string]any{"type": "boolean", "title": "Publish and enable"},
				}},
			Validate: func(p capability.Params) error {
				d, err := readDraft(p)
				if err != nil {
					return err
				}
				return validateDraft(d)
			},
			DryRun: func(_ context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				d, err := readDraft(p)
				if err != nil {
					return capability.Result{}, err
				}
				if err := validateDraft(d); err != nil {
					return capability.Result{}, err
				}
				g := BuildGraph(d)
				state := t(e, "taslak olarak kaydedilecek", "saved as a draft")
				if d.Enable {
					state = t(e, "kaydedilip yayınlanacak ve etkinleşecek", "saved, published and enabled")
				}
				return capability.Result{
					Summary: t(e, "“%s” geçerli (%d düğüm); %s. Kaydedince son konuşma üzerinde simüle edilecek.",
						"“%s” is valid (%d nodes); it will be %s. After saving it is simulated on the latest conversation.", d.Name, len(g.Nodes), state),
					Details: describe(e, d),
				}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				d, err := readDraft(p)
				if err != nil {
					return capability.Result{}, err
				}
				r, err := e.API.Call(ctx, http.MethodPost, workflowsPath, map[string]any{"name": d.Name, "graph": BuildGraph(d), "revision": 0})
				if err != nil {
					return capability.Result{}, err
				}
				var w workflowView
				if err := r.Decode(&w); err != nil {
					return capability.Result{}, err
				}
				if d.Enable {
					pr, err := e.API.Call(ctx, http.MethodPost, workflowsPath+"/"+w.UUID.String()+"/publish", map[string]any{"revision": w.Revision})
					if err != nil {
						return capability.Result{}, err
					}
					if err := pr.Err(); err != nil {
						return capability.Result{}, err
					}
				}
				route := "/workflows/" + w.UUID.String()
				return capability.Result{Summary: w.UUID.String(), Details: describe(e, d), Route: route, Focus: &capability.Focus{Route: route}}, nil
			},
			Verify: func(ctx context.Context, e capability.Exec, p capability.Params, res capability.Result) capability.Check {
				w, err := getWorkflow(ctx, e.API, res.Summary)
				if err != nil {
					return capability.Check{Status: capability.VerifyFailed}
				}
				if enable, _ := p["enable"].(bool); enable && (!w.Enabled || w.PublishedVersion == nil) {
					return capability.Check{Status: capability.VerifyFailed, Message: t(e, "İş akışı kaydedildi ama yayında değil.", "Saved but not published.")}
				}
				trace, _, err := simulate(ctx, e, res.Summary, "")
				if errors.Is(err, errNoConversation) {
					return capability.Check{Status: capability.VerifyOK, Message: t(e, "Kaydedildi; simülasyon için henüz konuşma yok.", "Saved; no conversation to simulate on yet.")}
				}
				if err != nil {
					return capability.Check{Status: capability.VerifyFailed, Message: t(e, "Kaydedildi ama simülasyon çalışmadı.", "Saved but the simulation failed.")}
				}
				return capability.Check{Status: capability.VerifyOK, Message: t(e, "Son konuşmada simülasyon: %s", "Simulation on the latest conversation: %s", traceSummary(e, trace))}
			},
		},
		{
			Name: "workflows.dry_run", Module: "automation", Kind: capability.Read, Permission: rbac.PermTenantAutomationWrite, Routes: routes,
			Title:       map[string]string{"tr": "İş akışını kuru çalıştır", "en": "Dry-run a workflow"},
			Description: "Simulate a saved workflow on a conversation (default: the latest) without changing anything: which branch is taken and which actions would run.",
			Simulations: []string{"POST " + workflowsPath + "/{uuid}/test"},
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"workflow_uuid"},
				"properties": map[string]any{
					"workflow_uuid":     map[string]any{"type": "string", "pattern": "^[0-9a-fA-F-]{36}$"},
					"conversation_uuid": map[string]any{"type": "string", "pattern": "^[0-9a-fA-F-]{36}$"},
				}},
			Run: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				if _, err := getWorkflow(ctx, e.API, str(p, "workflow_uuid")); err != nil {
					return capability.Result{}, err
				}
				trace, conv, err := simulate(ctx, e, str(p, "workflow_uuid"), str(p, "conversation_uuid"))
				if errors.Is(err, errNoConversation) {
					return capability.Result{Summary: "There is no conversation to simulate on yet."}, nil
				}
				if err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: traceSummary(e, trace), Data: map[string]any{"conversation_uuid": conv, "trace": trace}}, nil
			},
		},
		{
			Name: "workflows.enable", Module: "automation", Kind: capability.Write, Permission: rbac.PermTenantAutomationWrite, Routes: routes,
			Title:       map[string]string{"tr": "İş akışını aç/kapat", "en": "Turn a workflow on or off"},
			Description: "Turn a saved workflow on (publishing its latest revision if it was never published) or off.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"workflow_uuid", "enabled"},
				"properties": map[string]any{
					"workflow_uuid": map[string]any{"type": "string", "title": "Workflow", "pattern": "^[0-9a-fA-F-]{36}$"},
					"enabled":       map[string]any{"type": "boolean", "title": "Enabled"},
				}},
			DryRun: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				w, err := getWorkflow(ctx, e.API, str(p, "workflow_uuid"))
				if err != nil {
					return capability.Result{}, err
				}
				on, _ := p["enabled"].(bool)
				if on == w.Enabled && (!on || w.PublishedVersion != nil) {
					return capability.Result{}, capability.Invalid("%s", t(e, "“%s” zaten bu durumda.", "“%s” is already in this state.", w.Name))
				}
				if on && w.PublishedVersion == nil {
					return capability.Result{Summary: t(e, "“%s” revizyon %d yayınlanıp etkinleşecek.", "“%s” revision %d will be published and enabled.", w.Name, w.Revision)}, nil
				}
				return capability.Result{Summary: t(e, "“%s” %s.", "“%s” will be turned %s.", w.Name, map[bool]string{true: t(e, "etkinleşecek", "on"), false: t(e, "kapatılacak", "off")}[on])}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				id := str(p, "workflow_uuid")
				w, err := getWorkflow(ctx, e.API, id)
				if err != nil {
					return capability.Result{}, err
				}
				on, _ := p["enabled"].(bool)
				var r capability.Response
				if on && w.PublishedVersion == nil {
					r, err = e.API.Call(ctx, http.MethodPost, workflowsPath+"/"+id+"/publish", map[string]any{"revision": w.Revision})
				} else {
					r, err = e.API.Call(ctx, http.MethodPost, workflowsPath+"/"+id+"/enable", map[string]any{"enabled": on})
				}
				if err != nil {
					return capability.Result{}, err
				}
				if err := r.Err(); err != nil {
					return capability.Result{}, err
				}
				route := "/workflows/" + id
				return capability.Result{Summary: id, Route: route, Focus: &capability.Focus{Route: route}}, nil
			},
			Verify: func(ctx context.Context, e capability.Exec, p capability.Params, res capability.Result) capability.Check {
				w, err := getWorkflow(ctx, e.API, res.Summary)
				on, _ := p["enabled"].(bool)
				if err != nil || w.Enabled != on {
					return capability.Check{Status: capability.VerifyFailed}
				}
				return capability.Check{Status: capability.VerifyOK}
			},
		},
	}
}
