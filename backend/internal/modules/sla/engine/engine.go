// Package engine is the pure SLA clock logic (CF-46), ported from the Laravel
// clickfix-app SyncTicketSlaStateSubscriber / EvaluateTicketSlaCommand and
// extended with business hours and pausing.
//
// A clock measures one metric of one conversation in working time:
//
//   - first_response: from the conversation start until the first agent answer
//   - next_response: from each new customer turn (waiting_since) after the
//     first answer until the next agent answer
//   - resolution: from the conversation start until it is resolved; a reopen
//     resumes the same clock
//
// While the conversation is pending or snoozed (waiting on the customer) every
// running clock is paused. consumed is the working time spent before the last
// pause, so due = calendar.Add(resumed, target - consumed).
//
// Plan is deterministic: it reconciles stored clocks with the current
// conversation state, so replaying it for the same state changes nothing.
package engine

import (
	"slices"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/businesshours"
)

// Metrics.
const (
	FirstResponse = "first_response"
	NextResponse  = "next_response"
	Resolution    = "resolution"
)

// Metrics in display order.
var Metrics = []string{FirstResponse, NextResponse, Resolution}

// Transition kinds Plan reports (sla.* events).
const (
	Applied  = "applied"
	Met      = "met"
	Breached = "breached"
)

// Conversation is the state the clocks follow.
type Conversation struct {
	Status          string // open, pending, snoozed, resolved, closed
	Source          string // inbound, outbound
	CreatedAt       time.Time
	FirstResponseAt *time.Time
	WaitingSince    *time.Time
	LastOutboundAt  *time.Time
	ResolvedAt      *time.Time
	ClosedAt        *time.Time
}

// Paused tells whether the conversation waits on the customer.
func (c Conversation) Paused() bool { return c.Status == "pending" || c.Status == "snoozed" }

// Done tells whether the conversation is resolved or closed.
func (c Conversation) Done() bool { return c.Status == "resolved" || c.Status == "closed" }

// Targets are the working-time goals of the applying policy for the
// conversation's priority. Zero means the metric is not measured.
type Targets struct {
	FirstResponse time.Duration
	NextResponse  time.Duration
	Resolution    time.Duration
}

func (t Targets) of(metric string) time.Duration {
	switch metric {
	case FirstResponse:
		return t.FirstResponse
	case NextResponse:
		return t.NextResponse
	default:
		return t.Resolution
	}
}

// Policy is the applying policy. A nil *Policy means no SLA.
type Policy struct {
	ID             int64
	Targets        Targets
	WarningPercent int
	Calendar       *businesshours.Calendar // nil: 24/7
}

// Clock is one stored metric clock.
type Clock struct {
	Metric      string
	PolicyID    *int64
	Target      time.Duration
	Cycle       int
	StartedAt   time.Time
	ResumedAt   time.Time
	Consumed    time.Duration
	PausedAt    *time.Time
	WarnAt      *time.Time
	DueAt       *time.Time
	WarnedAt    *time.Time
	BreachedAt  *time.Time
	CompletedAt *time.Time
}

// Running tells whether the clock is ticking.
func (c Clock) Running() bool { return c.CompletedAt == nil && c.PausedAt == nil }

// Status is the display state: running, paused, met, breached.
func (c Clock) Status() string {
	switch {
	case c.BreachedAt != nil:
		return "breached"
	case c.CompletedAt != nil:
		return "met"
	case c.PausedAt != nil:
		return "paused"
	default:
		return "running"
	}
}

// Remaining is the working time left (negative once overdue) at now.
func (c Clock) Remaining(cal *businesshours.Calendar, now time.Time) time.Duration {
	if c.CompletedAt != nil {
		return 0
	}
	left := c.Target - c.Consumed
	if c.PausedAt == nil {
		if c.DueAt != nil && now.After(*c.DueAt) {
			return -now.Sub(*c.DueAt)
		}
		left -= cal.Between(c.ResumedAt, now)
	}
	return left
}

// Transition is something worth an event.
type Transition struct {
	Kind   string
	Metric string
	Clock  Clock
	At     time.Time
}

