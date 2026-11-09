package sla

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Assistant capability and page state of SLA (CF-187, scenario 3): which
// policy applies to a conversation and where its clocks stand, and what
// is missing on the SLA settings page. Writes (policies, business hours)
// move here in CF-188.

func tr(e capability.Exec, trText, en string, a ...any) string {
	if e.En() {
		return fmt.Sprintf(en, a...)
	}
	return fmt.Sprintf(trText, a...)
}

func count(ctx context.Context, api capability.API, path string) (int, error) {
	r, err := api.Call(ctx, http.MethodGet, path, nil)
	if err != nil {
		return 0, err
	}
	var out struct {
		Items []json.RawMessage `json:"items"`
	}
	decErr := r.Decode(&out)
	return len(out.Items), decErr
}

// Capabilities are the SLA capabilities.
func Capabilities() []*capability.Capability {
	return []*capability.Capability{{
		Name: "sla.list", Surfaces: capability.ReadSurfaces, Module: "sla", Kind: capability.Read, Permission: rbac.PermTenantSlaRead,
		Routes:      []string{"/settings/sla"},
		Title:       map[string]string{"tr": "SLA ayarlarını listele", "en": "List SLA settings"},
		Description: "List the organization's business hours schedules and SLA policies with their uuids, targets and whether they are active or default.",
		Run: func(ctx context.Context, e capability.Exec, _ capability.Params) (capability.Result, error) {
			out := map[string]json.RawMessage{}
			for key, path := range map[string]string{"business_hours": "/v1/tenant/sla/business-hours", "policies": "/v1/tenant/sla/policies"} {
				r, err := e.API.Call(ctx, http.MethodGet, path, nil)
				if err != nil {
					return capability.Result{}, err
				}
				var page struct {
					Items json.RawMessage `json:"items"`
				}
				if err := r.Decode(&page); err != nil {
					return capability.Result{}, err
				}
				out[key] = page.Items
			}
			return capability.Result{Summary: "SLA settings", Data: out}, nil
		},
	}, {
		Name: "sla.check", Surfaces: capability.ReadSurfaces, Module: "sla", Kind: capability.Read, Permission: rbac.PermTenantSlaRead, Routes: []string{"/settings/sla"},
		Title:       map[string]string{"tr": "SLA durumunu kontrol et", "en": "Check SLA status"},
		Description: "Show which SLA policy applies to a conversation (default: the latest) and each clock's status (running, paused, met, breached) with the time left. Use it to test an SLA set-up.",
		Input: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
			"conversation_uuid": map[string]any{"type": "string", "pattern": "^[0-9a-fA-F-]{36}$"},
		}},
		Run: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
			id, _ := p["conversation_uuid"].(string)
			id = strings.TrimSpace(id)
			if id == "" {
				r, err := e.API.Call(ctx, http.MethodGet, "/v1/tenant/conversations?limit=1", nil)
				if err != nil {
					return capability.Result{}, err
				}
				var page struct {
					Items []struct {
						UUID string `json:"uuid"`
					} `json:"items"`
				}
				if r.Decode(&page) != nil || len(page.Items) == 0 {
					return capability.Result{Summary: "There is no conversation to check yet."}, nil
				}
				id = page.Items[0].UUID
			}
			if _, err := uuid.Parse(id); err != nil {
				return capability.Result{}, capability.Invalid("conversation_uuid must be a UUID")
			}
			r, err := e.API.Call(ctx, http.MethodGet, "/v1/tenant/conversations/"+id+"/sla", nil)
			if err != nil {
				return capability.Result{}, err
			}
			var out json.RawMessage
			if err := r.Decode(&out); err != nil {
				return capability.Result{}, err
			}
			return capability.Result{Summary: "SLA of " + id, Data: out}, nil
		},
	}}
}

