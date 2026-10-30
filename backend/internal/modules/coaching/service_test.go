package coaching

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"testing"
)

func TestVariancePopulation(t *testing.T) {
	s := Summarize([]float64{70, 80, 90})
	if s.Count != 3 || s.Mean != 80 || s.Range != 20 || s.Variance < 66.66 || s.Variance > 66.67 {
		t.Fatalf("%+v", s)
	}
	if s := Summarize(nil); s.Count != 0 || s.Variance != 0 {
		t.Fatal(s)
	}
}
func TestBadgesRequireSamplesAndFRTLowerIsBetter(t *testing.T) {
	rules := []db.GameRulesRow{{Name: "Fast", Metric: "frt", Threshold: 30, Points: 10, MinSamples: 2, Enabled: true}, {Name: "Quality", Metric: "qa", Threshold: 90, Points: 20, MinSamples: 2, Enabled: true}, {Name: "Disabled", Metric: "resolutions", Threshold: 1, Points: 50, MinSamples: 1}}
	m := db.GameMetricsRow{Frt: 20, FrtSamples: 2, Qa: 95, QaSamples: 1, Resolutions: 5}
	a := Award(m, rules)
	if a.Points != 10 || len(a.Badges) != 1 {
		t.Fatal(a)
	}
	m.FrtSamples = 0
	if a = Award(m, rules); a.Points != 0 {
		t.Fatal(a)
	}
	m.Frt = 40
	m.FrtSamples = 100
	if a = Award(m, rules); a.Points != 0 {
		t.Fatal(a)
	}
}
