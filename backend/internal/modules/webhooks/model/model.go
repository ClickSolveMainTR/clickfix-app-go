package model

// Input replaces a webhook subscription. The signing secret is returned only at creation.
type Input struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Events  []string `json:"events"`
	Enabled bool     `json:"enabled"`
}
