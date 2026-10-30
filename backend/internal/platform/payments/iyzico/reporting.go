package iyzico

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/money"
)

// ReportingTransaction is the minimal evidence retained from iyzico's official
// Reporting Service. Full cardholder/merchant response bodies are never stored.
type ReportingTransaction struct {
	Type           string      `json:"transactionType"`
	Date           string      `json:"transactionDate"`
	ID             json.Number `json:"transactionId"`
	PaymentID      json.Number `json:"paymentId"`
	ConversationID string      `json:"conversationId"`
	Status         int         `json:"transactionStatus"`
	Price          json.Number `json:"price"`
	PaidPrice      json.Number `json:"paidPrice"`
	Currency       string      `json:"transactionCurrency"`
}

// DailyTransactions follows all pages; truncated or inconsistent pagination is
// an error, so the nightly run cannot claim a complete merchant statement.
func (p *Provider) DailyTransactions(ctx context.Context, a payments.Account, day time.Time) ([]ReportingTransaction, error) {
	var result []ReportingTransaction
	for page := 1; page <= 10000; page++ {
		values := url.Values{"locale": {"tr"}, "transactionDate": {day.Format("2006-01-02")}, "page": {strconv.Itoa(page)}}
		path := "/v2/reporting/payment/transactions"
		query := values.Encode()
		nonceBytes := make([]byte, 16)
		if _, e := rand.Read(nonceBytes); e != nil {
			return nil, e
		}
		nonce := hex.EncodeToString(nonceBytes)
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, p.base(a)+path+"?"+query, strings.NewReader("{}"))
		if e != nil {
			return nil, e
		}
		req.Header.Set("Authorization", Authorization(a.Credentials["api_key"], a.Credentials["secret_key"], nonce, path, []byte("{}")))
		req.Header.Set("x-iyzi-rnd", nonce)
		req.Header.Set("Content-Type", "application/json")
		res, e := p.Client.Do(req)
		if e != nil {
			return nil, e
		}
		var out struct {
			apiResult
			Transactions   []ReportingTransaction `json:"transactions"`
			CurrentPage    int                    `json:"currentPage"`
			TotalPageCount int                    `json:"totalPageCount"`
		}
		decoder := json.NewDecoder(io.LimitReader(res.Body, 8<<20))
		decoder.UseNumber()
		e = decoder.Decode(&out)
		_ = res.Body.Close()
		if e != nil {
			return nil, e
		}
		if res.StatusCode != 200 {
			return nil, fmt.Errorf("iyzico reporting HTTP %d", res.StatusCode)
		}
		if out.Status != "success" {
			return nil, out.refusal()
		}
		if out.CurrentPage != page || out.TotalPageCount < page {
			if len(out.Transactions) == 0 && page == 1 && out.TotalPageCount == 0 {
				return result, nil
			}
			return nil, payments.ErrMalformed
		}
		for _, row := range out.Transactions {
			if row.Type != "PAYMENT" && row.Type != "REFUND" && row.Type != "CANCEL" {
				return nil, payments.ErrUnsupported
			}
			if row.ID.String() == "" || row.PaymentID.String() == "" || row.ConversationID == "" {
				return nil, payments.ErrMalformed
			}
			amount := row.PaidPrice
			if row.Type != "PAYMENT" {
				amount = row.Price
			}
			m, e := money.ParseRound(amount.String())
			if e != nil || m.Minor() <= 0 {
				return nil, payments.ErrMalformed
			}
		}
		result = append(result, out.Transactions...)
		if page == out.TotalPageCount {
			return result, nil
		}
	}
	return nil, fmt.Errorf("iyzico reporting pagination exceeds limit")
}
