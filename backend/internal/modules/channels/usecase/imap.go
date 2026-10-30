package usecase

import (
	"context"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/email"
	"github.com/jackc/pgx/v5/pgtype"
)

// IMAPLocker serializes polling of one instance across worker processes.
type IMAPLocker interface {
	WithLock(context.Context, int64, func() error) error
}

// WithIMAPLocker enables the polling worker.
func (s *Service) WithIMAPLocker(locker IMAPLocker) *Service { s.imapLocker = locker; return s }

// EnqueueIMAPPolls fans out a minute tick into independent instance jobs.
// ListLiveChannelInstances is the existing cross-org worker query; all later
// reads and writes use the organization from each returned row.
func (s *Service) EnqueueIMAPPolls(ctx context.Context, enqueue func(context.Context, int64, int64) error) error {
	rows, err := s.q.ListLiveChannelInstances(ctx, []string{channels.TypeEmail})
	if err != nil {
		return err
	}
	for _, row := range rows {
		inst, _, err := s.open(row)
		if err != nil {
			continue
		}
		if !email.IsIMAP(inst.Credentials) {
			continue
		}
		if err := enqueue(ctx, row.OrganizationID, row.ID); err != nil {
			return err
		}
	}
	return nil
}

// PollIMAP stores messages through the exact same Receiver used by forwarding.
// A failed mailbox never prevents another organization's poll from running.
func (s *Service) PollIMAP(ctx context.Context, orgID, instanceID int64, receive Receiver) error {
	if s.imapLocker == nil {
		return errors.New("IMAP worker lock is not configured")
	}
	return s.imapLocker.WithLock(ctx, instanceID, func() error {
		inst, adapter, err := s.Instance(ctx, orgID, instanceID)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if inst.Type != channels.TypeEmail || !email.IsIMAP(inst.Credentials) || (inst.Status != StatusActive && inst.Status != StatusError) {
			return nil
		}
		a, ok := adapter.(*email.Adapter)
		if !ok {
			return nil
		}
		if err := s.q.EnsureEmailIMAPState(ctx, db.EnsureEmailIMAPStateParams{OrganizationID: orgID, ChannelInstanceID: instanceID}); err != nil {
			return err
		}
		state, err := s.q.GetEmailIMAPState(ctx, db.GetEmailIMAPStateParams{OrganizationID: orgID, ChannelInstanceID: instanceID})
		if err != nil {
			return err
		}
		if state.NextAttemptAt.Time.After(s.now()) {
			return nil
		}
		cursor := email.IMAPCursor{MailboxKey: state.MailboxKey, UIDValidity: safeconv.ToUint32(state.UidValidity), LastUID: safeconv.ToUint32(state.LastUid), InitialUID: safeconv.ToUint32(state.InitialUid)}
		err = a.PollIMAP(ctx, inst, cursor, func(res channels.WebhookResult) error { return receive(ctx, inst, res) }, func(c email.IMAPCursor) error {
			return s.q.SaveEmailIMAPCursor(ctx, db.SaveEmailIMAPCursorParams{MailboxKey: c.MailboxKey, UidValidity: int64(c.UIDValidity), LastUid: int64(c.LastUID), InitialUid: int64(c.InitialUID), OrganizationID: orgID, ChannelInstanceID: instanceID})
		}, nil)
		// Persist health/backoff even if a network operation exhausted the job context.
		finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		failures := int32(0)
		delay := time.Minute
		if err != nil {
			failures = min(state.Failures+1, 30)
			delay = time.Minute * time.Duration(1<<min(failures, 6))
		}
		if saveErr := s.q.FinishEmailIMAPPoll(finish, db.FinishEmailIMAPPollParams{Failures: failures, NextAttemptAt: pgtype.Timestamptz{Time: s.now().Add(delay), Valid: true}, OrganizationID: orgID, ChannelInstanceID: instanceID}); saveErr != nil {
			return saveErr
		}
		if err == nil || failures >= 3 {
			detail := ""
			if err != nil {
				// Do not persist parser, database, or provider error payloads.
				detail = (&email.IMAPError{}).Error()
				var ie *email.IMAPError
				if errors.As(err, &ie) {
					detail = ie.Error()
				}
			}
			if _, saveErr := s.q.SetChannelInstanceHealth(finish, db.SetChannelInstanceHealthParams{Ok: err == nil, Detail: detail, OrganizationID: orgID, ID: instanceID}); saveErr != nil {
				return saveErr
			}
		}
		// Backoff is persisted rather than delegated to Asynq retries, so ticks and
		// retries cannot defeat one another. Never put raw provider errors in logs.
		return nil
	})
}

// A successful reconnect or edited connection should not wait behind an old failure.
func (s *Service) resetIMAPBackoff(ctx context.Context, orgID, instanceID int64) error {
	return s.q.FinishEmailIMAPPoll(ctx, db.FinishEmailIMAPPollParams{OrganizationID: orgID, ChannelInstanceID: instanceID, NextAttemptAt: pgtype.Timestamptz{Time: s.now(), Valid: true}})
}
