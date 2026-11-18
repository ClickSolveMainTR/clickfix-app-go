// Package inbox composes the omnichannel inbox (CF-8): the channel adapter
// registry, the channel instance service and the conversations core with
// their queue, outbox, realtime, attachment and rate limit adapters. The API
// server and the worker build it the same way, so a message queued by the
// API is delivered by the worker with the same adapters.
package inbox

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	attachmentsmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/model"
	attachmentsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/usecase"
	channelsrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/repository"
	channelsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	convmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	convrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	notifmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/apichannel"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/email"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/googlechat"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/instagram"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/messenger"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/slack"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/sms"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/teams"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/telegram"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/testchannel"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/voice"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/whatsapp"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/queue"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Deps are the shared services the inbox is built on. Queue, Realtime,
// Attachments and Redis are optional.
type Deps struct {
	Config      config.Config
	DB          *pgxpool.Pool
	Queries     *db.Queries
	Redis       *redis.Client
	Queue       *queue.Client
	Realtime    realtime.Publisher
	Attachments *attachmentsusecase.Service
	// Notifications receives @mention and assignment notifications.
	Notifications Enqueuer
	// Search indexes conversations, messages and contacts after each change
	// and SearchClient serves the inbox ?q from the index (CF-45). Both are
	// optional; they are wired only when search is enabled.
	Search       *searchengine.Indexer
	SearchClient *searchengine.Client
	Log          *slog.Logger
}

// Inbox is the composed inbox.
type Inbox struct {
	RecordingCompleted func(context.Context) error
	netgsmStore        *channelsrepository.Netgsm
	jobs               *queue.Client
	redis              *redis.Client
	env                string
	notifications      Enqueuer
	Registry           *channels.Registry
	Channels           *channelsusecase.Service
	Conversations      *convusecase.Service
	Adapters           Adapters
	queries            *db.Queries
	Test               *testchannel.Adapter
}

// Adapters are the channel adapters of this build, typed, for the modules
// that need provider specific calls (signup, templates, domain wizard).
// A nil field is a channel that is not registered.
type Adapters struct {
	Voice *voice.Adapter
	Slack *slack.Adapter
	// GoogleChat lists the spaces of the settings.space_ids allowlist.
	GoogleChat *googlechat.Adapter
	Test       *testchannel.Adapter
	Email      *email.Adapter
	WhatsApp   *whatsapp.Adapter
	Telegram   *telegram.Adapter
	// Instagram and Messenger share the Meta app with WhatsApp.
	Instagram *instagram.Adapter
	Messenger *messenger.Adapter
}

// DefaultRegistry registers the channel adapters of this build. The test
// channel is registered outside production only; Meta channels only when
// META_CHANNELS_ENABLED (app review) and the Meta app is configured; e-mail
// is always available for IMAP; forwarding requires its receiving domain.
func DefaultRegistry(cfg config.Config) (*channels.Registry, Adapters) {
	reg := channels.NewRegistry()
	var a Adapters
	if cfg.App.Env != "production" {
		a.Test = testchannel.New()
		reg.Register(a.Test)
	}
	ch := cfg.Channels
	a.Slack = slack.New(slack.Config{ClientID: ch.SlackClientID, ClientSecret: ch.SlackClientSecret, SigningSecret: ch.SlackSigningSecret, RedirectURL: ch.SlackRedirectURL})
	reg.Register(a.Slack)
	reg.Register(apichannel.New(nil))
	reg.Register(sms.New(sms.Config{PublicBaseURL: ch.PublicAPIURL, Consent: sms.IYSConsentCheck(nil, "")}))
	reg.Register(teams.New(teams.Config{}))
	// Google Chat credentials are per organization (service account), no env.
	a.GoogleChat = googlechat.New(googlechat.Config{})
	reg.Register(a.GoogleChat)
	a.Voice = voice.New(voice.Config{PublicBaseURL: ch.PublicAPIURL})
	reg.Register(a.Voice)
	// IMAP works without a platform Resend receiving domain.
	a.Email = email.New(email.Config{
		InboundDomain: ch.Email.InboundDomain, ResendAPIKey: ch.Email.ResendAPIKey,
		WebhookSecret: ch.Email.WebhookSecret, BaseURL: ch.Email.ResendBaseURL,
	})
	reg.Register(a.Email)
	if ch.TelegramEnabled {
		a.Telegram = telegram.New(telegram.Config{PublicBaseURL: ch.PublicAPIURL})
		reg.Register(a.Telegram)
	}
	if ch.Meta.Enabled {
		wa := whatsapp.New(whatsapp.Config{
			AppID: ch.Meta.AppID, AppSecret: ch.Meta.AppSecret, VerifyToken: ch.Meta.VerifyToken,
			ConfigID: ch.Meta.WhatsAppConfigID, APIVersion: ch.Meta.GraphVersion,
		})
		if wa.Enabled() {
			a.WhatsApp = wa
			reg.Register(wa)
		}
		redirect := strings.TrimRight(cfg.Auth.FrontendURL, "/") + "/channels/connect"
		ms := messenger.New(messenger.Config{
			AppID: ch.Meta.AppID, AppSecret: ch.Meta.AppSecret, VerifyToken: ch.Meta.VerifyToken,
			APIVersion: ch.Meta.GraphVersion, RedirectURL: redirect + "/messenger",
		})
		if ms.Enabled() {
			a.Messenger = ms
			reg.Register(ms)
		}
		// Instagram Login has its own app id and secret (webhooks are
		// signed with it); fall back to the Meta app.
		ig := instagram.New(instagram.Config{
			AppID: firstNonEmpty(ch.Meta.InstagramAppID, ch.Meta.AppID), AppSecret: firstNonEmpty(ch.Meta.InstagramAppSecret, ch.Meta.AppSecret),
			VerifyToken: ch.Meta.VerifyToken, APIVersion: ch.Meta.GraphVersion, RedirectURL: redirect + "/instagram",
		})
		if ig.Enabled() {
			a.Instagram = ig
			reg.Register(ig)
		}
	}
	return reg, a
}

