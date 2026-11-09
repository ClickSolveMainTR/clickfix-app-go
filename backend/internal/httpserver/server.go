package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	forummodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forum"
	forumhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forum/handler"
	forumuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forum/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/retention"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/google/uuid"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/aisystem"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/cache"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/demoseed"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/developerplatform"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/eventconsumers"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/inbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	accessmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/access"
	accesshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/access/handler"
	activitymodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/activity"
	activityhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/activity/handler"
	activityusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/activity/usecase"
	agentmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents"
	agenthandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/handler"
	agentrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/repository"
	agentusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/usecase"
	aimodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore"
	aihandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/handler"
	analyticsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics"
	analyticshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/handler"
	analyticsrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/repository"
	analyticsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/usecase"
	appointmentsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/appointments"
	appointmentshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/appointments/handler"
	appointmentsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/appointments/usecase"
	assistantmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/assistant"
	attachmentsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments"
	attachmentshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/handler"
	attachmentsrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/repository"
	attachmentsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/usecase"
	auditmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/audit"
	authmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth"
	authhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/identity"
	authrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/repository"
	authusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/usecase"
	authsettingsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/authsettings"
	authsettingshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/authsettings/handler"
	authsettingsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/authsettings/usecase"
	billingmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing"
	billinghandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/handler"
	billingrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	billingusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/brands"
	bulkmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/bulk"
	bulkhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/bulk/handler"
	bulkusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/bulk/usecase"
	channelsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/coaching"
	compliancemodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/compliance"
	compliancerepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/compliance/repository"
	complianceuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/compliance/usecase"
	contactsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts"
	contactshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/handler"
	contactsio "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/ioadapter"
	contactsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/usecase"
	conversationsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations"
	convaccess "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	conversationshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/handler"
	convrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	healthmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customerhealth"
	healthhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customerhealth/handler"
	healthusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customerhealth/usecase"
	customfieldsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields"
	customfieldshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/handler"
	customfieldsrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/repository"
	customfieldsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/enterprise"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/entitlements"
	exportmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/exports"
	exporthandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/exports/handler"
	exportusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/exports/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/feedback"
	formsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms"
	formshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/handler"
	formsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/usecase"
	guidesmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/guides"
	helpmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/helpcenter"
	helphandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/helpcenter/handler"
	helpusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/helpcenter/usecase"
	importmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/imports"
	importhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/imports/handler"
	importusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/imports/usecase"
	githubmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/github"
	githubhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/github/handler"
	githubusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/github/usecase"
	oauthprovidermodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/oauthprovider"
	oauthproviderhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/oauthprovider/handler"
	oauthproviderusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/oauthprovider/usecase"
	kbmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge"
	kbhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/handler"
	logsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/logs"
	logshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/logs/handler"
	logsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/logs/usecase"
	macrosmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/macros"
	macroshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/macros/handler"
	macrosusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/macros/usecase"
	marketingmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/marketing"
	mobileextmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mobileext"
	mobileexthandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mobileext/handler"
	notifmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications"
	notifhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/usecase"
	onboardingmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/onboarding"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/oncall"
	oncallrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/oncall/repository"
	organizationsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations"
	orgadapters "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/adapters"
	orghandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/handler"
	orgrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/platformhealth"
	presencemodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/presence"
	presencehandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/presence/handler"
	presenceusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/presence/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/qrlogin"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/quality"
	reportsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/reports"
	reportsrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/reports/repository"
	reportsuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/reports/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/revenue"
	routingmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing"
	routinghandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/handler"
	routingusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/schedules"
	searchmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/search"
	searchhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/search/handler"
	searchusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/search/usecase"
	settingsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/settings"
	settingshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/settings/handler"
	settingsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/settings/usecase"
	slamodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla"
	slahandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/handler"
	slausecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/usecase"
	ssomodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sso"
	ssorepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sso/repository"
	ssouc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sso/usecase"
	statusmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage"
	statushandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/handler"
	statusrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/repository"
	statususecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/usecase"
	storagemodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/storage"
	storagehandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/storage/handler"
	storageusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/storage/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportaccess"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	surveysmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys"
	surveyshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys/handler"
	surveysrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys/repository"
	surveysusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/teamchat"
	teamsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/teams"
	teamshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/teams/handler"
	teamsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/teams/usecase"
	tenantmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/tenant"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/transcription"
	transcriptrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/transcription/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/videorooms"
	widgetmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget"
	widgethandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/handler"
	widgetusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/workforce"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/appleauth"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/bulkengine"
	bulkadapters "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/bulkengine/adapters"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/clientip"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine"
	ioadapters "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine/adapters"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/mail"
	platformmetrics "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/metrics"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/oidc"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx/orgcache"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine"
	searchadapters "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine/adapters"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/stepup"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/queue"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/supportoperations"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Deps holds infrastructure clients wired into the HTTP server.
type Deps struct {
	DB       *pgxpool.Pool
	Queries  *db.Queries
	Redis    *redis.Client
	Queue    *queue.Client
	Storage  storage.Driver
	Realtime realtime.Publisher
	Worker   *queue.Worker
	Events   events.Bus
}

// Server is the HTTP composition root for infrastructure routes.
type Server struct {
	cfg           config.Config
	log           *slog.Logger
	db            *pgxpool.Pool
	queries       *db.Queries
	redis         *redis.Client
	queueClient   *queue.Client
	storage       storage.Driver
	realtime      realtime.Publisher
	worker        *queue.Worker
	events        events.Bus
	outboxPub     *outbox.Publisher
	outboxStop    func()
	searchClient  *searchengine.Client
	searchIndexer *searchengine.Indexer
	mailMonitor   *mail.Monitor
	githubSvc     *githubusecase.Service
	oauthProvSvc  *oauthproviderusecase.Service
	authSettings  *authsettingsusecase.Service
	http          *http.Server

	// demoCancel stops background demo data fills (CF-158).
	demoCancel context.CancelFunc
}

