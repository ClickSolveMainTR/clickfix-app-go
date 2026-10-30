// Package model defines the community forum input contract.
package model

import "github.com/google/uuid"

type PostInput struct {
	Category string `json:"category"`
	Title    string `json:"title"`
	Body     string `json:"body"`
}
type ConversionInput struct {
	Category     string    `json:"category"`
	Title        string    `json:"title"`
	Body         string    `json:"body"` // Agent-redacted text; private transcripts are never copied automatically.
	Conversation uuid.UUID `json:"conversation_uuid"`
	Inbox        uuid.UUID `json:"inbox_uuid"`
}