// New builds the inbox.
func New(deps Deps) (*Inbox, error) {
	log := deps.Log
	if log == nil {
		log = slog.Default()
	}
	box, err := crypto.NewSecretBox(deps.Config.Encryption.Key)
	if err != nil {
		return nil, err
	}
	vault, err := channels.NewVault(box)
	if err != nil {
		return nil, err
	}
	reg, adapters := DefaultRegistry(deps.Config)
	adapters.Voice.WithCallLookup(func(ctx context.Context, inst channels.Instance, id string) (channels.CallEvent, error) {
		call, err := deps.Queries.GetChannelVoiceCall(ctx, db.GetChannelVoiceCallParams{OrganizationID: inst.OrganizationID, ChannelInstanceID: inst.ID, ExternalID: id})
		if err != nil {
			return channels.CallEvent{}, err
		}
		return channels.CallEvent{ExternalID: call.ExternalID, From: call.FromNumber, To: call.ToNumber, Direction: call.Direction}, nil
	})
	// Web widget (CF-55): replies go out on the visitor's realtime channel.
	reg.Register(widget.New(deps.Realtime, log))
	netgsmStore := &channelsrepository.Netgsm{Pool: deps.DB}
	chSvc := channelsusecase.New(deps.Queries, reg, vault).WithNetgsmStore(netgsmStore).WithPublicAPIURL(deps.Config.Channels.PublicAPIURL).WithIMAPLocker(&channelsrepository.IMAP{Pool: deps.DB})
	conv := convusecase.New(convrepository.New(deps.DB, deps.Queries), chSvc, log).
		WithOutbox(outbox.NewStore(deps.DB, deps.Queries)).
		WithRegistry(reg)
	if deps.Notifications != nil {
		conv.WithMentions(MentionNotifier{N: deps.Notifications, Lookup: notifmodule.DBLookup{Q: deps.Queries}, Log: log})
	}
	if deps.Queue != nil {
		conv.WithQueue(sendQueue{c: deps.Queue})
	}
	if deps.Realtime != nil {
		conv.WithNotifier(NewLiveNotifier(deps.Realtime, deps.Queries, log))
	}
	if deps.Attachments != nil {
		conv.WithAttachments(&attachmentStore{svc: deps.Attachments, q: deps.Queries})
	}
	if deps.Search.Enabled() {
		var finder convusecase.SearchFinder
		if deps.SearchClient.Enabled() {
			finder = searchFinder{c: deps.SearchClient}
		}
		conv.WithSearch(searchHook{ix: deps.Search}, finder)
	}
	if deps.Redis != nil {
		conv.WithLimiter(sendLimiter{l: ratelimit.New(deps.Redis, deps.Config.App.Env)})
		conv.WithPresence(NewRedisPresence(deps.Redis, deps.Config.App.Env))
		adapters.Slack.WithNameCache(NewRedisSlackNames(deps.Redis, deps.Config.App.Env))
		adapters.GoogleChat.WithNameCache(NewRedisChannelNames(deps.Redis, deps.Config.App.Env, channels.TypeGoogleChat))
	}
	return &Inbox{netgsmStore: netgsmStore, jobs: deps.Queue, redis: deps.Redis, env: deps.Config.App.Env, notifications: deps.Notifications, Registry: reg, Channels: chSvc, Conversations: conv, Adapters: adapters, queries: deps.Queries, Test: adapters.Test}, nil
}

