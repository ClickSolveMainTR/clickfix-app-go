package outbox

import (
	"math"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
)

func TestMarshalEventVersionRange(t *testing.T) {
	for _, version := range []int{0, 1, math.MaxInt32, math.MaxInt32 + 1, math.MaxInt} {
		ev := events.New("test.created")
		ev.Version = version
		_, err := marshalEvent(ev)
		if (err != nil) != (version > math.MaxInt32) {
			t.Errorf("version %d: err=%v", version, err)
		}
	}
}
