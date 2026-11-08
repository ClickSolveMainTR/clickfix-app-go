// Package capability is the single registry of things the ClickFix
// assistant (and, from CF-192, the MCP server) can do in an organization
// (CF-183, docs/design/asistan-yetenekleri.md).
//
// A Capability is defined once by its module: name, permission, input
// schema, kind (read/write/destructive), secret fields, dry run, apply and
// verify. Read capabilities run as model tools; write and destructive ones
// only ever become proposals that the user confirms. Implementations call
// the panel's own /v1 routes through API (apibridge), so the route's
// permission, entitlement, validation and audit rules apply unchanged.
//
// The tool shape follows takwim-app-go modules/mcp/catalog.go (permission
// list, Write/Destructive, schema resolved once at registration).
package capability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// Kind is what a capability does to data.
type Kind string

// Kinds.
const (
	// Read never changes data; the assistant runs it as a tool.
	Read Kind = "read"
	// Write changes data; it only runs after the user confirms.
	Write Kind = "write"
	// Destructive deletes, disables or rotates; confirmed with a warning.
	Destructive Kind = "destructive"
)

// Surfaces the assistant runs on.
const (
	SurfaceWeb      = "web"
	SurfaceMobile   = "mobile"
	SurfacePlatform = "platform"
	SurfaceWidget   = "widget"
)

// DefaultSurfaces are used when a capability names none: the member apps.
var DefaultSurfaces = []string{SurfaceWeb, SurfaceMobile}

// Params are the arguments of one call (decoded JSON).
type Params map[string]any

// Secret is a field the model never sees and never supplies.
type Secret struct {
	Key string
	// Label is the form label (tr/en).
	Label map[string]string
	// Generate creates the value at apply time when the user leaves it
	// empty; nil means the user must type it in the confirmation card.
	Generate func() (string, error)
}

// Focus is "go to the page and highlight the field".
type Focus struct {
	Route  string `json:"route"`
	Target string `json:"target,omitempty"`
}

// Detail is one block of a result shown on the card.
type Detail struct {
	Label string `json:"label"`
	Value string `json:"value,omitempty"`
	// Code is shown as a copyable code block (an example request).
	Code string `json:"code,omitempty"`
}

// Result is what a read, dry run or apply returns.
type Result struct {
	// Summary is one human line (card and model).
	Summary string `json:"summary"`
	// Data goes to the model (read tools) and stays out of the card.
	Data any `json:"data,omitempty"`
	// Details are shown on the card.
	Details []Detail `json:"details,omitempty"`
	// Route / Focus: where the user can see the result.
	Route string `json:"route,omitempty"`
	Focus *Focus `json:"focus,omitempty"`
	// Secrets generated or rotated by Apply, shown to the user once and
	// never stored, logged or sent to the model.
	Secrets map[string]string `json:"-"`
}

// Verification statuses.
const (
	VerifyOK      = "ok"
	VerifyFailed  = "failed"
	VerifySkipped = "skipped"
)

// Check is the result of Verify.
type Check struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// Exec is who runs a capability and how it reaches the API.
type Exec struct {
	OrgID    int64
	OrgUUID  uuid.UUID
	UserID   int64
	UserUUID uuid.UUID
	Locale   string
	Thread   uuid.UUID
	API      API
	// Secrets are the secret values of this apply (generated or typed by
	// the user in the card); empty during reads and dry runs.
	Secrets map[string]string
}

// En reports whether the result should be in English.
func (e Exec) En() bool { return strings.HasPrefix(strings.ToLower(e.Locale), "en") }

// Capability is one thing the assistant can do.
type Capability struct {
	// Name is module.object.verb ("channels.api.create").
	Name string
	// Aliases are older action kinds stored in threads (open_route).
	Aliases []string
	Module  string
	// Title is the card title (tr/en).
	Title map[string]string
	// Description is given to the model (English).
	Description string
	// Surfaces it may run on; empty = DefaultSurfaces.
	Surfaces []string
	// Permission the caller must hold (tenant.*); "" = any member.
	Permission string
	Kind       Kind
	// Input is the JSON schema of the params (an object). Every property
	// should carry "title" (form label) and "description".
	Input   map[string]any
	Secrets []Secret
	// Routes are the pages (route prefixes) the capability belongs to.
	Routes []string
	// Undo tells the user how to revert (human text or a capability name).
	Undo string
	// Simulations are dry-run endpoints ("POST /v1/tenant/workflows/{uuid}/test")
	// that change no data; only they may pass the read-only API besides GET.
	Simulations []string

	// Validate checks rules beyond the schema (cross-field, formats).
	Validate func(p Params) error
	// Run is the read capability.
	Run func(ctx context.Context, e Exec, p Params) (Result, error)
	// DryRun previews a write without changing data or leaving the
	// process (the API it gets refuses every method but GET).
	DryRun func(ctx context.Context, e Exec, p Params) (Result, error)
	// Apply executes a write.
	Apply func(ctx context.Context, e Exec, p Params) (Result, error)
	// Verify re-reads the state after Apply.
	Verify func(ctx context.Context, e Exec, p Params, r Result) Check

	schema *jsonschema.Schema
}

// Errors.
var (
	ErrInvalid   = errors.New("capability: invalid params")
	ErrForbidden = errors.New("capability: not allowed")
	ErrReadOnly  = errors.New("capability: only GET is allowed here")
)

// ParamError is an invalid param the model can correct.
type ParamError struct{ Message string }

func (e *ParamError) Error() string        { return e.Message }
func (e *ParamError) Is(target error) bool { return target == ErrInvalid }