// Result is what Plan decided.
type Result struct {
	Upsert      []Clock
	Delete      []string
	Transitions []Transition
	// Clocks is the full resulting set (for mirroring onto the conversation).
	Clocks map[string]Clock
}

// Plan reconciles the stored clocks with the conversation at instant at.
func Plan(conv Conversation, stored map[string]Clock, pol *Policy, at time.Time) Result {
	res := Result{Clocks: map[string]Clock{}}
	for _, m := range Metrics {
		cur, has := stored[m]
		var target time.Duration
		if pol != nil {
			target = pol.Targets.of(m)
		}
		if target <= 0 {
			// Not measured (any more): unfinished clocks go, history stays.
			if has && cur.CompletedAt == nil {
				res.Delete = append(res.Delete, m)
			} else if has {
				res.Clocks[m] = cur
			}
			continue
		}
		next, changed, trs := planMetric(m, conv, cur, has, pol, target, at)
		res.Transitions = append(res.Transitions, trs...)
		if next == nil {
			if has {
				res.Clocks[m] = cur
			}
			continue
		}
		res.Clocks[m] = *next
		if changed {
			res.Upsert = append(res.Upsert, *next)
		}
	}
	return res
}

func planMetric(m string, conv Conversation, cur Clock, has bool, pol *Policy, target time.Duration, at time.Time) (*Clock, bool, []Transition) {
	var trs []Transition
	start, end, want := window(m, conv)
	if !has {
		if !want || end != nil {
			// Never measured and already over (answered before the policy
			// applied) or not applicable.
			return nil, false, nil
		}
		c := Clock{Metric: m, Cycle: 1, StartedAt: start, ResumedAt: start}
		c = retarget(c, pol, target)
		c = follow(c, conv, pol, maxTime(at, start))
		trs = append(trs, Transition{Kind: Applied, Metric: m, Clock: c, At: at})
		return &c, true, trs
	}
	orig := cur
	if m == NextResponse && want && end == nil && !start.Equal(cur.StartedAt) {
		// A new customer turn: close the previous cycle and start another.
		if cur.CompletedAt == nil {
			doneAt := maxTime(cur.ResumedAt, timeOr(conv.LastOutboundAt, start))
			var t []Transition
			cur, t = complete(cur, pol, minTime(doneAt, start))
			trs = append(trs, t...)
		}
		c := Clock{Metric: m, Cycle: cur.Cycle + 1, StartedAt: start, ResumedAt: start}
		c = retarget(c, pol, target)
		c = follow(c, conv, pol, maxTime(at, start))
		trs = append(trs, Transition{Kind: Applied, Metric: m, Clock: c, At: at})
		return &c, true, trs
	}
	cur = retarget(cur, pol, target)
	switch {
	case cur.CompletedAt == nil && end != nil:
		var t []Transition
		cur, t = complete(cur, pol, maxTime(*end, cur.ResumedAt))
		trs = append(trs, t...)
	case cur.CompletedAt != nil && m == Resolution && !conv.Done():
		// Reopened: the resolution clock resumes where it stopped.
		cur.CompletedAt = nil
		cur.ResumedAt = maxTime(at, cur.ResumedAt)
		cur.PausedAt = nil
		cur = follow(cur, conv, pol, at)
	case cur.CompletedAt == nil:
		cur = follow(cur, conv, pol, at)
	}
	return &cur, !equal(orig, cur), trs
}

// window returns where the metric's current cycle starts, where it ended (if
// it did) and whether the metric applies at all.
func window(m string, conv Conversation) (time.Time, *time.Time, bool) {
	// Resolving a conversation stops every clock.
	var done *time.Time
	if conv.Done() {
		done = conv.ResolvedAt
		if done == nil {
			done = conv.ClosedAt
		}
		if done == nil {
			t := conv.CreatedAt
			done = &t
		}
	}
	switch m {
	case FirstResponse:
		if conv.FirstResponseAt != nil {
			return conv.CreatedAt, conv.FirstResponseAt, conv.Source != "outbound"
		}
		return conv.CreatedAt, done, conv.Source != "outbound"
	case NextResponse:
		if conv.FirstResponseAt == nil {
			return time.Time{}, done, false
		}
		if conv.WaitingSince != nil && conv.WaitingSince.After(*conv.FirstResponseAt) {
			return *conv.WaitingSince, done, true
		}
		// Not waiting: a running cycle ended with the last agent answer.
		end := conv.LastOutboundAt
		if end == nil {
			end = done
		}
		return time.Time{}, end, false
	default:
		return conv.CreatedAt, done, true
	}
}

