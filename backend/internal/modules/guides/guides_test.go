package guides

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"testing"
)

func TestLoadSeedIsValid(t *testing.T) {
	files, err := LoadSeed()
	if err != nil {
		t.Fatal(err)
	}
	web, mobile, setup := 0, 0, 0
	for _, f := range files {
		for _, g := range f.Guides {
			switch g.Surface {
			case "web":
				web++
			case "mobile":
				mobile++
			}
			if g.Surface == "web" && g.SetupOrder > 0 {
				setup++
			}
			if len(g.TR.Steps) == 0 || len(g.EN.Steps) == 0 || len(g.TR.Steps) != len(g.EN.Steps) {
				t.Errorf("%s: tr/en steps must exist and match", g.Key)
			}
		}
	}
	if web < 50 || mobile < 10 || setup != 8 {
		t.Fatalf("web=%d mobile=%d setup=%d", web, mobile, setup)
	}
}

func TestRouteMatches(t *testing.T) {
	for _, c := range []struct {
		pattern, path string
		want          bool
	}{
		{"/inbox", "/inbox", true},
		{"/teams/[uuid]", "/teams/0b6a", true},
		{"/teams/[uuid]", "/teams/[uuid]", true},
		{"/teams/[uuid]", "/teams", false},
		{"/settings/roles/create", "/settings/roles/x", false},
		{"/", "/", true},
	} {
		if got := RouteMatches(c.pattern, c.path); got != c.want {
			t.Errorf("RouteMatches(%q,%q)=%v", c.pattern, c.path, got)
		}
	}
	if NormalizeRoute("/inbox/?view=mine#x") != "/inbox" || NormalizeRoute("teams") != "/teams" {
		t.Fatal("NormalizeRoute")
	}
}

func TestValidateSteps(t *testing.T) {
	ok := []Step{{Title: "A", Body: "b"}, {Title: "B", Target: "page-header", Action: &StepAction{Kind: "navigate", Route: "/inbox"}}}
	if err := ValidateSteps(ok); err != nil || ok[0].ID != "s1" || ok[0].Placement != "auto" {
		t.Fatalf("valid steps rejected: %v %+v", err, ok[0])
	}
	for _, bad := range [][]Step{
		{{Title: ""}},
		{{Title: "x", Target: "<script>"}},
		{{Title: "x", Placement: "center"}},
		{{Title: "x", Action: &StepAction{Kind: "navigate"}}},
		{{ID: "a", Title: "x"}, {ID: "a", Title: "y"}},
	} {
		if ValidateSteps(bad) == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

type progressRows struct {
	Queries
	rows []db.GuideProgressListRow
}

func (q progressRows) GuideProgressList(context.Context, db.GuideProgressListParams) ([]db.GuideProgressListRow, error) {
	return q.rows, nil
}

func TestCanonicalManualReplayOverridesLegacyDismissal(t *testing.T) {
	old := db.GuideProgressListRow{GuideKey: "web.inbox", Status: "skipped", StepIndex: 2}
	replay := db.GuideProgressListRow{GuideKey: "inbox", Status: "in_progress", StepIndex: 1}
	for _, rows := range [][]db.GuideProgressListRow{{old, replay}, {replay, old}} {
		svc := New(progressRows{rows: rows})
		got, err := svc.progress(context.Background(), 1, 2)
		if err != nil {
			t.Fatal(err)
		}
		if got["inbox"].Status != "in_progress" || got["inbox"].StepIndex != 1 {
			t.Fatalf("manual replay lost: %+v", got)
		}
	}
}

func TestSurfaceTopicAliases(t *testing.T) {
	for _, pair := range [][2]string{{"mobile.inbox", "web.inbox"}, {"mobile.more_channels", "web.channels"}, {"mobile.more_members", "web.settings_members"}, {"mobile.more_team", "web.teams"}, {"mobile.more_ai", "web.settings_ai"}} {
		if progressKey(pair[0]) != progressKey(pair[1]) {
			t.Fatalf("progress is not shared: %v", pair)
		}
	}
}
