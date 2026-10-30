package engine

import (
	"testing"
	"time"
)

func ip(v int) *int { return &v }

func TestComputeHealthyAndRenormalized(t *testing.T) {
	now := time.Now()
	// No CSAT, no billing, no usage: only open/sla/frequency count, all perfect.
	r := Compute(Inputs{}, DefaultWeights(), now)
	if r.Score == nil || *r.Score != 100 {
		t.Fatalf("empty company should be healthy: %+v", r.Score)
	}
	if r.Signals[SignalCSAT].Used || r.Signals[SignalBilling].Used || r.Signals[SignalUsage].Used {
		t.Fatal("signals without data must not count")
	}
}

func TestComputeAtRisk(t *testing.T) {
	now := time.Now()
	old := now.Add(-40 * 24 * time.Hour)
	in := Inputs{
		CSATAvg: 1.5, CSATCount: 4, OpenConversations: 5, SLABreaches: 3,
		RecentConversations: 12, PreviousConversations: 4, BillingStatus: "past_due",
		UsageScore: ip(90), UsageReportedAt: &old, // stale: ignored
	}
	r := Compute(in, DefaultWeights(), now)
	// csat 13*25 + open 25*15 + sla 40*20 + freq 0*15 + billing 25*15 = 1875 / 90 = 20.8
	if r.Score == nil || *r.Score != 21 {
		t.Fatalf("score = %v %+v", r.Score, r.Signals)
	}
	if r.Signals[SignalUsage].Used {
		t.Fatal("stale usage counted")
	}
	fresh := now.Add(-time.Hour)
	in.UsageReportedAt = &fresh
	if r2 := Compute(in, DefaultWeights(), now); *r2.Score <= *r.Score {
		t.Fatalf("fresh high usage should lift the score: %d", *r2.Score)
	}
}

func TestComputeZeroWeights(t *testing.T) {
	r := Compute(Inputs{}, map[string]int{}, time.Now())
	if r.Score != nil {
		t.Fatal("no weights, no score")
	}
	only := Compute(Inputs{BillingStatus: "canceled"}, map[string]int{SignalBilling: 10}, time.Now())
	if only.Score == nil || *only.Score != 0 {
		t.Fatalf("billing only: %v", only.Score)
	}
}

func TestFrequency(t *testing.T) {
	for _, tc := range []struct {
		recent, prev int64
		want         int
	}{{0, 0, 100}, {3, 5, 100}, {4, 2, 50}, {3, 0, 70}, {20, 0, 0}, {10, 2, 0}} {
		if got := frequencyScore(tc.recent, tc.prev); got != tc.want {
			t.Errorf("%d/%d = %d want %d", tc.recent, tc.prev, got, tc.want)
		}
	}
}

func TestCrossing(t *testing.T) {
	if below, alert := Crossing(ip(30), 40, false); !below || !alert {
		t.Fatal("first drop alerts")
	}
	if below, alert := Crossing(ip(20), 40, true); !below || alert {
		t.Fatal("still below: no repeat alert")
	}
	if below, alert := Crossing(ip(60), 40, true); below || alert {
		t.Fatal("recovered")
	}
	if below, alert := Crossing(nil, 40, true); !below || alert {
		t.Fatal("no score keeps the state")
	}
}
