package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	slamodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/businesshours"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// ProposedAction is a change the assistant suggests. Nothing runs until
// the user confirms it; the confirm endpoint executes it with the
// caller's own permissions (checked server-side against Permission).
type ProposedAction struct {
	ID      string         `json:"id"`
	Kind    string         `json:"kind"`
	Title   string         `json:"title"`
	Summary string         `json:"summary"`
	Params  map[string]any `json:"params"`
	Status  string         `json:"status"`
	Route   string         `json:"route,omitempty"`
	Result  string         `json:"result,omitempty"`

	// Card v2 (CF-184), set for registry capabilities.
	Capability   string              `json:"capability,omitempty"`
	Risk         string              `json:"risk,omitempty"`
	Form         []capability.Field  `json:"form,omitempty"`
	DryRun       bool                `json:"dry_run,omitempty"`
	Undo         string              `json:"undo,omitempty"`
	Preview      *Preview            `json:"preview,omitempty"`
	Verification *capability.Check   `json:"verification,omitempty"`
	Details      []capability.Detail `json:"details,omitempty"`
	Focus        *capability.Focus   `json:"focus,omitempty"`
}

// Preview is the last dry run of an action.
type Preview struct {
	OK      bool                `json:"ok"`
	Summary string              `json:"summary"`
	Details []capability.Detail `json:"details,omitempty"`
}

// Action statuses.
const (
	ActionProposed  = "proposed"
	ActionConfirmed = "confirmed"
	ActionRejected  = "rejected"
	ActionDone      = "done"
	ActionFailed    = "failed"
)

// ActionContext is who confirmed the action, in which organization.
type ActionContext struct {
	Scope    orgctx.Scope
	UserID   int64
	UserName string
	Thread   uuid.UUID
}

// Executor runs one kind of action.
type Executor struct {
	// Permission the caller must hold (tenant.*); "" = any member.
	Permission string
	// Route the client opens after the action (optional).
	Route string
	// Describe is the one-line description given to the model.
	Describe string
	// Validate checks the params when the model proposes the action.
	Validate func(params map[string]any) error
	// Run executes it and returns a short human result.
	Run func(ctx context.Context, ac ActionContext, params map[string]any) (string, error)
}

// Registry maps an action kind to its executor.
type Registry map[string]Executor

// ErrActionInvalid: the params do not fit the action kind.
var ErrActionInvalid = errors.New("assistant: invalid action params")

func str(p map[string]any, key string) string {
	v, _ := p[key].(string)
	return strings.TrimSpace(v)
}

