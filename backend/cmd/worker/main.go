package main

import (
	appointmentsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/appointments/usecase"
	authrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/repository"
	authuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/usecase"

	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/retention"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/aisystem"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/cache"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/developerplatform"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/eventconsumers"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/inbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/logging"
	agentrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/repository"
	agentusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/usecase"
	analyticsrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/repository"
	analyticsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/usecase"
	attachmentsrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/repository"
	attachmentsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/autoreply"
	billingmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing"
	billingrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	billingusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	bulkusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/bulk/usecase"
	compliancerepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/compliance/repository"
	complianceuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/compliance/usecase"
	contactsio "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/ioadapter"
	contactsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/usecase"
	convrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	healthusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customerhealth/usecase"
	customfieldsrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/repository"
	customfieldsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/usecase"
	departmentsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/entitlements"
	exportusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/exports/usecase"
	importusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/imports/usecase"
	systemsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems/usecase"
	logsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/logs/usecase"
	mobileextmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mobileext"
	notifmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/oncall"
	oncallrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/oncall/repository"
	orgrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orguc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	presenceusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/presence/usecase"
	reportsrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/reports/repository"
	reportsuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/reports/usecase"
	routingusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/usecase"
	slausecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/usecase"
	ssouc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sso/usecase"
	statusrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/repository"
	statususecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/usecase"
	surveysrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys/repository"
	surveysusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/transcription"
	transcriptrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/transcription/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/videorooms"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/workforce"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/observability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/bulkengine"
	bulkadapters "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/bulkengine/adapters"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine"
	ioadapters "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine/adapters"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/mail"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine"
	searchadapters "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine/adapters"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/queue"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/supportoperations"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/workerschedule"
	"github.com/hibiken/asynq"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = os.Stderr.WriteString("config load failed: " + err.Error() + "\n")
		os.Exit(1)
	}

	log := logging.New(cfg.Log.Level, cfg.Log.Format)
	flushSentry, err := observability.Init(cfg.Sentry, "worker")
	if err != nil {
		log.Warn("sentry_init_failed", "error", err)
	} else if observability.Enabled() {
		log.Info("sentry_enabled", "environment", cfg.Sentry.Environment, "traces_sample_rate", cfg.Sentry.TracesSampleRate)
	}
	defer flushSentry()
	// exit flushes pending error events before leaving (os.Exit skips defers).
	exit := func(code int) {
		flushSentry()
		os.Exit(code)
	}
	log = slog.New(observability.NewSlogHandler(log.Handler()))
	if !cfg.Queue.Enabled {
		log.Error("queue_disabled")
		exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		log.Error("database_init_failed", "error", err)
		exit(1)
	}
	defer pool.Close()

	store, err := storage.NewFromConfig(ctx, cfg.Storage)
	if err != nil {
		log.Error("storage_init_failed", "error", err)
		exit(1)
	}

	var publisher realtime.Publisher
	if cfg.Centrifugo.Enabled {
		publisher = realtime.NewCentrifugoClient(cfg.Centrifugo)
	} else {
		publisher = realtime.NoopPublisher{}
	}

	queries := database.NewQueries(pool)
	mailer := mail.NewSMTPSender(cfg.SMTP)
	provs := []providers.Provider{
		providers.InappProvider{},
		providers.EmailProvider{Mail: mailer, AppURL: cfg.Auth.FrontendURL},
		providers.RealtimeProvider{Pub: publisher},
		providers.NoopProvider{Name: "sms", Log: log},
		providers.NoopProvider{Name: "push", Log: log},
	}
	// Deliveries go through the notifications queue (bounded retries with
	// backoff, then dead letter), never inline: an event consumer or outbox
	// handler that creates a notification must not block on SMTP or fail
	// (and replay the whole event) because mail is unreachable.
	queueClient := queue.NewClient(cfg.Redis)
	defer func() { _ = queueClient.Close() }()
	notifSvc := notifusecase.New(queries, queueClient, provs, log)
	notifSvc.WithVAPID(notifusecase.VAPIDConfig{
		PublicKey:  cfg.VAPID.PublicKey,
		PrivateKey: cfg.VAPID.PrivateKey,
		Subject:    cfg.VAPID.Subject,
	})
	notifSvc.WithExpo(notifusecase.ExpoConfig{AccessToken: cfg.Expo.AccessToken})
	notifSvc.WithAppURL(cfg.Auth.FrontendURL)

	eventBus := events.NewBus(log)
	notifmodule.RegisterEventHandlers(eventBus, notifSvc, log)
	billingmodule.RegisterTrialNotifications(eventBus, queries, notifSvc)
	billingmodule.RegisterPaymentNotifications(eventBus, queries, notifSvc)
	entitlements.New(billingrepo.New(pool, queries), nil, cfg.App.Env).RegisterMetering(eventBus)
	statusSvc := statususecase.New(statusrepo.New(pool, queries), nil, notifSvc, cfg.Encryption.Key, cfg.Auth.FrontendURL)
	surveysSvc := surveysusecase.New(surveysrepo.New(pool, queries), nil, cfg.Encryption.Key, cfg.Auth.FrontendURL)
	analyticsSvc := analyticsusecase.New(analyticsrepo.New(pool, queries))
	slaSvc := slausecase.New(pool, queries, outbox.NewStore(pool, queries), log).WithResyncer(queueClient).
		WithFields(customfieldsusecase.New(customfieldsrepository.New(pool, queries)))
	// Routing (CF-53) assigns through the inbox core built below.
	routingAssign := &inbox.RoutingAssigner{}
	routingSvc := routingusecase.New(pool, queries, routingAssign, log)
	aiBilling := billingrepo.New(pool, queries)
	aiCore := aisystem.New(pool, aiBilling, entitlements.New(aiBilling, nil, cfg.App.Env), cfg.Search)
	agentSvc := agentusecase.New(&agentrepo.Store{Repository: aiBilling}, aiCore.AI, nil)
	aiCore.AI.Context = agentSvc
	agentSvc.Sources = &agentusecase.SourceService{Agents: agentSvc, Storage: store}
	agentSvc.Fields = customfieldsusecase.New(customfieldsrepository.New(pool, queries))
	agentSvc.Hours = slaSvc
	autoReplySvc := autoreply.New(queries, nil, slaSvc, slaSvc, log)
	agentSvc.Notices = autoReplySvc
	oncallSvc := oncall.New(oncallrepo.New(pool, queries), notifSvc)
	reportsSvc := &reportsuc.Service{Repo: reportsrepo.New(pool, queries), Resolver: orguc.New(orgrepo.NewPostgres(pool, queries), nil), Notifier: notifSvc, BaseURL: cfg.Auth.FrontendURL, Secret: cfg.Encryption.Key}
	consumerReg, err := eventconsumers.New(eventconsumers.Deps{
		Oncall: oncallSvc, Reports: reportsSvc,
		Routing:             routingSvc,
		Notifications:       notifSvc,
		NotificationsLookup: notifmodule.DBLookup{Q: queries},
		SLA:                 slaSvc,
		AutoReply:           autoReplySvc,
		Knowledge:           aiCore.RAG,
		Agents:              agentSvc,
		Analytics:           analyticsSvc,
		Surveys:             surveysSvc,
		Status:              statusSvc,
		LiveActivities:      mobileextmodule.NewService(cfg.APNs, pool, queries, log),
	})
	if err != nil {
		log.Error("event_consumers_init_failed", "error", err)
		exit(1)
	}
	eventDispatcher := eventdispatch.NewDispatcher(consumerReg, queueClient, queries, log)
	eventProcessor := eventdispatch.NewProcessor(consumerReg, pool, queries, log)
	outboxStore := outbox.NewStore(pool, queries)
	outboxPub := outbox.NewPublisher(outboxStore, eventBus, log, outbox.WithSink(eventDispatcher))

	presenceSvc := presenceusecase.New(pool, queries, publisher, nil, log)
	activityRec := activity.NewRecorder(queries, log)
	workerContacts := contactsusecase.New(convrepository.New(pool, queries))
	ioReg := ioengine.NewRegistry(
		ioadapters.NewUsers(queries),
		ioadapters.NewRoles(queries),
		ioadapters.NewNotifications(queries),
		ioadapters.NewActivity(queries),
		&workforce.TimesheetAdapter{Service: &workforce.Service{Repo: billingrepo.New(pool, queries)}},
		// CF-84: tenant contact / company imports run here too.
		contactsio.NewContacts(workerContacts, queries).WithFields(customfieldsusecase.New(customfieldsrepository.New(pool, queries))),
		contactsio.NewCompanies(workerContacts, queries).WithFields(customfieldsusecase.New(customfieldsrepository.New(pool, queries))),
	)
	exportSvc := exportusecase.New(queries, store, ioReg, nil, notifSvc, activityRec, log)
	importSvc := importusecase.New(queries, store, ioReg, nil, notifSvc, activityRec, log)
	bulkReg := bulkengine.NewRegistry(
		bulkadapters.NewUsers(queries),
		bulkadapters.NewRoles(queries),
	)
	bulkSvc := bulkusecase.New(queries, bulkReg, nil, notifSvc, activityRec, cfg.Bulk, log)
	logsSvc := logsusecase.New(queries)
	searchReg := searchengine.NewRegistry(searchadapters.All(queries)...)
	searchClient := searchengine.NewClient(cfg.Search, log)
	searchIndexer := searchengine.NewIndexer(searchClient, searchReg, nil, log)
	notifSvc.WithPushQueue(queueClient)
	persist := logging.Attach(log, logsSvc)
	log = persist.Logger()
	defer persist.Close()

	billingRepository := billingrepo.New(pool, queries)
	billingService := billingusecase.New(billingRepository)
	videoSvc := videorooms.New(queries, cfg, billingService, entitlements.New(billingRepository, nil, cfg.App.Env), store, notifSvc, log)
	attachmentsSvc := attachmentsusecase.New(attachmentsrepository.NewPostgres(pool, queries), store, cfg.Encryption.Key, cfg.Attachment, log)
	transcriptSvc := &transcription.Service{Repo: &transcriptrepo.Store{Queries: queries, Pool: pool}, Speech: aiCore.Speech, AI: aiCore.AI, Files: attachmentsSvc, Video: videoSvc, Jobs: queueClient, Summarize: os.Getenv("AI_TRANSCRIBE_SUMMARY") == "true"}
	videoSvc.RecordingCompleted = transcriptSvc.Sweep
	if err := billingService.InvoicingFromEnvironment(cfg.App.Env); err != nil {
		log.Error("billing_invoice_init_failed", "error", err)
		os.Exit(1)
	}
	if err := billingService.DevelopmentGateways(billingRepository, cfg.App.Env, cfg.Encryption.Key, cfg.Auth.FrontendURL); err != nil {
		log.Error("billing_gateway_init_failed", "error", err)
		os.Exit(1)
	}
	// Omnichannel inbox: delivers queued outbound messages through the
	// channel adapters (same composition as the API server).
	inboxRedis, err := cache.NewRedisClient(ctx, cfg.Redis)
	if err != nil {
		log.Warn("inbox_redis_unavailable_send_rate_limit_off", "error", err)
		inboxRedis = nil
	} else {
		defer func() { _ = inboxRedis.Close() }()
	}
	inboxCore, err := inbox.New(inbox.Deps{
		Config: cfg, DB: pool, Queries: queries, Redis: inboxRedis, Queue: queueClient, Realtime: publisher, Log: log, Attachments: attachmentsSvc,
		// Inbound messages handled here are indexed through the search queue.
		Search: searchengine.NewIndexer(searchClient, searchReg, queueClient, log), SearchClient: searchClient,
	})
	if err != nil {
		log.Error("inbox_init_failed", "error", err)
		os.Exit(1)
	}
	inboxCore.RecordingCompleted = transcriptSvc.Sweep
	// Appointment reminders (CF-138); booking runs in the API server.
	appointmentsSvc := appointmentsusecase.New(appointmentsusecase.Deps{
		Pool: pool, Queries: queries, Notifier: notifSvc, Timeline: inboxCore.Conversations, Video: videoSvc, Activity: activityRec,
		Realtime: publisher, Secret: cfg.Encryption.Key, BaseURL: cfg.Auth.FrontendURL, Log: log,
	})
	routingAssign.Conversations = inboxCore.Conversations
	inboxCore.Conversations.WithHours(slaSvc)
	autoReplySvc.WithPoster(inboxCore.Conversations)
	surveysSvc.WithSender(inboxCore.Conversations)
	statusSvc.WithSender(inboxCore.Conversations)

	supportoperations.RegisterEvents(eventBus, aiBilling, aiCore.AI, inboxCore.Conversations, notifSvc, publisher)
	agentSvc.Sender = inboxCore.Conversations
	// Departments (CF-215): AI routing of first messages and transfers.
	departmentsSvc := departmentsusecase.New(pool, queries)
	departmentsSvc.Conversations, departmentsSvc.SLA, departmentsSvc.Log = inboxCore.Conversations, slaSvc, log
	departmentsSvc.Classifier, agentSvc.Departments = agentSvc, departmentsSvc
	agentBox, agentBoxErr := crypto.NewSecretBox(cfg.Encryption.Key)
	if agentBoxErr != nil {
		log.Warn("ai_tool_credentials_unavailable", "error", agentBoxErr)
	}
	agentSvc.Tools = &agentusecase.ToolService{Repo: agentSvc.Repo, Core: aiCore.AI, Conversations: inboxCore.Conversations, Box: agentBox}
	// Connected system (CF-197): playbooks, concept data and AI actions.
	var systemsLimiter systemsusecase.Limiter
	if inboxRedis != nil {
		systemsLimiter = ratelimit.New(inboxRedis, cfg.App.Env)
	}
	agentSvc.Systems = systemsusecase.New(queries, agentBox, systemsLimiter)
	complianceSvc := &complianceuc.Service{Search: searchClient, Repo: compliancerepo.New(pool, queries), Store: store, Jobs: queueClient, Activity: activityRec}
	accountRepo := authrepo.NewPostgres(pool, queries)
	accountRepo.SetDeletionSearch(searchClient)
	worker := queue.NewWorker(cfg, log, notifSvc.Deliver).
		WithConversationSend(inboxCore.Deliver).
		WithNetgsm(inboxCore.EnqueueNetgsm, inboxCore.PollNetgsm, inboxCore.ArchiveNetgsm).
		WithIMAPPolling(func(ctx context.Context) error {
			return inboxCore.Channels.EnqueueIMAPPolls(ctx, queueClient.EnqueueIMAPPoll)
		}, func(ctx context.Context, orgID, instanceID int64) error {
			return inboxCore.Channels.PollIMAP(ctx, orgID, instanceID, inboxCore.Receive)
		}).
		WithConversationTick(inboxCore.Tick).
		WithChannelMaintenance(inboxCore.Channels.HealthSweep, func(ctx context.Context) (int, error) {
			return inboxCore.Channels.Reconcile(ctx, cfg.Channels.ReconcileWindow, inboxCore.Receive)
		}).
		WithBillingLifecycle(billingService.Sweep).
		WithBillingPayments(func(ctx context.Context) error { return billingService.CommerceSweep(ctx, time.Now().UTC()) }, billingService.ProviderSync, billingService.NightlyCommerce).
		WithExport(exportSvc.ProcessExport).
		WithImport(importSvc.ProcessImport).
		WithBulk(bulkSvc.ProcessBulk).
		WithLogPurge(logsSvc.ApplyDueRules).
		WithMobilePush(notifSvc.SendMobilePush, notifSvc.ProcessPushReceipts).
		WithSearch(
			searchIndexer.ProcessUpsert,
			searchIndexer.ProcessDelete,
			searchIndexer.ProcessReindex,
		).
		Handle(queue.TaskUptimeSweep, func(ctx context.Context, _ *asynq.Task) error { return oncallSvc.SweepMonitors(ctx) }).
		Handle(queue.TaskOncallSweep, func(ctx context.Context, _ *asynq.Task) error { return oncallSvc.SweepAlerts(ctx) }).
		Handle(queue.TaskVideoSweep, func(ctx context.Context, _ *asynq.Task) error { return videoSvc.Sweep(ctx) }).
		Handle(queue.TaskVideoRecordingPurge, func(ctx context.Context, _ *asynq.Task) error { return videoSvc.PurgeExpired(ctx) }).
		Handle(queue.TaskReportsSweep, func(ctx context.Context, _ *asynq.Task) error { return reportsSvc.Sweep(ctx) }).
		Handle(retention.Task, func(ctx context.Context, _ *asynq.Task) error {
			return (retention.Service{Q: queries, Store: store}).Purge(ctx)
		}).
		Handle(queue.TaskIntegrationTriggerPurge, func(ctx context.Context, _ *asynq.Task) error { return queries.IntegrationPurgeTriggers(ctx) }).
		Handle(queue.TaskKnowledgeSync, func(ctx context.Context, _ *asynq.Task) error { return agentSvc.Sources.Sweep(ctx) }).
		Handle(queue.TaskRecordingTranscribe, transcriptSvc.Handle).
		Handle(queue.TaskRecordingTranscriptSweep, func(ctx context.Context, _ *asynq.Task) error { return transcriptSvc.Sweep(ctx) }).
		Handle(queue.TaskAIReconcile, func(ctx context.Context, _ *asynq.Task) error { return aiCore.AI.Reconcile(ctx) }).
		Handle(eventdispatch.TaskConsume, eventProcessor.ProcessTask).
		Handle(eventdispatch.TaskPurgeConsumptions, eventProcessor.PurgeConsumptions).
		Handle(queue.TaskSLASweep, queue.SLASweepHandler(slaSvc.Sweep)).
		Handle(queue.TaskAnalyticsBackfill, queue.AnalyticsBackfillHandler(analyticsSvc.Backfill)).
		Handle(queue.TaskSLAResync, queue.SLAResyncHandler(slaSvc.ResyncOrganization)).
		Handle(queue.TaskRoutingSweep, queue.RoutingSweepHandler(routingSvc.Sweep)).
		Handle(ssouc.TaskCleanup, func(ctx context.Context, _ *asynq.Task) error { return queries.SSOExpireStates(ctx) }).
		Handle(authuc.TaskAccountPurge, authuc.AccountPurgeHandler(accountRepo)).
		Handle(complianceuc.TaskProcess, complianceSvc.ProcessTask).
		Handle(complianceuc.TaskSweep, complianceSvc.Sweep).
		Handle(complianceuc.TaskMetaDeletion, complianceSvc.MetaDeletionTask).
		Handle(queue.TaskCustomerHealthSweep, queue.CustomerHealthSweepHandler(healthusecase.New(pool, queries, activityRec, log).Sweep)).
		Handle(queue.TaskNotificationDigest, func(ctx context.Context, _ *asynq.Task) error {
			if n, err := notifSvc.ProcessDigests(ctx); err != nil {
				return err
			} else if n > 0 {
				log.Info("notification_digests_sent", "count", n)
			}
			return nil
		}).
		Handle(queue.TaskAppointmentReminders, func(ctx context.Context, _ *asynq.Task) error {
			if n, err := appointmentsSvc.SweepReminders(ctx); err != nil {
				return err
			} else if n > 0 {
				log.Info("appointment_reminders_sent", "count", n)
			}
			return nil
		}).
		Handle(queue.TaskPresenceSweep, func(ctx context.Context, _ *asynq.Task) error {
			if n, err := presenceSvc.SweepIdle(ctx); err != nil {
				return err
			} else if n > 0 {
				log.Info("presence_idle_swept", "count", n)
			}
			return nil
		})

	developerRuntime, err := developerplatform.Mount(developerplatform.Deps{Routing: routingSvc, AI: aiCore.AI, Config: cfg, DB: pool, Q: queries, Redis: inboxRedis, Log: log, Conversations: inboxCore.Conversations, Notifications: notifSvc, Activity: activityRec, Bus: eventBus})
	if err != nil {
		log.Error("developer_platform_init_failed", "error", err)
		exit(1)
	}
	developerRuntime.RegisterWorker(worker)
	outboxStop := outboxPub.StartRun(ctx)
	defer outboxStop()
	if searchIndexer.Enabled() {
		go searchIndexer.Bootstrap(ctx)
	}

	scheduler, err := workerschedule.Start(cfg, log)
	if err != nil {
		log.Error("scheduler_failed", "error", err)
		exit(1)
	}
	if scheduler != nil {
		defer scheduler.Shutdown()
	}

	log.Info(
		"worker_started",
		"app", cfg.App.Name,
		"env", cfg.App.Env,
		"concurrency", cfg.Queue.Concurrency,
	)

	// Only the worker that serves the notifications queue reclaims (the
	// sync worker shares this binary). Reclaim only enqueues.
	if servesQueue(cfg.Queue.Queues, queue.QueueNotifications) {
		if n, err := notifSvc.ReclaimStuck(ctx, notifusecase.DefaultStuckProcessingMinutes); err != nil {
			log.Warn("notification_reclaim_failed", "error", err)
		} else if n > 0 {
			log.Info("notification_reclaim_completed", "count", n)
		}
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- worker.Start()
	}()

	select {
	case <-ctx.Done():
		log.Info("worker_shutdown_signal")
		worker.Shutdown()
	case err := <-errCh:
		if err != nil {
			log.Error("worker_failed", "error", err)
			exit(1)
		}
	}
	log.Info("worker_stopped")
}

// servesQueue reports whether this worker consumes name (an empty
// QUEUE_QUEUES means the default set, which includes every queue).
func servesQueue(queues, name string) bool {
	parsed, err := config.ParseQueueQueues(queues)
	if err != nil || parsed == nil {
		return true
	}
	_, ok := parsed[name]
	return ok
}
