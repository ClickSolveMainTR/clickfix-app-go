package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"strconv"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	notifusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
)

func registerBillingNotice(bus interface{ SubscribeReliable(string, events.Handler) }, q *db.Queries, svc *notifusecase.Service, name, template string) {
	bus.SubscribeReliable(name, func(ctx context.Context, event events.Event) error {
		if name == "subscription.active" {
			from := fmt.Sprint(event.Payload["from"])
			if from != "pending_payment" && from != "trialing" {
				return nil
			}
		}
		var recipients []db.BillingBillingRecipientsRow
		if event.TenantID == nil {
			if event.Name != "billing.statement_orphan" {
				return fmt.Errorf("billing notice: organization missing")
			}
			rows, err := q.BillingPlatformRecipients(ctx)
			if err != nil {
				return err
			}
			for _, row := range rows {
				recipients = append(recipients, db.BillingBillingRecipientsRow(row))
			}
		} else {
			var err error
			recipients, err = q.BillingBillingRecipients(ctx, *event.TenantID)
			if err != nil {
				return err
			}
		}
		payload := make(map[string]any, len(event.Payload)+2)
		vars := map[string]string{"billing_path": "/billing", "days": "", "invoice_uuid": "", "feature": "", "percent": "100"}
		for k, v := range event.Payload {
			payload[k] = v
			vars[k] = fmt.Sprint(v)
		}
		payload["source_event_id"] = event.EventID.String()
		payload["billing_path"] = "/billing"
		selectedTemplate := template
		if name == events.EntitlementsQuotaWarning {
			if percent, e := strconv.Atoi(vars["percent"]); e == nil && percent >= 100 {
				selectedTemplate = "billing.quota_exhausted"
			}
		}
		channels := []string{notifmodel.ChannelEmail, notifmodel.ChannelInapp}
		if name == "billing.invoice_issued" {
			channels = []string{notifmodel.ChannelInapp}
		}
		for _, recipient := range recipients {
			if _, err := svc.Enqueue(ctx, notifmodel.EnqueueInput{TenantID: event.TenantID, UserID: &recipient.ID, Channels: channels, TemplateCode: selectedTemplate, TemplateVars: vars, Language: recipient.Locale, SourceEvent: event.Name, Payload: payload}); err != nil {
				return err
			}
		}
		if name == "billing.invoice_issued" {
			var data struct {
				Emails []string `json:"billing_emails"`
			}
			raw, err := json.Marshal(event.Payload)
			if err != nil {
				return err
			}
			if err = json.Unmarshal(raw, &data); err != nil {
				return err
			}
			if len(data.Emails) > 0 && len(recipients) == 0 {
				return fmt.Errorf("invoice delivery requires an organization owner")
			}
			seen := map[string]bool{}
			for _, email := range data.Emails {
				if seen[email] {
					continue
				}
				seen[email] = true
				emailPayload := make(map[string]any, len(payload))
				for k, v := range payload {
					emailPayload[k] = v
				}
				emailPayload["source_event_id"] = uuid.NewSHA1(event.EventID, []byte("invoice-email:"+email)).String()
				owner := recipients[0]
				if _, err = svc.Enqueue(ctx, notifmodel.EnqueueInput{TenantID: event.TenantID, UserID: &owner.ID, Recipient: &email, Channels: []string{notifmodel.ChannelEmail}, Language: owner.Locale, TemplateCode: template, TemplateVars: vars, SourceEvent: event.Name, Payload: emailPayload}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
func RegisterTrialNotifications(bus interface{ SubscribeReliable(string, events.Handler) }, q *db.Queries, svc *notifusecase.Service) {
	registerBillingNotice(bus, q, svc, "billing.trial_reminder", "billing.trial_ending")
}
func RegisterPaymentNotifications(bus interface{ SubscribeReliable(string, events.Handler) }, q *db.Queries, svc *notifusecase.Service) {
	for name, template := range map[string]string{
		"subscription.active": "billing.welcome", "subscription.trialing": "billing.welcome", "payment.succeeded": "billing.payment_received", "payment.failed": "billing.payment_failed",
		"billing.invoice_issued": "billing.invoice_issued", "billing.card_expiring": "billing.card_expiring", "billing.renewal_reminder": "billing.renewal_reminder", "billing.renewal_due": "billing.transfer_pending", "billing.transfer_reported": "billing.transfer_pending", "billing.transfer_approved": "billing.transfer_approved", "billing.transfer_rejected": "billing.transfer_rejected",
		events.EntitlementsQuotaWarning: "billing.quota_warning", "billing.usage_denied": "billing.quota_exhausted", "subscription.suspended": "billing.suspended", "subscription.canceled": "billing.canceled", events.SubscriptionCancelScheduled: "billing.canceled", "billing.payment_required": "billing.payment_required", "billing.reconciliation_mismatch": "billing.reconciliation_mismatch", "billing.refund_succeeded": "billing.refund_succeeded", "billing.statement_orphan": "billing.reconciliation_mismatch", events.BillingWalletLow: "billing.wallet_low",
	} {
		registerBillingNotice(bus, q, svc, name, template)
	}
}
