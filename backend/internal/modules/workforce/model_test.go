package workforce

import (
	"math"
	"testing"
	"time"
)

func TestErlangCReferenceAndMonotonicity(t *testing.T) {
	// Offered load 10 Erlangs, 80% answered in 20s with 180s AHT needs 14 agents.
	n, limited := RequiredAgents(200, 180, 20, .8)
	if n != 14 || limited {
		t.Fatalf("got %d limited=%v", n, limited)
	}
	if n, _ := RequiredAgents(0, 180, 20, .8); n != 0 {
		t.Fatal(n)
	}
	prev := 0
	for v := 1.; v <= 1000; v += 13 {
		n, l := RequiredAgents(v, 300, 30, .8)
		if l || n < prev {
			t.Fatalf("non-monotonic at %v", v)
		}
		prev = n
	}
	if _, l := RequiredAgents(10000000, 3600, 1, .99); !l {
		t.Fatal("must mark ceiling")
	}
}
func TestSeasonalIncludesZerosAndRecentWeight(t *testing.T) {
	start := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	end := start.Add(14 * 24 * time.Hour)
	counts := map[time.Time]float64{start: 10, start.Add(7 * 24 * time.Hour): 20}
	m := Seasonal(start, end, time.UTC, counts, .3)
	if math.Abs(m[24]-13) > 1e-9 {
		t.Fatal(m[24])
	}
	delete(counts, start.Add(7*24*time.Hour))
	m = Seasonal(start, end, time.UTC, counts, .3)
	if m[24] != 7 {
		t.Fatal("zero week omitted", m[24])
	}
	if m[25] != 0 {
		t.Fatal(m[25])
	}
}
func TestForecastRejectsInvalidRangesAndNaN(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	base := ForecastInput{Since: now, Until: now.Add(time.Hour)}
	if err := base.validate(now); err != nil {
		t.Fatal(err)
	}
	for _, v := range []float64{-1, 1.1, math.NaN(), math.Inf(1)} {
		in := base
		in.Alpha = v
		if in.validate(now) == nil {
			t.Fatal(v)
		}
	}
	in := base
	in.Until = in.Since
	if in.validate(now) == nil {
		t.Fatal("empty range")
	}
}

func TestSeasonalRepeatedDSTHour(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	counts := map[time.Time]float64{start: 10, start.Add(time.Hour): 20}
	m := Seasonal(start, start.Add(2*time.Hour), loc, counts, .3)
	if m[2] != 13 {
		t.Fatalf("repeated local Sunday 02:00 must smooth twice: %v", m[2])
	}
}
