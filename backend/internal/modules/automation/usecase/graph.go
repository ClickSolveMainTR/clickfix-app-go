package usecase

import (
	"encoding/json"
	"fmt"
	routing "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/usecase"
	"slices"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
)

const MaxWorkflowSteps = 256

func WorkflowTriggers() []string {
	return []string{events.ConversationCreated, events.ConversationUpdated, events.MessageReceived,
		events.ConversationTagAdded, "sla.breached", "scheduled", "webhook"}
}

// ValidateGraph enforces control port types, bounded conditions and explicit wait-only loops.
func ValidateGraph(g model.Graph) error {
	if g.SchemaVersion != 1 || len(g.Nodes) < 2 || len(g.Nodes) > 100 || len(g.Edges) > 200 {
		return fmt.Errorf("%w: graph version 1 requires 2-100 nodes and <=200 edges", ErrInvalid)
	}
	nodes := map[string]model.Node{}
	triggers := 0
	for _, n := range g.Nodes {
		if strings.TrimSpace(n.ID) == "" || len(n.ID) > 80 {
			return ErrInvalid
		}
		if _, ok := nodes[n.ID]; ok {
			return fmt.Errorf("%w: duplicate node", ErrInvalid)
		}
		nodes[n.ID] = n
		switch n.Type {
		case "trigger":
			triggers++
			if !slices.Contains(WorkflowTriggers(), n.Trigger) {
				return fmt.Errorf("%w: unknown trigger", ErrInvalid)
			}
			if (n.Trigger == "scheduled") != (n.ScheduleMinutes > 0) || n.ScheduleMinutes > 525600 {
				return ErrInvalid
			}
		case "condition", "branch":
			count := 0
			if n.Condition == nil {
				return ErrInvalid
			}
			if err := validateCondition(*n.Condition, 0, &count); err != nil {
				return fmt.Errorf("%w: %v", ErrInvalid, err)
			}
		case "action":
			if n.Action == nil {
				return ErrInvalid
			}
			if err := validateGraphAction(*n.Action); err != nil {
				return err
			}
		case "wait":
			if n.DelaySeconds < 60 || n.DelaySeconds > 2592000 {
				return fmt.Errorf("%w: wait must be 60-2592000 seconds", ErrInvalid)
			}
		case "end":
		default:
			return fmt.Errorf("%w: unknown node type", ErrInvalid)
		}
	}
	if triggers != 1 {
		return fmt.Errorf("%w: exactly one trigger required", ErrInvalid)
	}
	outgoing := map[string]map[string]string{}
	adj := map[string][]string{}
	reverse := map[string][]string{}
	for _, e := range g.Edges {
		from, ok := nodes[e.From]
		if !ok {
			return ErrInvalid
		}
		to, ok := nodes[e.To]
		if !ok || to.Type == "trigger" || e.Input != "in" {
			return fmt.Errorf("%w: incompatible input port", ErrInvalid)
		}
		if !slices.Contains(ports(from), e.Port) || e.Loop && from.Type != "wait" {
			return fmt.Errorf("%w: incompatible output or loop port", ErrInvalid)
		}
		if outgoing[e.From] == nil {
			outgoing[e.From] = map[string]string{}
		}
		if _, ok := outgoing[e.From][e.Port]; ok {
			return fmt.Errorf("%w: output ports have one target", ErrInvalid)
		}
		outgoing[e.From][e.Port] = e.To
		if !e.Loop {
			adj[e.From] = append(adj[e.From], e.To)
		}
		reverse[e.To] = append(reverse[e.To], e.From)
	}
	state := map[string]int{}
	var visit func(string) bool
	visit = func(id string) bool {
		if state[id] == 1 {
			return false
		}
		if state[id] == 2 {
			return true
		}
		state[id] = 1
		for _, to := range adj[id] {
			if !visit(to) {
				return false
			}
		}
		state[id] = 2
		return true
	}
	for id, n := range nodes {
		if !visit(id) {
			return fmt.Errorf("%w: cycles require an explicit wait loop edge", ErrInvalid)
		}
		if len(outgoing[id]) != len(ports(n)) {
			return fmt.Errorf("%w: every output must be connected", ErrInvalid)
		}
	}
	// Every node must be reachable from the trigger and able to reach an end.
	reachable := map[string]bool{}
	var walk func(string, map[string][]string, map[string]bool)
	walk = func(id string, a map[string][]string, seen map[string]bool) {
		if seen[id] {
			return
		}
		seen[id] = true
		for _, v := range a[id] {
			walk(v, a, seen)
		}
	}
	all := map[string][]string{}
	for _, e := range g.Edges {
		all[e.From] = append(all[e.From], e.To)
	}
	ends := map[string]bool{}
	for _, n := range g.Nodes {
		if n.Type == "trigger" {
			walk(n.ID, all, reachable)
		}
		if n.Type == "end" {
			walk(n.ID, reverse, ends)
		}
	}
	for id := range nodes {
		if !reachable[id] || !ends[id] {
			return fmt.Errorf("%w: unreachable node or end", ErrInvalid)
		}
	}
	return nil
}
func ports(n model.Node) []string {
	switch n.Type {
	case "end":
		return []string{}
	case "branch", "condition":
		return []string{"true", "false"}
	default:
		return []string{"next"}
	}
}
func validateGraphAction(a model.Action) error {
	switch a.Type {
	case "run_ai_procedure", "send_template":
		if a.TargetUUID == uuid.Nil {
			return ErrInvalid
		}
		return nil
	case "create_engineering_issue":
		if strings.TrimSpace(a.Value) == "" || len(a.Value) > 200 || len(a.Content) > 10000 {
			return ErrInvalid
		}
		return nil
	case "route":
		var snapshot routing.WorkflowSnapshot
		if len(a.Content) > 10000 || json.Unmarshal([]byte(a.Content), &snapshot) != nil {
			return ErrInvalid
		}
		if !slices.Contains([]string{"round_robin", "load_balanced"}, snapshot.Settings.Strategy) || len(snapshot.Skills) > 32 {
			return ErrInvalid
		}
		for _, skill := range snapshot.Skills {
			if strings.TrimSpace(skill) == "" || len(skill) > 80 {
				return ErrInvalid
			}
		}
		return nil
	default:
		return Validate(model.RuleInput{Name: "graph action", Trigger: events.ConversationCreated,
			Conditions: model.Condition{All: []model.Condition{}}, Actions: []model.Action{a}})
	}
}
func graphNode(g model.Graph, id string) model.Node {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n
		}
	}
	return model.Node{}
}
func graphNext(g model.Graph, id, port string) string {
	for _, e := range g.Edges {
		if e.From == id && e.Port == port {
			return e.To
		}
	}
	return ""
}
func graphTrigger(g model.Graph) model.Node {
	for _, n := range g.Nodes {
		if n.Type == "trigger" {
			return n
		}
	}
	return model.Node{}
}
func graphPort(n model.Node, data map[string]any) string {
	if n.Type == "condition" || n.Type == "branch" {
		if matches(*n.Condition, data) {
			return "true"
		}
		return "false"
	}
	return "next"
}
