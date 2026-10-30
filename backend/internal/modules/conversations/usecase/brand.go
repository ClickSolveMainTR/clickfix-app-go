package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/email"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// applyEmailBrand uses the conversation's original brand, independent of later channel changes.
// Sender verification still belongs to the connected email transport/provider.
func (s *Service) applyEmailBrand(ctx context.Context, conv db.Conversation, inst *channels.Instance, out *channels.OutboundMessage) error {
	if inst.Type != channels.TypeEmail || !conv.BrandID.Valid {
		return nil
	}
	b, err := s.store.Q().GetBrand(ctx, db.GetBrandParams{OrganizationID: conv.OrganizationID, Uuid: uuid.UUID(conv.BrandID.Bytes)})
	if err != nil {
		return err
	}
	creds := channels.Credentials{}
	for k, v := range inst.Credentials {
		creds[k] = v
	}
	inst.Credentials = creds
	if b.SupportEmail != "" {
		// Resend credentials are platform-wide: preserve the channel wizard's org-domain guard.
		provider := creds[email.FieldProvider]
		if provider == "" || provider == email.ProviderResend {
			at := strings.LastIndexByte(b.SupportEmail, '@')
			if at < 0 {
				return fmt.Errorf("%w: brand sender", ErrInvalid)
			}
			if _, err := s.store.Q().GetEmailDomain(ctx, db.GetEmailDomainParams{OrganizationID: conv.OrganizationID, Domain: strings.ToLower(b.SupportEmail[at+1:])}); err != nil {
				if !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
				return fmt.Errorf("%w: brand sender domain must be registered by this organization", ErrInvalid)
			}
		}
		creds[email.FieldFromAddress] = b.SupportEmail
	}
	if b.SenderName != "" {
		creds[email.FieldFromName] = b.SenderName
	}
	settings := map[string]any{}
	for k, v := range inst.Settings {
		settings[k] = v
	}
	inst.Settings = settings
	if b.ReplyTo != "" {
		inst.Settings["brand_reply_to"] = b.ReplyTo
	}
	// The optional reply wrapper only recognizes these literal placeholders; no HTML/template execution.
	t, err := s.store.Q().GetBrandEmailTemplate(ctx, db.GetBrandEmailTemplateParams{OrganizationID: conv.OrganizationID, BrandID: uuid.UUID(conv.BrandID.Bytes), Code: "reply"})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil {
		replace := strings.NewReplacer("{{content}}", out.Text, "{{brand_name}}", b.Name, "{{subject}}", out.Subject)
		if t.Subject != "" {
			out.Subject = replace.Replace(t.Subject)
			if out.Attributes == nil {
				out.Attributes = map[string]any{}
			}
			out.Attributes[email.AttrNewThread] = true
		}
		out.Text = replace.Replace(t.Body)
	}
	if b.Signature != "" {
		out.Text += "\n\n" + b.Signature
	}
	return nil
}
