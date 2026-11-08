package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/guides"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
)

// maxCapabilityTools caps the capabilities offered to the model in one
// reply; the page's own come first (docs/design/asistan-yetenekleri.md).
const maxCapabilityTools = 24

// APIFactory gives a capability the panel's /v1 routes as the user.
type APIFactory func(user, org uuid.UUID) capability.API

// ConfirmResult is the confirm response: the tool message and, once, the
// secrets the action generated (never stored).
type ConfirmResult struct {
	Message
	SecretOnce map[string]string `json:"secret_once,omitempty"`
}

// offered are the capabilities of this caller on this page.
func (s *Service) offered(c Caller, pc Context) []*capability.Capability {
	return s.Caps.Available(capability.Filter{Surface: pc.Surface, Route: pc.Route, Has: c.Scope.Has, Max: maxCapabilityTools})
}

func proposalSchema(cp *capability.Capability) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"params"},
		"properties": map[string]any{
			"summary": map[string]any{"type": "string", "description": "One sentence: what will change, in the user's language."},
			"params":  cp.ToolSchema(),
		}}
}

func capabilityTools(caps []*capability.Capability) []ai.Tool {
	out := make([]ai.Tool, 0, len(caps))
	for _, cp := range caps {
		if cp.Writes() {
			desc := "PROPOSES (does not run): " + cp.Description + " Shown to the user as a confirmation card; nothing changes until they confirm."
			if cp.Kind == capability.Destructive {
				desc += " Destructive: say clearly what is lost."
			}
			out = append(out, ai.Tool{Name: cp.ToolName(), Description: desc, Schema: proposalSchema(cp)})
			continue
		}
		out = append(out, ai.Tool{Name: cp.ToolName(), Description: cp.Description, Schema: cp.ToolSchema()})
	}
	return out
}

// exec builds the execution context; reads and dry runs get a GET-only API.
func (s *Service) exec(ctx context.Context, c Caller, thread uuid.UUID, readOnly bool) capability.Exec {
	e := capability.Exec{OrgID: c.Scope.ID, OrgUUID: c.Scope.UUID, UserID: c.UserID, Locale: guides.NormalizeLocale(c.Locale), Thread: thread}
	if u, err := s.Q.GetUserByID(ctx, c.UserID); err == nil {
		e.UserUUID = u.Uuid
	}
	if s.API != nil && e.UserUUID != uuid.Nil {
		e.API = s.API(e.UserUUID, c.Scope.UUID)
	}
	if readOnly {
		e.API = capability.ReadOnly(e.API)
	}
	return e
}

// runCapability runs a read tool or records a write proposal. A write is
// never applied here: only the confirm endpoint applies it.
func (r *toolRun) runCapability(ctx context.Context, cp *capability.Capability, input json.RawMessage) ai.ToolResult {
	if !cp.Allowed(r.caller.Scope.Has, r.surface) {
		return toolError("this capability is not available to the user")
	}
	if !cp.Writes() {
		var p capability.Params
		if len(input) > 0 && string(input) != "null" && json.Unmarshal(input, &p) != nil {
			return toolError("params must be a JSON object")
		}
		if err := cp.Check(p); err != nil {
			return toolError(err.Error())
		}
		res, err := cp.Run(ctx, r.svc.exec(ctx, r.caller, r.thread, true), p)
		if err != nil {
			return toolError(safeError(err))
		}
		return compactResult(res)
	}
	if len(r.actions) >= maxActions {
		return toolError("too many proposals in one reply")
	}
	var in struct {
		Summary string            `json:"summary"`
		Params  capability.Params `json:"params"`
	}
	if json.Unmarshal(input, &in) != nil {
		return toolError("invalid proposal")
	}
	if in.Params == nil {
		in.Params = capability.Params{}
	}
	// Masked e-mails of the user's message are restored after the loop;
	// check the shape with a sample address.
	raw, _ := json.Marshal(in.Params)
	check := capability.Params{}
	_ = json.Unmarshal([]byte(placeholderSample(string(raw))), &check)
	if err := cp.Check(check); err != nil {
		return toolError(err.Error())
	}
	a := ProposedAction{
		ID: newActionID(), Kind: cp.Name, Capability: cp.Name, Status: ActionProposed,
		Title: cp.TitleFor(r.locale), Summary: clipRunes(strings.TrimSpace(in.Summary), 500), Params: in.Params,
		Risk: string(cp.Kind), Form: cp.Form(r.locale), DryRun: cp.DryRun != nil, Undo: cp.Undo,
	}
	r.actions = append(r.actions, a)
	return toolJSON(map[string]any{"status": "proposed", "id": a.ID, "note": "Shown to the user as a confirmation card; wait for their decision. Never say it is done."})
}

