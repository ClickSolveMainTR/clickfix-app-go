package inbox

import (
	"context"
	"crypto/sha256"
	"errors"
	"log/slog"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	channelshandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/handler"
	channelsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/email"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
)

// ChannelsHandler builds the channel endpoints with the connect wizards of
// the registered provider channels.
func ChannelsHandler(cfg config.Config, i *Inbox, rec *activity.Recorder, log *slog.Logger) (*channelshandler.Handler, error) {
	box, err := crypto.NewSecretBox(cfg.Encryption.Key)
	if err != nil {
		return nil, err
	}
	if i.Adapters.Email != nil {
		i.Channels.WithCredentialGuard(emailDomainGuard(i.Channels))
	}
	key := sha256.Sum256([]byte("clickfix/channels/oauth-state\n" + cfg.Encryption.Key))
	return channelshandler.New(i.Channels, i.Receive, rec, log).
		WithProviders(channelshandler.Providers{
			Email: i.Adapters.Email, WhatsApp: i.Adapters.WhatsApp, Instagram: i.Adapters.Instagram,
			Messenger: i.Adapters.Messenger, Slack: i.Adapters.Slack, GoogleChat: i.Adapters.GoogleChat, Box: box,
		}).
		WithOAuthKey(key[:]), nil
}

// emailDomainGuard: sending through ClickFix's Resend account requires the
// from_address domain to be registered by the same organization (the
// account is shared; Resend only knows it is verified).
func emailDomainGuard(svc *channelsusecase.Service) channelsusecase.CredentialGuard {
	return func(ctx context.Context, orgID int64, channelType string, creds channels.Credentials) error {
		if channelType != channels.TypeEmail || creds[email.FieldFromAddress] == "" ||
			(creds[email.FieldProvider] != "" && creds[email.FieldProvider] != email.ProviderResend) {
			return nil
		}
		_, domain, _ := strings.Cut(strings.ToLower(creds[email.FieldFromAddress]), "@")
		owns, err := svc.OwnsEmailDomain(ctx, orgID, domain)
		if err != nil {
			return err
		}
		if !owns {
			return errors.New("from_address: register and verify the domain " + domain + " first")
		}
		return nil
	}
}
