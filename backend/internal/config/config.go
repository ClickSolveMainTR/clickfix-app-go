package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/joho/godotenv"
)

// Config holds process configuration loaded from the environment.
type Config struct {
	SSOTrustedProxies []string
	App               AppConfig
	Encryption        EncryptionConfig
	HTTP              HTTPConfig
	DB                DBConfig
	Redis             RedisConfig
	Storage           StorageConfig
	Attachment        AttachmentConfig
	CORS              CORSConfig
	SMTP              SMTPConfig
	Queue             QueueConfig
	Bulk              BulkConfig
	Centrifugo        CentrifugoConfig
	Auth              AuthConfig
	JWT               JWTConfig
	Log               LogConfig
	VAPID             VAPIDConfig
	Expo              ExpoConfig
	APNs              APNsConfig
	Search            SearchConfig
	Sentry            SentryConfig
	Tenancy           TenancyConfig
	Channels          ChannelsConfig
	Video             VideoConfig
}

// VideoConfig connects the video rooms module (CF-139) to LiveKit. Empty URL
// or keys leave the module unavailable (platform panel shows it as not
// configured).
type VideoConfig struct {
	AppointmentOpenBeforeMinutes int
	AppointmentGraceMinutes      int
	// LiveKitURL is the public signalling URL clients dial (wss://...).
	LiveKitURL string
	// LiveKitAPIURL is the server API address (private network in production).
	LiveKitAPIURL    string
	LiveKitAPIKey    string
	LiveKitAPISecret string
	// EgressS3Endpoint is the platform object store as the egress container
	// reaches it; empty = S3_ENDPOINT.
	EgressS3Endpoint string
}

// ChannelsConfig configures the messaging channel adapters (CF-55..CF-61).
// Meta channels (WhatsApp, Instagram, Messenger) are gated by
// META_CHANNELS_ENABLED until the Meta app review is through.
type ChannelsConfig struct {
	// PublicAPIURL is the API origin providers post webhooks to.
	PublicAPIURL string
	Meta         MetaChannelsConfig
	Email        EmailChannelConfig
	// TelegramEnabled registers the Telegram bot channel.
	TelegramEnabled    bool
	SlackClientID      string
	SlackClientSecret  string
	SlackSigningSecret string
	SlackRedirectURL   string
	// ReconcileInterval / ReconcileWindow drive the Meta reconcile job
	// (0 interval disables it).
	ReconcileInterval time.Duration
	ReconcileWindow   time.Duration
	// HealthInterval drives the channel health check job.
	HealthInterval time.Duration
}

// MetaChannelsConfig is the Meta app (Tech Provider) shared by every
// organization.
type MetaChannelsConfig struct {
	Enabled          bool
	AppID            string
	AppSecret        string
	VerifyToken      string
	GraphVersion     string
	WhatsAppConfigID string
	// Instagram API with Instagram Login uses the Instagram app of the
	// Meta app (its own id and secret).
	InstagramAppID     string
	InstagramAppSecret string
}

// EmailChannelConfig is the Resend receiving / sending setup.
type EmailChannelConfig struct {
	InboundDomain string
	ResendAPIKey  string
	WebhookSecret string
	ResendBaseURL string
}

// TenancyConfig controls organizations (the multi-tenant shell). See
// docs/design/tenant-core.md.
type TenancyConfig struct {
	// Enabled mounts the /v1/tenant routes. ClickFix is always
	// multi-tenant, so it defaults to true; false only for tooling/tests.
	Enabled bool
	// OnboardingAICredits is the one-time AI credit allowance (ai_credits,
	// valid 30 days) a self-serve organization gets for the AI-assisted
	// set-up (CF-140); 0 turns it off.
	OnboardingAICredits int64
	// CacheTTL is how long a resolved organization + membership stays in
	// Redis. Changes invalidate it immediately; the TTL only bounds memory.
	CacheTTL time.Duration
	// InviteTTL is the lifetime of a staff invitation link.
	InviteTTL time.Duration
}

// SentryConfig configures error reporting to a Sentry-protocol endpoint.
// An empty DSN disables reporting entirely.
type SentryConfig struct {
	DSN         string
	Environment string
	// Release defaults to the SDK's own detection (SENTRY_RELEASE, build info).
	Release string
	// TracesSampleRate is the share of HTTP requests sent as performance
	// transactions (0 = tracing off).
	TracesSampleRate float64
}

