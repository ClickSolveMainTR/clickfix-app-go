package usecase

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/textproto"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/providers"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/mail"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type deliverTestQuerier struct {
	pushTestQuerier
	row      db.Notification
	claimErr error
	sent     int
	failed   int
	dead     int
	skipped  int
	stuck    []int64
}

func (d *deliverTestQuerier) MarkNotificationProcessing(context.Context, int64) (db.Notification, error) {
	if d.claimErr != nil {
		return db.Notification{}, d.claimErr
	}
	return d.row, nil
}

func (d *deliverTestQuerier) GetNotificationByID(context.Context, int64) (db.Notification, error) {
	return d.row, nil
}

func (d *deliverTestQuerier) MarkNotificationSent(context.Context, db.MarkNotificationSentParams) (db.Notification, error) {
	d.sent++
	d.row.Status = model.StatusDelivered
	return d.row, nil
}

func (d *deliverTestQuerier) ListStuckProcessingNotificationIDs(context.Context, int32) ([]int64, error) {
	return d.stuck, nil
}

func (d *deliverTestQuerier) ResumeNotificationProcessing(context.Context, int64) (db.Notification, error) {
	if d.row.Status != model.StatusProcessing || d.row.AttemptCount >= d.row.MaxAttempts {
		return db.Notification{}, pgx.ErrNoRows
	}
	d.row.AttemptCount++
	return d.row, nil
}

func (d *deliverTestQuerier) MarkNotificationFailed(_ context.Context, arg db.MarkNotificationFailedParams) (db.Notification, error) {
	d.failed++
	d.row.Status, d.row.LastError = model.StatusFailed, arg.LastError
	return d.row, nil
}

func (d *deliverTestQuerier) MarkNotificationDead(_ context.Context, arg db.MarkNotificationDeadParams) (db.Notification, error) {
	d.dead++
	d.row.Status, d.row.LastError = model.StatusFailed, arg.LastError
	return d.row, nil
}

func (d *deliverTestQuerier) MarkNotificationSkipped(_ context.Context, arg db.MarkNotificationSkippedParams) (db.Notification, error) {
	d.skipped++
	d.row.Status, d.row.LastError = model.StatusCancelled, arg.LastError
	return d.row, nil
}

type failingMail struct{ err error }

func (f failingMail) Send(context.Context, mail.Message) error { return f.err }

type recordingEnqueuer struct{ tasks []*asynq.Task }

func (r *recordingEnqueuer) Enqueue(task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	r.tasks = append(r.tasks, task)
	return &asynq.TaskInfo{}, nil
}

func emailRow(attempt, maxAttempts int32) db.Notification {
	return db.Notification{
		ID: 11, Channel: model.ChannelEmail, Status: model.StatusProcessing,
		// .local is not an RFC 2606 reserved name: the row is sendable.
		Recipient:    pgtype.Text{String: "agent@mailbox.local", Valid: true},
		AttemptCount: attempt, MaxAttempts: maxAttempts,
	}
}

var smtpTimeout = &mail.SendError{Stage: "dial", Addr: "smtp.example.com:587", Err: &net.OpError{Op: "dial", Err: context.DeadlineExceeded}}

func TestDeliverTransientFailureRetriesWithoutDeadLetter(t *testing.T) {
	t.Parallel()
	q := &deliverTestQuerier{row: emailRow(1, 5)}
	svc := New(q, &recordingEnqueuer{}, []providers.Provider{providers.EmailProvider{Mail: failingMail{smtpTimeout}}}, slog.Default())
	err := svc.Deliver(context.Background(), 11)
	var retry *RetryError
	if !errors.As(err, &retry) || retry.Cause != mail.CauseTimeout {
		t.Fatalf("want RetryError(timeout), got %v", err)
	}
	if q.failed != 1 || q.dead != 0 {
		t.Fatalf("failed=%d dead=%d", q.failed, q.dead)
	}
}

