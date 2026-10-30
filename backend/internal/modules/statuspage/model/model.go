package model

import (
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
	"time"
)

var ErrInvalid = errors.New("status: invalid input")
var ErrConflict = errors.New("status: incident already resolved")

type PageInput struct {
	Slug         string `json:"slug"`
	Title        string `json:"title"`
	CustomDomain string `json:"custom_domain"`
	Published    bool   `json:"published"`
}
type ComponentInput struct {
	Name        string `json:"name"`
	Group       string `json:"group_name"`
	Description string `json:"description"`
	SortOrder   int32  `json:"sort_order"`
}
type IncidentInput struct {
	Title                string             `json:"title"`
	Kind                 string             `json:"kind"`
	Severity             string             `json:"severity"`
	Public               bool               `json:"public"`
	StartedAt            time.Time          `json:"started_at"`
	EndsAt               *time.Time         `json:"ends_at"`
	Components           []uuid.UUID        `json:"components"`
	Body                 string             `json:"body"`
	NotificationBody     string             `json:"notification_body"`
	NotificationTemplate *channels.Template `json:"notification_template"`
	Postmortem           string             `json:"postmortem"`
}
type UpdateInput struct {
	Status     string `json:"status"`
	Body       string `json:"body"`
	Postmortem string `json:"postmortem"`
}