// compactResult fits a read result in one tool result.
func compactResult(res capability.Result) ai.ToolResult {
	out := map[string]any{"summary": res.Summary}
	if res.Data != nil {
		out["data"] = res.Data
	}
	raw, _ := json.Marshal(out)
	if len(raw) > maxToolResultBytes {
		raw, _ = json.Marshal(map[string]any{"summary": res.Summary, "note": "The result is too large; ask a narrower question."})
	}
	return ai.ToolResult{Content: string(raw)}
}

// safeError is an error text that can reach the user and the model: API
// and param errors carry the route's own validation message; anything
// else is generic (it may hold internals).
func safeError(err error) string {
	var api *capability.APIError
	var pe *capability.ParamError
	switch {
	case errors.As(err, &pe):
		return pe.Message
	case errors.As(err, &api):
		return clipRunes(api.Message, 300)
	case errors.Is(err, capability.ErrReadOnly):
		return "this step cannot change data"
	}
	return "the request failed"
}

// mergeParams applies the fields the user edited in the card.
func mergeParams(stored map[string]any, edited capability.Params) capability.Params {
	out := capability.Params{}
	maps.Copy(out, stored)
	for k, v := range edited {
		if v == nil {
			delete(out, k)
			continue
		}
		out[k] = v
	}
	return out
}

func threadSurface(t db.AssistantThread) string {
	var pc Context
	_ = json.Unmarshal(t.Context, &pc)
	if pc.Surface == "" {
		return capability.SurfaceWeb
	}
	return pc.Surface
}

// findAction loads the message holding a pending action.
func findAction(ctx context.Context, q *db.Queries, t db.AssistantThread, actionID string) (db.AssistantMessage, []ProposedAction, int, error) {
	msg, err := q.AssistantMessageWithAction(ctx, db.AssistantMessageWithActionParams{OrganizationID: t.OrganizationID, ThreadID: t.ID, ActionID: actionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return msg, nil, -1, ErrNotFound
	}
	if err != nil {
		return msg, nil, -1, err
	}
	var actions []ProposedAction
	_ = json.Unmarshal(msg.Actions, &actions)
	for i := range actions {
		if actions[i].ID == actionID {
			return msg, actions, i, nil
		}
	}
	return msg, nil, -1, ErrNotFound
}

// DryRun previews a pending capability action with the card's params. It
// never changes data: the capability gets a GET-only API.
func (s *Service) DryRun(ctx context.Context, c Caller, threadID uuid.UUID, actionID string, edited capability.Params) (ProposedAction, error) {
	t, err := s.thread(ctx, c.Scope.ID, c.UserID, threadID)
	if err != nil {
		return ProposedAction{}, err
	}
	var out ProposedAction
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := s.Q.WithTx(tx)
		msg, actions, idx, err := findAction(ctx, q, t, actionID)
		if err != nil {
			return err
		}
		a := &actions[idx]
		if a.Status != ActionProposed {
			return ErrActionState
		}
		cp, ok := s.Caps.Get(a.Kind)
		if !ok || cp.DryRun == nil {
			return ErrNotFound
		}
		if !cp.Allowed(c.Scope.Has, threadSurface(t)) {
			return ErrForbidden
		}
		params, _ := cp.SplitSecrets(mergeParams(a.Params, edited))
		if err := cp.Check(params); err != nil {
			return err
		}
		res, runErr := cp.DryRun(ctx, s.exec(ctx, c, t.Uuid, true), params)
		a.Preview = &Preview{OK: runErr == nil, Summary: res.Summary, Details: res.Details}
		if runErr != nil {
			a.Preview.Summary, a.Preview.Details = safeError(runErr), nil
		}
		out = *a
		return q.AssistantMessageSetActions(ctx, db.AssistantMessageSetActionsParams{OrganizationID: t.OrganizationID, ID: msg.ID, Actions: encodeActions(actions)})
	})
	return out, err
}

