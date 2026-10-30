package usecase

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/url"
	"regexp"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/google/uuid"
)

var sentryEventPattern = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
var sentryIssuePattern = regexp.MustCompile(`^[0-9]{1,30}$`)

// ErrorInput attaches a Sentry error to the affected conversation and the
// shared issue. Future Sentry resolution webhooks notify every linked customer.
type ErrorInput struct {
	IssueUUID     uuid.UUID `json:"issue_uuid"`
	SentryIssueID string    `json:"sentry_issue_id"`
	EventID       string    `json:"event_id"`
	EventURL      string    `json:"event_url"`
	Summary       string    `json:"summary"`
}

func (s *Service) AttachError(ctx context.Context, org, actor int64, conversation uuid.UUID, in ErrorInput) (map[string]any, error) {
	u, err := url.Parse(in.EventURL)
	if err != nil || !validURL(in.EventURL) || (u.Hostname() != "sentry.io" && !strings.HasSuffix(u.Hostname(), ".sentry.io")) || !sentryEventPattern.MatchString(in.EventID) || !sentryIssuePattern.MatchString(in.SentryIssueID) || strings.TrimSpace(in.Summary) == "" || len(in.Summary) > 2000 {
		return nil, invalid("valid Sentry issue, event, HTTPS URL and summary are required")
	}
	if _, err = s.Inbox.Get(ctx, org, actor, conversation); err != nil {
		return nil, missing(err)
	}
	var event db.EngineeringErrorEvent
	err = s.Store.InTx(ctx, func(ctx context.Context, q *db.Queries) error {
		integration, err := q.EngineeringIntegration(ctx, db.EngineeringIntegrationParams{OrganizationID: org, Provider: "sentry"})
		if err != nil {
			return missing(err)
		}
		if !integration.Enabled {
			return ErrUnavailable
		}
		issue, err := q.EngineeringLockIssue(ctx, db.EngineeringLockIssueParams{OrganizationID: org, Uuid: in.IssueUUID})
		if err != nil {
			return missing(err)
		}
		c, err := q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: conversation})
		if err != nil {
			return missing(err)
		}
		// Do not change an existing customer's opt-out when attaching another event.
		notify, err := q.EngineeringLinkCurrent(ctx, db.EngineeringLinkCurrentParams{OrganizationID: org, IssueID: issue.ID, ConversationID: c.ID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if errors.Is(err, pgx.ErrNoRows) {
			notify = true
		}
		if _, err = q.EngineeringLink(ctx, db.EngineeringLinkParams{OrganizationID: org, IssueID: issue.ID, ConversationID: c.ID, NotifyCustomer: notify}); err != nil {
			return err
		}
		current, e := q.EngineeringExternalIssue(ctx, db.EngineeringExternalIssueParams{OrganizationID: org, IntegrationID: integration.ID, IssueID: issue.ID})
		if e == nil && current.ExternalID != in.SentryIssueID {
			return ErrConflict
		}
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if _, err = q.EngineeringExternalSave(ctx, db.EngineeringExternalSaveParams{OrganizationID: org, IntegrationID: integration.ID, IssueID: issue.ID, ExternalID: in.SentryIssueID, ExternalKey: in.SentryIssueID, ExternalUrl: in.EventURL, SyncedRevision: issue.Revision}); err != nil {
			return err
		}
		event, err = q.EngineeringAttachError(ctx, db.EngineeringAttachErrorParams{OrganizationID: org, IssueID: issue.ID, ConversationID: c.ID, SentryIssueID: in.SentryIssueID, EventID: strings.ToLower(in.EventID), EventUrl: in.EventURL, Summary: in.Summary})
		if err != nil {
			return err
		}
		return emit(ctx, q, issue, actor, "error_attached")
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"uuid": event.Uuid, "issue_uuid": in.IssueUUID, "sentry_issue_id": event.SentryIssueID, "event_id": event.EventID, "event_url": event.EventUrl, "summary": event.Summary}, nil
}
func (s *Service) Errors(ctx context.Context, org, actor int64, conversation uuid.UUID) ([]map[string]any, error) {
	if _, err := s.Inbox.Get(ctx, org, actor, conversation); err != nil {
		return nil, missing(err)
	}
	c, err := s.Store.Q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: conversation})
	if err != nil {
		return nil, missing(err)
	}
	rows, err := s.Store.Q.EngineeringErrorEvents(ctx, db.EngineeringErrorEventsParams{OrganizationID: org, ConversationID: c.ID})
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, r := range rows {
		out = append(out, map[string]any{"uuid": r.Uuid, "issue_uuid": r.IssueUuid, "sentry_issue_id": r.SentryIssueID, "event_id": r.EventID, "event_url": r.EventUrl, "summary": r.Summary})
	}
	return out, nil
}
func (s *Service) sentryIssue(ctx context.Context, c Credentials, id string) (RemoteIssue, error) {
	if !sentryIssuePattern.MatchString(id) {
		return RemoteIssue{}, invalid("invalid Sentry issue ID")
	}
	adapter := providerAdapter{Provider: "sentry", Client: s.SentryClient}
	var r struct{ ID, Title, Permalink, Status string }
	if err := adapter.request(ctx, c, "GET", "https://sentry.io/api/0/issues/"+id+"/", nil, &r); err != nil {
		return RemoteIssue{}, err
	}
	status := "todo"
	if r.Status == "resolved" {
		status = "done"
	}
	if r.Status == "ignored" || r.Status == "archived" {
		status = "canceled"
	}
	// A Sentry update changes status/title, preserving the support context.
	return RemoteIssue{ID: r.ID, Key: r.ID, URL: r.Permalink, Title: r.Title, Status: status}, nil
}