// Pages is the SLA settings page state.
func Pages() []*capability.Page {
	return []*capability.Page{{
		Module: "sla", Routes: []string{"/settings/sla"}, Permission: rbac.PermTenantSlaRead,
		State: func(ctx context.Context, e capability.Exec) (capability.PageState, error) {
			st := capability.PageState{Module: "sla", Title: tr(e, "SLA ve çalışma saatleri", "SLA and business hours")}
			hours, err := count(ctx, e.API, "/v1/tenant/sla/business-hours")
			if err != nil {
				return st, err
			}
			policies, err := count(ctx, e.API, "/v1/tenant/sla/policies")
			if err != nil {
				return st, err
			}
			if hours == 0 {
				st.Missing = append(st.Missing, capability.Step{Key: "business_hours", Capability: "sla.business_hours.create",
					Title:  tr(e, "Çalışma saatleri tanımlı değil", "No business hours yet"),
					Detail: tr(e, "SLA süreleri çalışma saatlerine göre işler.", "SLA clocks run on business hours."),
					Prompt: tr(e, "Çalışma saatlerimi kurmama yardım et", "Help me set up my business hours")})
			}
			if policies == 0 {
				st.Missing = append(st.Missing, capability.Step{Key: "sla_policy", Capability: "sla.policies.create",
					Title:  tr(e, "SLA politikası yok", "No SLA policy yet"),
					Prompt: tr(e, "Önceliklere göre bir SLA politikası oluşturmama yardım et", "Help me create an SLA policy by priority")})
			} else {
				st.Tests = append(st.Tests, capability.Test{Capability: "sla.check",
					Title:  tr(e, "Son konuşmanın SLA durumunu kontrol et", "Check the SLA of the latest conversation"),
					Prompt: tr(e, "Son konuşmanın SLA durumunu kontrol et", "Check the SLA status of the latest conversation")})
			}
			return st, nil
		},
	}}
}

// SLA writes (CF-188): business hours and policies, through the SLA
// routes. The CF-141 actions set_business_hours and create_sla are
// aliases, so their stored cards still confirm.

const (
	hoursPath    = "/v1/tenant/sla/business-hours"
	policiesPath = "/v1/tenant/sla/policies"
)

var dayKeys = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

func dayProp() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
		"open":   map[string]any{"type": "string", "pattern": "^([01][0-9]|2[0-3]):[0-5][0-9]$|^24:00$"},
		"close":  map[string]any{"type": "string", "pattern": "^([01][0-9]|2[0-3]):[0-5][0-9]$|^24:00$"},
		"closed": map[string]any{"type": "boolean"},
	}}
}

func weekProp() map[string]any {
	props := map[string]any{}
	for _, d := range dayKeys {
		props[d] = dayProp()
	}
	return map[string]any{"type": "object", "title": "Days", "additionalProperties": false, "minProperties": 1, "properties": props,
		"description": "{mon:{open:\"09:00\",close:\"18:00\"}, sat:{closed:true}, ...}; missing days are closed"}
}

var minutesProp = map[string]any{"type": "integer", "minimum": 1, "maximum": 129600}

var priorityEnum = []string{"low", "normal", "high", "urgent"}

func targetsProp() map[string]any {
	return map[string]any{"type": "array", "title": "Targets", "minItems": 1, "maxItems": 4, "items": map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"priority"},
		"properties": map[string]any{
			"priority":               map[string]any{"type": "string", "enum": priorityEnum},
			"first_response_minutes": minutesProp,
			"next_response_minutes":  minutesProp,
			"resolution_minutes":     minutesProp,
		}}}
}

func str(p capability.Params, k string) string {
	s, _ := p[k].(string)
	return strings.TrimSpace(s)
}

func validTimezone(p capability.Params) error {
	if tz := str(p, "timezone"); tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			return capability.Invalid("timezone must be an IANA name such as Europe/Istanbul")
		}
	}
	return nil
}

func validWeek(p capability.Params) error {
	days, _ := p["days"].(map[string]any)
	for key, v := range days {
		d, _ := v.(map[string]any)
		if closed, _ := d["closed"].(bool); closed {
			continue
		}
		open, _ := d["open"].(string)
		closeAt, _ := d["close"].(string)
		if open == "" || closeAt == "" || open >= closeAt {
			return capability.Invalid("%s needs open < close, or closed: true", key)
		}
	}
	return nil
}

