package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/usecase"
)

func TestCompanyRevenueUpsertHistoryAndKeys(t *testing.T) {
	svc, _, org := setup(t)
	ctx := context.Background()
	acme, err := svc.CreateCompany(ctx, org, usecase.CompanyInput{Name: ptr("Acme"), Domain: ptr("acme.io"), ExternalID: ptr("cus_42")})
	if err != nil {
		t.Fatal(err)
	}
	if rev, hist, err := svc.GetRevenue(ctx, org, acme.UUID, 10); err != nil || rev != nil || len(hist) != 0 {
		t.Fatalf("fresh company has no revenue: %+v %+v %v", rev, hist, err)
	}

	renewal := time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC)
	rp := &renewal
	_, rev, err := svc.SetRevenue(ctx, org, usecase.CompanyKey{UUID: &acme.UUID}, usecase.RevenueInput{
		MRRMinor: ptr(int64(250000)), Currency: ptr("try"), Plan: ptr("Pro"), BillingStatus: ptr("active"),
		RenewalAt: &rp, Source: usecase.RevenueSourceManual,
	})
	if err != nil || rev.MRRMinor != 250000 || rev.Currency != "TRY" || rev.Source != "manual" || rev.RenewalAt == nil {
		t.Fatalf("manual set: %+v %v", rev, err)
	}

	// Integration push by external id: partial update keeps other fields.
	id, rev, err := svc.SetRevenue(ctx, org, usecase.CompanyKey{ExternalID: "cus_42"}, usecase.RevenueInput{
		BillingStatus: ptr("past_due"), Source: usecase.RevenueSourceAPI,
	})
	if err != nil || id != acme.UUID || rev.BillingStatus != "past_due" || rev.Plan != "Pro" || rev.MRRMinor != 250000 || rev.Source != "api" {
		t.Fatalf("api by external id: %v %+v %v", id, rev, err)
	}
	// By domain (normalized), same values: no new history entry.
	if _, _, err := svc.SetRevenue(ctx, org, usecase.CompanyKey{Domain: "https://www.ACME.io/billing"}, usecase.RevenueInput{
		BillingStatus: ptr("past_due"), Source: usecase.RevenueSourceStripe,
	}); err != nil {
		t.Fatalf("by domain: %v", err)
	}
	_, hist, err := svc.GetRevenue(ctx, org, acme.UUID, 10)
	if err != nil || len(hist) != 2 || hist[0].BillingStatus != "past_due" || hist[1].BillingStatus != "active" {
		t.Fatalf("history: %+v %v", hist, err)
	}

	// The company shape carries revenue in list and detail.
	got, _ := svc.GetCompany(ctx, org, acme.UUID)
	if got.Revenue == nil || got.Revenue.MRRMinor != 250000 {
		t.Fatalf("detail revenue: %+v", got.Revenue)
	}
	list, _, _ := svc.ListCompanies(ctx, org, "", 10, 0)
	if len(list) != 1 || list[0].Revenue == nil || list[0].Revenue.Source != "stripe" {
		t.Fatalf("list revenue: %+v", list)
	}

	// Validation and unknown companies.
	if _, _, err := svc.SetRevenue(ctx, org, usecase.CompanyKey{UUID: &acme.UUID}, usecase.RevenueInput{MRRMinor: ptr(int64(-1)), Source: "manual"}); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("negative mrr: %v", err)
	}
	if _, _, err := svc.SetRevenue(ctx, org, usecase.CompanyKey{UUID: &acme.UUID}, usecase.RevenueInput{BillingStatus: ptr("weird"), Source: "manual"}); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("bad status: %v", err)
	}
	if _, _, err := svc.SetRevenue(ctx, org, usecase.CompanyKey{ExternalID: "nope"}, usecase.RevenueInput{Source: "api"}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("unknown company: %v", err)
	}
	if _, _, err := svc.SetRevenue(ctx, org, usecase.CompanyKey{}, usecase.RevenueInput{Source: "api"}); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("no key: %v", err)
	}
	// Another organization cannot reach it.
	_, _, other := setup(t)
	if _, _, err := svc.SetRevenue(ctx, other, usecase.CompanyKey{UUID: &acme.UUID}, usecase.RevenueInput{Source: "manual"}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("cross org: %v", err)
	}

	if err := svc.ClearRevenue(ctx, org, acme.UUID); err != nil {
		t.Fatal(err)
	}
	if rev, hist, _ := svc.GetRevenue(ctx, org, acme.UUID, 10); rev != nil || len(hist) != 2 {
		t.Fatalf("cleared: %+v %d", rev, len(hist))
	}
}
