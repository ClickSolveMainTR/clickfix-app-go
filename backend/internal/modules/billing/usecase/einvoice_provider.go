package usecase

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
)

// EInvoiceProvider is adapted from olexfilms modules/einvoice/provider. Invoice
// UUID is the idempotency key, including recovery after a lost Send response.
var ErrEInvoiceNotFound = errors.New("e-invoice: document not found at provider")

type EInvoiceProvider interface {
	Code() string
	Taxpayer(context.Context, string) (bool, string, error)
	Send(context.Context, EInvoiceEnvelope) (EInvoiceStatus, error)
	Status(context.Context, string) (EInvoiceStatus, error)
	Cancel(context.Context, string, string) (EInvoiceStatus, error)
}
type EInvoiceEnvelope struct {
	UUID, Number, Profile, ReceiverAlias string
	XML                                  []byte
}
type EInvoiceStatus struct{ State, Reference string }
type FakeEInvoiceProvider struct {
	mu        sync.Mutex
	states    map[string]EInvoiceStatus
	hashes    map[string][32]byte
	Taxpayers map[string]string
}

func (p *FakeEInvoiceProvider) Code() string { return "fake" }
func (p *FakeEInvoiceProvider) Taxpayer(_ context.Context, id string) (bool, string, error) {
	alias, ok := p.Taxpayers[id]
	return ok, alias, nil
}
func (p *FakeEInvoiceProvider) Send(_ context.Context, in EInvoiceEnvelope) (EInvoiceStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.states == nil {
		p.states = map[string]EInvoiceStatus{}
		p.hashes = map[string][32]byte{}
	}
	hash := sha256.Sum256(in.XML)
	if old, ok := p.hashes[in.UUID]; ok && old != hash {
		return EInvoiceStatus{}, fmt.Errorf("invoice UUID payload conflict")
	}
	p.hashes[in.UUID] = hash
	p.states[in.UUID] = EInvoiceStatus{State: "issued", Reference: "fake:" + in.UUID}
	return p.states[in.UUID], nil
}
func (p *FakeEInvoiceProvider) Status(_ context.Context, id string) (EInvoiceStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s, ok := p.states[id]
	if !ok {
		return s, ErrEInvoiceNotFound
	}
	return s, nil
}
func (p *FakeEInvoiceProvider) Cancel(_ context.Context, id, reason string) (EInvoiceStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s, ok := p.states[id]
	if !ok || reason == "" {
		return s, payments.ErrMalformed
	}
	s.State = "void"
	p.states[id] = s
	return s, nil
}