func TestDeliverLastAttemptDeadLetters(t *testing.T) {
	t.Parallel()
	q := &deliverTestQuerier{row: emailRow(5, 5)}
	svc := New(q, &recordingEnqueuer{}, []providers.Provider{providers.EmailProvider{Mail: failingMail{smtpTimeout}}}, slog.Default())
	if err := svc.Deliver(context.Background(), 11); err != nil {
		t.Fatalf("dead letter must stop retries, got %v", err)
	}
	if q.dead != 1 || q.failed != 0 {
		t.Fatalf("failed=%d dead=%d", q.failed, q.dead)
	}
}

func TestDeliverPermanentFailureDeadLettersAtOnce(t *testing.T) {
	t.Parallel()
	q := &deliverTestQuerier{row: emailRow(1, 5)}
	svc := New(q, &recordingEnqueuer{}, []providers.Provider{providers.EmailProvider{Mail: failingMail{mail.ErrNotConfigured}}}, slog.Default())
	if err := svc.Deliver(context.Background(), 11); err != nil {
		t.Fatalf("permanent failure must not be retried, got %v", err)
	}
	if q.dead != 1 {
		t.Fatalf("dead=%d", q.dead)
	}
}

func TestDeliverStuckRowWithoutAttemptsIsDeadLettered(t *testing.T) {
	t.Parallel()
	q := &deliverTestQuerier{claimErr: pgx.ErrNoRows, row: emailRow(5, 5)}
	svc := New(q, &recordingEnqueuer{}, []providers.Provider{providers.EmailProvider{Mail: failingMail{smtpTimeout}}}, slog.Default())
	if err := svc.Deliver(context.Background(), 11); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if q.dead != 1 || q.sent != 0 {
		t.Fatalf("dead=%d sent=%d", q.dead, q.sent)
	}
}

func TestReclaimStuckEnqueuesInsteadOfSending(t *testing.T) {
	t.Parallel()
	q := &deliverTestQuerier{claimErr: pgx.ErrNoRows, row: emailRow(1, 5), stuck: []int64{11, 12}}
	enq := &recordingEnqueuer{}
	svc := New(q, enq, []providers.Provider{providers.EmailProvider{Mail: failingMail{smtpTimeout}}}, slog.Default())
	n, err := svc.ReclaimStuck(context.Background(), 0)
	if err != nil || n != 2 || len(enq.tasks) != 2 {
		t.Fatalf("n=%d tasks=%d err=%v", n, len(enq.tasks), err)
	}
	if q.failed != 0 || q.dead != 0 || q.sent != 0 {
		t.Fatalf("reclaim must not deliver inline: failed=%d dead=%d sent=%d", q.failed, q.dead, q.sent)
	}
}

func TestDeliverResumesProcessingAfterClaimMiss(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		claimErr: pgx.ErrNoRows,
		row: db.Notification{
			ID:      7,
			Channel: model.ChannelInapp,
			Status:  model.StatusProcessing,
			Title:   "Hello",
			// DB default; resuming a processing row counts as an attempt.
			MaxAttempts: 5,
		},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	if err := svc.Deliver(context.Background(), 7); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if q.sent != 1 {
		t.Fatalf("MarkNotificationSent calls = %d, want 1", q.sent)
	}
}

func TestDeliverSkipsTerminalStatus(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		claimErr: pgx.ErrNoRows,
		row: db.Notification{
			ID:      7,
			Channel: model.ChannelInapp,
			Status:  model.StatusDelivered,
		},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	if err := svc.Deliver(context.Background(), 7); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if q.sent != 0 {
		t.Fatalf("MarkNotificationSent calls = %d, want 0", q.sent)
	}
}

func TestReclaimStuckDeliversProcessingRows(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		claimErr: pgx.ErrNoRows,
		row: db.Notification{
			ID:          3,
			Channel:     model.ChannelInapp,
			Status:      model.StatusProcessing,
			MaxAttempts: 5,
		},
		stuck: []int64{3},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	n, err := svc.ReclaimStuck(context.Background(), 0)
	if err != nil {
		t.Fatalf("ReclaimStuck: %v", err)
	}
	if n != 1 {
		t.Fatalf("reclaimed = %d, want 1", n)
	}
	if q.sent != 1 {
		t.Fatalf("MarkNotificationSent calls = %d, want 1", q.sent)
	}
}

