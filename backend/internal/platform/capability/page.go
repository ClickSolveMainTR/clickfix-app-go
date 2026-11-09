package capability

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Step is one thing the user left undone (or should look at) on a page.
type Step struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	// Detail explains it (an error message, a count).
	Detail string `json:"detail,omitempty"`
	// Capability is the tool that does it, when there is one.
	Capability string `json:"capability,omitempty"`
	// Prompt is what the drawer sends when the user taps "help me".
	Prompt string `json:"prompt,omitempty"`
	Focus  *Focus `json:"focus,omitempty"`
}

// Test is a check the user can run from the assistant on this page.
type Test struct {
	Capability string `json:"capability"`
	Title      string `json:"title"`
	Prompt     string `json:"prompt"`
}

// PageState is what the assistant knows about the current page (CF-187):
// what can be done there, what is missing, what needs attention and which
// tests can run. Steps whose capability the user cannot use are dropped.
type PageState struct {
	Module   string `json:"module"`
	Title    string `json:"title"`
	Missing  []Step `json:"missing"`
	Warnings []Step `json:"warnings"`
	Tests    []Test `json:"tests"`
	// Actions are the capabilities of the page the user may run.
	Actions []string `json:"actions"`
}

// Page is a module's page-state provider.
type Page struct {
	Module string
	Routes []string
	// Permission needed to see the page's state ("" = any member).
	Permission string
	// State reads the page; it gets a GET-only API.
	State func(ctx context.Context, e Exec) (PageState, error)
}

// Matches reports whether the page serves route (longest prefix wins in
// PageFor).
func (p *Page) match(route string) int {
	best := -1
	for _, r := range p.Routes {
		r = strings.TrimSuffix(r, "/")
		if route == r || strings.HasPrefix(route, r+"/") {
			best = max(best, len(r))
		}
	}
	return best
}

// RegisterPages adds page-state providers.
func (r *Registry) RegisterPages(pages ...*Page) *Registry {
	for _, p := range pages {
		if p == nil || p.Module == "" || p.State == nil || len(p.Routes) == 0 {
			panic(fmt.Sprintf("capability: invalid page %+v", p))
		}
		r.pages = append(r.pages, p)
	}
	return r
}

// PageFor finds the provider of a route the caller may see.
func (r *Registry) PageFor(route string, has func(string) bool) *Page {
	if r == nil {
		return nil
	}
	var out *Page
	best := -1
	for _, p := range r.pages {
		if p.Permission != "" && (has == nil || !has(p.Permission)) {
			continue
		}
		if m := p.match(route); m > best {
			out, best = p, m
		}
	}
	return out
}

// Finish drops steps and tests the caller cannot act on and lists the
// page's capabilities they may use.
func (r *Registry) Finish(st PageState, f Filter) PageState {
	usable := map[string]*Capability{}
	for _, c := range r.Available(Filter{Surface: f.Surface, Has: f.Has}) {
		usable[c.Name] = c
	}
	keep := func(steps []Step) []Step {
		out := []Step{}
		for _, s := range steps {
			if s.Capability != "" && usable[s.Capability] == nil {
				s.Capability = ""
			}
			out = append(out, s)
		}
		return out
	}
	st.Missing, st.Warnings = keep(st.Missing), keep(st.Warnings)
	tests := []Test{}
	for _, t := range st.Tests {
		if usable[t.Capability] != nil {
			tests = append(tests, t)
		}
	}
	st.Tests = tests
	st.Actions = []string{}
	for name, c := range usable {
		if c.Module == st.Module && c.Writes() {
			st.Actions = append(st.Actions, name)
		}
	}
	sort.Strings(st.Actions)
	return st
}
