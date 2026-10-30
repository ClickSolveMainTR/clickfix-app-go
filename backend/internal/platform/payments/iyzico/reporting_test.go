package iyzico_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments/iyzico"
)

func TestReportingPagesUseSDKGetSignature(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		nonce := r.Header.Get("x-iyzi-rnd")
		if r.Method != "GET" || r.URL.Path != "/v2/reporting/payment/transactions" || r.URL.Query().Get("transactionDate") != "2026-10-27" || string(body) != "{}" {
			t.Errorf("request %s %s %s", r.Method, r.URL, body)
		}
		expected := iyzico.Authorization("sandbox-api-key", secret, nonce, r.URL.Path, body)
		if r.Header.Get("Authorization") != expected {
			t.Error("GET signature differs from official SDK: sign path + {} without query")
		}
		_, _ = fmt.Fprintf(w, `{"status":"success","currentPage":%d,"totalPageCount":2,"transactions":[{"transactionType":"PAYMENT","transactionId":%d,"paymentId":42,"conversationId":"local-order","transactionDate":"2026-10-27 10:00:00","transactionStatus":2,"price":10,"paidPrice":10,"transactionCurrency":"TRY"}]}`, calls, calls)
	}))
	defer server.Close()
	p := iyzico.New(server.URL, server.Client())
	rows, e := p.DailyTransactions(context.Background(), account(), time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC))
	if e != nil || len(rows) != 2 || calls != 2 {
		t.Fatalf("rows=%d calls=%d error=%v", len(rows), calls, e)
	}
}
