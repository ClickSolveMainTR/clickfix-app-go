package usecase

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/ubl"
	"github.com/jackc/pgx/v5/pgtype"
)

type BillingAddress struct {
	Street     string `json:"street"`
	District   string `json:"district"`
	City       string `json:"city"`
	PostalCode string `json:"postal_code"`
}
type ProfileInput struct {
	Kind            string         `json:"kind"`
	LegalName       string         `json:"legal_name"`
	TaxID           string         `json:"tax_id"`
	TaxOffice       string         `json:"tax_office"`
	CountryCode     string         `json:"country_code"`
	Address         BillingAddress `json:"address"`
	Emails          []string       `json:"emails"`
	ExportExemption bool           `json:"export_exemption"`
}

func (in ProfileInput) Validate() error {
	if in.Kind != "individual" && in.Kind != "company" || strings.TrimSpace(in.LegalName) == "" || len(in.LegalName) > 240 || len(in.CountryCode) != 2 || in.CountryCode != strings.ToUpper(in.CountryCode) || in.Address.Street == "" || in.Address.City == "" || len(in.Emails) == 0 || len(in.Emails) > 5 {
		return model.ErrInvalid
	}
	for _, email := range in.Emails {
		a, e := mail.ParseAddress(email)
		if e != nil || a.Address != email {
			return model.ErrInvalid
		}
	}
	if in.CountryCode == "TR" {
		if in.ExportExemption || in.Kind == "company" && (!ubl.ValidVKN(in.TaxID) || in.TaxOffice == "") || in.Kind == "individual" && !ubl.ValidTCKN(in.TaxID) {
			return model.ErrInvalid
		}
	} else if len(in.TaxID) > 40 || in.TaxID == "" {
		return model.ErrInvalid
	}
	return nil
}
func profileInput(p db.BillingProfile) ProfileInput {
	in := ProfileInput{Kind: p.Kind, LegalName: p.LegalName, TaxID: p.TaxID, TaxOffice: p.TaxOffice, CountryCode: p.CountryCode, Emails: p.Emails, ExportExemption: p.ExportExemption}
	_ = json.Unmarshal(p.Address, &in.Address)
	if len(in.Emails) == 0 && p.Email != "" {
		in.Emails = []string{p.Email}
	}
	return in
}
func (s *Service) Profile(ctx context.Context, org int64) (ProfileInput, error) {
	p, e := s.repo.Q.BillingProfile(ctx, org)
	return profileInput(p), e
}
func (s *Service) SaveProfile(ctx context.Context, org int64, in ProfileInput) (ProfileInput, error) {
	if err := in.Validate(); err != nil {
		return in, err
	}
	address, err := json.Marshal(in.Address)
	if err != nil {
		return in, err
	}
	_, err = s.repo.Q.BillingSaveProfile(ctx, db.BillingSaveProfileParams{OrganizationID: org, LegalName: in.LegalName, TaxID: in.TaxID, TaxOffice: in.TaxOffice, Address: address, Email: in.Emails[0], Kind: in.Kind, CountryCode: in.CountryCode, Emails: in.Emails, ExportExemption: in.ExportExemption})
	return in, err
}

// FetchTCMB reads the fixed official endpoint through a bounded client. The
// published business date is used, so weekends never create invented rates.
func (s *Service) FetchTCMB(ctx context.Context) error {
	return s.FetchRates(ctx, &http.Client{Timeout: 20 * time.Second}, "https://www.tcmb.gov.tr/kurlar/today.xml")
}
func (s *Service) FetchRates(ctx context.Context, client *http.Client, endpoint string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("TCMB: HTTP %d", res.StatusCode)
	}
	var doc struct {
		Date       string `xml:"Date,attr"`
		Currencies []struct {
			Code    string `xml:"CurrencyCode,attr"`
			Unit    string `xml:"Unit"`
			Selling string `xml:"ForexSelling"`
		} `xml:"Currency"`
	}
	if err = xml.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&doc); err != nil {
		return err
	}
	date, err := time.Parse("01/02/2006", doc.Date)
	if err != nil {
		return err
	}
	if date.After(time.Now().UTC().Add(24 * time.Hour)) {
		return model.ErrInvalid
	}
	rates := map[string]pgtype.Numeric{}
	for _, c := range doc.Currencies {
		if c.Code != "USD" && c.Code != "EUR" {
			continue
		}
		r, ok := new(big.Rat).SetString(c.Selling)
		u, uok := new(big.Rat).SetString(c.Unit)
		if !ok || !uok || r.Sign() <= 0 || u.Sign() <= 0 {
			return model.ErrInvalid
		}
		r.Quo(r, u)
		var n pgtype.Numeric
		if err = n.Scan(r.FloatString(8)); err != nil {
			return err
		}
		rates[c.Code] = n
	}
	if len(rates) != 2 {
		return fmt.Errorf("TCMB: missing USD/EUR")
	}
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		for code, n := range rates {
			if e := r.Q.BillingSaveFX(ctx, db.BillingSaveFXParams{Currency: code, Rate: n, ObservedOn: pgtype.Date{Time: date, Valid: true}}); e != nil {
				return e
			}
		}
		return nil
	})
}