// Receive adapts the conversations pipeline to the webhook handler and
// meters paid provider messages (WhatsApp pricing.billable on a status).
func (i *Inbox) Receive(ctx context.Context, inst channels.Instance, res channels.WebhookResult) error {
	if _, err := i.Conversations.Receive(ctx, inst, res); err != nil {
		return err
	}
	for _, call := range res.Calls {
		if err := i.storeVoiceCall(ctx, inst, call); err != nil {
			return err
		}
	}
	for _, call := range res.Calls {
		if call.Provider == "netgsm" && call.RecordingURL != "" && call.RecordingMessageExternalID == "" && i.jobs != nil {
			if err := i.jobs.EnqueueNetgsmRecording(ctx, inst.OrganizationID, inst.ID, call.ExternalID); err != nil {
				return err
			}
		}
	}
	if len(res.Calls) > 0 && i.RecordingCompleted != nil {
		if err := i.RecordingCompleted(ctx); err != nil {
			slog.WarnContext(ctx, "transcript_enqueue_deferred", "error", err)
		}
	}
	for _, st := range res.Statuses {
		if feature := billableFeature(inst.Type, st); feature != "" {
			key := inst.Type + ":" + st.ExternalID
			ev := events.New(events.BillingUsageRecorded).WithTenant(inst.OrganizationID).WithIdempotencyKey("usage:" + key).WithPayload(map[string]any{
				"feature": feature, "quantity": 1, "event_key": key, "occurred_at": st.At,
			})
			if err := outbox.EnqueueQueries(ctx, i.queries, ev); err != nil {
				return err
			}
		}
	}
	return nil
}

// FeatureWhatsAppTemplates is the metered plan feature of paid WhatsApp
// messages (business initiated templates).
const FeatureWhatsAppTemplates = "whatsapp.templates"

// billableFeature reports the plan feature a status bills, once per
// message: Meta sends pricing on sent and delivered; only "sent" counts
// (the event key also dedupes redeliveries).
func billableFeature(channelType string, st channels.StatusUpdate) string {
	if channelType != channels.TypeWhatsApp || st.ExternalID == "" || st.Status != channels.StatusSent {
		return ""
	}
	if b, _ := st.Attributes["billable"].(bool); !b {
		return ""
	}
	return FeatureWhatsAppTemplates
}

// Tick runs the follow-up tick (snooze wake-ups, reminders) for the worker.
func (i *Inbox) Tick(ctx context.Context) error {
	_, err := i.Conversations.Tick(ctx)
	return err
}

// Deliver adapts the conversations deliverer to the queue worker.
func (i *Inbox) Deliver(ctx context.Context, orgID, messageID int64, retried, maxRetry int) error {
	return i.Conversations.Deliver(ctx, orgID, messageID, convusecase.DeliverAttempt{Retried: retried, MaxRetry: maxRetry})
}

// ---------------------------------------------------------------------------

// LiveNotifier publishes inbox events on the organization's inbox channel
// (realtime.InboxChannel, read_all members) and fans each one out to the
// per-member channels of the conversation's audience (CF-44 team
// visibility, realtime.InboxUserChannel).
type LiveNotifier struct {
	pub   realtime.Publisher
	q     *db.Queries
	log   *slog.Logger
	cache sync.Map // org id → uuid
}

// NewLiveNotifier builds the notifier.
func NewLiveNotifier(pub realtime.Publisher, q *db.Queries, log *slog.Logger) *LiveNotifier {
	return &LiveNotifier{pub: pub, q: q, log: log}
}

