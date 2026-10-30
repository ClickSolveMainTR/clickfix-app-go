package model

import (
	"github.com/google/uuid"
	"time"
)

// Filter selects a creation cohort; To is exclusive. Backlog is the cohort's current open count.
type Filter struct {
	From, To                  time.Time
	Bucket, Timezone, Channel string
	Team, Agent, Tag          *uuid.UUID
}
