package sla

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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
				st.Missing = append(st.Missing, capability.Step{Key: "business_hours",
					Title:  tr(e, "Çalışma saatleri tanımlı değil", "No business hours yet"),
					Detail: tr(e, "SLA süreleri çalışma saatlerine göre işler.", "SLA clocks run on business hours."),
					Prompt: tr(e, "Çalışma saatlerimi kurmama yardım et", "Help me set up my business hours")})
			}
			if policies == 0 {
				st.Missing = append(st.Missing, capability.Step{Key: "sla_policy",
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