type countingMail struct{ calls int }

func (c *countingMail) Send(context.Context, mail.Message) error { c.calls++; return nil }

func TestDeliverSkipsReservedDomain(t *testing.T) {
	t.Parallel()
	for _, addr := range []string{"demo@example.com", "seed@acme.test", "x@foo.invalid", "y@localhost"} {
		row := emailRow(1, 5)
		row.Recipient = pgtype.Text{String: addr, Valid: true}
		q := &deliverTestQuerier{row: row}
		sender := &countingMail{}
		svc := New(q, &recordingEnqueuer{}, []providers.Provider{providers.EmailProvider{Mail: sender}}, slog.Default())
		if err := svc.Deliver(context.Background(), 11); err != nil {
			t.Fatalf("%s: skip must not be retried, got %v", addr, err)
		}
		if sender.calls != 0 {
			t.Fatalf("%s: mail sent %d times, want 0", addr, sender.calls)
		}
		if q.skipped != 1 || q.dead != 0 || q.failed != 0 || q.sent != 0 {
			t.Fatalf("%s: skipped=%d dead=%d failed=%d sent=%d", addr, q.skipped, q.dead, q.failed, q.sent)
		}
		if q.row.Status != model.StatusCancelled || q.row.LastError.String != "skipped: "+providers.SkipReservedDomain {
			t.Fatalf("%s: row %q %q", addr, q.row.Status, q.row.LastError.String)
		}
	}
}

func TestDeliverReservedDomainAllowedInDevelopment(t *testing.T) {
	t.Parallel()
	row := emailRow(1, 5)
	row.Recipient = pgtype.Text{String: "demo@example.com", Valid: true}
	q := &deliverTestQuerier{row: row}
	sender := &countingMail{}
	svc := New(q, &recordingEnqueuer{}, []providers.Provider{providers.EmailProvider{Mail: sender, DeliverReserved: true}}, slog.Default())
	if err := svc.Deliver(context.Background(), 11); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if sender.calls != 1 || q.sent != 1 || q.skipped != 0 {
		t.Fatalf("calls=%d sent=%d skipped=%d", sender.calls, q.sent, q.skipped)
	}
}

// smtpReject is a permanent provider rejection (Resend answers 550 at DATA).
var smtpReject = &mail.SendError{Stage: "data", Addr: "smtp.example.com:587", Err: &textproto.Error{Code: 550, Msg: "Invalid `to` field"}}

func TestDeliverPermanentRejectionIsNotRetried(t *testing.T) {
	t.Parallel()
	q := &deliverTestQuerier{row: emailRow(1, 5)}
	enq := &recordingEnqueuer{}
	svc := New(q, enq, []providers.Provider{providers.EmailProvider{Mail: failingMail{smtpReject}}}, slog.Default())
	if err := svc.Deliver(context.Background(), 11); err != nil {
		t.Fatalf("5xx must not return a retry error, got %v", err)
	}
	if q.dead != 1 || q.failed != 0 || len(enq.tasks) != 0 {
		t.Fatalf("dead=%d failed=%d enqueued=%d", q.dead, q.failed, len(enq.tasks))
	}
}

func TestPermanentRejectionIsReportedAsWarning(t *testing.T) {
	t.Parallel()
	cause, transient := deliveryCause(smtpReject)
	if cause != mail.CauseRejected || transient {
		t.Fatalf("cause=%q transient=%v", cause, transient)
	}
	if !recipientRejected(cause) {
		t.Fatal("smtp 5xx must be reported as a warning, not an error")
	}
	for _, c := range []string{mail.CauseNotConfigured, mail.CauseAuth, mail.CauseTLS, mail.CauseTimeout} {
		if recipientRejected(c) {
			t.Fatalf("%s must stay an error", c)
		}
	}
}
