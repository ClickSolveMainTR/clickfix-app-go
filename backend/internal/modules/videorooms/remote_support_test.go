package videorooms

import (
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/livekit"
)

func TestRemoteSupportEventsAreRouted(t *testing.T) {
	cases := []struct {
		ev   livekit.Event
		want bool
	}{
		{livekit.Event{Event: "participant_joined", Room: &livekit.Room{Name: "rs_0b4f7c55"}}, true},
		{livekit.Event{Event: "egress_ended", EgressInfo: &livekit.EgressInfo{RoomName: "rs_0b4f7c55"}}, true},
		{livekit.Event{Event: "participant_joined", Room: &livekit.Room{Name: "cf-0b4f7c55"}}, false},
		{livekit.Event{Event: "egress_ended", EgressInfo: &livekit.EgressInfo{RoomName: "cf-0b4f7c55"}}, false},
		{livekit.Event{Event: "room_started"}, false},
	}
	for _, c := range cases {
		if got := remoteSupportEvent(c.ev); got != c.want {
			t.Fatalf("%+v: got %v", c.ev, got)
		}
	}
}
