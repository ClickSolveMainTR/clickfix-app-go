package engine

import (
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/businesshours"
)

var t0 = time.Date(2026, 10, 27, 7, 0, 0, 0, time.UTC) // Tuesday 10:00 Istanbul

func pol(cal *businesshours.Calendar) *Policy {
	return &Policy{ID: 7, WarningPercent: 50, Calendar: cal, Targets: Targets{
		FirstResponse: time.Hour, NextResponse: 30 * time.Minute, Resolution: 8 * time.Hour,
	}}
}

// apply runs Plan and folds the result into the stored set, like the service.
func apply(t *testing.T, conv Conversation, stored map[string]Clock, p *Policy, at time.Time) (map[string]Clock, []Transition) {
	t.Helper()
	res := Plan(conv, stored, p, at)
	out := map[string]Clock{}
	for k, v := range res.Clocks {
		out[k] = v
	}
	return out, res.Transitions
}

func kinds(trs []Transition) []string {
	out := []string{}
	for _, tr := range trs {
		out = append(out, tr.Kind+":"+tr.Metric)
	}
	return out
}

func TestNewConversationStartsClocks(t *testing.T) {
	conv := Conversation{Status: "open", Source: "inbound", CreatedAt: t0}
	clocks, trs := apply(t, conv, nil, pol(nil), t0)
	if len(clocks) != 2 || len(trs) != 2 {
		t.Fatalf("clocks=%v trs=%v", clocks, kinds(trs))
	}
	fr := clocks[FirstResponse]
	if !fr.DueAt.Equal(t0.Add(time.Hour)) || !fr.WarnAt.Equal(t0.Add(30*time.Minute)) {
		t.Fatalf("first response due=%s warn=%s", fr.DueAt, fr.WarnAt)
	}
	if _, ok := clocks[NextResponse]; ok {
		t.Fatal("next response must wait for the first answer")
	}
	// Replaying the same state changes nothing.
	res := Plan(conv, clocks, pol(nil), t0.Add(5*time.Minute))
	if len(res.Upsert) != 0 || len(res.Transitions) != 0 {
		t.Fatalf("replay changed: %+v", res)
	}
}

func TestOutboundConversationHasNoFirstResponse(t *testing.T) {
	clocks, _ := apply(t, Conversation{Status: "open", Source: "outbound", CreatedAt: t0}, nil, pol(nil), t0)
	if _, ok := clocks[FirstResponse]; ok {
		t.Fatal("outbound conversation measured first response")
	}
}

func TestFirstAnswerMeetsAndNextResponseCycles(t *testing.T) {
	p := pol(nil)
	conv := Conversation{Status: "open", Source: "inbound", CreatedAt: t0}
	clocks, _ := apply(t, conv, nil, p, t0)

	answered := t0.Add(20 * time.Minute)
	conv.FirstResponseAt, conv.LastOutboundAt = &answered, &answered
	clocks, trs := apply(t, conv, clocks, p, answered)
	if got := kinds(trs); len(got) != 1 || got[0] != "met:first_response" {
		t.Fatalf("transitions %v", got)
	}

	// Customer writes again: next response cycle 1.
	w1 := t0.Add(time.Hour)
	conv.WaitingSince = &w1
	clocks, _ = apply(t, conv, clocks, p, w1)
	nr := clocks[NextResponse]
	if nr.Cycle != 1 || !nr.DueAt.Equal(w1.Add(30*time.Minute)) {
		t.Fatalf("next response %+v", nr)
	}

	// Agent answers late (45m): breached at the due time.
	a2 := w1.Add(45 * time.Minute)
	conv.WaitingSince, conv.LastOutboundAt = nil, &a2
	clocks, trs = apply(t, conv, clocks, p, a2)
	if got := kinds(trs); len(got) != 1 || got[0] != "breached:next_response" {
		t.Fatalf("transitions %v", got)
	}
	if !clocks[NextResponse].BreachedAt.Equal(w1.Add(30 * time.Minute)) {
		t.Fatal("breach time must be the due time")
	}

	// Another turn starts cycle 2 with a fresh clock.
	w3 := a2.Add(time.Hour)
	conv.WaitingSince = &w3
	clocks, trs = apply(t, conv, clocks, p, w3)
	if c := clocks[NextResponse]; c.Cycle != 2 || c.BreachedAt != nil || c.CompletedAt != nil {
		t.Fatalf("cycle 2 %+v (%v)", c, kinds(trs))
	}
}

