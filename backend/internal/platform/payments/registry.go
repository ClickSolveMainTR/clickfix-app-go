package payments

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var ErrUnsupported = errors.New("payments: unsupported capability")
var ErrNotConfigured = errors.New("payments: provider not configured")

// ErrProviderDisabled: the caller named a provider that is not enabled in this
// deployment (PAYMENTS_<P>_ENABLED is not true). It wraps ErrNotConfigured.
var ErrProviderDisabled = fmt.Errorf("%w: provider disabled", ErrNotConfigured)

// fallbackOrder picks a provider when platform settings name none (or name one
// that is not enabled): a deployment with a single enabled provider works
// without an extra admin step (CF-154: an empty default_provider used to fail
// every checkout).
var fallbackOrder = []string{"iyzico", "paytr", "fake"}

// Selection is read on each selection, so platform changes take effect immediately.
type Selection struct {
	Default    string
	Currencies map[string]string
}
type SettingsReader interface {
	PaymentSelection(context.Context) (Selection, error)
}
type Registry struct {
	mu         sync.RWMutex
	providers  map[string]Provider
	settings   SettingsReader
	production bool
}

func NewRegistry(reader SettingsReader, production bool) *Registry {
	return &Registry{providers: map[string]Provider{}, settings: reader, production: production}
}
func (r *Registry) Register(p Provider) error {
	if p == nil || p.Code() == "" {
		return ErrNotConfigured
	}
	if r.production && p.Code() == "fake" {
		return fmt.Errorf("%w: fake gateway in production", ErrNotConfigured)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.providers[p.Code()]; ok {
		return fmt.Errorf("payments: duplicate provider %s", p.Code())
	}
	r.providers[p.Code()] = p
	return nil
}
func (r *Registry) Get(code string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[code]
	if !ok {
		return nil, ErrNotConfigured
	}
	return p, nil
}

// Codes lists the registered provider codes in fallback order.
func (r *Registry) Codes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []string{}
	for _, code := range fallbackOrder {
		if _, ok := r.providers[code]; ok {
			out = append(out, code)
		}
	}
	return out
}

// Select resolves the provider of a checkout: the caller's explicit choice,
// else the platform setting for the currency, else the default, else the
// first enabled provider. An explicit choice that is not enabled is
// ErrProviderDisabled; nothing enabled at all is ErrNotConfigured.
func (r *Registry) Select(ctx context.Context, currency, preferred string) (Provider, error) {
	if currency != "TRY" && currency != "USD" && currency != "EUR" {
		return nil, ErrNotConfigured
	}
	if preferred != "" {
		p, err := r.Get(preferred)
		if err != nil {
			return nil, ErrProviderDisabled
		}
		return p, nil
	}
	if r.settings != nil {
		cfg, err := r.settings.PaymentSelection(ctx)
		if err != nil {
			return nil, err
		}
		code := cfg.Currencies[currency]
		if code == "" {
			code = cfg.Default
		}
		if p, err := r.Get(code); err == nil {
			return p, nil
		}
	}
	codes := r.Codes()
	if len(codes) == 0 {
		return nil, ErrNotConfigured
	}
	return r.Get(codes[0])
}