func retarget(c Clock, pol *Policy, target time.Duration) Clock {
	c.Target = target
	if pol != nil {
		id := pol.ID
		c.PolicyID = &id
	}
	return c
}

// follow pauses or resumes a running clock and recomputes its deadlines.
func follow(c Clock, conv Conversation, pol *Policy, at time.Time) Clock {
	cal := calendar(pol)
	if conv.Paused() && c.PausedAt == nil {
		t := maxTime(at, c.ResumedAt)
		c.Consumed = (c.Consumed + cal.Between(c.ResumedAt, t)).Truncate(time.Second)
		c.PausedAt = &t
	} else if !conv.Paused() && c.PausedAt != nil {
		c.ResumedAt = maxTime(at, *c.PausedAt)
		c.PausedAt = nil
	}
	if c.PausedAt != nil {
		c.DueAt, c.WarnAt = nil, nil
		return c
	}
	due := cal.Add(c.ResumedAt, c.Target-c.Consumed)
	c.DueAt = &due
	pct := 80
	if pol != nil && pol.WarningPercent > 0 && pol.WarningPercent < 100 {
		pct = pol.WarningPercent
	}
	warn := cal.Add(c.ResumedAt, c.Target*time.Duration(pct)/100-c.Consumed)
	if warn.After(due) {
		warn = due
	}
	c.WarnAt = &warn
	return c
}

// complete stops the clock at t: met, or breached when t is past due and the
// sweep did not catch it yet.
func complete(c Clock, pol *Policy, t time.Time) (Clock, []Transition) {
	if c.PausedAt == nil {
		c.Consumed = (c.Consumed + calendar(pol).Between(c.ResumedAt, t)).Truncate(time.Second)
	}
	c.CompletedAt = &t
	var trs []Transition
	switch {
	case c.BreachedAt != nil:
	case c.Consumed > c.Target || (c.PausedAt == nil && c.DueAt != nil && t.After(*c.DueAt)):
		b := t
		if c.DueAt != nil && c.DueAt.Before(t) {
			b = *c.DueAt
		}
		c.BreachedAt = &b
		trs = append(trs, Transition{Kind: Breached, Metric: c.Metric, Clock: c, At: b})
	default:
		trs = append(trs, Transition{Kind: Met, Metric: c.Metric, Clock: c, At: t})
	}
	return c, trs
}

func calendar(pol *Policy) *businesshours.Calendar {
	if pol == nil {
		return nil
	}
	return pol.Calendar
}

func equal(a, b Clock) bool {
	eq := func(x, y *time.Time) bool { return (x == nil) == (y == nil) && (x == nil || x.Equal(*y)) }
	ids := (a.PolicyID == nil) == (b.PolicyID == nil) && (a.PolicyID == nil || *a.PolicyID == *b.PolicyID)
	return ids && a.Target == b.Target && a.Cycle == b.Cycle && a.StartedAt.Equal(b.StartedAt) &&
		a.ResumedAt.Equal(b.ResumedAt) && a.Consumed == b.Consumed && eq(a.PausedAt, b.PausedAt) &&
		eq(a.WarnAt, b.WarnAt) && eq(a.DueAt, b.DueAt) && eq(a.WarnedAt, b.WarnedAt) &&
		eq(a.BreachedAt, b.BreachedAt) && eq(a.CompletedAt, b.CompletedAt)
}

func maxTime(ts ...time.Time) time.Time {
	return slices.MaxFunc(ts, func(a, b time.Time) int { return a.Compare(b) })
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

func timeOr(p *time.Time, d time.Time) time.Time {
	if p == nil {
		return d
	}
	return *p
}