func TestPendingPausesAndResumes(t *testing.T) {
	p := pol(nil)
	conv := Conversation{Status: "open", Source: "inbound", CreatedAt: t0}
	clocks, _ := apply(t, conv, nil, p, t0)

	// 2h in, the agent sets pending for 5h.
	answered := t0.Add(10 * time.Minute)
	conv.FirstResponseAt, conv.LastOutboundAt = &answered, &answered
	conv.Status = "pending"
	pauseAt := t0.Add(2 * time.Hour)
	clocks, _ = apply(t, conv, clocks, p, pauseAt)
	res := clocks[Resolution]
	if res.PausedAt == nil || res.DueAt != nil || res.Consumed != 2*time.Hour {
		t.Fatalf("paused clock %+v", res)
	}
	if left := res.Remaining(nil, pauseAt.Add(3*time.Hour)); left != 6*time.Hour {
		t.Fatalf("remaining while paused %s", left)
	}

	conv.Status = "open"
	resume := pauseAt.Add(5 * time.Hour)
	clocks, _ = apply(t, conv, clocks, p, resume)
	res = clocks[Resolution]
	if res.PausedAt != nil || !res.DueAt.Equal(resume.Add(6*time.Hour)) {
		t.Fatalf("resumed clock %+v", res)
	}
}

func TestResolveAndReopen(t *testing.T) {
	p := pol(nil)
	conv := Conversation{Status: "open", Source: "inbound", CreatedAt: t0}
	clocks, _ := apply(t, conv, nil, p, t0)
	resolved := t0.Add(3 * time.Hour)
	conv.Status, conv.ResolvedAt = "resolved", &resolved
	clocks, trs := apply(t, conv, clocks, p, resolved)
	// First response never answered: stopped by the resolution too.
	if got := kinds(trs); len(got) != 2 {
		t.Fatalf("transitions %v", got)
	}
	if clocks[Resolution].Consumed != 3*time.Hour {
		t.Fatalf("consumed %s", clocks[Resolution].Consumed)
	}
	reopen := resolved.Add(24 * time.Hour)
	conv.Status, conv.ResolvedAt = "open", nil
	clocks, _ = apply(t, conv, clocks, p, reopen)
	if r := clocks[Resolution]; r.CompletedAt != nil || !r.DueAt.Equal(reopen.Add(5*time.Hour)) {
		t.Fatalf("reopened resolution %+v", r)
	}
}

func TestBusinessHoursDeadline(t *testing.T) {
	cal, err := businesshours.New("Europe/Istanbul", businesshours.Week{
		"mon": {Open: "09:00", Close: "18:00"}, "tue": {Open: "09:00", Close: "18:00"},
		"wed": {Open: "09:00", Close: "18:00"}, "thu": {Open: "09:00", Close: "18:00"},
		"fri": {Open: "09:00", Close: "18:00"},
	}, []businesshours.Exception{{Date: "2026-10-28", Open: "00:00", Close: businesshours.HalfDayClose}, {Date: "2026-10-29", Closed: true}})
	if err != nil {
		t.Fatal(err)
	}
	// Tuesday 17:30 Istanbul, resolution 8h: 0.5h Tue + 4h Wed (half day) +
	// Thu holiday + 3.5h Fri → Friday 12:30 Istanbul (09:30 UTC).
	start := time.Date(2026, 10, 27, 14, 30, 0, 0, time.UTC)
	clocks, _ := apply(t, Conversation{Status: "open", Source: "inbound", CreatedAt: start}, nil, pol(cal), start)
	if want := time.Date(2026, 10, 30, 9, 30, 0, 0, time.UTC); !clocks[Resolution].DueAt.Equal(want) {
		t.Fatalf("due %s want %s", clocks[Resolution].DueAt, want)
	}
}

func TestPolicyRemovedDropsRunningClocks(t *testing.T) {
	conv := Conversation{Status: "open", Source: "inbound", CreatedAt: t0}
	clocks, _ := apply(t, conv, nil, pol(nil), t0)
	res := Plan(conv, clocks, nil, t0.Add(time.Minute))
	if len(res.Delete) != 2 || len(res.Clocks) != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestRetargetMovesDeadline(t *testing.T) {
	conv := Conversation{Status: "open", Source: "inbound", CreatedAt: t0}
	clocks, _ := apply(t, conv, nil, pol(nil), t0)
	urgent := pol(nil)
	urgent.Targets.FirstResponse = 15 * time.Minute
	res := Plan(conv, clocks, urgent, t0.Add(time.Minute))
	if !res.Clocks[FirstResponse].DueAt.Equal(t0.Add(15 * time.Minute)) {
		t.Fatalf("due %s", res.Clocks[FirstResponse].DueAt)
	}
}
