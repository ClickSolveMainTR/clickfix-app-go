package inbox

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/sms"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/voice"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func (i *Inbox) EnqueueNetgsm(ctx context.Context) error {
	if i.jobs == nil {
		return nil
	}
	rows, err := i.queries.ListLiveChannelInstances(ctx, []string{channels.TypeSMS, channels.TypeVoice})
	if err != nil {
		return err
	}
	for _, row := range rows {
		inst, _, err := i.Channels.Instance(ctx, row.OrganizationID, row.ID)
		if err != nil || inst.Credentials["provider"] != "netgsm" {
			continue
		}
		if err := i.jobs.EnqueueNetgsmPoll(ctx, row.OrganizationID, row.ID); err != nil {
			return err
		}
	}
	return nil
}
func (i *Inbox) PollNetgsm(ctx context.Context, org, id int64) error {
	inst, a, err := i.Channels.Instance(ctx, org, id)
	if err != nil {
		return err
	}
	if inst.Credentials["provider"] != "netgsm" || (inst.Status != "active" && inst.Status != "error") {
		return nil
	}
	if inst.Type == channels.TypeVoice {
		// Account cooldown intentionally survives success and failure; retries cannot defeat rate limits.
		if i.redis == nil {
			return errors.New("netgsm report requires Redis cooldown")
		}
		key := fmt.Sprintf("app:%s:netgsm:cdr:%x", i.env, sha256.Sum256([]byte(inst.Credentials["usercode"])))
		ok, err := i.redis.SetNX(ctx, key, "1", 31*time.Second).Result()
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("netgsm report cooldown active")
		}
		v, ok := a.(*voice.Adapter)
		if !ok {
			return nil
		}
		day := time.Now()
		if day.Unix()/60%2 == 0 {
			day = day.AddDate(0, 0, -1)
		}
		res, err := v.ReconcileNetgsm(ctx, inst, day)
		if err != nil {
			return err
		}
		if err = i.Receive(ctx, inst, res); err != nil {
			return err
		}
		pending, err := i.queries.ListPendingNetgsmRecordings(ctx,
			db.ListPendingNetgsmRecordingsParams{OrganizationID: org,
				ChannelInstanceID: id})
		if err != nil {
			return err
		}
		for _, c := range pending {
			if err := i.jobs.EnqueueNetgsmRecording(ctx, org, id, c.ExternalID); err != nil {
				return err
			}
		}
		return nil
	}
	s, ok := a.(*sms.Adapter)
	if !ok {
		return nil
	}
	ids, err := i.queries.ListPendingNetgsmSMS(ctx,
		db.ListPendingNetgsmSMSParams{OrganizationID: org,
			ChannelInstanceID: pgtype.Int8{Int64: id,
				Valid: true}})
	if err != nil {
		return err
	}
	res, pollErr := s.PollNetgsm(ctx, inst, ids)
	return errors.Join(i.Receive(ctx, inst, res), pollErr)
}
func (i *Inbox) ArchiveNetgsm(ctx context.Context, org, id int64, external string) error {
	if i.redis != nil {
		key := fmt.Sprintf("app:%s:netgsm:recording:%d:%d:%x", i.env, org, id, sha256.Sum256([]byte(external)))
		token := uuid.NewString()
		acquired, err := i.redis.SetNX(ctx, key, token, 4*time.Minute).Result()
		if err != nil {
			return err
		}
		if !acquired {
			return errors.New("recording archive already running")
		}
		defer func() {
			finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = i.redis.Eval(finish, `if redis.call("get",KEYS[1])==ARGV[1] then return redis.call("del",KEYS[1]) end return 0`, []string{key}, token).Err()
		}()
	}

	inst, a, err := i.Channels.Instance(ctx, org, id)
	if err != nil {
		return err
	}
	if inst.Type != channels.TypeVoice || inst.Credentials["provider"] != "netgsm" || inst.Status == "paused" || inst.Status == "disconnected" {
		return nil
	}
	c, err := i.queries.GetChannelVoiceCall(ctx,
		db.GetChannelVoiceCallParams{OrganizationID: org,
			ChannelInstanceID: id,
			ExternalID:        external})
	if err != nil {
		return err
	}
	if c.RecordingMessageID.Valid || c.RecordingUrl == "" {
		return nil
	}
	v, ok := a.(*voice.Adapter)
	if !ok {
		return nil
	}
	res, err := v.RecordingMessage(channels.CallEvent{ExternalID: c.ExternalID,
		Provider:        c.Provider,
		Direction:       c.Direction,
		Status:          c.Status,
		From:            c.FromNumber,
		To:              c.ToNumber,
		InternalNumber:  c.InternalNumber,
		DurationSeconds: c.DurationSeconds,
		RecordingURL:    c.RecordingUrl,
		At:              c.LastEventAt.Time})
	if err != nil {
		return err
	}
	m := res.Messages[0]
	files := m.Attachments
	m.Attachments = nil
	res.Messages = []channels.InboundMessage{m}
	if _, err = i.Conversations.Receive(ctx, inst, res); err != nil {
		return err
	}
	if err = i.Conversations.ArchiveInbound(ctx, inst, m.ExternalID, files); err != nil {
		return err
	}
	res.Messages = nil
	return i.Receive(ctx, inst, res)
}

