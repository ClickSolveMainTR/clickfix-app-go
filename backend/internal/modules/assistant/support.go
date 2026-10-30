package assistant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	helpusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/helpcenter/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
)

// InstanceLoader loads a channel instance (channels usecase).
type InstanceLoader interface {
	Instance(ctx context.Context, orgID, id int64) (channels.Instance, channels.Adapter, error)
}

// Receiver is the inbound conversation pipeline (conversations usecase).
type Receiver interface {
	Receive(ctx context.Context, inst channels.Instance, res channels.WebhookResult) (convusecase.ReceiveResult, error)
}

// SupportEscalator files an escalation as an inbound message in
// ClickFix's own support organization (its API, widget or e-mail inbox),
// so it is routed, SLA-tracked and answered like any other ticket. The
// requester is the escalating user; replies reach them by e-mail.
type SupportEscalator struct {
	Q         *db.Queries
	Instances InstanceLoader
	Receiver  Receiver
}

// Available reports whether the support organization has an inbox that
// can receive escalations.
func (e *SupportEscalator) Available(ctx context.Context, supportOrg uuid.UUID) bool {
	_, err := e.Q.AssistantSupportInbox(ctx, supportOrg)
	return err == nil
}

// Escalate implements Escalator.
func (e *SupportEscalator) Escalate(ctx context.Context, supportOrg uuid.UUID, in Escalation) (string, error) {
	inbox, err := e.Q.AssistantSupportInbox(ctx, supportOrg)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrSupportUnavailable
	}
	if err != nil {
		return "", err
	}
	inst, _, err := e.Instances.Instance(ctx, inbox.OrganizationID, inbox.ID)
	if err != nil {
		return "", err
	}
	external := "assistant-" + in.Thread.String()
	var body strings.Builder
	fmt.Fprintf(&body, "ClickFix Asistan üzerinden destek talebi\n\nİşletme: %s (%s)\nKullanıcı: %s <%s>\nSayfa: %s\n",
		in.Organization.Name, in.Organization.UUID, in.UserName, in.UserEmail, in.Route)
	if in.Note != "" {
		fmt.Fprintf(&body, "\nNot: %s\n", in.Note)
	}
	fmt.Fprintf(&body, "\n--- Asistan sohbeti ---\n%s", in.Transcript)
	sender := in.UserEmail
	if sender == "" {
		sender = "user-" + fmt.Sprint(in.UserID) + "@" + in.Organization.Slug
	}
	msg := channels.InboundMessage{
		ExternalID: external, Kind: "text", Text: clipRunes(body.String(), 15000),
		ThreadID: external, Subject: "ClickFix Asistan: " + in.Organization.Name, At: time.Now(),
		From:       channels.Sender{ExternalID: sender, Email: in.UserEmail, Name: in.UserName},
		Attributes: map[string]any{"source": "clickfix_assistant", "organization_uuid": in.Organization.UUID.String(), "thread_uuid": in.Thread.String()},
	}
	if _, err := e.Receiver.Receive(ctx, inst, channels.WebhookResult{Messages: []channels.InboundMessage{msg}}); err != nil {
		return "", err
	}
	conv, err := e.Q.AssistantEscalatedConversation(ctx, db.AssistantEscalatedConversationParams{
		OrganizationID: inbox.OrganizationID, ChannelInstanceID: i8(inbox.ID), ExternalID: pgtype.Text{String: external, Valid: true},
	})
	if err != nil {
		// Stored but not found (e.g. merged): the thread id is still a usable reference.
		return "CFA-" + strings.ToUpper(in.Thread.String()[:8]), nil
	}
	return fmt.Sprintf("#%d", conv.Number), nil
}

// HelpAdapter searches the support organization's public help center.
type HelpAdapter struct {
	Q    *db.Queries
	Help *helpusecase.Service
}

// SearchHelp implements HelpSearcher.
func (h *HelpAdapter) SearchHelp(ctx context.Context, supportOrg uuid.UUID, query, locale string) ([]map[string]any, error) {
	org, err := h.Q.GetOrganizationByUUID(ctx, supportOrg)
	if err != nil {
		return nil, err
	}
	items, err := h.Help.SearchOrganization(ctx, org.ID, query, locale, 4)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, a := range items {
		out = append(out, map[string]any{"title": a.Title, "excerpt": clipRunes(a.Excerpt, 300), "slug": a.Slug})
	}
	return out, nil
}
