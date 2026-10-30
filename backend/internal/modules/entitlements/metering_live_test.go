package entitlements

import (
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/google/uuid"
)

func TestWithLiveGauges(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	trt := time.FixedZone("TRT", 10800)
	oct, sep := uuid.New(), uuid.New()
	row := func(p uuid.UUID, start time.Time, closed bool, key string, q int64) db.BillingUsageSummaryRow {
		r := db.BillingUsageSummaryRow{PeriodUuid: p, StartsAt: repository.TS(start), EndsAt: repository.TS(start.AddDate(0, 1, 0)), Key: key, Aggregation: "sum", Quantity: q}
		if closed {
			r.ClosedAt = repository.TS(start.AddDate(0, 1, 0))
		}
		return r
	}
	octStart, sepStart := time.Date(2026, 10, 1, 0, 0, 0, 0, trt), time.Date(2026, 9, 1, 0, 0, 0, 0, trt)
	live := []db.BillingLiveGaugesRow{{Key: "seats", Quantity: 3}, {Key: "channels", Quantity: 2}}

	t.Run("current period first, live fills unmetered gauges", func(t *testing.T) {
		rows := []db.BillingUsageSummaryRow{
			row(oct, octStart, false, "conversations.monthly", 7),
			row(oct, octStart, false, "seats", 5),
			row(sep, sepStart, true, "seats", 9),
		}
		got := withLiveGauges(rows, live, now)
		first := map[string]int64{}
		for _, r := range got {
			if _, ok := first[r.Key]; !ok {
				first[r.Key] = r.Quantity
			}
		}
		if first["seats"] != 5 || first["channels"] != 2 || first["conversations.monthly"] != 7 {
			t.Fatalf("first per key = %v (rows %+v)", first, got)
		}
		if got[len(got)-1].PeriodUuid != sep || got[len(got)-1].Quantity != 9 {
			t.Fatalf("closed period row changed: %+v", got[len(got)-1])
		}
	})

	t.Run("no stored period yet", func(t *testing.T) {
		got := withLiveGauges(nil, live, now)
		if len(got) != 2 || got[0].Key != "seats" || got[0].Quantity != 3 || !got[0].StartsAt.Time.Equal(octStart) {
			t.Fatalf("rows = %+v", got)
		}
	})
}