// AuthConfig holds NextAuth adapter integration settings.
type AuthConfig struct {
	AccountDeletionGrace time.Duration
	AdapterSecret        string
	WebAuthnRPID         string
	FrontendURL          string
	// ReviewAccounts maps a lower-cased email to a fixed sign-in code for
	// App Store / Play review (AUTH_REVIEW_ACCOUNTS=email:code,...). Empty = off.
	ReviewAccounts map[string]string
	// AppleNativeClientIDs are accepted id_token audiences for native Apple
	// sign-in (iOS bundle id). GoogleNativeClientIDs are the iOS/Android OAuth
	// client ids; the web client id from OAuth settings is accepted as well.
	AppleNativeClientIDs  []string
	GoogleNativeClientIDs []string
	// Sign in with Apple key (.p8) used to exchange native authorization codes
	// and revoke refresh tokens on account deletion. Optional.
	AppleTeamID     string
	AppleKeyID      string
	ApplePrivateKey string
	// QR sign-in location lookup. QRGeoIPDB is an optional MaxMind
	// GeoLite2-City .mmdb path (AUTH_QR_GEOIP_DB). QRTrustGeoHeaders trusts
	// edge geo headers (CF-IPCountry / CF-IPCity / CF-Region) and must only be
	// on when the edge (Cloudflare) overwrites them (AUTH_QR_TRUST_GEO_HEADERS).
	QRGeoIPDB         string
	QRTrustGeoHeaders bool
}

// VAPIDConfig holds Web Push keys (empty = push disabled).
type VAPIDConfig struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

// ExpoConfig holds the Expo Push Service settings (mobile push). The access
// token is optional: only needed with Expo "enhanced push security".
type ExpoConfig struct {
	AccessToken string
}

// APNsConfig holds the APNs token-auth key for Live Activity pushes
// (CF-152). Empty key id, team id or private key = Live Activity push off.
// PrivateKey is the .p8 PEM, base64-encoded or raw.
type APNsConfig struct {
	KeyID      string
	TeamID     string
	PrivateKey string
	Topic      string
}

// JWTConfig holds access JWT and opaque refresh token settings.
type JWTConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
}

type AppConfig struct {
	Name string
	Env  string
}

// EncryptionConfig holds at-rest secret encryption material.
type EncryptionConfig struct {
	Key string
}

type HTTPConfig struct {
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

type DBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
	MaxConns int32
}

func (c DBConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User,
		c.Password,
		c.Host,
		c.Port,
		c.Name,
		c.SSLMode,
	)
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// AttachmentConfig holds the default organization attachment limits. Plan
// based limits (entitlements) replace them through the usecase LimitsProvider.
type AttachmentConfig struct {
	MaxFileBytes  int64         // per file
	OrgQuotaBytes int64         // total live bytes per organization; 0 = unlimited
	SignedURLTTL  time.Duration // default lifetime of signed download URLs
}

type StorageConfig struct {
	Driver    string // s3 (SeaweedFS or any S3-compatible store)
	LocalPath string
	MaxBytes  int64
	S3        ObjectStoreConfig
}

// ObjectStoreConfig holds S3-compatible credentials (SeaweedFS or S3).
// Objects are never served from a public bucket URL; the API streams them.
type ObjectStoreConfig struct {
	Endpoint     string
	Region       string
	Bucket       string
	AccessKey    string
	SecretKey    string
	UsePathStyle bool
}

type CORSConfig struct {
	AllowedOrigins []string
}

// SMTPConfig configures outbound email (MailHog in development).
type SMTPConfig struct {
	Driver   string
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
	// TLSMode is "starttls" (default: plain connect, upgrade when offered)
	// or "implicit" (TLS from the first byte, SMTPS on 465).
	TLSMode string
	// FallbackPort, when set (usually 465), is tried with implicit TLS
	// after the primary port cannot be reached (egress on 587 blocked).
	FallbackPort int
	// DialTimeout bounds the TCP connect; SendTimeout the whole session.
	DialTimeout time.Duration
	SendTimeout time.Duration
}

type LogConfig struct {
	Level  string
	Format string
}

// QueueConfig controls Asynq background jobs.
type QueueConfig struct {
	Enabled          bool
	Concurrency      int
	WorkerInProcess  bool
	Queues           string
	SchedulerEnabled bool
}

// BulkConfig controls bulk action engine thresholds.
type BulkConfig struct {
	SyncMax       int
	RollbackHours int
}

// CentrifugoConfig controls realtime pub/sub via Centrifugo.
type CentrifugoConfig struct {
	Enabled   bool
	APIURL    string
	APIKey    string
	TokenHMAC string
	WSURL     string
	TokenTTL  time.Duration
}

