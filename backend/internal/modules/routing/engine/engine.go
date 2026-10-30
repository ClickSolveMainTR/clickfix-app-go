// Package engine picks the agent a conversation goes to (CF-53) under the
// capacity limits of CF-54. It is pure: the usecase loads agents, their
// open conversation counts and limits, and stores the result.
//
// Only online agents (CF-10 presence) are eligible; away, busy and offline
// agents get nothing new. Capacity has two checks: the channel limit
// compares the agent's open conversations on that channel, the '*' limit
// compares all of them.
package engine

import (
	"slices"
	"strings"
)

// Strategies.
const (
	RoundRobin   = "round_robin"
	LoadBalanced = "load_balanced"
)

// AnyChannel is the limit key for all channels together.
const AnyChannel = "*"

// Agent is one candidate.
type Agent struct {
	UserID int64
	Online bool
	Skills []string
	// Open counts open conversations by channel type.
	Open map[string]int
}

// Total is the agent's open conversations on every channel.
func (a Agent) Total() int {
	n := 0
	for _, v := range a.Open {
		n += v
	}
	return n
}

// Limits holds capacity rows: by user (0 = organization default), then
// channel ('*' = all channels).
type Limits map[int64]map[string]int

// Limit is the effective limit of a user for key (a channel or '*'): the
// user's own row, else the organization default. ok is false when unlimited.
func (l Limits) Limit(userID int64, key string) (int, bool) {
	if v, ok := l[userID][key]; ok {
		return v, true
	}
	if v, ok := l[0][key]; ok {
		return v, true
	}
	return 0, false
}

// HasRoom tells whether the agent can take one more conversation on channel.
func (l Limits) HasRoom(a Agent, channel string) bool {
	if max, ok := l.Limit(a.UserID, channel); ok && channel != AnyChannel && a.Open[channel] >= max {
		return false
	}
	if max, ok := l.Limit(a.UserID, AnyChannel); ok && a.Total() >= max {
		return false
	}
	return true
}

// Request describes one assignment.
type Request struct {
	Channel  string
	Strategy string
	// Pool restricts candidates (a team's members); nil means every agent.
	Pool []int64
	// Skills the agent must all have (case-insensitive).
	Skills []string
	// Exclude never picks these (the agent who left it unanswered).
	Exclude []int64
	// Preferred is tried first when eligible (a VIP company's account owner).
	Preferred int64
	// RespectCapacity applies the limits.
	RespectCapacity bool
	// LastUserID is the round-robin cursor of the pool.
	LastUserID int64
}

// Pick returns the chosen agent (0 when nobody is eligible) and whether the
// preferred agent was taken.
func Pick(agents []Agent, limits Limits, req Request) (int64, bool) {
	eligible := make([]Agent, 0, len(agents))
	for _, a := range agents {
		if !a.Online || slices.Contains(req.Exclude, a.UserID) {
			continue
		}
		if req.Pool != nil && !slices.Contains(req.Pool, a.UserID) {
			continue
		}
		if !hasSkills(a.Skills, req.Skills) {
			continue
		}
		if req.RespectCapacity && !limits.HasRoom(a, req.Channel) {
			continue
		}
		eligible = append(eligible, a)
	}
	if len(eligible) == 0 {
		return 0, false
	}
	slices.SortFunc(eligible, func(a, b Agent) int { return compare(a.UserID, b.UserID) })
	if req.Preferred > 0 {
		if i := slices.IndexFunc(eligible, func(a Agent) bool { return a.UserID == req.Preferred }); i >= 0 {
			return req.Preferred, true
		}
	}
	// Round-robin order: the agents after the cursor first, then wrap.
	i := slices.IndexFunc(eligible, func(a Agent) bool { return a.UserID > req.LastUserID })
	if i < 0 {
		i = 0
	}
	order := append(slices.Clone(eligible[i:]), eligible[:i]...)
	if req.Strategy != LoadBalanced {
		return order[0].UserID, false
	}
	best := order[0]
	for _, a := range order[1:] {
		if load(a, limits) < load(best, limits) {
			best = a
		}
	}
	return best.UserID, false
}

// load is the share of capacity in use (open count when unlimited, scaled
// so limited agents compare by ratio).
func load(a Agent, l Limits) float64 {
	if max, ok := l.Limit(a.UserID, AnyChannel); ok && max > 0 {
		return float64(a.Total()) / float64(max)
	}
	return float64(a.Total()) / 1000
}

func hasSkills(have, need []string) bool {
	for _, n := range need {
		if !slices.ContainsFunc(have, func(h string) bool { return strings.EqualFold(h, n) }) {
			return false
		}
	}
	return true
}

func compare(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
