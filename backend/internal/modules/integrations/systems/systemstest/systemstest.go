// Package systemstest is a fake accounting system and fixtures for the
// connected system tests (CF-197). Only tests import it.
package systemstest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems/usecase"
)

// Token is the fake system's bearer token.
const Token = "acc-test-token-7f3a9c" // #nosec G101 -- sahte test sisteminin sabit anahtarı

// Accounting is a fake accounting system: GET /faturalar?musteri=<id>
// lists invoices, POST /faturalar/<no>/tekrar-gonder resends one.
type Accounting struct {
	*httptest.Server
	mu      sync.Mutex
	Resent  []string
	Queries []string
}

// NewAccounting starts the fake system. Customer CUST-42 has a failed invoice.
func NewAccounting(t *testing.T) *Accounting {
	t.Helper()
	a := &Accounting{}
	a.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+Token {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/faturalar":
			a.Queries = append(a.Queries, r.URL.Query().Get("musteri"))
			invoices := []any{}
			if r.URL.Query().Get("musteri") == "CUST-42" {
				invoices = append(invoices,
					map[string]any{"no": "F-2026-101", "durum": "Hata", "hata": map[string]any{"metin": "Alıcı e-posta adresi reddetti"}, "tutar": 1250.0, "ic_kod": "x-991"},
					map[string]any{"no": "F-2026-088", "durum": "Gönderildi", "hata": map[string]any{"metin": ""}, "tutar": 980.0, "ic_kod": "x-990"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"faturalar": invoices}})
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/faturalar/") && strings.HasSuffix(r.URL.Path, "/tekrar-gonder"):
			no := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/faturalar/"), "/tekrar-gonder")
			a.Resent = append(a.Resent, no)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "no": no})
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	t.Cleanup(a.Close)
	return a
}

// Caller reaches the fake system (tests only: it skips the public-address
// rule that production enforces).
func (a *Accounting) Caller() *usecase.Caller {
	c := usecase.NewCaller(nil)
	c.Validate = func(string) error { return nil }
	c.HTTP = func(timeout time.Duration) *http.Client {
		cl := a.Client()
		cl.Timeout = timeout
		return cl
	}
	return c
}

// Fixture is an organization with an invoice concept and a customer.
type Fixture struct {
	Org            int64
	User           int64
	ObjectTypeUUID uuid.UUID
	Contact        int64
	ContactUUID    uuid.UUID
	Conversation   int64
	ConvUUID       uuid.UUID
}

// Seed creates an organization, a "fatura" custom object type and a
// widget contact identified as CUST-42 with a company and a conversation.
func Seed(t *testing.T, ctx context.Context, pool *pgxpool.Pool) Fixture {
	t.Helper()
	var f Fixture
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname) VALUES($1,'fake','Sys','Test') RETURNING id", uuid.NewString()+"@example.test").Scan(&f.User))
	must(pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Muhasebe test') RETURNING id", uuid.NewString()).Scan(&f.Org))
	return SeedInto(t, ctx, pool, f.Org, f.User)
}

// SeedInto seeds the concept and the customer into an existing organization.
func SeedInto(t *testing.T, ctx context.Context, pool *pgxpool.Pool, org, user int64) Fixture {
	t.Helper()
	f := Fixture{Org: org, User: user}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, "INSERT INTO custom_object_types(organization_id,key,singular_label,plural_label) VALUES($1,'fatura','Fatura','Faturalar') RETURNING uuid", f.Org).Scan(&f.ObjectTypeUUID))
	var company int64
	must(pool.QueryRow(ctx, "INSERT INTO companies(organization_id,name,external_id) VALUES($1,'Acme Ltd','COMP-7') RETURNING id", f.Org).Scan(&company))
	must(pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name,company_id) VALUES($1,'Ayşe Müşteri',$2) RETURNING id,uuid", f.Org, company).Scan(&f.Contact, &f.ContactUUID))
	_, err := pool.Exec(ctx, "INSERT INTO contact_identities(organization_id,contact_id,channel_type,external_id,last_seen_at) VALUES($1,$2,'widget','user:CUST-42',now())", f.Org, f.Contact)
	must(err)
	must(pool.QueryRow(ctx, "INSERT INTO conversations(organization_id,number,contact_id,channel_type,source) VALUES($1,1,$2,'widget','inbound') RETURNING id,uuid", f.Org, f.Contact).Scan(&f.Conversation, &f.ConvUUID))
	return f
}
