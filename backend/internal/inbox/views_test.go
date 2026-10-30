package inbox_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
)

func TestSavedViewsAndSLAFilter(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	for _, id := range []string{"sv-1", "sv-2"} {
		body := `{"messages":[{"id":"` + id + `","from":{"id":"` + id + `-v"},"text":"merhaba"}]}`
		if code := e.post(t, body, ""); code != http.StatusOK {
			t.Fatalf("webhook = %d", code)
		}
	}
	// One conversation is due within 30 minutes, the other in two days.
	if _, err := e.pool.Exec(ctx, `UPDATE conversations SET first_response_due_at = NOW() + CASE WHEN number = 1 THEN interval '30 minutes' ELSE interval '2 days' END WHERE organization_id = $1`, e.org); err != nil {
		t.Fatal(err)
	}
	soon, total, err := e.ib.Conversations.List(ctx, e.org, e.agent, convusecase.ListFilter{SLADueWithin: time.Hour})
	if err != nil || total != 1 || soon[0].Number != 1 {
		t.Fatalf("sla due within an hour: %d (%v)", total, err)
	}

	conv := e.ib.Conversations
	if _, err := conv.CreateView(ctx, e.org, e.agent, false, convusecase.ViewInput{Scope: "organization", Name: "x", Filters: convusecase.ViewFilters{}}); !errors.Is(err, convusecase.ErrForbidden) {
		t.Fatalf("shared view without manage = %v", err)
	}
	if _, err := conv.CreateView(ctx, e.org, e.agent, false, convusecase.ViewInput{Name: "bad", Filters: convusecase.ViewFilters{Statuses: []string{"solved"}}}); !errors.Is(err, convusecase.ErrInvalid) {
		t.Fatalf("invalid status = %v", err)
	}
	v, err := conv.CreateView(ctx, e.org, e.agent, false, convusecase.ViewInput{Name: "SLA yakın", Filters: convusecase.ViewFilters{SLADueWithinMinutes: 60}})
	if err != nil || !v.Mine || v.Scope != "personal" {
		t.Fatalf("personal view: %+v %v", v, err)
	}
	views, _ := conv.Views(ctx, e.org, e.agent)
	if len(views) != 1 || views[0].Filters.SLADueWithinMinutes != 60 {
		t.Fatalf("views = %+v", views)
	}
	counts, err := conv.ViewCounts(ctx, e.org, e.agent)
	if err != nil || counts[v.UUID.String()] != 1 {
		t.Fatalf("view counts = %v (%v)", counts, err)
	}
	if err := conv.DeleteView(ctx, e.org, e.agent, false, v.UUID); err != nil {
		t.Fatal(err)
	}
}
