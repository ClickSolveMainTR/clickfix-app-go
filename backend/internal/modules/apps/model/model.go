package model

import (
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var ErrInvalid = errors.New("invalid sidebar app")
var ErrConflict = errors.New("sidebar app revision conflict")

var ErrDenied = errors.New("sidebar app action denied")
var Scopes = map[string]string{
	"conversation.read":     "tenant.conversations.read",
	"contact.read":          "tenant.contacts.read",
	"conversation.reply":    "tenant.conversations.reply",
	"conversation.note":     "tenant.conversations.note",
	"conversation.status":   "tenant.conversations.manage",
	"conversation.priority": "tenant.conversations.manage",
	"conversation.assign":   "tenant.conversations.assign",
	"conversation.tag":      "tenant.conversations.manage",
}

type Input struct {
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Icon      string   `json:"icon"`
	Placement string   `json:"placement"`
	Scopes    []string `json:"scopes"`
	Enabled   bool     `json:"enabled"`
	Revision  int32    `json:"revision"`
}

func (in Input) Validate() error {
	u, e := url.Parse(in.URL)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || len(in.URL) > 2048 {
		return ErrInvalid
	}
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 120 || len(in.Icon) > 80 || len(in.Scopes) > len(Scopes) {
		return ErrInvalid
	}
	// Icon is an inert icon identifier, never HTML or a fetched resource.
	for _, c := range in.Icon {
		isLetter := c >= 'a' && c <= 'z'
		isDigit := c >= '0' && c <= '9'
		if !isLetter && !isDigit && c != '-' {
			return ErrInvalid
		}
	}
	if !slices.Contains([]string{"conversation_sidebar", "contact_panel", "nav"}, in.Placement) {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, scope := range in.Scopes {
		if _, ok := Scopes[scope]; !ok || seen[scope] {
			return ErrInvalid
		}
		seen[scope] = true
	}
	return nil
}

type Claims struct {
	App          uuid.UUID  `json:"app_uuid"`
	Organization uuid.UUID  `json:"organization_uuid"`
	Conversation *uuid.UUID `json:"conversation_uuid,omitempty"`
	Contact      *uuid.UUID `json:"contact_uuid,omitempty"`
	Scopes       []string   `json:"scopes"`
	Revision     int32      `json:"app_revision"`
	jwt.RegisteredClaims
}
type LaunchInput struct {
	Conversation *uuid.UUID `json:"conversation_uuid,omitempty"`
	Contact      *uuid.UUID `json:"contact_uuid,omitempty"`
}
type Launch struct {
	URL       string   `json:"url"`
	Origin    string   `json:"origin"`
	Token     string   `json:"context_token"`
	ExpiresIn int      `json:"expires_in"`
	Scopes    []string `json:"scopes"`
}
type ActionInput struct {
	Token     string    `json:"context_token"`
	Action    string    `json:"action"`
	Value     string    `json:"value,omitempty"`
	Content   string    `json:"content,omitempty"`
	Target    uuid.UUID `json:"target_uuid,omitempty"`
	RequestID uuid.UUID `json:"request_id"`
}