// confirmCapability applies a registry capability inside Confirm's
// transaction: the caller's own permission, the (edited) params checked
// again, secrets generated here and returned once, then Verify.
func (s *Service) confirmCapability(ctx context.Context, q *db.Queries, c Caller, t db.AssistantThread, msg db.AssistantMessage,
	actions []ProposedAction, idx int, cp *capability.Capability, edited capability.Params) (ConfirmResult, error) {
	a := &actions[idx]
	if !cp.Allowed(c.Scope.Has, threadSurface(t)) {
		return ConfirmResult{}, ErrForbidden
	}
	params, secrets := cp.SplitSecrets(mergeParams(a.Params, edited))
	if err := cp.Check(params); err != nil {
		return ConfirmResult{}, err
	}
	if err := cp.FillSecrets(secrets); err != nil {
		return ConfirmResult{}, err
	}
	e := s.exec(ctx, c, t.Uuid, false)
	e.Secrets = secrets
	res, runErr := cp.Apply(ctx, e, params)
	a.Params, a.Capability = params, cp.Name
	a.Status, a.Result = ActionDone, res.Summary
	a.Details, a.Route, a.Focus = res.Details, res.Route, res.Focus
	if runErr != nil {
		a.Status, a.Result, a.Details = ActionFailed, safeError(runErr), nil
	} else if cp.Verify != nil {
		e.Secrets = nil
		check := cp.Verify(ctx, e, params, res)
		a.Verification = &check
	}
	text := resultText(c.Locale, *a, runErr)
	if a.Verification != nil && a.Verification.Status == capability.VerifyFailed {
		text += "\n" + verifyText(c.Locale, a.Verification.Message)
	}
	if err := q.AssistantMessageSetActions(ctx, db.AssistantMessageSetActionsParams{OrganizationID: t.OrganizationID, ID: msg.ID, Actions: encodeActions(actions)}); err != nil {
		return ConfirmResult{}, err
	}
	out, err := s.addMessage(ctx, q, t, "tool", text, nil, []ProposedAction{*a})
	if err != nil {
		return ConfirmResult{}, err
	}
	result := ConfirmResult{Message: out}
	if runErr == nil && len(res.Secrets) > 0 {
		result.SecretOnce = res.Secrets
	}
	return result, nil
}

func verifyText(locale, detail string) string {
	if guides.NormalizeLocale(locale) == "en" {
		return strings.TrimSpace("⚠️ The change was saved but the check after it failed. " + detail)
	}
	return strings.TrimSpace("⚠️ Değişiklik kaydedildi ama sonrasındaki denetim başarısız oldu. " + detail)
}

// capabilityPrompt lists the page's capabilities for the instructions.
func capabilityPrompt(caps []*capability.Capability, route string) string {
	var page []string
	for _, cp := range caps {
		if cp.Relevant(route) {
			page = append(page, cp.ToolName())
		}
	}
	if len(page) == 0 {
		return ""
	}
	return fmt.Sprintf("Tools that belong to the current page: %s.\n", strings.Join(page, ", "))
}
