package searchengine

import "fmt"

// Registry maps spec ids to adapters (registration order is kept so the
// palette lists specs in a stable order).
type Registry struct {
	bySpec map[string]Adapter
	order  []string
}

// NewRegistry builds an adapter registry.
func NewRegistry(adapters ...Adapter) *Registry {
	m := make(map[string]Adapter, len(adapters))
	order := make([]string, 0, len(adapters))
	for _, a := range adapters {
		if a == nil {
			continue
		}
		spec := a.Spec()
		if _, dup := m[spec.ID]; !dup {
			order = append(order, spec.ID)
		}
		m[spec.ID] = a
	}
	return &Registry{bySpec: m, order: order}
}

// Get returns an adapter by spec id.
func (r *Registry) Get(spec string) (Adapter, error) {
	a, ok := r.bySpec[spec]
	if !ok {
		return nil, fmt.Errorf("searchengine: unknown spec %q", spec)
	}
	return a, nil
}

// Specs returns all registered spec metadata.
func (r *Registry) Specs() []Spec {
	out := make([]Spec, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.bySpec[id].Spec())
	}
	return out
}

// SpecIDs lists registered spec ids.
func (r *Registry) SpecIDs() []string {
	return append([]string(nil), r.order...)
}
