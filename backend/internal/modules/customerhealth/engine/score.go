// Package engine computes the customer health score (CF-87): each signal is
// turned into a 0-100 sub-score, and the score is their weighted average
// over the signals that have data (weights renormalized), rounded.
package engine

import (
	"math"
	"time"
)

// Signal keys.
const (
	SignalCSAT          = "csat"
	SignalOpen          = "open_conversations"
	SignalSLA           = "sla_breaches"
	SignalFrequency     = "frequency_trend"
	SignalBilling       = "billing"
	SignalUsage         = "usage"
	DefaultThreshold    = 40
	UsageFreshnessLimit = 30 * 24 * time.Hour
)

// Signals lists every signal in display order.
var Signals = []string{SignalCSAT, SignalOpen, SignalSLA, SignalFrequency, SignalBilling, SignalUsage}

// DefaultWeights are used for signals an organization did not set.
func DefaultWeights() map[string]int {
	return map[string]int{
		SignalCSAT: 25, SignalOpen: 15, SignalSLA: 20, SignalFrequency: 15, SignalBilling: 15, SignalUsage: 10,
	}
}

// Inputs are the raw facts of one company.
type Inputs struct {
	CSATAvg               float64 // 1..5, valid when CSATCount > 0
	CSATCount             int64
	OpenConversations     int64
	SLABreaches           int64 // last 30 days
	RecentConversations   int64 // last 30 days
	PreviousConversations int64 // the 30 days before
	BillingStatus         string
	UsageScore            *int // 0..100 reported through the API
	UsageReportedAt       *time.Time
}

// SubScore is one signal's contribution.
type SubScore struct {
	Score  *int `json:"score"`
	Weight int  `json:"weight"`
	Value  any  `json:"value"`
	Used   bool `json:"used"`
}

// Result is a computed health score; Score is nil when no signal had data
// or every weight is zero.
type Result struct {
	Score   *int                `json:"score"`
	Signals map[string]SubScore `json:"signals"`
}

// Compute scores in with the given weights at now.
func Compute(in Inputs, weights map[string]int, now time.Time) Result {
	sub := map[string]*int{}
	values := map[string]any{}

	if in.CSATCount > 0 {
		v := clamp(int(math.Round((in.CSATAvg - 1) / 4 * 100)))
		sub[SignalCSAT] = &v
		values[SignalCSAT] = math.Round(in.CSATAvg*100) / 100
	} else {
		values[SignalCSAT] = nil
	}

	open := clamp(100 - int(in.OpenConversations)*15)
	sub[SignalOpen] = &open
	values[SignalOpen] = in.OpenConversations

	sla := clamp(100 - int(in.SLABreaches)*20)
	sub[SignalSLA] = &sla
	values[SignalSLA] = in.SLABreaches

	freq := frequencyScore(in.RecentConversations, in.PreviousConversations)
	sub[SignalFrequency] = &freq
	values[SignalFrequency] = map[string]int64{"recent": in.RecentConversations, "previous": in.PreviousConversations}

	if b, ok := billingScore(in.BillingStatus); ok {
		sub[SignalBilling] = &b
	}
	values[SignalBilling] = in.BillingStatus

	if in.UsageScore != nil && in.UsageReportedAt != nil && now.Sub(*in.UsageReportedAt) <= UsageFreshnessLimit {
		u := clamp(*in.UsageScore)
		sub[SignalUsage] = &u
		values[SignalUsage] = u
	} else {
		values[SignalUsage] = nil
	}

	out := Result{Signals: map[string]SubScore{}}
	total, weightSum := 0.0, 0
	for _, key := range Signals {
		w := weights[key]
		s := sub[key]
		used := s != nil && w > 0
		out.Signals[key] = SubScore{Score: s, Weight: w, Value: values[key], Used: used}
		if used {
			total += float64(*s * w)
			weightSum += w
		}
	}
	if weightSum > 0 {
		v := clamp(int(math.Round(total / float64(weightSum))))
		out.Score = &v
	}
	return out
}

// frequencyScore penalizes a rising number of conversations: no change or
// fewer is healthy; doubling costs 50 points.
func frequencyScore(recent, previous int64) int {
	if recent <= previous {
		return 100
	}
	if previous == 0 {
		return clamp(100 - int(recent)*10)
	}
	ratio := float64(recent) / float64(previous)
	return clamp(int(math.Round(100 - (ratio-1)*50)))
}

func billingScore(status string) (int, bool) {
	switch status {
	case "active":
		return 100, true
	case "trialing":
		return 70, true
	case "past_due":
		return 25, true
	case "canceled":
		return 0, true
	}
	return 0, false
}

func clamp(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// Crossing reports whether a new score starts a churn-risk alert: it is
// below the threshold and the company was not already below it.
func Crossing(score *int, threshold int, wasBelow bool) (below, alert bool) {
	if score == nil {
		return wasBelow, false
	}
	below = *score < threshold
	return below, below && !wasBelow
}
