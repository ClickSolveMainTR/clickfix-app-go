// Package supportoperations wires the operations modules to reliable events.
package supportoperations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	notifmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	notifusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/quality"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"strings"
)

func RegisterEvents(bus events.ReliableBus, repo *repository.Repository, ai supportops.AI, conversations *convusecase.Service, notifications *notifusecase.Service, pub realtime.Publisher) {
	qa := &quality.Service{Repo: repo, AI: ai}
	bus.SubscribeReliable(events.ConversationResolved, qa.AutoResolved)
	chat := func(ctx context.Context, ev events.Event) error {
		if ev.TenantID == nil || ev.EntityID == nil || ev.EntityUUID == nil {
			return nil
		}
		org := *ev.TenantID
		members, err := repo.Q.ChatThreadMembers(ctx, db.ChatThreadMembersParams{OrganizationID: org, ThreadID: *ev.EntityID})
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(ev.Payload["mentions"])
		var mentions []uuid.UUID
		_ = json.Unmarshal(raw, &mentions)
		mentioned := map[uuid.UUID]bool{}
		for _, id := range mentions {
			mentioned[id] = true
		}
		for _, m := range members {
			if pub != nil {
				if err = pub.Publish(ctx, "user:"+m.Uuid.String(), map[string]any{"type": "teamchat.changed", "thread_uuid": ev.EntityUUID, "organization_id": org}); err != nil {
					return err
				}
			}
			if notifications != nil && mentioned[m.Uuid] && (ev.ActorUserID == nil || *ev.ActorUserID != m.ID) {
				payload := map[string]any{"source_event_id": ev.EventID.String(), "thread_uuid": ev.EntityUUID}
				vars := map[string]string{}
				if ev.ActorUserID != nil {
					if u, e := repo.Q.GetUserByID(ctx, *ev.ActorUserID); e == nil {
						vars["actor"] = strings.TrimSpace(u.Name + " " + u.Surname)
					}
				}
				// In-app also reaches the realtime channel and the push mirror.
				_, err = notifications.Enqueue(ctx, notifmodel.EnqueueInput{TenantID: &org, UserID: &m.ID, Channels: []string{"inapp", "email"}, Title: "Ekip sohbetinde sizden bahsedildi", Body: "Ekip sohbetinde yeni bir mesaj var.", SourceEvent: "teamchat.mentioned", Payload: payload, ContentVars: vars})
				if err != nil {
					return err
				}
			}
		}
		return nil
	}
	bus.SubscribeReliable("teamchat.changed", chat)
	bus.SubscribeReliable("teamchat.message_created", chat)
	notice := func(ctx context.Context, ev events.Event) error {
		if ev.TenantID == nil || ev.EntityID == nil {
			return nil
		}
		org := *ev.TenantID
		request := *ev.EntityID
		if ev.Name == "feedback.changelog_published" {
			raw, _ := json.Marshal(ev.Payload["request_id"])
			if json.Unmarshal(raw, &request) != nil || request == 0 {
				return nil
			}
		} else {
			if fmt.Sprint(ev.Payload["status"]) != "released" || ev.Payload["public"] != true {
				return nil
			}
		}
		enabled, err := repo.Q.FeedbackSettings(ctx, org)
		if err != nil {
			return err
		}
		if !enabled {
			return nil
		}
		voters, err := repo.Q.FeedbackVoters(ctx, db.FeedbackVotersParams{OrganizationID: org, RequestID: request})
		if err != nil {
			return err
		}
		for _, v := range voters {
			title := fmt.Sprint(ev.Payload["title"])
			body := "İstediğiniz özellik yayınlandı: " + title
			client := uuid.NewSHA1(ev.EventID, []byte(fmt.Sprint(v.ID)))
			if conversations != nil {
				id, e := repo.Q.FeedbackVoterConversation(ctx, db.FeedbackVoterConversationParams{OrganizationID: org, ContactID: v.ID})
				if e == nil {
					_, e = conversations.Post(ctx, convusecase.PostInput{OrganizationID: org, ConversationUUID: id, AuthorName: "ClickFix", Content: body, ClientID: client})
					if e != nil {
						return e
					}
					continue
				}
				if !errors.Is(e, pgx.ErrNoRows) {
					return e
				}
			}
			if notifications != nil && v.Email.Valid && v.Email.String != "" {
				email := v.Email.String
				_, err = notifications.Enqueue(ctx, notifmodel.EnqueueInput{TenantID: &org, Channels: []string{"email"}, Recipient: &email, Title: title, Body: body, SourceEvent: ev.Name, Payload: map[string]any{"source_event_id": client.String()}})
				if err != nil {
					return err
				}
			}
		}
		return nil
	}
	bus.SubscribeReliable("feedback.status_changed", notice)
	bus.SubscribeReliable("feedback.changelog_published", notice)
}
