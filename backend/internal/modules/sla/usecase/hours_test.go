package usecase_test

import (
	"context"
	"testing"
	"time"

	agentmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/businesshours"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
)

// CF-198: the SLA schedule is the one working-hours source. Changing it in
// one place moves the widget's online state, the AI agent's "inside hours"
// answer and the SLA calendar together.
func TestHoursStateIsTheSingleSource(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	// Wednesday 11 Nov 2026 10:00 Istanbul (07:00 UTC) and 20:00 Istanbul.
	morning := time.Date(2026, 11, 11, 7, 0, 0, 0, time.UTC)
	evening := time.Date(2026, 11, 11, 17, 0, 0, 0, time.UTC)
	widgetSet := widget.Defaults()
	agentCfg := agentmodel.DefaultConfig()
	agentCfg.Channels = []string{"widget"}
	agentCfg.BusinessHoursOnly = true

	// No schedule: open around the clock for all three.
	st, err := f.svc.HoursState(ctx, f.org, evening)
	if err != nil || !st.Open || st.Scheduled || st.NextOpenAt != nil {
		t.Fatalf("no schedule: %+v err=%v", st, err)
	}
	if !widgetSet.Online(st.Open) || !agentCfg.Active("widget", st.Open) {
		t.Fatal("without a schedule the widget and the agent are on")
	}

	week := businesshours.Week{
		"mon": {Open: "09:00", Close: "18:00"}, "tue": {Open: "09:00", Close: "18:00"},
		"wed": {Open: "09:00", Close: "18:00"}, "thu": {Open: "09:00", Close: "18:00"},
		"fri": {Open: "09:00", Close: "18:00"}, "sat": {Closed: true}, "sun": {Closed: true},
	}
	hours, err := f.svc.CreateBusinessHours(ctx, f.org, model.BusinessHoursInput{Name: "Merkez", WorkingHours: week})
	if err != nil {
		t.Fatal(err)
	}
	// Inside hours.
	if st, err = f.svc.HoursState(ctx, f.org, morning); err != nil || !st.Open || !st.Scheduled {
		t.Fatalf("morning: %+v err=%v", st, err)
	}
	if !widgetSet.Online(st.Open) || !agentCfg.Active("widget", st.Open) {
		t.Fatal("inside hours: widget online, agent active")
	}
	// Outside hours: all three see the same closed state and the same
	// next opening (Thursday 09:00 Istanbul = 06:00 UTC).
	if st, err = f.svc.HoursState(ctx, f.org, evening); err != nil || st.Open {
		t.Fatalf("evening: %+v err=%v", st, err)
	}
	if st.NextOpenAt == nil || !st.NextOpenAt.Equal(time.Date(2026, 11, 12, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("next opening: %v", st.NextOpenAt)
	}
	if widgetSet.Online(st.Open) || agentCfg.Active("widget", st.Open) {
		t.Fatal("outside hours: widget offline, agent inactive")
	}
	if est := widgetSet.EstimateReply(st, nil); est.Value != "day" || est.NextOpenAt == nil {
		t.Fatalf("widget reply estimate outside hours: %+v", est)
	}
	cal, err := f.svc.OrganizationCalendar(ctx, f.org)
	if err != nil || cal == nil || cal.IsOpen(evening) || !cal.IsOpen(morning) {
		t.Fatalf("SLA calendar disagrees: %v err=%v", cal, err)
	}

	// Change the schedule in the one place: the evening becomes working
	// time for the widget, the agent and the SLA calendar at once.
	week["wed"] = businesshours.Day{Open: "09:00", Close: "22:00"}
	if _, err := f.svc.UpdateBusinessHours(ctx, f.org, hours.UUID, model.BusinessHoursInput{Name: "Merkez", WorkingHours: week, IsDefault: true}); err != nil {
		t.Fatal(err)
	}
	if st, err = f.svc.HoursState(ctx, f.org, evening); err != nil || !st.Open || st.NextOpenAt != nil {
		t.Fatalf("after change: %+v err=%v", st, err)
	}
	if !widgetSet.Online(st.Open) || !agentCfg.Active("widget", st.Open) {
		t.Fatal("after the change the widget and the agent follow it")
	}
	if cal, err = f.svc.OrganizationCalendar(ctx, f.org); err != nil || !cal.IsOpen(evening) {
		t.Fatalf("SLA calendar after change: err=%v", err)
	}
	// Away mode is the widget's own switch; it never comes from the hours.
	widgetSet.AwayMode = true
	if widgetSet.Online(st.Open) {
		t.Fatal("away mode overrides open hours")
	}
}
