package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/money"
	"github.com/google/uuid"
)

// IyzicoStatementSource normalizes split basket transactions to our one-payment
// ledger. Refunds keep the refund conversation id; a provider's original-order
// reference cannot prove which ambiguous refund succeeded and stays orphaned.
type IyzicoStatementSource struct {
	Provider *iyzico.Provider
	Account  payments.Account
}

func (s *IyzicoStatementSource) Fetch(ctx context.Context, day time.Time) ([]StatementInput, error) {
	rows, err := s.Provider.DailyTransactions(ctx, s.Account, day)
	if err != nil {
		return nil, err
	}
	merchant, err := uuid.Parse(s.Account.UUID)
	if err != nil {
		return nil, err
	}
	type group struct {
		row StatementInput
		ids []string
	}
	groups := map[string]*group{}
	for _, row := range rows {
		operation := "charge"
		amount := row.PaidPrice
		if row.Type != "PAYMENT" {
			operation = "refund"
			amount = row.Price
		}
		value, e := money.ParseRound(amount.String())
		if e != nil {
			return nil, e
		}
		if row.Currency != "TRY" {
			return nil, payments.ErrUnsupported
		}
		observed, e := time.ParseInLocation("2006-01-02 15:04:05", row.Date, time.FixedZone("TRT", 3*3600))
		if e != nil {
			return nil, e
		}
		status := "succeeded"
		if operation == "charge" && row.Status != 2 {
			status = "pending"
		}
		key := operation + ":" + row.PaymentID.String() + ":" + row.ConversationID + ":" + status
		if operation == "refund" {
			key += ":" + row.Date
		}
		g := groups[key]
		if g == nil {
			g = &group{row: StatementInput{Provider: "iyzico", MerchantAccountUUID: merchant, OrderReference: row.ConversationID, ProviderReference: row.PaymentID.String(), Operation: operation, Status: status, Currency: row.Currency, ObservedAt: observed}}
			groups[key] = g
		}
		if value.Minor() > money.MaxMinor-g.row.AmountMinor {
			return nil, model.ErrInvalid
		}
		g.row.AmountMinor += value.Minor()
		g.ids = append(g.ids, row.ID.String())
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]StatementInput, 0, len(groups))
	for _, key := range keys {
		g := groups[key]
		sort.Strings(g.ids)
		hash := sha256.Sum256([]byte(strings.Join(g.ids, ":")))
		g.row.ExternalID = "iyzico:" + hex.EncodeToString(hash[:])
		out = append(out, g.row)
	}
	return out, nil
}
