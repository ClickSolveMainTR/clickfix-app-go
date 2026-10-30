package usecase_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCompanyTimelineAndOverview(t *testing.T) {
	svc, _, org := setup(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	acme, err := svc.CreateCompany(ctx, org, usecase.CompanyInput{Name: ptr("Acme"), Domain: ptr("acme.io"), ExternalID: ptr("cus_1")})
	if err != nil {
		t.Fatal(err)
	}
	ayse, err := svc.CreateContact(ctx, org, usecase.ContactInput{Name: ptr("Ayşe"), Email: ptr("ayse@acme.io")})
	if err != nil || ayse.Company == nil || ayse.Company.UUID != acme.UUID {
		t.Fatalf("contact adopted by domain: %+v %v", ayse, err)
	}
	var contactID, userID int64
	if err := pool.QueryRow(ctx, "SELECT id FROM contacts WHERE uuid = $1", ayse.UUID).Scan(&contactID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname, status)
		VALUES ($1, 'x', 'Cem', 'Kaya', 'active') RETURNING id`, "csm-"+uuid.NewString()[:8]+"@example.com").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID) })

	base := time.Now().Add(-48 * time.Hour).UTC().Truncate(time.Second)
	// An open conversation linked only through the contact (company_id
	// NULL) and a resolved one that names the company.
	var openConv, doneConv int64
	if err := pool.QueryRow(ctx, `INSERT INTO conversations (organization_id, number, channel_type, contact_id, subject, status, created_at, last_message_at)
		VALUES ($1, 1, 'email', $2, 'Fatura sorunu', 'open', $3, $3) RETURNING id`, org, contactID, base).Scan(&openConv); err != nil {
		t.Fatal(err)
	}
	var companyID int64
	if err := pool.QueryRow(ctx, "SELECT id FROM companies WHERE uuid = $1", acme.UUID).Scan(&companyID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO conversations (organization_id, number, channel_type, contact_id, company_id, subject, status, created_at, last_message_at, resolved_at)
		VALUES ($1, 2, 'whatsapp', $2, $3, 'Kurulum', 'resolved', $4, $5, $5) RETURNING id`,
		org, contactID, companyID, base.Add(time.Hour), base.Add(2*time.Hour)).Scan(&doneConv); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO conversation_ratings (organization_id, conversation_id, contact_id, score, comment, created_at)
		VALUES ($1, $2, $3, 4, 'Hızlıydı', $4)`, org, doneConv, contactID, base.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}

	_, note, err := svc.AddNote(ctx, org, acme.UUID, userID, "  Yenileme görüşmesi planlandı  ")
	if err != nil || note.Body != "Yenileme görüşmesi planlandı" || note.Author == nil || note.Author.Name != "Cem Kaya" {
		t.Fatalf("note: %+v %v", note, err)
	}
	if _, _, err := svc.AddNote(ctx, org, acme.UUID, userID, "   "); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("empty note: %v", err)
	}

	occurred := base.Add(4 * time.Hour)
	ev, err := svc.RecordEvent(ctx, org, usecase.CompanyEventInput{
		Company: usecase.CompanyKey{ExternalID: "cus_1"}, ContactEmail: "AYSE@acme.io", Name: "seat_limit_reached",
		Properties: map[string]any{"seats": 10}, IdempotencyKey: "evt-1", OccurredAt: &occurred,
	})
	if err != nil || ev.Duplicate || ev.CompanyUUID != acme.UUID || ev.Source != "api" {
		t.Fatalf("event: %+v %v", ev, err)
	}
	again, err := svc.RecordEvent(ctx, org, usecase.CompanyEventInput{
		Company: usecase.CompanyKey{Domain: "acme.io"}, Name: "seat_limit_reached", IdempotencyKey: "evt-1",
	})
	if err != nil || !again.Duplicate || again.UUID != ev.UUID {
		t.Fatalf("idempotent retry: %+v %v", again, err)
	}
	future := time.Now().Add(time.Hour)
	if _, err := svc.RecordEvent(ctx, org, usecase.CompanyEventInput{Company: usecase.CompanyKey{UUID: &acme.UUID}, Name: "x", OccurredAt: &future}); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("future event: %v", err)
	}
	if _, _, err := svc.SetRevenue(ctx, org, usecase.CompanyKey{UUID: &acme.UUID}, usecase.RevenueInput{
		MRRMinor: ptr(int64(990000)), Currency: ptr("TRY"), Plan: ptr("Business"), Source: usecase.RevenueSourceManual,
	}); err != nil {
		t.Fatal(err)
	}

	page, err := svc.Timeline(ctx, org, acme.UUID, usecase.TimelineQuery{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, it := range page.Items {
		got[it.Kind]++
	}
	want := map[string]int{"conversation_opened": 2, "conversation_resolved": 1, "csat": 1, "note": 1, "event": 1, "revenue": 1}
	for k, n := range want {
		if got[k] != n {
			t.Fatalf("kind %s: got %d want %d (%+v)", k, got[k], n, got)
		}
	}
	for i := 1; i < len(page.Items); i++ {
		if page.Items[i].OccurredAt.After(page.Items[i-1].OccurredAt) {
			t.Fatal("timeline must be newest first")
		}
	}
	if page.NextBefore != nil {
		t.Fatal("single page expected")
	}
	for _, it := range page.Items {
		if it.Kind == "csat" && (it.Data["score"] != float64(4) || it.Body != "Hızlıydı" || it.Actor != "Ayşe") {
			t.Fatalf("csat item: %+v", it)
		}
		if it.Kind == "event" && (it.Title != "seat_limit_reached" || it.Actor != "Ayşe") {
			t.Fatalf("event item: %+v", it)
		}
	}

	// Kind filter and paging.
	only, err := svc.Timeline(ctx, org, acme.UUID, usecase.TimelineQuery{Kinds: []string{"conversation_opened"}, Limit: 1})
	if err != nil || len(only.Items) != 1 || only.Items[0].Title != "Kurulum" || only.NextBefore == nil {
		t.Fatalf("filtered first page: %+v %v", only, err)
	}
	next, err := svc.Timeline(ctx, org, acme.UUID, usecase.TimelineQuery{Kinds: []string{"conversation_opened"}, Limit: 1, Before: *only.NextBefore})
	if err != nil || len(next.Items) != 1 || next.Items[0].Title != "Fatura sorunu" {
		t.Fatalf("second page: %+v %v", next, err)
	}
	// Events sharing a timestamp page one by one without loss.
	tie := base.Add(5 * time.Hour)
	for _, n := range []string{"tie_a", "tie_b", "tie_c"} {
		if _, err := svc.RecordEvent(ctx, org, usecase.CompanyEventInput{Company: usecase.CompanyKey{UUID: &acme.UUID}, Name: n, OccurredAt: &tie}); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for range 10 {
		p, err := svc.Timeline(ctx, org, acme.UUID, usecase.TimelineQuery{Kinds: []string{"event"}, Limit: 1, Before: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range p.Items {
			if seen[it.Title] {
				t.Fatalf("event %s repeated", it.Title)
			}
			seen[it.Title] = true
		}
		if p.NextBefore == nil {
			break
		}
		cursor = *p.NextBefore
	}
	if len(seen) != 4 {
		t.Fatalf("paging over tied events: %v", seen)
	}
	if _, err := svc.Timeline(ctx, org, acme.UUID, usecase.TimelineQuery{Before: "%%"}); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("bad cursor: %v", err)
	}
	if _, err := svc.Timeline(ctx, org, acme.UUID, usecase.TimelineQuery{Kinds: []string{"bogus"}}); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("unknown kind: %v", err)
	}

	ov, err := svc.Overview(ctx, org, acme.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if ov.Stats.OpenConversations != 1 || ov.Stats.ClosedConversations != 1 || ov.Stats.CSATCount != 1 ||
		ov.Stats.CSATAverage == nil || *ov.Stats.CSATAverage != 4 || len(ov.OpenConversations) != 1 ||
		len(ov.RecentlyClosed) != 1 || ov.OpenConversations[0].Contact.Name != "Ayşe" || ov.Company.Revenue == nil {
		t.Fatalf("overview: %+v", ov)
	}

	// Only the author (or a contacts deleter) removes a note; other orgs see nothing.
	if err := svc.DeleteNote(ctx, org, note.UUID, userID+1000000, false); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatalf("foreign delete: %v", err)
	}
	_, _, other := setup(t)
	if _, err := svc.Timeline(ctx, other, acme.UUID, usecase.TimelineQuery{}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("cross org timeline: %v", err)
	}
	if err := svc.DeleteNote(ctx, other, note.UUID, userID, true); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("cross org note: %v", err)
	}
	if err := svc.DeleteNote(ctx, org, note.UUID, userID, false); err != nil {
		t.Fatal(err)
	}
	if notes, _ := svc.Timeline(ctx, org, acme.UUID, usecase.TimelineQuery{Kinds: []string{"note"}}); len(notes.Items) != 0 {
		t.Fatalf("note must be gone: %+v", notes)
	}
}
