package model

import "time"

// KeyInput issues a server-side key, whose effective access is also limited by its creator's current membership.
type KeyInput struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	RateLimit int32      `json:"rate_limit"`
	ExpiresAt *time.Time `json:"expires_at"`
}

var Scopes = []string{"conversations:read", "conversations:write", "contacts:read", "contacts:write", "triggers:read", "companies:read", "webhooks:read", "webhooks:write", "revenue:write", "health:write", "events:write"}
