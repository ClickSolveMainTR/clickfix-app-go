package onboarding

import (
	"strings"
	"testing"
)

func TestTemplatesLoad(t *testing.T) {
	templates, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) < 8 {
		t.Fatalf("want at least 8 templates, got %d", len(templates))
	}
	seen := map[string]bool{}
	for _, tpl := range templates {
		if seen[tpl.Key] {
			t.Fatalf("duplicate template key %s", tpl.Key)
		}
		seen[tpl.Key] = true
		items := tpl.Plan.Items()
		kinds := map[string]int{}
		keys := map[string]bool{}
		for _, it := range items {
			if keys[it.Key] {
				t.Fatalf("%s: duplicate item key %s", tpl.Key, it.Key)
			}
			keys[it.Key] = true
			kinds[it.Kind]++
		}
		for _, k := range []string{KindTeam, KindTag, KindBusinessHours, KindSLAPolicy, KindCannedReply, KindMacro,
			KindCustomField, KindForm, KindCollection, KindArticle, KindWidget, KindAIPersona, KindRoutingRule, KindView, KindSurvey} {
			if kinds[k] == 0 {
				t.Errorf("%s: no %s items", tpl.Key, k)
			}
		}
		// Normalize must not have dropped anything the template author wrote.
		raw, _ := templateFS.ReadFile("templates/" + tpl.Key + ".json")
		if strings.Count(string(raw), `"shortcut"`) != kinds[KindCannedReply] {
			t.Errorf("%s: a canned reply was dropped by Normalize", tpl.Key)
		}
		if strings.Count(string(raw), `"intent"`) != kinds[KindAIProcedure] {
			t.Errorf("%s: a procedure was dropped by Normalize", tpl.Key)
		}
	}
}
