package model

// Graph is a versioned editor document. UI positions are inert metadata.
type Graph struct {
	SchemaVersion int    `json:"schema_version"`
	Nodes         []Node `json:"nodes"`
	Edges         []Edge `json:"edges"`
}
type Node struct {
	ID              string             `json:"id"`
	Type            string             `json:"type"`
	Trigger         string             `json:"trigger,omitempty"`
	ScheduleMinutes int32              `json:"schedule_minutes,omitempty"`
	Condition       *Condition         `json:"condition,omitempty"`
	Action          *Action            `json:"action,omitempty"`
	DelaySeconds    int32              `json:"delay_seconds,omitempty"`
	Position        map[string]float64 `json:"position,omitempty"`
}
type Edge struct {
	From  string `json:"from"`
	Port  string `json:"port"`
	To    string `json:"to"`
	Input string `json:"input"`
	Loop  bool   `json:"loop,omitempty"`
}
type WorkflowInput struct {
	Name     string `json:"name"`
	Graph    Graph  `json:"graph"`
	Revision int32  `json:"revision"`
}
type TraceStep struct {
	NodeID string `json:"node_id"`
	Type   string `json:"type"`
	Port   string `json:"port"`
	Status string `json:"status"`
}
