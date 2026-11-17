package model

import "testing"

func TestInactiveReason(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Channels = []string{"whatsapp"}
	cases := []struct {
		name, channel string
		hoursOnly     bool
		open          bool
		want          string
	}{
		{"enabled channel, around the clock", "whatsapp", false, false, ""},
		{"enabled channel, open", "whatsapp", true, true, ""},
		{"enabled channel, closed", "whatsapp", true, false, ReasonOutsideBusinessHours},
		{"channel not in scope", "email", false, true, ReasonChannelNotEnabled},
		{"channel not in scope wins over hours", "email", true, false, ReasonChannelNotEnabled},
	}
	for _, tc := range cases {
		c := cfg
		c.BusinessHoursOnly = tc.hoursOnly
		if got := c.InactiveReason(tc.channel, tc.open); got != tc.want {
			t.Errorf("%s: InactiveReason = %q, want %q", tc.name, got, tc.want)
		}
		if got := c.Active(tc.channel, tc.open); got != (tc.want == "") {
			t.Errorf("%s: Active = %v", tc.name, got)
		}
	}
}
