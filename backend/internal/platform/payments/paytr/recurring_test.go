package paytr_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments/paytr"
)

func TestRecurringWaitCallbackIsUnknownAndStatusQueryNeverCharges(t *testing.T) {
	calls := 0
	status := "wait_callback"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/odeme":
			calls++
			if r.Form.Get("recurring_payment") != "1" || r.Form.Get("non_3d") != "1" || r.Form.Get("payment_amount") != "12.50" {
				t.Errorf("form %+v", r.Form)
			}
			want := hmacB64("KEY123", "123456127.0.0.1refbuyer@example.test12.50card0TL11SALT456")
			if r.Form.Get("paytr_token") != want {
				t.Errorf("signature %s", r.Form.Get("paytr_token"))
			}
			_, _ = fmt.Fprintf(w, `{"status":%q}`, status)
		case "/odeme/durum-sorgu":
			if r.Form.Get("paytr_token") != hmacB64("KEY123", "123456refSALT456") {
				t.Error("status signature")
			}
			_, _ = w.Write([]byte(`{"status":"success","payment_amount":"12.50","payment_total":"12.50","currency":"TL"}`))
		case "/odeme/capi/list":
			if r.Form.Get("paytr_token") != hmacB64("KEY123", "ownerSALT456") {
				t.Error("list signature")
			}
			_, _ = w.Write([]byte(`[{"ctoken":"card","last_4":"4242"}]`))
		case "/odeme/capi/delete":
			if r.Form.Get("paytr_token") != hmacB64("KEY123", "cardownerSALT456") {
				t.Error("delete signature")
			}
			_, _ = w.Write([]byte(`{"status":"success"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	p := paytr.New(srv.URL, srv.Client())
	in := payments.ChargeRequest{OrderRef: "ref", Amount: 1250, Currency: "TRY", Buyer: payments.Buyer{IP: "127.0.0.1", Email: "buyer@example.test"}, Card: payments.SavedCard{OwnerKey: "owner", Token: "card"}, Recurring: true}
	var pe *payments.ProviderError
	if _, err := p.ChargeSavedCard(context.Background(), account, in); !errors.As(err, &pe) || !pe.Temporary {
		t.Fatalf("wait_callback %v", err)
	}
	n, err := p.CheckCharge(context.Background(), account, in)
	if err != nil || n.Status != payments.StatusSucceeded || n.Amount != 1250 || calls != 1 {
		t.Fatalf("query %+v %v calls %d", n, err, calls)
	}
	cards, err := p.ListSavedCards(context.Background(), account, "owner")
	if err != nil || len(cards) != 1 || cards[0].Token != "card" {
		t.Fatalf("cards %+v %v", cards, err)
	}
	if err = p.RemoveSavedCard(context.Background(), account, in.Card); err != nil {
		t.Fatal(err)
	}
	status = "failed"
	if _, err = p.ChargeSavedCard(context.Background(), account, in); !errors.As(err, &pe) || pe.Temporary {
		t.Fatalf("decline %v", err)
	}
}
