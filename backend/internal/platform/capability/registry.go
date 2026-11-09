package capability

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Registry holds every capability; modules register theirs at start.
type Registry struct {
	list   []*Capability
	byName map[string]*Capability
	pages  []*Page
}

// NewRegistry builds an empty registry.
func NewRegistry() *Registry { return &Registry{byName: map[string]*Capability{}} }

// Register adds capabilities. A bad definition is a programming error and
// panics at start (as takwim's catalog does for bad schemas).
func (r *Registry) Register(caps ...*Capability) *Registry {
	for _, c := range caps {
		if err := r.add(c); err != nil {
			panic(err)
		}
	}
	return r
}

func (r *Registry) add(c *Capability) error {
	if c == nil || !nameRxp.MatchString(c.Name) || len(c.ToolName()) > 64 {
		return fmt.Errorf("capability: invalid name %v", c)
	}
	switch c.Kind {
	case Read:
		if c.Run == nil {
			return fmt.Errorf("capability %s: read needs Run", c.Name)
		}
	case Write, Destructive:
		if c.Apply == nil {
			return fmt.Errorf("capability %s: write needs Apply", c.Name)
		}
	default:
		return fmt.Errorf("capability %s: unknown kind %q", c.Name, c.Kind)
	}
	if c.Input == nil {
		c.Input = map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}
	}
	if c.Input["type"] != "object" || c.Input["additionalProperties"] != false {
		return fmt.Errorf("capability %s: input must be an object with additionalProperties false", c.Name)
	}
	if props, ok := c.Input["properties"].(map[string]any); ok {
		for _, s := range c.Secrets {
			if _, ok := props[s.Key]; !ok {
				return fmt.Errorf("capability %s: secret %s must be a declared property", c.Name, s.Key)
			}
		}
	}
	if c.OnSurface(SurfaceMCP) && len(c.Secrets) > 0 {
		// A generated secret is shown once in the confirmation card; MCP
		// has no such card, so these capabilities stay off it.
		return fmt.Errorf("capability %s: capabilities with secrets cannot be on the MCP surface", c.Name)
	}
	schema, err := compile(c.Name, c.Input)
	if err != nil {
		return fmt.Errorf("capability %s: schema: %w", c.Name, err)
	}
	c.schema = schema
	for _, key := range append([]string{c.Name, c.ToolName()}, c.Aliases...) {
		if old, dup := r.byName[key]; dup && old != c {
			return fmt.Errorf("capability: duplicate name %s", key)
		}
		r.byName[key] = c
	}
	r.list = append(r.list, c)
	return nil
}

// Get finds a capability by name, tool name or alias.
func (r *Registry) Get(name string) (*Capability, bool) {
	if r == nil {
		return nil, false
	}
	c, ok := r.byName[name]
	return c, ok
}

// All lists every capability (docs and tests).
func (r *Registry) All() []*Capability {
	if r == nil {
		return nil
	}
	return slices.Clone(r.list)
}

// Filter selects the capabilities offered to one caller on one page.
type Filter struct {
	Surface string
	Route   string
	// Has reports whether the caller holds a tenant permission.
	Has func(string) bool
	// Max caps the list (0 = no cap); page capabilities are kept first.
	Max int
}

// Available returns the capabilities the caller may use on the surface,
// the page's own first, then by name.
func (r *Registry) Available(f Filter) []*Capability {
	if r == nil {
		return nil
	}
	out := []*Capability{}
	for _, c := range r.list {
		if c.Allowed(f.Has, f.Surface) {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := out[i].Relevant(f.Route), out[j].Relevant(f.Route)
		if ri != rj {
			return ri
		}
		return out[i].Name < out[j].Name
	})
	if f.Max > 0 && len(out) > f.Max {
		out = out[:f.Max]
	}
	return out
}

// Field is one editable field of the confirmation card.
type Field struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
	Help     string   `json:"help,omitempty"`
	// Generated: a secret the server creates when left empty.
	Generated bool `json:"generated,omitempty"`
}

// Form describes the card form, in the order of the schema's "x-order"
// (or alphabetical); secrets come last.
func (c *Capability) Form(locale string) []Field {
	props, _ := c.Input["properties"].(map[string]any)
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	req := c.required()
	en := strings.HasPrefix(strings.ToLower(locale), "en")
	out := []Field{}
	secrets := []Field{}
	for _, k := range keys {
		p, _ := props[k].(map[string]any)
		f := Field{Key: k, Label: k, Required: slices.Contains(req, k)}
		if t, ok := p["title"].(string); ok && t != "" {
			f.Label = t
		}
		if d, ok := p["description"].(string); ok {
			f.Help = d
		}
		switch t, _ := p["type"].(string); t {
		case "boolean":
			f.Type = "boolean"
		case "integer", "number":
			f.Type = "number"
		case "object", "array":
			f.Type = "json"
		default:
			f.Type = "string"
			if p["format"] == "uri" {
				f.Type = "url"
			}
		}
		if enum, ok := p["enum"].([]string); ok {
			f.Type, f.Options = "enum", enum
		} else if enum, ok := p["enum"].([]any); ok {
			f.Type = "enum"
			for _, v := range enum {
				f.Options = append(f.Options, fmt.Sprint(v))
			}
		}
		if idx := slices.IndexFunc(c.Secrets, func(s Secret) bool { return s.Key == k }); idx >= 0 {
			s := c.Secrets[idx]
			f.Type, f.Generated, f.Required = "secret", s.Generate != nil, false
			if l := s.Label["tr"]; l != "" {
				f.Label = l
			}
			if l := s.Label["en"]; en && l != "" {
				f.Label = l
			}
			secrets = append(secrets, f)
			continue
		}
		out = append(out, f)
	}
	return append(out, secrets...)
}
