package usecase

import (
	"context"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/businesshours"
)

// The organization's working hours as the other modules see them (CF-198):
// the widget's online state, the AI agent's "inside hours" check and the
// automatic replies all read the default schedule of this module, so a
// change here moves all of them at once.

// OrganizationCalendar compiles the organization's default schedule with its
// holidays. nil means no schedule: open around the clock.
func (s *Service) OrganizationCalendar(ctx context.Context, orgID int64) (*businesshours.Calendar, error) {
	bh, err := s.q.GetDefaultBusinessHours(ctx, orgID)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.compile(ctx, s.q, bh)
}

// HoursState implements businesshours.Source.
func (s *Service) HoursState(ctx context.Context, orgID int64, at time.Time) (businesshours.State, error) {
	cal, err := s.OrganizationCalendar(ctx, orgID)
	if err != nil {
		return businesshours.State{}, err
	}
	return cal.State(at), nil
}

// FirstResponseTarget is the first-response target (working time) of the
// policy that applies to a conversation, nil when no policy matches. The
// automatic acknowledgement quotes it as the estimated reply time.
func (s *Service) FirstResponseTarget(ctx context.Context, orgID, conversationID int64) (*time.Duration, error) {
	conv, err := s.q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: orgID, ID: conversationID})
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	pol, err := s.match(ctx, s.q, conv)
	if err != nil || pol == nil || pol.Targets.FirstResponse <= 0 {
		return nil, err
	}
	d := pol.Targets.FirstResponse
	return &d, nil
}