func (i *Inbox) storeVoiceCall(ctx context.Context, inst channels.Instance, call channels.CallEvent) error {
	apply := func(q *db.Queries) error {
		if call.Provider == "netgsm" {
			phone := call.From
			if call.Direction == "outbound" {
				phone = call.To
			}
			if err := q.ConsolidateNetgsmClientCall(ctx,
				db.ConsolidateNetgsmClientCallParams{OrganizationID: inst.OrganizationID,
					ChannelInstanceID: inst.ID,
					ExternalID:        call.ExternalID,
					InternalNumber:    call.InternalNumber,
					Phone:             voice.PhoneLast10(phone),
					Direction:         call.Direction,
					InitiatedAt: pgtype.Timestamptz{Time: call.InitiatedAt,
						Valid: !call.InitiatedAt.IsZero()}}); err != nil {
				return err
			}
			if err := q.MergeNetgsmProviderCall(ctx,
				db.MergeNetgsmProviderCallParams{OrganizationID: inst.OrganizationID,
					ChannelInstanceID: inst.ID,
					ExternalID:        call.ExternalID,
					InternalNumber:    call.InternalNumber,
					Phone:             voice.PhoneLast10(phone),
					Direction:         call.Direction,
					InitiatedAt: pgtype.Timestamptz{Time: call.InitiatedAt,
						Valid: !call.InitiatedAt.IsZero()}}); err != nil {
				return err
			}
		}
		return q.UpsertChannelVoiceCall(ctx,
			db.UpsertChannelVoiceCallParams{IsReport: call.FromReport, OrganizationID: inst.OrganizationID,
				ChannelInstanceID:   inst.ID,
				ExternalID:          call.ExternalID,
				Provider:            call.Provider,
				Direction:           call.Direction,
				Status:              call.Status,
				FromNumber:          call.From,
				ToNumber:            call.To,
				DurationSeconds:     call.DurationSeconds,
				RecordingExternalID: call.RecordingMessageExternalID,
				LastEventAt: pgtype.Timestamptz{Time: call.At,
					Valid: true},
				InternalNumber: call.InternalNumber,
				RecordingUrl:   call.RecordingURL,
				AnsweredAt: pgtype.Timestamptz{Time: call.AnsweredAt,
					Valid: !call.AnsweredAt.IsZero()},
				EndedAt: pgtype.Timestamptz{Time: call.EndedAt,
					Valid: !call.EndedAt.IsZero()},
				InitiatedAt: pgtype.Timestamptz{Time: call.InitiatedAt,
					Valid: !call.InitiatedAt.IsZero()},
				ProviderFinal: call.ProviderFinal})
	}
	if call.Provider == "netgsm" && i.netgsmStore != nil {
		if err := i.netgsmStore.InTx(ctx, inst.OrganizationID, inst.ID, apply); err != nil {
			return err
		}
	} else {
		if err := apply(i.queries); err != nil {
			return err
		}
	}
	if call.Provider == "netgsm" && i.notifications != nil {
		users, err := i.queries.ClaimMissedNetgsmCall(ctx,
			db.ClaimMissedNetgsmCallParams{OrganizationID: inst.OrganizationID,
				ChannelInstanceID: inst.ID,
				ExternalID:        call.ExternalID})
		if err != nil {
			return err
		}
		for _, user := range users {
			org := inst.OrganizationID
			actionURL := "/channels/" + inst.UUID.String() + "/calls"
			// The caller's number stays out of the text (lock screen, e-mail):
			// the calls page shows it to members who may see it.
			_, err = i.notifications.Enqueue(ctx,
				notifmodel.EnqueueInput{TenantID: &org,
					UserID:    &user,
					Channels:  []string{notifmodel.ChannelInapp, notifmodel.ChannelEmail},
					Priority:  notifmodel.PriorityHigh,
					Title:     "Cevapsız arama",
					Body:      "Ses kanalınızda cevapsız bir arama var.",
					ActionURL: &actionURL, SourceEvent: "calls.missed",
					ContentVars: map[string]string{"channel_name": inst.Name, "url": actionURL},
					DedupeKey:   "calls.missed:" + inst.UUID.String() + ":" + call.ExternalID,
					Payload:     map[string]any{"call_id": call.ExternalID}})
			if err != nil {
				return err
			}
		}
	}
	return nil
}