func validTargets(p capability.Params) error {
	items, _ := p["targets"].([]any)
	seen := map[string]bool{}
	for _, it := range items {
		m, _ := it.(map[string]any)
		pr, _ := m["priority"].(string)
		if seen[pr] {
			return capability.Invalid("priority %s appears twice", pr)
		}
		seen[pr] = true
		if m["first_response_minutes"] == nil && m["next_response_minutes"] == nil && m["resolution_minutes"] == nil {
			return capability.Invalid("target %s needs at least one time", pr)
		}
	}
	return nil
}

func duration(e capability.Exec, v any) string {
	m, ok := v.(float64)
	if !ok {
		return "—"
	}
	if int(m)%60 == 0 {
		return tr(e, "%d sa", "%d h", int(m)/60)
	}
	return tr(e, "%d dk", "%d min", int(m))
}

func describeTargets(e capability.Exec, p capability.Params) []capability.Detail {
	out := []capability.Detail{}
	items, _ := p["targets"].([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		out = append(out, capability.Detail{Label: fmt.Sprint(m["priority"]),
			Value: tr(e, "ilk yanıt %s · sonraki yanıt %s · çözüm %s", "first response %s · next response %s · resolution %s",
				duration(e, m["first_response_minutes"]), duration(e, m["next_response_minutes"]), duration(e, m["resolution_minutes"]))})
	}
	return out
}

type named struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

func list(ctx context.Context, api capability.API, path string) ([]named, error) {
	r, err := api.Call(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []named `json:"items"`
	}
	decErr := r.Decode(&out)
	return out.Items, decErr
}

func duplicate(ctx context.Context, e capability.Exec, path, name string) error {
	items, err := list(ctx, e.API, path)
	if err != nil {
		return err
	}
	for _, it := range items {
		if strings.EqualFold(it.Name, strings.TrimSpace(name)) {
			return capability.Invalid("%s", tr(e, "“%s” adında bir kayıt zaten var; güncellemeyi öneririm.", "“%s” already exists; update it instead.", it.Name))
		}
	}
	return nil
}

func created(r capability.Response, route string) (capability.Result, error) {
	var v named
	if err := r.Decode(&v); err != nil {
		return capability.Result{}, err
	}
	return capability.Result{Summary: v.UUID.String(), Route: route, Focus: &capability.Focus{Route: route}}, nil
}

func exists(ctx context.Context, e capability.Exec, path string) capability.Check {
	r, err := e.API.Call(ctx, http.MethodGet, path, nil)
	if err != nil || !r.OK {
		return capability.Check{Status: capability.VerifyFailed}
	}
	return capability.Check{Status: capability.VerifyOK}
}

// WriteCapabilities are the confirmed SLA changes.
func WriteCapabilities() []*capability.Capability {
	routes := []string{"/settings/sla"}
	hoursInput := func(p capability.Params) map[string]any {
		name, tz := str(p, "name"), str(p, "timezone")
		if name == "" {
			name = "Çalışma saatleri"
		}
		if tz == "" {
			tz = "Europe/Istanbul"
		}
		def, _ := p["is_default"].(bool)
		body := map[string]any{"name": name, "timezone": tz, "working_hours": p["days"], "is_default": def}
		if v, ok := p["use_public_holidays"].(bool); ok {
			body["use_public_holidays"] = v
			body["holiday_country"] = "TR"
		}
		return body
	}
	policyInput := func(p capability.Params) map[string]any {
		active := true
		if v, ok := p["is_active"].(bool); ok {
			active = v
		}
		body := map[string]any{"name": str(p, "name"), "description": str(p, "description"), "is_active": active, "targets": p["targets"]}
		if id := str(p, "business_hours_uuid"); id != "" {
			body["business_hours_uuid"] = id
		}
		if pr, ok := p["priorities"].([]any); ok {
			body["conditions"] = map[string]any{"priorities": pr}
		}
		return body
	}
	idProp := func(title string) map[string]any {
		return map[string]any{"type": "string", "title": title, "pattern": "^[0-9a-fA-F-]{36}$"}
	}
	hoursProps := map[string]any{
		"name":                map[string]any{"type": "string", "title": "Name", "maxLength": 120},
		"timezone":            map[string]any{"type": "string", "title": "Time zone", "maxLength": 64, "description": "IANA, default Europe/Istanbul"},
		"days":                weekProp(),
		"is_default":          map[string]any{"type": "boolean", "title": "Default schedule"},
		"use_public_holidays": map[string]any{"type": "boolean", "title": "Close on Turkish public holidays"},
	}
	policyProps := map[string]any{
		"name":                map[string]any{"type": "string", "title": "Name", "minLength": 1, "maxLength": 120},
		"description":         map[string]any{"type": "string", "title": "Description", "maxLength": 500},
		"targets":             targetsProp(),
		"business_hours_uuid": idProp("Business hours"),
		"priorities":          map[string]any{"type": "array", "title": "Only these priorities", "items": map[string]any{"type": "string", "enum": priorityEnum}, "maxItems": 4},
		"is_active":           map[string]any{"type": "boolean", "title": "Active"},
	}
	withID := func(props map[string]any, key, title string) map[string]any {
		out := map[string]any{key: idProp(title)}
		for k, v := range props {
			out[k] = v
		}
		return out
	}
	return []*capability.Capability{
		{
			Name: "sla.business_hours.create", Aliases: []string{"set_business_hours"}, Module: "sla", Kind: capability.Write,
			Permission: rbac.PermTenantSlaWrite, Routes: routes,
			Title:       map[string]string{"tr": "Çalışma saatlerini kur", "en": "Set business hours"},
			Description: "Create a business hours schedule (SLA clocks run on it). Ask for the days and hours and the time zone if unclear.",
			Undo:        "/settings/sla",
			Input:       map[string]any{"type": "object", "additionalProperties": false, "required": []string{"days"}, "properties": hoursProps},
			Validate: func(p capability.Params) error {
				if err := validWeek(p); err != nil {
					return err
				}
				return validTimezone(p)
			},
			DryRun: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				body := hoursInput(p)
				if err := duplicate(ctx, e, hoursPath, body["name"].(string)); err != nil {
					return capability.Result{}, err
				}
				details := []capability.Detail{{Label: tr(e, "Saat dilimi", "Time zone"), Value: body["timezone"].(string)}}
				days, _ := p["days"].(map[string]any)
				for _, k := range dayKeys {
					d, ok := days[k].(map[string]any)
					v := tr(e, "kapalı", "closed")
					if ok {
						if closed, _ := d["closed"].(bool); !closed {
							v = fmt.Sprintf("%v–%v", d["open"], d["close"])
						}
					}
					details = append(details, capability.Detail{Label: k, Value: v})
				}
				return capability.Result{Summary: tr(e, "“%s” çalışma saatleri oluşturulacak.", "Business hours “%s” will be created.", body["name"]), Details: details}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				r, err := e.API.Call(ctx, http.MethodPost, hoursPath, hoursInput(p))
				if err != nil {
					return capability.Result{}, err
				}
				return created(r, "/settings/sla")
			},
			Verify: func(ctx context.Context, e capability.Exec, _ capability.Params, res capability.Result) capability.Check {
				return exists(ctx, e, hoursPath+"/"+res.Summary)
			},
		},
		{
			Name: "sla.business_hours.update", Module: "sla", Kind: capability.Write, Permission: rbac.PermTenantSlaWrite, Routes: routes,
			Title:       map[string]string{"tr": "Çalışma saatlerini güncelle", "en": "Update business hours"},
			Description: "Replace the days and hours (and name, time zone, default flag) of an existing schedule (business_hours_uuid from sla_check or the SLA page).",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"business_hours_uuid", "days"},
				"properties": withID(hoursProps, "business_hours_uuid", "Business hours")},
			Validate: func(p capability.Params) error {
				if err := validWeek(p); err != nil {
					return err
				}
				return validTimezone(p)
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				id := str(p, "business_hours_uuid")
				if _, err := uuid.Parse(id); err != nil {
					return capability.Result{}, capability.Invalid("business_hours_uuid must be a UUID")
				}
				r, err := e.API.Call(ctx, http.MethodPatch, hoursPath+"/"+id, hoursInput(p))
				if err != nil {
					return capability.Result{}, err
				}
				return created(r, "/settings/sla")
			},
			Verify: func(ctx context.Context, e capability.Exec, _ capability.Params, res capability.Result) capability.Check {
				return exists(ctx, e, hoursPath+"/"+res.Summary)
			},
		},
		{
			Name: "sla.policies.create", Aliases: []string{"create_sla"}, Module: "sla", Kind: capability.Write,
			Permission: rbac.PermTenantSlaWrite, Routes: routes,
			Title: map[string]string{"tr": "SLA politikası oluştur", "en": "Create an SLA policy"},
			Description: "Create an SLA policy with response and resolution targets per priority (minutes). Optionally limit it to priorities and " +
				"bind it to a business hours schedule.",
			Undo:     "/settings/sla",
			Input:    map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name", "targets"}, "properties": policyProps},
			Validate: validTargets,
			DryRun: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				if err := duplicate(ctx, e, policiesPath, str(p, "name")); err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: tr(e, "“%s” SLA politikası oluşturulacak.", "SLA policy “%s” will be created.", str(p, "name")), Details: describeTargets(e, p)}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				r, err := e.API.Call(ctx, http.MethodPost, policiesPath, policyInput(p))
				if err != nil {
					return capability.Result{}, err
				}
				res, err := created(r, "/settings/sla")
				res.Details = describeTargets(e, p)
				return res, err
			},
			Verify: func(ctx context.Context, e capability.Exec, _ capability.Params, res capability.Result) capability.Check {
				return exists(ctx, e, policiesPath+"/"+res.Summary)
			},
		},
		{
			Name: "sla.policies.update", Module: "sla", Kind: capability.Write, Permission: rbac.PermTenantSlaWrite, Routes: routes,
			Title:       map[string]string{"tr": "SLA politikasını güncelle", "en": "Update an SLA policy"},
			Description: "Replace the name, targets and settings of an existing SLA policy (policy_uuid from the SLA page or sla_check).",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"policy_uuid", "name", "targets"},
				"properties": withID(policyProps, "policy_uuid", "Policy")},
			Validate: validTargets,
			DryRun: func(_ context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				return capability.Result{Summary: tr(e, "“%s” SLA politikası güncellenecek.", "SLA policy “%s” will be updated.", str(p, "name")), Details: describeTargets(e, p)}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				id := str(p, "policy_uuid")
				if _, err := uuid.Parse(id); err != nil {
					return capability.Result{}, capability.Invalid("policy_uuid must be a UUID")
				}
				r, err := e.API.Call(ctx, http.MethodPatch, policiesPath+"/"+id, policyInput(p))
				if err != nil {
					return capability.Result{}, err
				}
				res, err := created(r, "/settings/sla")
				res.Details = describeTargets(e, p)
				return res, err
			},
			Verify: func(ctx context.Context, e capability.Exec, _ capability.Params, res capability.Result) capability.Check {
				return exists(ctx, e, policiesPath+"/"+res.Summary)
			},
		},
		{
			Name: "sla.policies.delete", Module: "sla", Kind: capability.Destructive, Permission: rbac.PermTenantSlaWrite, Routes: routes,
			Title:       map[string]string{"tr": "SLA politikasını sil", "en": "Delete an SLA policy"},
			Description: "Delete an SLA policy for good; open conversations lose its clocks. To pause it, update it with is_active=false.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"policy_uuid"},
				"properties": map[string]any{"policy_uuid": idProp("Policy")}},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				id := str(p, "policy_uuid")
				if _, err := uuid.Parse(id); err != nil {
					return capability.Result{}, capability.Invalid("policy_uuid must be a UUID")
				}
				r, err := e.API.Call(ctx, http.MethodDelete, policiesPath+"/"+id, nil)
				if err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: id, Route: "/settings/sla"}, r.Err()
			},
		},
	}
}
