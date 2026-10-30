package model

import (
	"errors"
	"github.com/google/uuid"
)

var ErrInvalid = errors.New("surveys: invalid input")
var ErrExpired = errors.New("surveys: invitation expired")

type TemplateInput struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Question  string `json:"question"`
	OnResolve bool   `json:"on_resolve"`
	Enabled   bool   `json:"enabled"`
}
type InviteInput struct {
	Template     uuid.UUID `json:"template_uuid"`
	Conversation uuid.UUID `json:"conversation_uuid"`
	PeriodKey    string    `json:"period_key"`
}
type Answer struct {
	Score   int16  `json:"score"`
	Comment string `json:"comment"`
}
