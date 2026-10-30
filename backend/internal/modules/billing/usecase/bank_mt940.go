package usecase

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/iban"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/money"
)

// MT940 :61: normalized credits. Bank-specific structured :86: information is
// retained as description; only explicit IBAN markers are interpreted. Reversal
// and debit entries cannot authorize incoming payments. NONREF entries need a
// bank reference after // for stable deduplication.
var mt940Line = regexp.MustCompile(`^(\d{6})(\d{4})?(R?[CD])([A-Z])?(\d+,\d{0,2})([NSF][A-Z0-9]{3})([^/]{1,16})(?://(.{1,16}))?$`)

func parseMT940(source io.Reader) ([]BankStatementInput, error) {
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 4096), 64<<10)
	out := []BankStatementInput{}
	account, currency, lastTag := "", "", ""
	current := -1
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(text, ":25:"):
			account = iban.Normalize(strings.Trim(strings.TrimPrefix(text, ":25:"), "/ "))
			if iban.Validate(account) != nil {
				return nil, model.ErrInvalid
			}
			current = -1
			lastTag = "25"
		case strings.HasPrefix(text, ":60F:"), strings.HasPrefix(text, ":60M:"):
			value := text[5:]
			if len(value) < 10 {
				return nil, model.ErrInvalid
			}
			currency = value[7:10]
			if currency != "TRY" {
				return nil, model.ErrInvalid
			}
			current = -1
			lastTag = "60"
		case strings.HasPrefix(text, ":61:"):
			lastTag = "61"
			current = -1
			fields := mt940Line.FindStringSubmatch(strings.TrimPrefix(text, ":61:"))
			if fields == nil || account == "" || currency != "TRY" {
				return nil, model.ErrInvalid
			}
			if fields[3] != "C" {
				continue
			}
			date, err := time.Parse("060102", fields[1])
			if err != nil {
				return nil, model.ErrInvalid
			}
			if fields[2] != "" {
				// Entry dates have no year. Resolve December/January around value date.
				booked, err := time.Parse("20060102", date.Format("2006")+fields[2])
				if err != nil {
					return nil, model.ErrInvalid
				}
				if booked.Sub(date) > 180*24*time.Hour {
					booked = booked.AddDate(-1, 0, 0)
				}
				if date.Sub(booked) > 180*24*time.Hour {
					booked = booked.AddDate(1, 0, 0)
				}
				date = booked
			}
			amount, err := money.Parse(strings.ReplaceAll(fields[5], ",", "."))
			if err != nil {
				return nil, model.ErrInvalid
			}
			external := strings.TrimSpace(fields[8])
			if external == "" {
				external = strings.TrimSpace(fields[7])
			}
			if external == "" || external == "NONREF" {
				return nil, model.ErrInvalid
			}
			row := BankStatementInput{ExternalID: external, Reference: strings.TrimSpace(fields[7]), AmountMinor: amount.Minor(), Currency: currency, BookedAt: date, DestinationIBAN: account}
			if row.Reference == "NONREF" {
				row.Reference = ""
			}
			if !row.valid() {
				return nil, model.ErrInvalid
			}
			out = append(out, row)
			current = len(out) - 1
			if len(out) > 1000 {
				return nil, model.ErrInvalid
			}
		case strings.HasPrefix(text, ":86:"):
			lastTag = "86"
			if current >= 0 {
				out[current].Description += " " + strings.TrimPrefix(text, ":86:")
			}
		case strings.HasPrefix(text, ":"):
			lastTag = "other"
			current = -1
		default:
			if lastTag == "86" && current >= 0 {
				out[current].Description += " " + text
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: MT940: %s", model.ErrInvalid, err)
	}
	if len(out) == 0 {
		return nil, model.ErrInvalid
	}
	for _, row := range out {
		if !row.valid() {
			return nil, model.ErrInvalid
		}
	}
	return out, nil
}
