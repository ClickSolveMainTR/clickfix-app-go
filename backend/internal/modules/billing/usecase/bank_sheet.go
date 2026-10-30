package usecase

import (
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/money"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Ported from takwim-app-go cash/adapters/statements.go.
var (
	amountNoise = regexp.MustCompile(`[^0-9,.\-+()]`)
	thousandsRE = regexp.MustCompile(`^\d{1,3}(\.\d{3})+$`)
	commaGroups = regexp.MustCompile(`^\d{1,3}(,\d{3})+$`)
	dateLayouts = []string{
		"2006-01-02", "02.01.2006", "02/01/2006", "02-01-2006", "2006/01/02", "2006.01.02",
		"02.01.2006 15:04", "02.01.2006 15:04:05", "02/01/2006 15:04", "2006-01-02 15:04", "2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05", "02.01.06", "02/01/06", "2 January 2006", "2 Jan 2006",
	}
)

// parseSheetAmount reads an amount the way a bank or a provider writes it:
// currency marks and spaces are dropped, "(250,00)" and "250,00-" are
// negative, and the decimal separator is the last of "," and "." (a lone
// "." followed by three digits groups thousands, Turkish style).
func parseSheetAmount(raw string) (money.Money, error) {
	s := strings.TrimSpace(raw)
	if !regexp.MustCompile(`^[0-9., +\-()₺]*(TL|TRY)?$`).MatchString(s) {
		return money.Money{}, money.ErrInvalid
	}
	s = strings.NewReplacer(" ", "", " ", "", " ", "").Replace(s)
	neg := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg, s = true, strings.TrimSuffix(strings.TrimPrefix(s, "("), ")")
	}
	s = amountNoise.ReplaceAllString(s, "")
	if strings.HasSuffix(s, "-") {
		neg, s = true, strings.TrimSuffix(s, "-")
	}
	if strings.HasPrefix(s, "-") {
		neg, s = true, strings.TrimPrefix(s, "-")
	}
	s = strings.TrimPrefix(s, "+")
	if s == "" {
		return money.Money{}, money.ErrInvalid
	}
	comma, dot := strings.LastIndex(s, ","), strings.LastIndex(s, ".")
	switch {
	case comma >= 0 && dot >= 0:
		if comma > dot {
			s = strings.ReplaceAll(s, ".", "")
		} else {
			s = strings.ReplaceAll(s, ",", "")
		}
	case dot >= 0 && thousandsRE.MatchString(s):
		s = strings.ReplaceAll(s, ".", "")
	case comma >= 0 && commaGroups.MatchString(s) && strings.Count(s, ",") > 1:
		s = strings.ReplaceAll(s, ",", "")
	}
	m, err := money.Parse(s)
	if err != nil {
		return money.Money{}, err
	}
	if neg {
		m = m.Neg()
	}
	return m, nil
}

// parseSheetDate reads the date layouts banks use and Excel serial dates.
func parseSheetDate(raw string) (time.Time, error) {
	s := strings.TrimSpace(raw)
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
		}
	}
	// Excel stores dates as days since 1899-12-30 (serial 1 = 1900-01-01).
	if n, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64); err == nil && n >= 20000 && n < 80000 {
		return time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(n)), nil
	}
	return time.Time{}, fmt.Errorf("not a date")
}
