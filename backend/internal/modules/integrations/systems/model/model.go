// Package model holds the connected system ("Sistemini bağla", CF-197)
// request and response types.
package model

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/screenctx"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrInvalid       = errors.New("invalid")
	ErrNotConnected  = errors.New("connected system is not configured")
	ErrNotAllowed    = errors.New("action is not allowed for this caller")
	ErrNeedsApproval = errors.New("action requires approval")
	ErrDisabled      = errors.New("disabled")
)

// Channel says who runs a call: the customer-facing AI agent, a member
// (panel, assistant card, MCP), a workflow, or the settings "Dene" button.
const (
	ChannelAI       = "ai"
	ChannelAgent    = "agent"
	ChannelWorkflow = "workflow"
	ChannelTest     = "test"
)

// Channels are the values an action's allowed_for may hold.
var Channels = []string{ChannelAI, ChannelAgent, ChannelWorkflow}

type channelKey struct{}

// WithWorkflow marks an in-process API call as made by a workflow. Only
// code in this process can set it (the automation engine calls the API
// through apibridge with this context); an HTTP client cannot.
func WithWorkflow(ctx context.Context) context.Context {
	return context.WithValue(ctx, channelKey{}, ChannelWorkflow)
}

// ChannelFrom returns the channel of a request: workflow when the context
// was marked with WithWorkflow, otherwise agent.
func ChannelFrom(ctx context.Context) string {
	if v, _ := ctx.Value(channelKey{}).(string); v == ChannelWorkflow {
		return ChannelWorkflow
	}
	return ChannelAgent
}

// Auth types of a connection.
const (
	AuthNone   = "none"
	AuthBearer = "bearer"
	AuthHeader = "header"
	AuthBasic  = "basic"
)

// Identity keys a source may be read by.
const (
	KeyExternalID        = "external_id"
	KeyCompanyExternalID = "company_external_id"
)

type Connection struct {
	UUID       uuid.UUID  `json:"uuid"`
	Name       string     `json:"name"`
	BaseURL    string     `json:"base_url"`
	AuthType   string     `json:"auth_type"`
	AuthHeader string     `json:"auth_header"`
	HasSecret  bool       `json:"has_secret"`
	TimeoutMS  int32      `json:"timeout_ms"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

type ConnectionInput struct {
	Name       string `json:"name"`
	BaseURL    string `json:"base_url"`
	AuthType   string `json:"auth_type"`
	AuthHeader string `json:"auth_header"`
	// Secret is the token, header value or "user:password"; write-only.
	Secret     string `json:"secret"`
	KeepSecret bool   `json:"keep_secret"`
	TimeoutMS  int32  `json:"timeout_ms"`
}

// Field is a response path picked from a real answer.
type Field struct {
	Path  string `json:"path"`
	Label string `json:"label"`
}

type Source struct {
	UUID         uuid.UUID `json:"uuid"`
	PathTemplate string    `json:"path_template"`
	IdentityKey  string    `json:"identity_key"`
	ItemsPath    string    `json:"items_path"`
	Fields       []Field   `json:"fields"`
	Enabled      bool      `json:"enabled"`
}

type SourceInput struct {
	PathTemplate string  `json:"path_template"`
	IdentityKey  string  `json:"identity_key"`
	ItemsPath    string  `json:"items_path"`
	Fields       []Field `json:"fields"`
	Enabled      bool    `json:"enabled"`
}

// Concept is a custom object type and where its data comes from.
type Concept struct {
	ObjectTypeUUID uuid.UUID `json:"object_type_uuid"`
	Key            string    `json:"key"`
	Label          string    `json:"label"`
	PluralLabel    string    `json:"plural_label"`
	Icon           string    `json:"icon"`
	Source         *Source   `json:"source"`
}

// Param is an input of an action, filled by the caller.
type Param struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     string `json:"type"` // string | number | boolean
	Required bool   `json:"required"`
}

type Action struct {
	UUID             uuid.UUID `json:"uuid"`
	Key              string    `json:"key"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	ConceptKey       string    `json:"concept_key"`
	Method           string    `json:"method"`
	PathTemplate     string    `json:"path_template"`
	Params           []Param   `json:"params"`
	RequiresApproval bool      `json:"requires_approval"`
	AllowedFor       []string  `json:"allowed_for"`
	Enabled          bool      `json:"enabled"`
}

type ActionInput struct {
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	ConceptKey       string   `json:"concept_key"`
	Method           string   `json:"method"`
	PathTemplate     string   `json:"path_template"`
	Params           []Param  `json:"params"`
	RequiresApproval bool     `json:"requires_approval"`
	AllowedFor       []string `json:"allowed_for"`
	Enabled          bool     `json:"enabled"`
}

type Playbook struct {
	UUID          uuid.UUID  `json:"uuid"`
	Intent        string     `json:"intent"`
	Title         string     `json:"title"`
	Instructions  string     `json:"instructions"`
	ConceptKeys   []string   `json:"concept_keys"`
	ActionKeys    []string   `json:"action_keys"`
	ProcedureUUID *uuid.UUID `json:"procedure_uuid"`
	Enabled       bool       `json:"enabled"`
}

