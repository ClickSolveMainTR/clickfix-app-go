package usecase

import (
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
)

func testGraph() model.Graph {
	return model.Graph{SchemaVersion: 1, Nodes: []model.Node{{ID: "t", Type: "trigger", Trigger: events.ConversationCreated},
		{ID: "a", Type: "action", Action: &model.Action{Type: "set_priority", Value: "urgent"}}, {ID: "e", Type: "end"}},
		Edges: []model.Edge{{From: "t", Port: "next", To: "a", Input: "in"}, {From: "a", Port: "next", To: "e", Input: "in"}}}
}
func TestGraphValidation(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*model.Graph)
		valid bool
	}{
		{name: "valid", edit: func(*model.Graph) {}, valid: true},
		{name: "schema", edit: func(g *model.Graph) { g.SchemaVersion = 2 }},
		{name: "duplicate", edit: func(g *model.Graph) { g.Nodes[1].ID = "t" }},
		{name: "unknown port", edit: func(g *model.Graph) { g.Edges[0].Port = "data" }},
		{name: "unknown input", edit: func(g *model.Graph) { g.Edges[0].Input = "value" }},
		{name: "unconnected", edit: func(g *model.Graph) { g.Edges = g.Edges[:1] }},
		{name: "unreachable", edit: func(g *model.Graph) { g.Nodes = append(g.Nodes, model.Node{ID: "orphan", Type: "end"}) }},
		{name: "ordinary cycle", edit: func(g *model.Graph) { g.Edges[1].To = "a" }},
		{name: "fake wait loop", edit: func(g *model.Graph) { g.Edges[1].Loop = true }},
		{name: "untyped condition", edit: func(g *model.Graph) { g.Nodes[1].Type = "condition" }},
		{name: "unknown action", edit: func(g *model.Graph) { g.Nodes[1].Action.Type = "delete_all" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := testGraph()
			c.edit(&g)
			if err := ValidateGraph(g); (err == nil) != c.valid {
				t.Fatalf("valid=%v err=%v", c.valid, err)
			}
		})
	}
}
func TestExplicitWaitLoopHasReachableExit(t *testing.T) {
	condition := model.Condition{Field: "status", Operator: "eq", Value: "open"}
	g := model.Graph{SchemaVersion: 1, Nodes: []model.Node{{ID: "t", Type: "trigger", Trigger: events.ConversationUpdated},
		{ID: "c", Type: "branch", Condition: &condition}, {ID: "w", Type: "wait", DelaySeconds: 60}, {ID: "e", Type: "end"}},
		Edges: []model.Edge{{From: "t", Port: "next", To: "c", Input: "in"}, {From: "c", Port: "true", To: "w", Input: "in"},
			{From: "c", Port: "false", To: "e", Input: "in"}, {From: "w", Port: "next", To: "c", Input: "in", Loop: true}}}
	if err := ValidateGraph(g); err != nil {
		t.Fatal(err)
	}
	g.Edges[2].To = "w"
	if err := ValidateGraph(g); err == nil {
		t.Fatal("accepted loop without reachable end")
	}
}