var (
	hhmm     = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$|^24:00$`)
	routeRxp = regexp.MustCompile(`^/[a-z0-9/_\-\[\]]{0,150}$`)
)

// navigate builds an executor that only sends the user to a page where
// the change needs a form the assistant cannot fill (credentials etc.).
func navigate(perm, route, describe string) Executor {
	return Executor{
		Permission: perm, Route: route, Describe: describe,
		Validate: func(map[string]any) error { return nil },
		Run:      func(context.Context, ActionContext, map[string]any) (string, error) { return route, nil },
	}
}

// Inviter invites members (organizations Management).
type Inviter interface {
	CreateInvitation(ctx context.Context, scope orgctx.Scope, actor orgusecase.Actor, in orgmodel.CreateInvitationInput) (orgmodel.Invitation, error)
}

// SLAService creates schedules and policies (sla usecase).
type SLAService interface {
	CreateBusinessHours(ctx context.Context, orgID int64, in slamodel.BusinessHoursInput) (slamodel.BusinessHours, error)
	CreatePolicy(ctx context.Context, orgID int64, in slamodel.PolicyInput) (slamodel.Policy, error)
}

// ActionDeps are the services the default actions call.
type ActionDeps struct {
	Q       *db.Queries
	Inviter Inviter
	SLA     SLAService
	// Escalate runs the support escalation (escalate_to_support).
	Escalate func(ctx context.Context, ac ActionContext, note string) (string, error)
}

var tenantRoles = []string{orgctx.RoleAdmin, orgctx.RoleSupervisor, orgctx.RoleAgent, orgctx.RoleLightAgent, orgctx.RoleMember, orgctx.RoleViewer, orgctx.RoleBilling}

func inList(v string, list []string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

var priorities = []string{"low", "normal", "high", "urgent"}

func minutes(v any) (*int32, bool) {
	f, ok := v.(float64)
	if !ok {
		if v == nil {
			return nil, true
		}
		return nil, false
	}
	if f < 1 || f > 60*24*90 || f != float64(int64(f)) {
		return nil, false
	}
	n := int32(f)
	return &n, true
}

func parseTargets(p map[string]any) ([]slamodel.Target, error) {
	raw, ok := p["targets"].([]any)
	if !ok || len(raw) == 0 || len(raw) > 4 {
		return nil, ErrActionInvalid
	}
	out := []slamodel.Target{}
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok || !inList(str(m, "priority"), priorities) {
			return nil, ErrActionInvalid
		}
		first, ok1 := minutes(m["first_response_minutes"])
		res, ok2 := minutes(m["resolution_minutes"])
		if !ok1 || !ok2 || (first == nil && res == nil) {
			return nil, ErrActionInvalid
		}
		out = append(out, slamodel.Target{Priority: str(m, "priority"), FirstResponseMinutes: first, ResolutionMinutes: res})
	}
	return out, nil
}

func parseWeek(p map[string]any) (businesshours.Week, error) {
	days, ok := p["days"].(map[string]any)
	if !ok || len(days) == 0 {
		return nil, ErrActionInvalid
	}
	week := businesshours.Week{}
	for key, v := range days {
		if !inList(key, businesshours.DayKeys[:]) {
			return nil, ErrActionInvalid
		}
		m, ok := v.(map[string]any)
		if !ok {
			return nil, ErrActionInvalid
		}
		closed, _ := m["closed"].(bool)
		d := businesshours.Day{Closed: closed, Open: str(m, "open"), Close: str(m, "close")}
		if !closed && (!hhmm.MatchString(d.Open) || !hhmm.MatchString(d.Close) || d.Open >= d.Close) {
			return nil, ErrActionInvalid
		}
		week[key] = d
	}
	return week, nil
}

func validTimezone(tz string) bool {
	if tz == "" {
		return true
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}

func validName(n string) bool { return utf8.RuneCountInString(n) <= 120 }

// DefaultActions are the CF-141 actions not yet moved to the capability
// registry (open_route, connect_channel and enable_ai now are
// capabilities, see CoreCapabilities); CF-188/CF-189 move the rest.
func DefaultActions(d ActionDeps) Registry {
	r := Registry{
		"create_channel_email": navigate(rbac.PermTenantChannelsManage, "/channels/connect/email",
			"Start connecting a support e-mail address (opens the e-mail channel form). params: {}"),
		"install_widget": navigate(rbac.PermTenantChannelsManage, "/widget",
			"Open the web widget set-up (snippet, colors, greeting). params: {}"),
	}
	if d.Inviter != nil {
		r["invite_member"] = Executor{
			Permission: rbac.PermTenantMembersWrite,
			Route:      "/settings/members",
			Describe:   "Invite a teammate by e-mail. params: {email, role: admin|supervisor|agent|light_agent|member|viewer|billing}",
			Validate: func(p map[string]any) error {
				if _, err := mail.ParseAddress(str(p, "email")); err != nil {
					return ErrActionInvalid
				}
				if role := str(p, "role"); role != "" && !inList(role, tenantRoles) {
					return ErrActionInvalid
				}
				return nil
			},
			Run: func(ctx context.Context, ac ActionContext, p map[string]any) (string, error) {
				role := str(p, "role")
				if role == "" {
					role = orgctx.RoleAgent
				}
				roleUUID, err := d.Q.AssistantTenantRoleUUID(ctx, db.AssistantTenantRoleUUIDParams{Slug: role, OrganizationID: i8(ac.Scope.ID)})
				if err != nil {
					return "", ErrActionInvalid
				}
				inv, err := d.Inviter.CreateInvitation(ctx, ac.Scope, orgusecase.Actor{UserID: ac.UserID, Name: ac.UserName},
					orgmodel.CreateInvitationInput{Email: str(p, "email"), RoleUUIDs: []uuid.UUID{roleUUID}})
				if err != nil {
					return "", err
				}
				return inv.Email, nil
			},
		}
	}
	if d.SLA != nil {
		r["set_business_hours"] = Executor{
			Permission: rbac.PermTenantSlaWrite,
			Route:      "/settings/sla",
			Describe: "Create the business hours schedule. params: {name, timezone (IANA, e.g. Europe/Istanbul), " +
				"days: {mon:{open:\"09:00\",close:\"18:00\"}, sat:{closed:true}, ...}, is_default: bool}",
			Validate: func(p map[string]any) error {
				if _, err := parseWeek(p); err != nil || !validTimezone(str(p, "timezone")) || !validName(str(p, "name")) {
					return ErrActionInvalid
				}
				return nil
			},
			Run: func(ctx context.Context, ac ActionContext, p map[string]any) (string, error) {
				week, err := parseWeek(p)
				if err != nil {
					return "", err
				}
				name, tz := str(p, "name"), str(p, "timezone")
				if name == "" {
					name = "Çalışma saatleri"
				}
				if tz == "" {
					tz = "Europe/Istanbul"
				}
				def, _ := p["is_default"].(bool)
				bh, err := d.SLA.CreateBusinessHours(ctx, ac.Scope.ID, slamodel.BusinessHoursInput{Name: name, Timezone: tz, WorkingHours: week, IsDefault: def})
				if err != nil {
					return "", err
				}
				return bh.Name, nil
			},
		}
		r["create_sla"] = Executor{
			Permission: rbac.PermTenantSlaWrite,
			Route:      "/settings/sla",
			Describe: "Create an SLA policy. params: {name, description, targets: [{priority: low|normal|high|urgent, " +
				"first_response_minutes, resolution_minutes}]}",
			Validate: func(p map[string]any) error {
				if _, err := parseTargets(p); err != nil || str(p, "name") == "" || !validName(str(p, "name")) {
					return ErrActionInvalid
				}
				return nil
			},
			Run: func(ctx context.Context, ac ActionContext, p map[string]any) (string, error) {
				targets, err := parseTargets(p)
				if err != nil {
					return "", err
				}
				active := true
				pol, err := d.SLA.CreatePolicy(ctx, ac.Scope.ID, slamodel.PolicyInput{Name: str(p, "name"), Description: str(p, "description"), IsActive: &active, Targets: targets})
				if err != nil {
					return "", err
				}
				return pol.Name, nil
			},
		}
	}
	if d.Escalate != nil {
		r[KindEscalate] = Executor{
			Permission: rbac.PermTenantAssistantUse,
			Describe:   "Hand the conversation to ClickFix's human support team. params: {note: short summary of the problem}",
			Validate: func(p map[string]any) error {
				if utf8.RuneCountInString(str(p, "note")) > 2000 {
					return ErrActionInvalid
				}
				return nil
			},
			Run: func(ctx context.Context, ac ActionContext, p map[string]any) (string, error) {
				return d.Escalate(ctx, ac, str(p, "note"))
			},
		}
	}
	return r
}

// KindEscalate is the escalation action (also POST .../escalate).
const KindEscalate = "escalate_to_support"

func (r Registry) describe() string {
	var b strings.Builder
	for _, k := range sortedKeys(r) {
		fmt.Fprintf(&b, "- %s: %s\n", k, r[k].Describe)
	}
	return b.String()
}

func sortedKeys(r Registry) []string { return slices.Sorted(maps.Keys(r)) }

func encodeActions(a []ProposedAction) []byte {
	if a == nil {
		a = []ProposedAction{}
	}
	raw, _ := json.Marshal(a)
	return raw
}