type PlaybookInput struct {
	Title         string     `json:"title"`
	Instructions  string     `json:"instructions"`
	ConceptKeys   []string   `json:"concept_keys"`
	ActionKeys    []string   `json:"action_keys"`
	ProcedureUUID *uuid.UUID `json:"procedure_uuid"`
	Enabled       bool       `json:"enabled"`
}

type Call struct {
	Kind       string    `json:"kind"`
	Target     string    `json:"target"`
	Channel    string    `json:"channel"`
	Method     string    `json:"method"`
	MaskedPath string    `json:"masked_path"`
	StatusCode int32     `json:"status_code"`
	OK         bool      `json:"ok"`
	ErrorCode  string    `json:"error_code"`
	DurationMS int32     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

// Overview is the whole "Sistemini bağla" screen in one response.
type Overview struct {
	Connection *Connection `json:"connection"`
	Concepts   []Concept   `json:"concepts"`
	Actions    []Action    `json:"actions"`
	Playbooks  []Playbook  `json:"playbooks"`
	Calls      []Call      `json:"calls"`
}

// Identity is what the connected system is asked with.
type Identity struct {
	ContactUUID       *uuid.UUID `json:"contact_uuid"`
	ContactName       string     `json:"contact_name"`
	ExternalID        string     `json:"external_id"`
	CompanyUUID       *uuid.UUID `json:"company_uuid"`
	CompanyName       string     `json:"company_name"`
	CompanyExternalID string     `json:"company_external_id"`
}

// PathOption is one leaf of a test response, offered for picking.
type PathOption struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Sample any    `json:"sample"`
}

type TestInput struct {
	ExternalID        string `json:"external_id"`
	CompanyExternalID string `json:"company_external_id"`
	// Unsaved edits may be tried before saving.
	PathTemplate *string `json:"path_template"`
	ItemsPath    *string `json:"items_path"`
}

type TestResult struct {
	OK         bool             `json:"ok"`
	StatusCode int              `json:"status_code"`
	ErrorCode  string           `json:"error_code"`
	Message    string           `json:"message"`
	DurationMS int64            `json:"duration_ms"`
	Request    string           `json:"request"`
	Body       any              `json:"body"`
	Truncated  bool             `json:"truncated"`
	Paths      []PathOption     `json:"paths"`
	Items      []map[string]any `json:"items"`
}

// Lookup is concept data for one customer.
type Lookup struct {
	Concept  string           `json:"concept"`
	Label    string           `json:"label"`
	Identity Identity         `json:"identity"`
	Items    []map[string]any `json:"items"`
	Status   string           `json:"status"` // ok | skipped | error
	Reason   string           `json:"reason"`
}

type RunInput struct {
	ContactUUID      *uuid.UUID     `json:"contact_uuid"`
	ExternalID       string         `json:"external_id"`
	ConversationUUID *uuid.UUID     `json:"conversation_uuid"`
	Params           map[string]any `json:"params"`
	DryRun           bool           `json:"dry_run"`
	Approved         bool           `json:"approved"`
}

type RunResult struct {
	Action     string `json:"action"`
	DryRun     bool   `json:"dry_run"`
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code"`
	ErrorCode  string `json:"error_code"`
	Request    string `json:"request"`
	Body       any    `json:"body"`
	Summary    string `json:"summary"`
}

// PreviewInput is "bu müşteri olarak dene": what an integrator's widget
// would send for one customer.
type PreviewInput struct {
	ContactUUID       *uuid.UUID        `json:"contact_uuid"`
	ExternalID        string            `json:"external_id"`
	CompanyExternalID string            `json:"company_external_id"`
	CompanyName       string            `json:"company_name"`
	Attributes        map[string]any    `json:"attributes"`
	PageURL           string            `json:"page_url"`
	Events            []screenctx.Event `json:"events"`
	Intent            string            `json:"intent"`
}

// Check is one accepted / rejected line of the preview.
type Check struct {
	Key    string `json:"key"`
	Status string `json:"status"` // accepted | rejected | skipped
	Reason string `json:"reason"`
}

type Rejection struct {
	ChannelUUID uuid.UUID `json:"channel_uuid"`
	ChannelName string    `json:"channel_name"`
	Code        string    `json:"code"`
	RejectedAt  time.Time `json:"rejected_at"`
}

type Preview struct {
	Identity         Identity          `json:"identity"`
	Checks           []Check           `json:"checks"`
	Attributes       map[string]any    `json:"attributes"`
	Screen           *screenctx.Screen `json:"screen"`
	Intents          []Intent          `json:"intents"`
	Intent           string            `json:"intent"`
	Lookups          []Lookup          `json:"lookups"`
	AIContext        string            `json:"ai_context"`
	WidgetRejections []Rejection       `json:"widget_rejections"`
}

// Intent is a playbook offered to the AI triage as a candidate.
type Intent struct {
	Intent string `json:"intent"`
	Title  string `json:"title"`
}