// Invalid builds a ParamError.
func Invalid(format string, a ...any) error { return &ParamError{Message: fmt.Sprintf(format, a...)} }

var nameRxp = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*){0,3}$`)

// ToolName is the model/MCP tool name (dots are not allowed there).
func (c *Capability) ToolName() string { return strings.ReplaceAll(c.Name, ".", "_") }

// Writes reports whether the capability changes data.
func (c *Capability) Writes() bool { return c.Kind == Write || c.Kind == Destructive }

// TitleFor is the card title in a locale.
func (c *Capability) TitleFor(locale string) string {
	if strings.HasPrefix(strings.ToLower(locale), "en") && c.Title["en"] != "" {
		return c.Title["en"]
	}
	if c.Title["tr"] != "" {
		return c.Title["tr"]
	}
	return c.Name
}

// OnSurface reports whether the capability may run on a surface.
func (c *Capability) OnSurface(surface string) bool {
	if len(c.Surfaces) == 0 {
		return slices.Contains(DefaultSurfaces, surface)
	}
	return slices.Contains(c.Surfaces, surface)
}

// Allowed: the caller holds the permission and the surface is allowed.
func (c *Capability) Allowed(has func(string) bool, surface string) bool {
	return c.OnSurface(surface) && (c.Permission == "" || (has != nil && has(c.Permission)))
}

// Relevant reports whether the capability belongs to the page.
func (c *Capability) Relevant(route string) bool {
	for _, r := range c.Routes {
		if route == r || strings.HasPrefix(route, strings.TrimSuffix(r, "/")+"/") {
			return true
		}
	}
	return false
}

// IsSecret reports whether key is a secret field.
func (c *Capability) IsSecret(key string) bool {
	return slices.ContainsFunc(c.Secrets, func(s Secret) bool { return s.Key == key })
}

// ToolSchema is the schema the model sees: the input without "required"
// (a missing field reaches Check, which tells the model to ask the user
// instead of failing the whole turn) and never with secret fields.
func (c *Capability) ToolSchema() map[string]any {
	out := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}
	if props, ok := c.Input["properties"].(map[string]any); ok {
		clean := map[string]any{}
		for k, v := range props {
			if !c.IsSecret(k) {
				clean[k] = v
			}
		}
		out["properties"] = clean
	}
	return out
}

// required lists the required keys of the input schema.
func (c *Capability) required() []string {
	switch v := c.Input["required"].(type) {
	case []string:
		return v
	case []any:
		out := []string{}
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// Check validates params coming from the model or the card: no secret
// fields, the schema, then Validate. Errors are ParamErrors.
func (c *Capability) Check(p Params) error {
	if p == nil {
		p = Params{}
	}
	for k := range p {
		if c.IsSecret(k) {
			return Invalid("%s is a secret: it is never accepted from the chat; the user fills it in the confirmation card", k)
		}
	}
	for _, k := range c.required() {
		if v, ok := p[k]; !ok || v == nil || v == "" {
			return Invalid("%s is required: ask the user for it", k)
		}
	}
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > 16<<10 {
		return Invalid("params are too large")
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Invalid("params must be a JSON object")
	}
	if err := c.schema.Validate(doc); err != nil {
		return Invalid("params do not fit the schema of %s: %s", c.ToolName(), shortSchemaError(err))
	}
	if c.Validate != nil {
		if err := c.Validate(p); err != nil {
			var pe *ParamError
			if errors.As(err, &pe) {
				return err
			}
			return Invalid("%s", err.Error())
		}
	}
	return nil
}

func shortSchemaError(err error) string {
	var ve *jsonschema.ValidationError
	if errors.As(err, &ve) {
		for len(ve.Causes) > 0 {
			ve = ve.Causes[0]
		}
		loc := "/" + strings.Join(ve.InstanceLocation, "/")
		return strings.TrimSpace(loc + " " + ve.ErrorKind.LocalizedString(message.NewPrinter(language.English)))
	}
	msg := err.Error()
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}

// WithoutSecrets returns params without secret fields (stored and logged).
func (c *Capability) WithoutSecrets(p Params) Params {
	out := Params{}
	for k, v := range p {
		if !c.IsSecret(k) {
			out[k] = v
		}
	}
	return out
}

// SplitSecrets separates secret values typed by the user in the card.
func (c *Capability) SplitSecrets(p Params) (Params, map[string]string) {
	secrets := map[string]string{}
	for _, s := range c.Secrets {
		if v, ok := p[s.Key].(string); ok && strings.TrimSpace(v) != "" {
			secrets[s.Key] = strings.TrimSpace(v)
		}
	}
	return c.WithoutSecrets(p), secrets
}

// FillSecrets generates the secrets the user left empty; a secret without
// a generator must have been typed.
func (c *Capability) FillSecrets(secrets map[string]string) error {
	for _, s := range c.Secrets {
		if secrets[s.Key] != "" {
			continue
		}
		if s.Generate == nil {
			// Optional secrets (rotate only when asked) have no generator
			// and may stay empty; Apply decides.
			continue
		}
		v, err := s.Generate()
		if err != nil {
			return err
		}
		secrets[s.Key] = v
	}
	return nil
}

func compile(name string, schema map[string]any) (*jsonschema.Schema, error) {
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	c.UseLoader(denyLoader{})
	url := "https://clickfix.invalid/capability/" + name
	if err := c.AddResource(url, doc); err != nil {
		return nil, err
	}
	return c.Compile(url)
}

type denyLoader struct{}

func (denyLoader) Load(string) (any, error) {
	return nil, errors.New("capability: remote schemas are not allowed")
}
