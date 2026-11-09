// Catalog presents the capability registry (CF-183,
// docs/design/asistan-yetenekleri.md) to MCP clients: every capability on
// the "mcp" surface is one tool, so the assistant and MCP share one
// definition. Permission filtering follows takwim-app-go
// modules/mcp/catalog.go.
package mcp

import (
	"errors"
	"slices"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
)

// Args are the arguments of one tools/call.
type Args map[string]any

// Tool is one MCP tool: a capability as MCP clients see it.
type Tool struct {
	Name, Title, Description string
	Write, Destructive       bool
	AnyOf                    []string
	Input                    map[string]any
	Capability               *capability.Capability
}

// ArgumentError is an invalid argument the model can correct.
type ArgumentError struct{ Message string }

func (e *ArgumentError) Error() string { return e.Message }

// Validate checks the arguments against the capability's schema and rules.
func (t *Tool) Validate(args Args) error {
	if err := t.Capability.Check(capability.Params(args)); err != nil {
		var pe *capability.ParamError
		if errors.As(err, &pe) {
			return &ArgumentError{Message: pe.Message}
		}
		return &ArgumentError{Message: "invalid arguments"}
	}
	return nil
}

// Catalog is the organization layer's tools.
type Catalog struct{ tools []*Tool }

// Available is the tools the principal may use now.
func (c *Catalog) Available(p Principal) []*Tool {
	out := []*Tool{}
	for _, t := range c.tools {
		if c.Allowed(p, t) {
			out = append(out, t)
		}
	}
	return out
}

// Allowed: organization layer, write tools only with mcp:write, and the
// capability's permission in the organization right now.
func (c *Catalog) Allowed(p Principal, t *Tool) bool {
	return p.Layer == model.LayerBusiness && p.Scope != nil && (!t.Write || p.CanWrite) &&
		(len(t.AnyOf) == 0 || slices.ContainsFunc(t.AnyOf, p.Scope.Has))
}

// NewCatalog builds the tools of every capability on the MCP surface.
func NewCatalog(reg *capability.Registry) *Catalog {
	c := &Catalog{}
	for _, cp := range reg.All() {
		if !cp.OnSurface(capability.SurfaceMCP) {
			continue
		}
		t := &Tool{Name: cp.MCPToolName(), Title: cp.MCPToolName(), Description: cp.Description,
			Write: cp.Writes(), Destructive: cp.Kind == capability.Destructive, Input: cp.Input, Capability: cp}
		if cp.Permission != "" {
			t.AnyOf = []string{cp.Permission}
		}
		c.tools = append(c.tools, t)
	}
	return c
}