// Notify implements usecase.Notifier (best effort; the inbox refetches on
// reconnect).
func (n *LiveNotifier) Notify(ctx context.Context, orgID int64, ev convmodel.LiveEvent) {
	orgUUID, err := orgUUID(ctx, n.q, &n.cache, orgID)
	if err != nil {
		n.log.Warn("inbox_notify_org_lookup_failed", "org", orgID, "error", err)
		return
	}
	if err := n.pub.Publish(ctx, realtime.InboxChannel(orgUUID), ev); err != nil {
		n.log.Warn("inbox_notify_failed", "org", orgID, "type", ev.Type, "error", err)
	}
	PublishToAudience(ctx, n.pub, n.q, n.log, orgID, orgUUID, ev.ConversationUUID, ev, ev.Also...)
}

// PublishToAudience sends payload to inbox:{org}:{user} of every member who
// may see the conversation without read_all, plus the members in also (the
// audience before a reassignment), once each (best effort).
func PublishToAudience(ctx context.Context, pub realtime.Publisher, q access.AudienceQueries, log *slog.Logger, orgID int64, orgUUID, conversationUUID uuid.UUID, payload any, also ...uuid.UUID) {
	if conversationUUID == uuid.Nil {
		return
	}
	users, err := access.Audience(ctx, q, orgID, conversationUUID)
	if err != nil {
		log.Warn("inbox_audience_failed", "org", orgID, "conversation", conversationUUID, "error", err)
		return
	}
	seen := make(map[uuid.UUID]bool, len(users)+len(also))
	for _, u := range append(users, also...) {
		if seen[u] {
			continue
		}
		seen[u] = true
		if err := pub.Publish(ctx, realtime.InboxUserChannel(orgUUID, u), payload); err != nil {
			log.Warn("inbox_notify_user_failed", "org", orgID, "error", err)
		}
	}
}

func orgUUID(ctx context.Context, q *db.Queries, cache *sync.Map, orgID int64) (uuid.UUID, error) {
	if v, ok := cache.Load(orgID); ok {
		return v.(uuid.UUID), nil
	}
	org, err := q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return uuid.Nil, err
	}
	cache.Store(orgID, org.Uuid)
	return org.Uuid, nil
}

// attachmentStore stores inbound files as message attachments.
type attachmentStore struct {
	svc   *attachmentsusecase.Service
	q     *db.Queries
	cache sync.Map
}

func (a *attachmentStore) StoreInbound(ctx context.Context, orgID int64, messageUUID uuid.UUID, filename, contentType string, body io.Reader) error {
	org, err := orgUUID(ctx, a.q, &a.cache, orgID)
	if err != nil {
		return err
	}
	owner := messageUUID
	_, err = a.svc.Upload(ctx, attachmentsmodel.UploadInput{
		OrganizationID: orgID, OrganizationUUID: org, Filename: filename, DeclaredType: contentType,
		OwnerType: attachmentsmodel.OwnerMessage, OwnerID: &owner, Body: body,
	})
	return err
}

type sendQueue struct{ c *queue.Client }

func (q sendQueue) EnqueueSend(ctx context.Context, orgID, messageID int64, delay time.Duration) error {
	return q.c.EnqueueConversationSend(ctx, orgID, messageID, delay)
}

func (q sendQueue) EnqueueSendPriority(ctx context.Context, orgID, messageID int64) error {
	return q.c.EnqueueConversationSendPriority(ctx, orgID, messageID)
}

// defaultSendsPerMinute bounds one instance's outbound rate unless its
// settings carry send_rate_per_minute.
const defaultSendsPerMinute = 120

type sendLimiter struct{ l *ratelimit.Limiter }

func (s sendLimiter) AllowSend(ctx context.Context, inst channels.Instance) (bool, time.Duration) {
	limit := defaultSendsPerMinute
	switch v := inst.Settings["send_rate_per_minute"].(type) {
	case float64:
		if v >= 1 {
			limit = int(v)
		}
	case string:
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			limit = n
		}
	}
	return s.l.Allow(ctx, "channel_send", inst.UUID.String(), limit, time.Minute)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// RoutingAssigner lets automatic routing (CF-53) assign through the
// conversations service, so the timeline, events and notifications match a
// manual assignment.
type RoutingAssigner struct{ Conversations *convusecase.Service }

// Assign sets (or with nil clears) the assignee.
func (a RoutingAssigner) Assign(ctx context.Context, orgID int64, conversation uuid.UUID, user *uuid.UUID) error {
	_, err := a.Conversations.Update(ctx, convusecase.UpdateInput{
		OrganizationID: orgID, ConversationUUID: conversation,
		Actor: convusecase.Actor{Name: "Otomatik yönlendirme"}, Assignee: &user,
	})
	return err
}
