package engine

import "testing"

func agents() []Agent {
	return []Agent{
		{UserID: 1, Online: true, Skills: []string{"tr", "billing"}, Open: map[string]int{"widget": 2}},
		{UserID: 2, Online: true, Skills: []string{"en"}, Open: map[string]int{"widget": 0, "email": 1}},
		{UserID: 3, Online: false, Skills: []string{"tr"}},
		{UserID: 4, Online: true, Skills: []string{"TR"}, Open: map[string]int{"whatsapp": 3}},
	}
}

func TestRoundRobinRotates(t *testing.T) {
	req := Request{Channel: "widget", Strategy: RoundRobin}
	var got []int64
	for range 4 {
		id, _ := Pick(agents(), nil, req)
		got = append(got, id)
		req.LastUserID = id
	}
	want := []int64{1, 2, 4, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rotation %v want %v", got, want)
		}
	}
}

func TestCapacityAndSkills(t *testing.T) {
	limits := Limits{0: {AnyChannel: 3}, 1: {"widget": 2}}
	// 1 is full on widget (2/2), 4 is full overall (3/3): only 2 remains.
	if id, _ := Pick(agents(), limits, Request{Channel: "widget", RespectCapacity: true}); id != 2 {
		t.Fatalf("got %d", id)
	}
	// Without capacity checks the cursor order applies.
	if id, _ := Pick(agents(), limits, Request{Channel: "widget"}); id != 1 {
		t.Fatalf("got %d", id)
	}
	// Skill "tr" (case-insensitive): 1 and 4 (3 is offline).
	if id, _ := Pick(agents(), nil, Request{Channel: "widget", Skills: []string{"tr"}, LastUserID: 1}); id != 4 {
		t.Fatalf("got %d", id)
	}
	// Nobody: skill nobody has.
	if id, _ := Pick(agents(), nil, Request{Channel: "widget", Skills: []string{"de"}}); id != 0 {
		t.Fatalf("got %d", id)
	}
	// A zero limit stops new assignments.
	if limits.HasRoom(Agent{UserID: 9}, "sms") != true || (Limits{9: {"sms": 0}}).HasRoom(Agent{UserID: 9}, "sms") {
		t.Fatal("zero limit")
	}
}

func TestPoolExcludePreferredAndLoad(t *testing.T) {
	if id, _ := Pick(agents(), nil, Request{Channel: "widget", Pool: []int64{2, 3}}); id != 2 {
		t.Fatalf("pool got %d", id)
	}
	if id, _ := Pick(agents(), nil, Request{Channel: "widget", Exclude: []int64{1, 2}}); id != 4 {
		t.Fatalf("exclude got %d", id)
	}
	if id, vip := Pick(agents(), nil, Request{Channel: "widget", Preferred: 4}); id != 4 || !vip {
		t.Fatalf("preferred got %d %v", id, vip)
	}
	// Offline account owner falls back to the pool.
	if id, vip := Pick(agents(), nil, Request{Channel: "widget", Preferred: 3}); id != 1 || vip {
		t.Fatalf("offline preferred got %d %v", id, vip)
	}
	// Load balanced: 2 has the fewest open conversations.
	if id, _ := Pick(agents(), nil, Request{Channel: "widget", Strategy: LoadBalanced}); id != 2 {
		t.Fatalf("load got %d", id)
	}
	// Ratio: 4 has 3 of 10, 2 has 1 of 1 → 4 is less loaded.
	limits := Limits{2: {AnyChannel: 1}, 4: {AnyChannel: 10}, 1: {AnyChannel: 2}}
	if id, _ := Pick(agents(), limits, Request{Channel: "email", Strategy: LoadBalanced}); id != 4 {
		t.Fatalf("ratio got %d", id)
	}
}