// New wires router and middleware for the API skeleton.
func New(cfg config.Config, log *slog.Logger, deps Deps) (*Server, error) {
	mux := http.NewServeMux()
	eventBus := deps.Events
	if eventBus == nil {
		eventBus = events.NewBus(log)
	}

	s := &Server{
		cfg:         cfg,
		log:         log,
		db:          deps.DB,
		queries:     deps.Queries,
		redis:       deps.Redis,
		queueClient: deps.Queue,
		storage:     deps.Storage,
		realtime:    deps.Realtime,
		worker:      deps.Worker,
		events:      eventBus,
	}

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("GET /v1/app/config", s.handleAppConfig)
	s.mountDocs(mux)

	tokens, err := jwt.NewManager(cfg.JWT.AccessSecret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	if err != nil {
		return nil, fmt.Errorf("httpserver: jwt: %w", err)
	}

	mailer := mail.NewSMTPSender(cfg.SMTP)
	s.mailMonitor = mail.NewMonitor(mailer, 5*time.Minute)
	provs := []providers.Provider{
		providers.InappProvider{},
		providers.EmailProvider{Mail: mailer, AppURL: cfg.Auth.FrontendURL},
		providers.RealtimeProvider{Pub: deps.Realtime},
		providers.NoopProvider{Name: "sms", Log: log},
		providers.NoopProvider{Name: "push", Log: log},
	}
	notifSvc := notifusecase.New(deps.Queries, deps.Queue, provs, log)
	notifSvc.WithActionSigner(cfg.JWT.AccessSecret, 0)
	notifSvc.WithVAPID(notifusecase.VAPIDConfig{
		PublicKey:  cfg.VAPID.PublicKey,
		PrivateKey: cfg.VAPID.PrivateKey,
		Subject:    cfg.VAPID.Subject,
	})
	notifSvc.WithExpo(notifusecase.ExpoConfig{AccessToken: cfg.Expo.AccessToken})
	notifSvc.WithAppURL(cfg.Auth.FrontendURL)

	repo := authrepo.NewPostgres(deps.DB, deps.Queries)
	repo.SetDeletionGrace(cfg.Auth.AccountDeletionGrace)
	uc := authusecase.New(repo, tokens)
	uc.SetNotifier(notifSvc)
	uc.SetLogger(log)

	searchReg := searchengine.NewRegistry(searchadapters.All(deps.Queries)...)
	searchClient := searchengine.NewClient(cfg.Search, log)
	repo.SetDeletionSearch(searchClient)
	searchIndexer := searchengine.NewIndexer(searchClient, searchReg, deps.Queue, log)
	s.searchClient = searchClient
	s.searchIndexer = searchIndexer
	uc.SetSearchIndexer(searchIndexer)

	rtIssuer, err := realtime.NewTokenIssuer(cfg.Centrifugo)
	if err != nil {
		return nil, fmt.Errorf("httpserver: realtime tokens: %w", err)
	}
	uc.SetRealtimeHints(authusecase.RealtimeHints{
		Enabled: rtIssuer.Enabled(),
		WSURL:   cfg.Centrifugo.WSURL,
	})

	activityRec := activity.NewRecorder(deps.Queries, log)

	stepUpStore := stepup.NewStore(deps.Redis, cfg.App.Env)
	var stepUpWebAuthn *stepup.WebAuthn
	stepUpOrigins := cfg.CORS.AllowedOrigins
	if len(stepUpOrigins) == 0 {
		stepUpOrigins = []string{strings.TrimRight(cfg.Auth.FrontendURL, "/")}
	}
	if webauthn, err := stepup.NewWebAuthn(stepup.WebAuthnConfig{
		RPID:          cfg.Auth.WebAuthnRPID,
		RPDisplayName: cfg.App.Name,
		RPOrigins:     stepUpOrigins,
	}); err == nil {
		stepUpWebAuthn = webauthn
	} else {
		log.Warn("stepup_webauthn_disabled", "error", err)
	}
	secretBox, err := crypto.NewSecretBox(cfg.Encryption.Key)
	if err != nil {
		return nil, fmt.Errorf("httpserver: encryption: %w", err)
	}
	stepUpSvc := stepup.NewService(deps.Queries, stepUpStore, stepUpWebAuthn, repo)
	stepUpSvc.SetSecretBox(secretBox)

	githubSvc := githubusecase.New(deps.Queries, secretBox)
	oauthProvSvc := oauthproviderusecase.New(deps.Queries, secretBox)
	authSettingsSvc := authsettingsusecase.New(deps.Queries)
	uc.SetAuthSettings(authSettingsSvc)
	uc.SetAccessPolicy(stepUpSvc)
	uc.SetSecretBox(secretBox, cfg.App.Name)
	oauthUC := authusecase.NewOAuth(repo, secretBox)
	uc.SetReviewAccounts(cfg.Auth.ReviewAccounts)
	// Apple signing key: admin-uploaded .p8 (DB) first, AUTH_APPLE_* env fallback.
	appleClient, err := appleauth.NewResolver(oauthProvSvc, appleauth.Config{
		TeamID: cfg.Auth.AppleTeamID, KeyID: cfg.Auth.AppleKeyID, PrivateKey: cfg.Auth.ApplePrivateKey,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("httpserver: apple sign-in key: %w", err)
	}
	oauthProvSvc.SetAppleKeys(appleClient)
	uc.SetNativeOAuth(authusecase.NativeOAuthConfig{
		Verifier:        oidc.New(oidc.DefaultProviders(), nil),
		Apple:           appleClient,
		Clients:         oauthProvSvc,
		AppleClientIDs:  cfg.Auth.AppleNativeClientIDs,
		GoogleClientIDs: cfg.Auth.GoogleNativeClientIDs,
	})
	s.githubSvc = githubSvc
	s.oauthProvSvc = oauthProvSvc
	s.authSettings = authSettingsSvc

	h := authhandler.New(uc, oauthUC, githubSvc, oauthProvSvc, authSettingsSvc, cfg.Auth.AdapterSecret, stepUpSvc, activityRec)
	h.SetRateLimiter(ratelimit.New(deps.Redis, cfg.App.Env))
	loader := identity.Loader{UC: uc}
	authmodule.RegisterRoutes(mux, h, tokens, loader, stepUpSvc)

	// Organizations (tenant core). Platform management is always mounted;
	// the /v1/tenant/* shell only with TENANCY_ENABLED.
	billingRepository := billingrepo.New(deps.DB, deps.Queries)
	entitlementService := entitlements.New(billingRepository, deps.Redis, cfg.App.Env)
	billingService := billingusecase.New(billingRepository)
	if err := billingService.InvoicingFromEnvironment(cfg.App.Env); err != nil {
		return nil, err
	}
	billingService.SetPaymentAPI(cfg.Channels.PublicAPIURL)
	if err := billingService.DevelopmentGateways(billingRepository, cfg.App.Env, cfg.Encryption.Key, cfg.Auth.FrontendURL); err != nil {
		return nil, err
	}
	// CF-154: say at boot what a purchase would need, instead of a 5xx later.
	log.Info("billing_payments_ready", "providers", billingService.PaymentProviders(), "seller_configured", billingService.SellerConfigured(), "public_api_url_set", cfg.Channels.PublicAPIURL != "")
	if !billingService.SellerConfigured() {
		log.Warn("billing_seller_not_configured", "hint", "set BILLING_SELLER_JSON; first invoices of an organization fail with BILLING_SELLER_NOT_CONFIGURED")
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(deps.DB, deps.Queries), billingusecase.TrialHooks{Service: billingService})
	// Support access (CF-14): staff with platform.support.access enter an
	// organization that granted access; /me lists those organizations too.
	supportSvc := supportaccess.NewService(deps.Queries)
	uc.SetOrganizationsLister(supportaccess.MeLister{Next: orgSvc, Svc: supportSvc})
	// Staff invitations, organization profile / logo and the platform
	// management views (ported from rentopia-app-go).
	orgMgmt := orgusecase.NewManagement(orgSvc, orgrepository.NewPostgres(deps.DB, deps.Queries), orgusecase.ManagementConfig{
		FrontendURL: cfg.Auth.FrontendURL,
		InviteTTL:   cfg.Tenancy.InviteTTL,
	})
	orgMgmt.SetNotifier(notifSvc)
	orgMgmt.SetAccounts(uc)
	orgMgmt.SetRateLimiter(ratelimit.New(deps.Redis, cfg.App.Env))
	orgMgmt.SetSignupPolicy(authSettingsSvc)
	orgMgmt.SetLogger(log)
	orgH := orghandler.New(orgSvc, activityRec)
	orgH.SetManagement(orgMgmt)
	orgH.SetStorage(deps.Storage)
	orgH.SetRegistrationLimiter(ratelimit.New(deps.Redis, cfg.App.Env))
	// Background demo fills stop when the server shuts down.
	demoCtx, demoCancel := context.WithCancel(context.Background())
	s.demoCancel = demoCancel
	orgH.SetDemoSeeder(demoSeeder{demoseed.NewPlatform(demoCtx, deps.DB, billingService, searchIndexer, log)})
	var tenantResolver orgctx.Resolver
	if cfg.Tenancy.Enabled {
		orgCache := orgcache.New(orgSvc, deps.Redis, cfg.Tenancy.CacheTTL, cfg.App.Env)
		orgSvc.SetInvalidator(orgCache)
		tenantResolver = supportaccess.NewResolver(orgCache, supportSvc, orgSvc)
	}
	// Self-serve sign-up and the set-up checklist (CF-13).
	ssoSvc := &ssouc.Service{Activity: activityRec, Repo: ssorepo.New(deps.DB, deps.Queries), Box: secretBox, Tokens: tokens, Resolver: tenantResolver, Entitlements: entitlementService, BaseURL: strings.TrimRight(cfg.Auth.FrontendURL, "/") + "/api"}
	for _, raw := range cfg.SSOTrustedProxies {
		prefix, e := netip.ParsePrefix(raw)
		if e != nil {
			return nil, fmt.Errorf("invalid SSO_TRUSTED_PROXY_CIDRS: %w", e)
		}
		ssoSvc.TrustedProxies = append(ssoSvc.TrustedProxies, prefix)
	}
	ssoLimiter := ratelimit.New(deps.Redis, cfg.App.Env)
	ssoSvc.Limit = func(ctx context.Context, action, subject string) (bool, time.Duration) {
		limit := 30
		if action == "scim" {
			limit = 120
		}
		return ssoLimiter.Allow(ctx, action, subject, limit, time.Minute)
	}
	if tenantResolver != nil {
		tenantResolver = ssoSvc
	}
	tenantmodule.RegisterRoutes(mux, tokens, loader, tenantResolver)
	onboardingSvc := onboardingmodule.New(deps.Queries, orgSvc, onboardingmodule.Config{SelfServeOpen: authSettingsSvc.SelfServeOpen}).
		WithAIAllowance(func(ctx context.Context, org int64, key string, credits int64, expires time.Time) error {
			_, err := billingService.WalletCommand(ctx, org, billingusecase.WalletInput{
				Unit: "ai_credits", Delta: credits, Reason: "Onboarding AI allowance", IdempotencyKey: key, ExpiresAt: &expires,
			})
			return err
		}, cfg.Tenancy.OnboardingAICredits)
	onboardingmodule.RegisterRoutes(mux, onboardingmodule.NewHandler(onboardingSvc, activityRec), tokens, loader, tenantResolver)
	// Organization attachments (CF-11). Download URLs are signed with a key
	// derived from the application encryption secret.
	attachmentsSvc := attachmentsusecase.New(
		attachmentsrepository.NewPostgres(deps.DB, deps.Queries),
		deps.Storage, cfg.Encryption.Key, cfg.Attachment, log,
	)
	// Message / conversation files follow the inbox team rule (CF-44 §14).
	attachmentsmodule.RegisterRoutes(mux, attachmentshandler.New(attachmentsSvc, activityRec).
		WithOwnerAccess(convaccess.AttachmentGuard{Q: deps.Queries}), tokens, loader, tenantResolver)
	// Omnichannel inbox (CF-8): channel instances, webhooks, conversations.
	inboxCore, err := inbox.New(inbox.Deps{
		Config: cfg, DB: deps.DB, Queries: deps.Queries, Redis: deps.Redis, Queue: deps.Queue,
		Realtime: deps.Realtime, Attachments: attachmentsSvc, Notifications: notifSvc, Log: log,
		Search: searchIndexer, SearchClient: searchClient,
	})
	if err != nil {
		return nil, fmt.Errorf("httpserver: inbox: %w", err)
	}
	// Dynamic request forms (CF-52): widget start form, conversation form
	// values and the "required before resolving" check.
	formsSvc := formsusecase.New(deps.DB, deps.Queries)
	inboxCore.Conversations.WithResolveGuard(formsSvc)
	formsmodule.RegisterRoutes(mux, formshandler.New(formsSvc, activityRec), tokens, loader, tenantResolver)
	convH := conversationshandler.New(inboxCore.Conversations, activityRec)
	conversationsmodule.RegisterRoutes(mux, convH, tokens, loader, tenantResolver)
	analyticsSvc := analyticsusecase.New(analyticsrepo.New(deps.DB, deps.Queries))
	analyticsmodule.RegisterRoutes(mux, analyticshandler.New(analyticsSvc, func(r *http.Request, org int64) error {
		if deps.Queue == nil {
			return fmt.Errorf("queue unavailable")
		}
		return deps.Queue.EnqueueAnalyticsBackfill(r.Context(), org)
	}), tokens, loader, tenantResolver)
	// SLA policies, business hours and conversation clocks (CF-46).
	slaSvc := slausecase.New(deps.DB, deps.Queries, outbox.NewStore(deps.DB, deps.Queries), log)
	if deps.Queue != nil {
		slaSvc.WithResyncer(deps.Queue)
	}
	slamodule.RegisterRoutes(mux, slahandler.New(slaSvc, activityRec), tokens, loader, tenantResolver)
	// Automatic routing and agent capacity (CF-53, CF-54).
	routingSvc := routingusecase.New(deps.DB, deps.Queries, inbox.RoutingAssigner{Conversations: inboxCore.Conversations}, log)
	routingmodule.RegisterRoutes(mux, routinghandler.New(routingSvc, activityRec), tokens, loader, tenantResolver)
	statusSvc := statususecase.New(statusrepo.New(deps.DB, deps.Queries), inboxCore.Conversations, notifSvc, cfg.Encryption.Key, cfg.Auth.FrontendURL)
	surveysSvc := surveysusecase.New(surveysrepo.New(deps.DB, deps.Queries), inboxCore.Conversations, cfg.Encryption.Key, cfg.Auth.FrontendURL)
	surveyLimiter := ratelimit.New(deps.Redis, cfg.App.Env)
	surveysmodule.RegisterRoutes(mux, surveyshandler.New(surveysSvc, activityRec, func(r *http.Request) bool {
		ok, _ := surveyLimiter.Allow(r.Context(), "survey_response", publicClientIP(r), 60, time.Minute)
		return ok
	}), tokens, loader, tenantResolver)
	statusmodule.RegisterRoutes(mux, statushandler.New(statusSvc, activityRec, func(r *http.Request) bool {
		ok, _ := surveyLimiter.Allow(r.Context(), "status_subscribe", publicClientIP(r), 5, time.Minute)
		return ok
	}), tokens, loader, tenantResolver)
	// Appointments and the organization calendar (CF-138).
	videoSvc := videorooms.New(deps.Queries, cfg, billingService, entitlementService, deps.Storage, notifSvc, log)
	appointmentsSvc := appointmentsusecase.New(appointmentsusecase.Deps{
		Pool: deps.DB, Queries: deps.Queries, Entitlements: entitlementService, Notifier: notifSvc, Video: videoSvc, Activity: activityRec,
		Timeline: inboxCore.Conversations, Intake: formsSvc, Realtime: deps.Realtime, Secret: cfg.Encryption.Key,
		BaseURL: cfg.Auth.FrontendURL, Log: log,
	})
	appointmentsLimiter := ratelimit.New(deps.Redis, cfg.App.Env)
	appointmentsmodule.RegisterRoutes(mux, appointmentshandler.New(appointmentsSvc, activityRec, deps.Queries, func(r *http.Request, action string, limit int, window time.Duration) bool {
		ok, _ := appointmentsLimiter.Allow(r.Context(), action, publicClientIP(r), limit, window)
		return ok
	}), tokens, loader, tenantResolver)
	// Public web widget API (CF-55).
	widgetLimiter := ratelimit.New(deps.Redis, cfg.App.Env)
	widgetService := widgetusecase.New(widgetusecase.Deps{
		Queries: deps.Queries, Store: convrepository.New(deps.DB, deps.Queries), Instances: inboxCore.Channels,
		Pipeline: inboxCore.Conversations, Files: attachmentsSvc, Signer: widget.NewSigner(cfg.Encryption.Key),
		Publisher: deps.Realtime, Issuer: rtIssuer, AppOrigin: cfg.Auth.FrontendURL, Forms: formsSvc, Log: log,
	})
	widgetService.WithReplay(deps.Storage, entitlementService, cfg.Encryption.Key)
	widgetH := widgethandler.New(widgetService, func(r *http.Request, action, subject string, limit int) (bool, time.Duration) {
		return widgetLimiter.Allow(r.Context(), action, subject, limit, time.Minute)
	}, log)
	widgetmodule.RegisterRoutes(mux, widgetH)
	widgetmodule.RegisterAgentRoutes(mux, widgetH, tokens, loader, tenantResolver)
	// Widget editor: drafts, publishing, versions, installs (CF-150).
	widgetConfigs := widgetusecase.NewConfigs(deps.Queries, convrepository.New(deps.DB, deps.Queries), inboxCore.Channels)
	widgetmodule.RegisterConfigRoutes(mux, widgethandler.NewConfig(widgetConfigs, activityRec), tokens, loader, tenantResolver, entitlementService)

	// Custom fields and custom objects (CF-7).
	customFieldsSvc := customfieldsusecase.New(customfieldsrepository.New(deps.DB, deps.Queries))
	widgetService.WithFields(customFieldsSvc)
	customfieldsmodule.RegisterRoutes(mux, customfieldshandler.New(customFieldsSvc, activityRec), tokens, loader, tenantResolver)
	// Customer health score and churn-risk alerts (CF-87).
	healthmodule.RegisterRoutes(mux, healthhandler.New(healthusecase.New(deps.DB, deps.Queries, activityRec, log), activityRec), tokens, loader, tenantResolver)
	contactsSvc := contactsusecase.New(convrepository.New(deps.DB, deps.Queries))
	contactsmodule.RegisterRoutes(mux, contactshandler.New(contactsSvc, activityRec), tokens, loader, tenantResolver)
	contactsIO, companiesIO := contactsio.NewContacts(contactsSvc, deps.Queries), contactsio.NewCompanies(contactsSvc, deps.Queries)
	macrosmodule.RegisterRoutes(mux, macroshandler.New(macrosusecase.New(deps.Queries, inboxCore.Conversations)), tokens, loader, tenantResolver)
	inboxCore.Channels.WithEntitlements(entitlementService).WithLogger(log)
	channelsHandler, err := inbox.ChannelsHandler(cfg, inboxCore, activityRec, log)
	if err != nil {
		return nil, fmt.Errorf("httpserver: channels: %w", err)
	}
	channelsmodule.RegisterRoutes(mux, channelsHandler, tokens, loader, tenantResolver)
	aiCore := aisystem.New(deps.DB, billingRepository, entitlementService, cfg.Search)
	agentSvc := agentusecase.New(&agentrepo.Store{Repository: billingRepository}, aiCore.AI, inboxCore.Conversations)
	aiCore.AI.Context = agentSvc
	agentSvc.Sources = &agentusecase.SourceService{Agents: agentSvc, Storage: deps.Storage}
	agentBox, agentBoxErr := crypto.NewSecretBox(cfg.Encryption.Key)
	if agentBoxErr != nil {
		log.Warn("ai_tool_credentials_unavailable", "error", agentBoxErr)
	}
	agentSvc.Tools = &agentusecase.ToolService{Repo: agentSvc.Repo, Core: aiCore.AI, Conversations: inboxCore.Conversations, Box: agentBox}
	agentmodule.RegisterRoutes(mux, &agenthandler.Handler{Service: agentSvc, Activity: activityRec}, tokens, loader, tenantResolver)
	aimodule.RegisterRoutes(mux, &aihandler.Handler{Service: aiCore.AI, Activity: activityRec}, tokens, loader, tenantResolver)
	kbmodule.RegisterRoutes(mux, &kbhandler.Handler{Service: aiCore.KB, RAG: aiCore.RAG, Activity: activityRec}, tokens, loader, tenantResolver)
	// AI-assisted set-up and use-case template gallery (CF-140).
	setupTemplates, err := onboardingmodule.LoadTemplates()
	if err != nil {
		return nil, fmt.Errorf("httpserver: onboarding templates: %w", err)
	}
	setupSvc := &onboardingmodule.SetupService{
		Q: deps.Queries, Templates: setupTemplates, Fetcher: onboardingmodule.NewWebsiteFetcher(),
		AI: aiCore.AI, Pub: deps.Realtime, Log: log,
		Applier: &onboardingmodule.Applier{
			Pool: deps.DB, Q: deps.Queries, KB: aiCore.KB, Profiles: orgMgmt,
			Appointments: onboardingmodule.AppointmentsAdapter{Svc: appointmentsSvc, Q: deps.Queries},
		},
	}
	onboardingmodule.RegisterSetupRoutes(mux, onboardingmodule.NewSetupHandler(setupSvc, activityRec), tokens, loader, tenantResolver)
	operationsRouter := supportops.Router{Mux: mux, Tokens: tokens, Loader: loader, Resolver: tenantResolver, Activity: activityRec}
	transcriptSvc := &transcription.Service{Repo: &transcriptrepo.Store{Queries: deps.Queries, Pool: deps.DB}, Speech: aiCore.Speech, AI: aiCore.AI, Files: attachmentsSvc, Video: videoSvc, Jobs: deps.Queue, Summarize: os.Getenv("AI_TRANSCRIBE_SUMMARY") == "true"}
	inboxCore.RecordingCompleted = transcriptSvc.Sweep
	videoSvc.RecordingCompleted = transcriptSvc.Sweep
	transcription.RegisterRoutes(operationsRouter, transcriptSvc)
	oncallSvc := oncall.New(oncallrepo.New(deps.DB, deps.Queries), notifSvc)
	reportsSvc := &reportsuc.Service{Repo: reportsrepo.New(deps.DB, deps.Queries), Resolver: tenantResolver, Notifier: notifSvc, BaseURL: cfg.Auth.FrontendURL, Secret: cfg.Encryption.Key}
	brands.RegisterRoutes(operationsRouter, &brands.Service{Repo: billingRepository, Entitlements: entitlementService, Files: attachmentsSvc})
	oncall.RegisterRoutes(operationsRouter, oncallSvc)
	// Video rooms (CF-139): LiveKit rooms metered against the video_minutes wallet.

	videoLimiter := ratelimit.New(deps.Redis, cfg.App.Env)
	videorooms.RegisterRoutes(operationsRouter, videoSvc, func(r *http.Request, action, subject string, limit int, window time.Duration) bool {
		ok, _ := videoLimiter.Allow(r.Context(), action, subject, limit, window)
		return ok
	})
	reportsmodule.RegisterRoutes(operationsRouter, reportsSvc)
	complianceSvc := &complianceuc.Service{Search: searchClient, Repo: compliancerepo.New(deps.DB, deps.Queries), Store: deps.Storage, Jobs: deps.Queue, Activity: activityRec}
	compliancemodule.RegisterRoutes(operationsRouter, complianceSvc)
	compliancemodule.RegisterMetaRoutes(mux, complianceSvc, cfg.Channels.Meta.AppSecret, cfg.Channels.Meta.InstagramAppSecret)
	ssomodule.RegisterRoutes(operationsRouter, ssoSvc)
	if s.worker != nil {
		s.worker.Handle(ssouc.TaskCleanup, ssoSvc.Cleanup)
	}
	if s.worker != nil {
		s.worker.Handle(authusecase.TaskAccountPurge, authusecase.AccountPurgeHandler(repo))
		s.worker.Handle(complianceuc.TaskProcess, complianceSvc.ProcessTask).Handle(complianceuc.TaskSweep, complianceSvc.Sweep).Handle(complianceuc.TaskMetaDeletion, complianceSvc.MetaDeletionTask)
	}

	feedback.RegisterRoutes(operationsRouter, &feedback.Service{Repo: billingRepository, AI: aiCore.AI, PublicBaseURL: cfg.Auth.FrontendURL}, widgetService)
	schedules.RegisterRoutes(operationsRouter, &schedules.Service{Repo: billingRepository})
	workforce.RegisterRoutes(operationsRouter, &workforce.Service{Repo: billingRepository})
	coaching.RegisterRoutes(operationsRouter, &coaching.Service{Repo: billingRepository})
	teamchat.RegisterRoutes(operationsRouter, &teamchat.Service{Repo: billingRepository, AI: aiCore.AI})
	quality.RegisterRoutes(operationsRouter, &quality.Service{Repo: billingRepository, AI: aiCore.AI})
	revenue.RegisterRoutes(operationsRouter, billingRepository)
	enterprise.RegisterRoutes(operationsRouter, &enterprise.Service{Repo: billingRepository, Billing: billingService})

	// Public help center (CF-74) and the widget help search (CF-55): published
	// public articles only, lexical search (no metered embeddings for visitors).
	helpSvc := helpusecase.New(deps.Queries, helpusecase.LexicalOnly(aiCore.RAG), deps.Storage, cfg.Encryption.Key)
	helpLimiter := ratelimit.New(deps.Redis, cfg.App.Env)
	helpmodule.RegisterRoutes(mux, helphandler.New(helpSvc, func(r *http.Request, action, subject string, limit int, window time.Duration) (bool, time.Duration) {
		return helpLimiter.Allow(r.Context(), action, subject, limit, window)
	}, log))
	widgetH.WithHelp(helpSvc, cfg.Auth.FrontendURL).WithBooking(appointmentsSvc)
	// Module guides, product feedback and the ClickFix Asistan (CF-141).
	// The assistant runs on the platform's AI routes, never the org wallet.
	guidesSvc := guidesmodule.New(deps.Queries)
	seedCtx, seedCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := guidesSvc.Seed(seedCtx); err != nil {
		log.Warn("guides_seed_failed", "error", err)
	}
	seedCancel()
	assistantAI := &assistantmodule.PlatformAI{Routes: aiCore.AI.Routes, Q: deps.Queries}
	assistantSvc := &assistantmodule.Service{
		Pool: deps.DB, Q: deps.Queries, AI: assistantAI, Guides: guidesSvc,
		Escalator: &assistantmodule.SupportEscalator{Q: deps.Queries, Instances: inboxCore.Channels, Receiver: inboxCore.Conversations},
		Help:      &assistantmodule.HelpAdapter{Q: deps.Queries, Help: helpSvc},
	}
	assistantSvc.Actions = assistantmodule.DefaultActions(assistantmodule.ActionDeps{
		Q: deps.Queries, Inviter: orgMgmt, SLA: slaSvc, Escalate: assistantSvc.EscalateFromAction,
	})
	// Capability registry (CF-183): every capability calls the panel's /v1
	// routes as the confirming user, so the routes re-check permissions.
	assistantBridge := &apibridge.Bridge{Handler: mux, Tokens: tokens}
	assistantSvc.Caps = capability.NewRegistry().Register(assistantmodule.CoreCapabilities(deps.Queries)...)
	assistantSvc.API = func(user, org uuid.UUID) capability.API {
		return capability.Bridge{API: assistantBridge, User: user, Org: org}
	}
	guidesSvc.Draft = &assistantmodule.Drafter{AI: assistantAI, Settings: assistantSvc.GetSettings}
	guidesmodule.RegisterRoutes(mux, guidesmodule.NewHandler(guidesSvc, activityRec), tokens, loader, tenantResolver)
	assistantLimiter := ratelimit.New(deps.Redis, cfg.App.Env)
	assistantmodule.RegisterRoutes(mux, assistantmodule.NewHandler(assistantSvc, activityRec, func(r *http.Request, action, subject string, limit int, window time.Duration) bool {
		ok, _ := assistantLimiter.Allow(r.Context(), action, subject, limit, window)
		return ok
	}), tokens, loader, tenantResolver)
	forummodule.RegisterRoutes(operationsRouter, &forumhandler.Handler{Service: &forumuc.Service{Repo: billingRepository, AI: aiCore.AI}, Widget: widgetService, Limiter: helpLimiter})
	billingLimiter := ratelimit.New(deps.Redis, cfg.App.Env)
	billingH := billinghandler.New(billingService, entitlementService, activityRec).WithAttachments(attachmentsSvc).
		WithWebhookGuards(log, func(r *http.Request, action, subject string, limit int, window time.Duration) (bool, time.Duration) {
			return billingLimiter.Allow(r.Context(), action, subject, limit, window)
		})
	billingmodule.RegisterRoutes(mux, billingH, tokens, loader, tenantResolver, middleware.RequireStepUp(stepUpSvc))
	organizationsmodule.RegisterTenantRoutes(mux, orgH, tokens, loader, tenantResolver)
	// Presence, typing and org/conversation channel authorization (CF-10).
	presenceSvc := presenceusecase.New(deps.DB, deps.Queries, deps.Realtime, ratelimit.New(deps.Redis, cfg.App.Env), log)
	presenceH := presencehandler.New(presenceSvc, deps.Queries)
	presencemodule.RegisterRoutes(mux, presenceH, tokens, loader, tenantResolver)
	// Mobile OS surfaces (CF-152): widget tokens and widget API (reusing the
	// presence and inbox count handlers), Live Activity tokens and settings.
	mobileSvc := mobileextmodule.NewService(cfg.APNs, deps.DB, deps.Queries, log)
	uc.SetSessionRotationHook(mobileSvc)
	mobileextmodule.RegisterRoutes(mux, mobileexthandler.New(mobileSvc, ratelimit.New(deps.Redis, cfg.App.Env)), mobileSvc, tokens, loader, tenantResolver,
		mobileextmodule.WidgetHandlers{
			Presence: http.HandlerFunc(presenceH.SetStatus),
			Counts:   convH.WithViewer(http.HandlerFunc(convH.Counts)),
		})
	rtHandler := realtime.NewHandler(rtIssuer, presencemodule.Authorizer{Next: inbox.Authorizer{Next: uc, Resolver: tenantResolver, Queries: deps.Queries}, Resolver: tenantResolver, Queries: deps.Queries}).
		WithConnInfo(func(ctx context.Context, userID int64) map[string]any {
			u, err := deps.Queries.GetUserByID(ctx, userID)
			if err != nil {
				return nil
			}
			return map[string]any{"name": strings.TrimSpace(u.Name + " " + u.Surname)}
		})
	realtime.RegisterRoutes(mux, rtHandler, tokens, loader)
	supportaccess.RegisterRoutes(mux, supportaccess.NewHandler(supportSvc, activityRec), tokens, loader, tenantResolver)
	// Platform health overview (CF-14).
	healthSvc := platformhealth.NewService(deps.Queries, asynq.NewInspector(queue.RedisOpt(cfg.Redis)))
	healthSvc.SetMailStatus(s.mailMonitor)
	platformhealth.RegisterRoutes(mux, healthSvc, tokens, loader)
	metrics := platformmetrics.NewMetrics(deps.Queries, healthSvc)
	// Metrics include global operational data; require a platform principal.
	mux.Handle("GET /metrics", middleware.Chain(metrics.Handler(), middleware.Authenticate(tokens, loader), middleware.RequirePermission(rbac.PermPlatformOrganizationsRead)))
	// Organization audit log (CF-12).
	auditmodule.RegisterRoutes(mux, auditmodule.NewHandler(auditmodule.NewService(deps.Queries)), tokens, loader, tenantResolver)
	// Teams and team membership (CF-5).
	teamsmodule.RegisterRoutes(mux, teamshandler.New(teamsusecase.New(deps.DB, deps.Queries), activityRec), tokens, loader, tenantResolver)

	qrGeo, err := qrlogin.NewLocator(cfg.Auth.QRGeoIPDB, cfg.Auth.QRTrustGeoHeaders)
	if err != nil {
		// Location is a hint only: keep QR sign-in up and show the IP.
		log.Warn("qrlogin_geoip_unavailable", "error", err)
	}
	// Country targeting of the web widget uses the same edge headers / GeoIP DB.
	widgetH.WithCountry(func(r *http.Request) string { return qrGeo.Locate(r, publicClientIP(r)).CountryCode })
	var qrIssuer qrlogin.TokenIssuer
	if rtIssuer != nil {
		qrIssuer = rtIssuer
	}
	qrSvc := qrlogin.NewService(qrlogin.NewStore(deps.Redis, cfg.App.Env), uc, qrIssuer, deps.Realtime, cfg.Auth.FrontendURL, log)
	qrlogin.RegisterRoutes(mux, qrlogin.NewHandler(qrSvc, qrGeo, ratelimit.New(deps.Redis, cfg.App.Env)), tokens, loader)

	nh := notifhandler.New(notifSvc)
	nh.SetRateLimiter(ratelimit.New(deps.Redis, cfg.App.Env))
	notifmodule.RegisterRoutes(mux, nh, tokens, loader)
	notifmodule.RegisterEventHandlers(eventBus, notifSvc, log)

	ioReg := ioengine.NewRegistry(
		ioadapters.NewUsers(deps.Queries),
		ioadapters.NewRoles(deps.Queries),
		ioadapters.NewNotifications(deps.Queries),
		ioadapters.NewActivity(deps.Queries),
		&workforce.TimesheetAdapter{Service: &workforce.Service{Repo: billingRepository}},
		orgadapters.NewIO(orgSvc),
		contactsIO, companiesIO,
	)
	exportSvc := exportusecase.New(deps.Queries, deps.Storage, ioReg, deps.Queue, notifSvc, activityRec, log)
	importSvc := importusecase.New(deps.Queries, deps.Storage, ioReg, deps.Queue, notifSvc, activityRec, log)
	contactsmodule.RegisterIORoutes(mux, contactshandler.NewIO(importSvc, contactsIO, companiesIO, activityRec), tokens, loader, tenantResolver)
	bulkReg := bulkengine.NewRegistry(
		bulkadapters.NewUsers(deps.Queries),
		bulkadapters.NewRoles(deps.Queries),
		orgadapters.NewBulk(orgSvc),
	)
	bulkSvc := bulkusecase.New(deps.Queries, bulkReg, deps.Queue, notifSvc, activityRec, cfg.Bulk, log)
	logsSvc := logsusecase.New(deps.Queries)
	if s.worker != nil {
		s.worker.WithExport(exportSvc.ProcessExport).
			WithImport(importSvc.ProcessImport).
			WithBulk(bulkSvc.ProcessBulk).
			WithLogPurge(logsSvc.ApplyDueRules).
			WithMobilePush(notifSvc.SendMobilePush, notifSvc.ProcessPushReceipts).
			WithConversationSend(inboxCore.Deliver).
			WithIMAPPolling(func(ctx context.Context) error {
				return inboxCore.Channels.EnqueueIMAPPolls(ctx, deps.Queue.EnqueueIMAPPoll)
			}, func(ctx context.Context, org, instance int64) error {
				return inboxCore.Channels.PollIMAP(ctx, org, instance, inboxCore.Receive)
			}).
			WithChannelMaintenance(inboxCore.Channels.HealthSweep, func(ctx context.Context) (int, error) {
				return inboxCore.Channels.Reconcile(ctx, cfg.Channels.ReconcileWindow, inboxCore.Receive)
			}).
			WithConversationTick(inboxCore.Tick).
			WithBillingLifecycle(billingService.Sweep).
			WithBillingPayments(func(ctx context.Context) error { return billingService.CommerceSweep(ctx, time.Now().UTC()) }, billingService.ProviderSync, billingService.NightlyCommerce).
			Handle(queue.TaskUptimeSweep, func(ctx context.Context, _ *asynq.Task) error { return oncallSvc.SweepMonitors(ctx) }).
			Handle(queue.TaskOncallSweep, func(ctx context.Context, _ *asynq.Task) error { return oncallSvc.SweepAlerts(ctx) }).
			Handle(queue.TaskVideoSweep, func(ctx context.Context, _ *asynq.Task) error { return videoSvc.Sweep(ctx) }).
			Handle(queue.TaskVideoRecordingPurge, func(ctx context.Context, _ *asynq.Task) error { return videoSvc.PurgeExpired(ctx) }).
			Handle(queue.TaskReportsSweep, func(ctx context.Context, _ *asynq.Task) error { return reportsSvc.Sweep(ctx) }).
			Handle(retention.Task, func(ctx context.Context, _ *asynq.Task) error {
				return (retention.Service{Q: deps.Queries, Store: deps.Storage}).Purge(ctx)
			}).
			Handle(queue.TaskIntegrationTriggerPurge, func(ctx context.Context, _ *asynq.Task) error { return deps.Queries.IntegrationPurgeTriggers(ctx) }).
			Handle(queue.TaskKnowledgeSync, func(ctx context.Context, _ *asynq.Task) error { return agentSvc.Sources.Sweep(ctx) }).
			Handle(queue.TaskRecordingTranscribe, transcriptSvc.Handle).
			Handle(queue.TaskRecordingTranscriptSweep, func(ctx context.Context, _ *asynq.Task) error { return transcriptSvc.Sweep(ctx) }).
			Handle(queue.TaskAIReconcile, func(ctx context.Context, _ *asynq.Task) error { return aiCore.AI.Reconcile(ctx) }).
			Handle(queue.TaskCustomerHealthSweep, queue.CustomerHealthSweepHandler(healthusecase.New(deps.DB, deps.Queries, activityRec, log).Sweep)).
			Handle(queue.TaskNotificationDigest, func(ctx context.Context, _ *asynq.Task) error { _, err := notifSvc.ProcessDigests(ctx); return err }).
			Handle(queue.TaskPresenceSweep, func(ctx context.Context, _ *asynq.Task) error { _, err := presenceSvc.SweepIdle(ctx); return err }).
			Handle(queue.TaskAppointmentReminders, func(ctx context.Context, _ *asynq.Task) error {
				_, err := appointmentsSvc.SweepReminders(ctx)
				return err
			})
		if searchIndexer != nil {
			s.worker.WithSearch(
				searchIndexer.ProcessUpsert,
				searchIndexer.ProcessDelete,
				searchIndexer.ProcessReindex,
			)
		}
	}
	exportH := exporthandler.New(exportSvc)
	bulkH := bulkhandler.New(bulkSvc)
	organizationsmodule.RegisterRoutes(mux, orgH, organizationsmodule.PlatformEngineHandlers{
		Export: exportH.PlatformExport(orgadapters.Resource, rbac.PermPlatformOrganizationsExport),
		Bulk:   bulkH.Execute(orgadapters.Resource),
		StepUp: middleware.RequireStepUp(stepUpSvc),
	}, tokens, loader)
	exportmodule.RegisterRoutes(mux, exportH, tokens, loader, stepUpSvc)
	importmodule.RegisterRoutes(mux, importhandler.New(importSvc), tokens, loader)
	bulkmodule.RegisterRoutes(mux, bulkH, tokens, loader)
	settingsmodule.RegisterRoutes(mux, settingshandler.New(
		settingsusecase.New(deps.Queries),
		deps.Storage,
		settingsusecase.NewSystemService(cfg, deps.DB, deps.Redis, deps.Storage, searchClient),
	), tokens, loader)
	// Marketing site tracking settings and sign-up sources (CF-144).
	marketingmodule.RegisterRoutes(mux, marketingmodule.NewHandler(marketingmodule.NewService(deps.Queries), deps.Storage, activityRec), tokens, loader)
	accessmodule.RegisterRoutes(mux, accesshandler.New(stepUpSvc, activityRec), tokens, loader)
	authsettingsmodule.RegisterRoutes(mux, authsettingshandler.New(authSettingsSvc, activityRec), tokens, loader)
	githubmodule.RegisterRoutes(mux, githubhandler.New(githubSvc, activityRec), tokens, loader)
	oauthprovidermodule.RegisterRoutes(mux, oauthproviderhandler.New(oauthProvSvc, activityRec), tokens, loader)
	activitymodule.RegisterRoutes(mux, activityhandler.New(activityusecase.New(deps.Queries)), tokens, loader)
	logsmodule.RegisterRoutes(mux, logshandler.New(logsSvc), tokens, loader)
	searchSvc := searchusecase.New(searchClient, searchReg, log).WithConversationAccess(deps.Queries)
	searchmodule.RegisterRoutes(mux, searchhandler.New(searchSvc), tokens, loader, tenantResolver)
	storagemodule.RegisterRoutes(
		mux,
		storagehandler.New(storageusecase.New(deps.Storage, deps.Queries, log)),
		tokens,
		loader,
	)

	reliableBus, ok := eventBus.(events.ReliableBus)
	if !ok {
		return nil, fmt.Errorf("httpserver: billing requires reliable event subscriptions")
	}
	developerRuntime, err := developerplatform.Mount(developerplatform.Deps{Routing: routingSvc, AI: aiCore.AI, Config: cfg, DB: deps.DB, Q: deps.Queries, Redis: deps.Redis, Log: log, Router: mux, Tokens: tokens, Loader: loader, Resolver: tenantResolver, Conversations: inboxCore.Conversations, Notifications: notifSvc, Activity: activityRec, Bus: reliableBus})
	if err != nil {
		return nil, fmt.Errorf("httpserver: developer platform: %w", err)
	}
	developerRuntime.RegisterWorker(s.worker)
	supportoperations.RegisterEvents(reliableBus, billingRepository, aiCore.AI, inboxCore.Conversations, notifSvc, deps.Realtime)
	billingmodule.RegisterTrialNotifications(reliableBus, deps.Queries, notifSvc)
	billingmodule.RegisterPaymentNotifications(reliableBus, deps.Queries, notifSvc)
	entitlementService.RegisterMetering(reliableBus)
	consumerReg, err := eventconsumers.New(eventconsumers.Deps{
		Oncall: oncallSvc, Reports: reportsSvc,
		Notifications:       notifSvc,
		NotificationsLookup: notifmodule.DBLookup{Q: deps.Queries},
		SLA:                 slaSvc,
		Analytics:           analyticsSvc,
		Surveys:             surveysSvc,
		Status:              statusSvc,
		Routing:             routingSvc,
		Knowledge:           aiCore.RAG,
		Agents:              agentSvc,
		LiveActivities:      mobileSvc,
	})
	if err != nil {
		return nil, fmt.Errorf("httpserver: event consumers: %w", err)
	}
	var eventEnq eventdispatch.Enqueuer
	if deps.Queue != nil {
		eventEnq = deps.Queue
	}
	dispatcher := eventdispatch.NewDispatcher(consumerReg, eventEnq, deps.Queries, log)
	if s.worker != nil {
		processor := eventdispatch.NewProcessor(consumerReg, deps.DB, deps.Queries, log)
		s.worker.Handle(eventdispatch.TaskConsume, processor.ProcessTask).
			Handle(eventdispatch.TaskPurgeConsumptions, processor.PurgeConsumptions).
			Handle(queue.TaskSLASweep, queue.SLASweepHandler(slaSvc.Sweep)).
			Handle(queue.TaskAnalyticsBackfill, queue.AnalyticsBackfillHandler(analyticsSvc.Backfill)).
			Handle(queue.TaskSLAResync, queue.SLAResyncHandler(slaSvc.ResyncOrganization)).
			Handle(queue.TaskRoutingSweep, queue.RoutingSweepHandler(routingSvc.Sweep))
	}
	outboxStore := outbox.NewStore(deps.DB, deps.Queries)
	outboxPub := outbox.NewPublisher(outboxStore, eventBus, log, outbox.WithSink(dispatcher))
	s.outboxPub = outboxPub

	s.http = &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      middleware.SecurityHeaders(middleware.RequestID(middleware.ServerErrors(log)(metrics.Middleware(marketingmodule.CaptureSignup(deps.Queries, log)(mux))))),
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}
	return s, nil
}

// Start listens until the server stops. Optionally starts an in-process queue worker.
func (s *Server) Start() error {
	if s.outboxPub != nil {
		s.outboxStop = s.outboxPub.StartRun(context.Background())
	}
	if s.worker != nil {
		go func() {
			if err := s.worker.Start(); err != nil {
				s.log.Error("queue_worker_failed", "error", err)
			}
		}()
	}
	if s.searchIndexer != nil {
		go s.searchIndexer.Bootstrap(context.Background())
	}
	s.log.Info("http_listen", "addr", s.cfg.HTTP.Addr, "env", s.cfg.App.Env)
	if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("httpserver: listen: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the HTTP server, queue worker, and queue client.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.demoCancel != nil {
		s.demoCancel()
	}
	if s.outboxStop != nil {
		s.outboxStop()
	}
	if s.worker != nil {
		s.worker.Shutdown()
	}
	if s.queueClient != nil {
		_ = s.queueClient.Close()
	}
	return s.http.Shutdown(ctx)
}

func (s *Server) handleAppConfig(w http.ResponseWriter, r *http.Request) {
	settingsSvc := settingsusecase.New(s.queries)
	letterhead, _ := settingsSvc.PublicLetterhead(r.Context())

	registrationEnabled := false
	signupMode := "invite"
	passwordLogin := true
	passwordRegister := false
	passkeyLogin := true
	if s.authSettings != nil {
		if p, err := s.authSettings.Policy(r.Context()); err == nil {
			registrationEnabled = p.RegistrationEnabled
			signupMode = p.SignupMode
			passwordLogin = p.PasswordLoginEnabled
			passwordRegister = p.RegistrationEnabled && p.PasswordRegisterEnabled
			passkeyLogin = p.PasskeyLoginEnabled
		}
	}

	githubLogin, githubRegister := false, false
	if s.githubSvc != nil {
		githubLogin, _ = s.githubSvc.IsAuthEnabled(r.Context())
		reg, _ := s.githubSvc.IsRegisterEnabled(r.Context())
		githubRegister = registrationEnabled && reg
	}

	method := func(login, register bool) map[string]bool {
		return map[string]bool{"login": login, "register": register}
	}
	oauthMethod := func(provider string) map[string]bool {
		login, register := false, false
		if s.oauthProvSvc != nil {
			login, _ = s.oauthProvSvc.IsLoginEnabled(r.Context(), provider)
			reg, _ := s.oauthProvSvc.IsRegisterEnabled(r.Context(), provider)
			register = registrationEnabled && reg
		}
		return method(login, register)
	}

	responseJSON := map[string]any{
		"mode":                 "platform",
		"name":                 s.cfg.App.Name,
		"vapid_configured":     s.cfg.VAPID.PublicKey != "" && s.cfg.VAPID.PrivateKey != "",
		"github_auth_enabled":  githubLogin, // legacy
		"registration_enabled": registrationEnabled,
		// CF-143: open / invite / closed; self-serve organizations follow
		// "open" (the landing page offers sign-up only then).
		"signup_mode":        signupMode,
		"self_serve_enabled": signupMode == "open",
		"auth_methods": map[string]any{
			"password": method(passwordLogin, passwordRegister),
			"passkey":  map[string]bool{"login": passkeyLogin},
			"github":   method(githubLogin, githubRegister),
			"google":   oauthMethod("google"),
			"facebook": oauthMethod("facebook"),
			"apple":    oauthMethod("apple"),
		},
		"letterhead": letterhead,
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    responseJSON,
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	checks := map[string]string{
		"postgres": "ok",
		"redis":    "ok",
	}
	ready := true

	if err := database.Ping(ctx, s.db); err != nil {
		checks["postgres"] = "down"
		ready = false
	}
	if err := cache.Ping(ctx, s.redis); err != nil {
		checks["redis"] = "down"
		ready = false
	}

	if s.storage != nil {
		if err := s.storage.Ping(ctx); err != nil {
			s.log.Warn("readyz_storage_soft_check", "error", err)
		}
	}
	if s.searchClient != nil && s.searchClient.Enabled() {
		if err := s.searchClient.Ping(ctx); err != nil {
			checks["meilisearch"] = "down"
			s.log.Warn("readyz_meilisearch_soft_check", "error", err)
		} else {
			checks["meilisearch"] = "ok"
		}
	}

	// Soft check: mail egress never makes the API unready (the cached SMTP
	// probe result is shown; a stale one refreshes in the background).
	if s.mailMonitor != nil {
		checks["mail"] = s.mailMonitor.Cached().State
	}

	status := http.StatusOK
	state := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		state = "not_ready"
	}

	writeJSON(w, status, map[string]any{
		"status": state,
		"checks": checks,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// publicClientIP keys the public per-IP limits (surveys, status subscriptions,
// booking) and widget country targeting. Behind the BFF the peer is always
// the BFF itself, so the forwarded client address is used (CF-153).
func publicClientIP(r *http.Request) string {
	return clientip.FromRequest(r)
}