// SearchConfig controls Meilisearch-backed command palette indexing.
type SearchConfig struct {
	Enabled     bool
	Driver      string
	MeiliHost   string
	MeiliKey    string
	IndexPrefix string
}

// Load reads optional .env then environment variables into Config.
func Load() (Config, error) {
	_ = godotenv.Load("../.env", ".env")

	frontendURL := getEnv("PUBLIC_FRONTEND_URL", "http://localhost:3000")

	cfg := Config{
		SSOTrustedProxies: strings.FieldsFunc(getEnv("SSO_TRUSTED_PROXY_CIDRS", ""), func(r rune) bool { return r == ',' || r == ' ' }),
		App: AppConfig{
			Name: getEnv("APP_NAME", "api"),
			Env:  getEnv("APP_ENV", "development"),
		},
		Encryption: EncryptionConfig{
			Key: getEnv("APP_ENCRYPTION_KEY", defaultEncryptionKey),
		},
		HTTP: HTTPConfig{
			Addr:         getEnv("APP_HTTP_ADDR", ":8080"),
			ReadTimeout:  getDuration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout: getDuration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:  getDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		},
		DB: DBConfig{
			Host:     getEnv("DB_HOST", "127.0.0.1"),
			Port:     getInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "app"),
			Password: getEnv("DB_PASSWORD", "change_me"),
			Name:     getEnv("DB_NAME", "app"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
			MaxConns: safeconv.ToInt32(getInt("DB_MAX_CONNS", 20)),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "127.0.0.1:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getInt("REDIS_DB", 0),
		},
		Storage: loadStorageConfig(),
		Attachment: AttachmentConfig{
			MaxFileBytes:  getInt64("ATTACHMENT_MAX_FILE_BYTES", 25<<20),
			OrgQuotaBytes: getInt64("ATTACHMENT_ORG_QUOTA_BYTES", 5<<30),
			SignedURLTTL:  getDuration("ATTACHMENT_SIGNED_URL_TTL", 5*time.Minute),
		},
		CORS: CORSConfig{
			AllowedOrigins: splitCSV(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
		},
		SMTP: SMTPConfig{
			Driver:       strings.ToLower(getEnv("MAIL_DRIVER", "smtp")),
			Host:         getEnv("SMTP_HOST", "127.0.0.1"),
			Port:         getInt("SMTP_PORT", 1025),
			Username:     getEnv("SMTP_USERNAME", ""),
			Password:     getEnv("SMTP_PASSWORD", ""),
			From:         firstNonEmpty(getEnv("MAIL_FROM", ""), getEnv("SMTP_FROM", "noreply@localhost")),
			FromName:     getEnv("MAIL_FROM_NAME", "ClickFix"),
			TLSMode:      strings.ToLower(getEnv("SMTP_TLS_MODE", "starttls")),
			FallbackPort: getInt("SMTP_FALLBACK_PORT", 0),
			DialTimeout:  getDuration("SMTP_DIAL_TIMEOUT", 10*time.Second),
			SendTimeout:  getDuration("SMTP_SEND_TIMEOUT", 30*time.Second),
		},
		Queue: QueueConfig{
			Enabled:          getBool("QUEUE_ENABLED", true),
			Queues:           getEnv("QUEUE_QUEUES", ""),
			SchedulerEnabled: getBool("QUEUE_SCHEDULER_ENABLED", true),
			Concurrency:      getInt("QUEUE_CONCURRENCY", 10),
			WorkerInProcess:  getBool("QUEUE_WORKER_INPROCESS", true),
		},
		Bulk: BulkConfig{
			SyncMax:       getInt("BULK_SYNC_MAX", 50),
			RollbackHours: getInt("BULK_ROLLBACK_HOURS", 24),
		},
		Centrifugo: CentrifugoConfig{
			Enabled:   getBool("CENTRIFUGO_ENABLED", true),
			APIURL:    getEnv("CENTRIFUGO_API_URL", "http://127.0.0.1:8000"),
			APIKey:    getEnv("CENTRIFUGO_API_KEY", defaultCentrifugoAPIKey),
			TokenHMAC: getEnv("CENTRIFUGO_TOKEN_HMAC_SECRET", defaultCentrifugoTokenHMAC),
			WSURL:     getEnv("CENTRIFUGO_WS_URL", "ws://127.0.0.1:8000/connection/websocket"),
			TokenTTL:  getDuration("CENTRIFUGO_TOKEN_TTL", time.Hour),
		},
		Auth: AuthConfig{
			AdapterSecret:        getEnv("AUTH_ADAPTER_SECRET", defaultAdapterSecret),
			WebAuthnRPID:         getEnv("AUTH_WEBAUTHN_RP_ID", hostOf(frontendURL, "localhost")),
			FrontendURL:          frontendURL,
			AccountDeletionGrace: getDuration("AUTH_ACCOUNT_DELETION_GRACE", 7*24*time.Hour),

			ReviewAccounts:        parseReviewAccounts(getEnv("AUTH_REVIEW_ACCOUNTS", "")),
			AppleNativeClientIDs:  splitCSV(getEnv("AUTH_APPLE_NATIVE_CLIENT_IDS", "")),
			GoogleNativeClientIDs: splitCSV(getEnv("AUTH_GOOGLE_NATIVE_CLIENT_IDS", "")),
			AppleTeamID:           getEnv("AUTH_APPLE_TEAM_ID", ""),
			AppleKeyID:            getEnv("AUTH_APPLE_KEY_ID", ""),
			ApplePrivateKey:       getEnv("AUTH_APPLE_PRIVATE_KEY", ""),
			QRGeoIPDB:             getEnv("AUTH_QR_GEOIP_DB", ""),
			QRTrustGeoHeaders:     getBool("AUTH_QR_TRUST_GEO_HEADERS", false),
		},
		JWT: JWTConfig{
			AccessSecret:  getEnv("JWT_ACCESS_SECRET", "app-dev-access-secret-change-me-32b"),
			RefreshSecret: getEnv("JWT_REFRESH_SECRET", "app-dev-refresh-secret-change-me-32b"),
			AccessTTL:     getDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTTL:    getDuration("JWT_REFRESH_TTL", 168*time.Hour),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "debug"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
		VAPID: VAPIDConfig{
			PublicKey:  getEnv("VAPID_PUBLIC_KEY", ""),
			PrivateKey: getEnv("VAPID_PRIVATE_KEY", ""),
			Subject:    getEnv("VAPID_SUBJECT", "mailto:destek@getclickfix.com"),
		},
		Expo: ExpoConfig{
			AccessToken: getEnv("EXPO_ACCESS_TOKEN", ""),
		},
		APNs: APNsConfig{
			KeyID:      getEnv("APNS_KEY_ID", ""),
			TeamID:     getEnv("APNS_TEAM_ID", ""),
			PrivateKey: getEnv("APNS_PRIVATE_KEY", ""),
			Topic:      getEnv("APNS_TOPIC", "com.getclickfix.app"),
		},
		Search: SearchConfig{
			Enabled:     getBool("SEARCH_ENABLED", true),
			Driver:      strings.ToLower(getEnv("SEARCH_DRIVER", "meilisearch")),
			MeiliHost:   getEnv("MEILI_HOST", "http://127.0.0.1:7700"),
			MeiliKey:    getEnv("MEILI_MASTER_KEY", "app-dev-meili-master-key-change-me"),
			IndexPrefix: getEnv("MEILI_INDEX_PREFIX", "app"),
		},
		Sentry: SentryConfig{
			DSN:              strings.TrimSpace(getEnv("SENTRY_DSN", "")),
			Environment:      getEnv("SENTRY_ENVIRONMENT", getEnv("APP_ENV", "development")),
			Release:          getEnv("SENTRY_RELEASE", ""),
			TracesSampleRate: clampRate(getFloat("SENTRY_TRACES_SAMPLE_RATE", 0)),
		},
		Tenancy: TenancyConfig{
			Enabled:             getBool("TENANCY_ENABLED", true),
			OnboardingAICredits: getInt64("ONBOARDING_AI_CREDITS", 40),
			CacheTTL:            getDuration("TENANCY_CACHE_TTL", 60*time.Second),
			InviteTTL:           getDuration("TENANCY_INVITE_TTL", 7*24*time.Hour),
		},
		Video: VideoConfig{
			AppointmentOpenBeforeMinutes: getInt("VIDEO_APPOINTMENT_OPEN_BEFORE_MINUTES", 10),
			AppointmentGraceMinutes:      getInt("VIDEO_APPOINTMENT_GRACE_MINUTES", 15),
			LiveKitURL:                   getEnv("LIVEKIT_URL", ""),
			LiveKitAPIURL:                getEnv("LIVEKIT_API_URL", ""),
			LiveKitAPIKey:                getEnv("LIVEKIT_API_KEY", ""),
			LiveKitAPISecret:             getEnv("LIVEKIT_API_SECRET", ""),
			EgressS3Endpoint:             getEnv("VIDEO_EGRESS_S3_ENDPOINT", ""),
		},
		Channels: ChannelsConfig{
			PublicAPIURL: strings.TrimRight(getEnv("PUBLIC_API_URL", ""), "/"),
			Meta: MetaChannelsConfig{
				Enabled:            getBool("META_CHANNELS_ENABLED", false),
				AppID:              getEnv("META_APP_ID", ""),
				AppSecret:          getEnv("META_APP_SECRET", ""),
				VerifyToken:        getEnv("META_WEBHOOK_VERIFY_TOKEN", ""),
				GraphVersion:       getEnv("META_GRAPH_VERSION", ""),
				WhatsAppConfigID:   getEnv("META_WHATSAPP_CONFIG_ID", ""),
				InstagramAppID:     getEnv("META_INSTAGRAM_APP_ID", ""),
				InstagramAppSecret: getEnv("META_INSTAGRAM_APP_SECRET", ""),
			},
			Email: EmailChannelConfig{
				InboundDomain: getEnv("EMAIL_INBOUND_DOMAIN", ""),
				ResendAPIKey:  getEnv("RESEND_API_KEY", ""),
				WebhookSecret: getEnv("RESEND_WEBHOOK_SECRET", ""),
				ResendBaseURL: getEnv("RESEND_BASE_URL", ""),
			},
			TelegramEnabled:    getBool("TELEGRAM_CHANNEL_ENABLED", true),
			SlackClientID:      getEnv("SLACK_CHANNEL_CLIENT_ID", ""),
			SlackClientSecret:  getEnv("SLACK_CHANNEL_CLIENT_SECRET", ""),
			SlackSigningSecret: getEnv("SLACK_CHANNEL_SIGNING_SECRET", ""),
			SlackRedirectURL:   getEnv("SLACK_CHANNEL_REDIRECT_URL", ""),
			ReconcileInterval:  getDuration("META_RECONCILE_INTERVAL", time.Minute),
			ReconcileWindow:    getDuration("META_RECONCILE_WINDOW", 3*time.Hour),
			HealthInterval:     getDuration("CHANNEL_HEALTH_INTERVAL", 15*time.Minute),
		},
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Public development/test fallbacks. validate() rejects them in other environments.
const (
	defaultEncryptionKey       = "app-dev-encryption-key-32bytes!!"      // #nosec G101 -- public development/test placeholder; validate rejects it in other environments
	defaultAdapterSecret       = "app-dev-auth-adapter-secret-change-me" // #nosec G101 -- public development/test placeholder; validate rejects it in other environments
	defaultCentrifugoAPIKey    = "app-centrifugo-api-key-change-me"      // #nosec G101 -- public development/test placeholder; validate rejects it in other environments
	defaultCentrifugoTokenHMAC = "app-centrifugo-token-hmac-change-me"   // #nosec G101 -- public development/test HMAC placeholder; validate rejects it in other environments
)

func (c Config) validate() error {
	if c.App.Name == "" {
		return fmt.Errorf("config: APP_NAME is required")
	}
	if c.HTTP.Addr == "" {
		return fmt.Errorf("config: APP_HTTP_ADDR is required")
	}
	if c.DB.Host == "" || c.DB.User == "" || c.DB.Name == "" {
		return fmt.Errorf("config: database settings are incomplete")
	}
	if c.DB.MaxConns <= 0 {
		return fmt.Errorf("config: DB_MAX_CONNS must be positive")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("config: REDIS_ADDR is required")
	}
	if _, err := ParseQueueQueues(c.Queue.Queues); err != nil {
		return err
	}
	if value, ok := os.LookupEnv("QUEUE_SCHEDULER_ENABLED"); ok && value != "" {
		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("config: QUEUE_SCHEDULER_ENABLED must be a boolean")
		}
	}
	if c.Queue.Concurrency <= 0 {
		return fmt.Errorf("config: QUEUE_CONCURRENCY must be positive")
	}
	if m := c.SMTP.TLSMode; m != "" && m != "starttls" && m != "implicit" {
		return fmt.Errorf("config: SMTP_TLS_MODE must be starttls or implicit")
	}
	if c.SMTP.FallbackPort < 0 || c.SMTP.FallbackPort > 65535 {
		return fmt.Errorf("config: SMTP_FALLBACK_PORT must be a TCP port")
	}
	if c.JWT.AccessTTL <= 0 || c.JWT.RefreshTTL <= 0 {
		return fmt.Errorf("config: JWT TTLs must be positive")
	}
	if c.App.Env != "development" && c.App.Env != "test" {
		if c.JWT.AccessSecret == "" || c.JWT.RefreshSecret == "" {
			return fmt.Errorf("config: JWT_ACCESS_SECRET and JWT_REFRESH_SECRET are required outside development")
		}
		if c.JWT.AccessSecret == "app-dev-access-secret-change-me-32b" ||
			c.JWT.RefreshSecret == "app-dev-refresh-secret-change-me-32b" {
			return fmt.Errorf("config: replace default JWT secrets outside development")
		}
		if len(c.JWT.AccessSecret) < 32 {
			return fmt.Errorf("config: JWT_ACCESS_SECRET must be at least 32 characters outside development")
		}
		// Fallbacks below are public (.env.example / source); refuse them so
		// production never encrypts secrets or signs tokens with known keys.
		if c.Encryption.Key == "" || c.Encryption.Key == defaultEncryptionKey {
			return fmt.Errorf("config: set a unique APP_ENCRYPTION_KEY outside development")
		}
		if c.Auth.AdapterSecret == defaultAdapterSecret {
			return fmt.Errorf("config: replace default AUTH_ADAPTER_SECRET outside development")
		}
		if c.Centrifugo.Enabled && (c.Centrifugo.TokenHMAC == defaultCentrifugoTokenHMAC ||
			c.Centrifugo.APIKey == defaultCentrifugoAPIKey) {
			return fmt.Errorf("config: replace default CENTRIFUGO_TOKEN_HMAC_SECRET / CENTRIFUGO_API_KEY outside development")
		}
	}
	if c.JWT.AccessSecret == "" {
		return fmt.Errorf("config: JWT_ACCESS_SECRET is required")
	}
	if c.Search.Enabled {
		if c.Search.MeiliHost == "" {
			return fmt.Errorf("config: MEILI_HOST is required when SEARCH_ENABLED=true")
		}
		if c.Search.IndexPrefix == "" {
			return fmt.Errorf("config: MEILI_INDEX_PREFIX is required when SEARCH_ENABLED=true")
		}
		if c.App.Env != "development" && c.App.Env != "test" {
			if c.Search.MeiliKey == "" {
				return fmt.Errorf("config: MEILI_MASTER_KEY is required when SEARCH_ENABLED=true")
			}
			if c.Search.MeiliKey == "app-dev-meili-master-key-change-me" {
				return fmt.Errorf("config: replace default MEILI_MASTER_KEY outside development")
			}
		}
	}
	return nil
}

func loadStorageConfig() StorageConfig {
	return StorageConfig{
		Driver:    strings.ToLower(getEnv("STORAGE_DRIVER", "s3")),
		LocalPath: getEnv("STORAGE_LOCAL_PATH", "./storage/objects"),
		MaxBytes:  getInt64("STORAGE_MAX_BYTES", 10<<20),
		S3: ObjectStoreConfig{
			Endpoint:     getEnv("S3_ENDPOINT", "http://127.0.0.1:9000"),
			Region:       getEnv("S3_REGION", "us-east-1"),
			Bucket:       getEnv("S3_BUCKET", "app"),
			AccessKey:    getEnv("S3_ACCESS_KEY", "s3admin"),
			SecretKey:    getEnv("S3_SECRET_KEY", "s3admin-secret"),
			UsePathStyle: getBool("S3_USE_PATH_STYLE", true),
		},
	}
}

// hostOf returns the hostname of rawURL (no port), or fallback when it has none.
func hostOf(rawURL, fallback string) string {
	if u, err := url.Parse(strings.TrimSpace(rawURL)); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func getBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return fallback
	}
	return f
}

func clampRate(v float64) float64 {
	if v != v || v < 0 { // NaN or negative
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func getDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseReviewAccounts parses "a@x.com:123456,b@y.com:654321". Entries without
// an email or a code of at least 6 characters are ignored.
func parseReviewAccounts(raw string) map[string]string {
	out := map[string]string{}
	for _, item := range splitCSV(raw) {
		email, code, ok := strings.Cut(item, ":")
		email = strings.ToLower(strings.TrimSpace(email))
		code = strings.TrimSpace(code)
		if !ok || email == "" || !strings.Contains(email, "@") || len(code) < 6 {
			continue
		}
		out[email] = code
	}
	return out
}
