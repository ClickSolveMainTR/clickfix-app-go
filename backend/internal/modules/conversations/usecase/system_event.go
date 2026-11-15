package usecase

import "context"

// SystemEvent writes a system event (attrs carry "event") into a
// conversation's timeline, e.g. a department transfer (CF-215).
func (s *Service) SystemEvent(ctx context.Context, orgID, convID int64, actor Actor, attrs map[string]any) error {
	return s.AppointmentEvent(ctx, orgID, convID, actor, attrs)
}
