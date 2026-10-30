package events

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const (
	BillingBankStatementImported = "billing.bank_statement_imported"
	BillingBankStatementReviewed = "billing.bank_statement_reviewed"
	AITriageCompleted            = "ai.triage_completed"
	AIReplySuggested             = "ai.reply_suggested"
	AIHandoff                    = "ai.handoff"

	BillingInvoiceCreated      = "billing.invoice_created"
	BillingInvoiceIssued       = "billing.invoice_issued"
	BillingCreditNoteCreated   = "billing.credit_note_created"
	BillingStatementOrphan     = "billing.statement_orphan"
	BillingCardExpiring        = "billing.card_expiring"
	BillingWalletChanged       = "billing.wallet_changed"
	BillingWalletLow           = "billing.wallet_low"
	BillingCouponAttached      = "billing.coupon_attached"
	BillingReferralAwarded     = "billing.referral_awarded"
	BillingPlanChangeScheduled = "billing.plan_change_scheduled"
	BillingPlanChangeAwaiting  = "billing.plan_change_awaiting_payment"
	BillingPlanChangeApplied   = "billing.plan_change_applied"
)

// Billing lifecycle and entitlement events.
const (
	PaymentSucceeded              = "payment.succeeded"
	PaymentFailed                 = "payment.failed"
	PaymentExpired                = "payment.expired"
	BillingRenewalDue             = "billing.renewal_due"
	BillingRenewalReminder        = "billing.renewal_reminder"
	BillingPaymentRequired        = "billing.payment_required"
	BillingTransferReported       = "billing.transfer_reported"
	BillingTransferApproved       = "billing.transfer_approved"
	BillingTransferRejected       = "billing.transfer_rejected"
	BillingReconciliationMismatch = "billing.reconciliation_mismatch"
	BillingRefundSucceeded        = "billing.refund_succeeded"
	BillingRefundFailed           = "billing.refund_failed"
	BillingRefundUnknown          = "billing.refund_unknown"
	BillingUsageRecorded          = "billing.usage_recorded"
	BillingUsageClosed            = "billing.usage_closed"
	BillingUsageDenied            = "billing.usage_denied"
	BillingTrialReminder          = "billing.trial_reminder"
	BillingPlanPublished          = "billing.plan_published"
	SubscriptionPendingPayment    = "subscription.pending_payment"
	SubscriptionTrialing          = "subscription.trialing"
	SubscriptionActive            = "subscription.active"
	SubscriptionPastDue           = "subscription.past_due"
	SubscriptionGrace             = "subscription.grace"
	SubscriptionSuspended         = "subscription.suspended"
	SubscriptionPaused            = "subscription.paused"
	SubscriptionCanceled          = "subscription.canceled"
	SubscriptionExpired           = "subscription.expired"
	SubscriptionRenewed           = "subscription.renewed"
	SubscriptionPlanScheduled     = "subscription.plan_scheduled"
	SubscriptionCancelScheduled   = "subscription.cancel_scheduled"
	EntitlementsQuotaWarning      = "entitlements.quota_warning"
	EntitlementsOverrideChanged   = "entitlements.override_changed"
)

// NamingStandard is the canonical event name pattern: {module}.{action}.
const NamingStandard = `{module}.{action}` // e.g. customers.created, auth.password_reset

// Customer domain events (ADR catalog v1).
// Archive is the archived flag → customers.flag_added / flag_removed (no customers.archived).
const (
	CustomersCreated                 = "customers.created"
	CustomersUpdated                 = "customers.updated"
	CustomersDeleted                 = "customers.deleted"
	CustomersRestored                = "customers.restored"
	CustomersStatusChanged           = "customers.status_changed"
	CustomersContactAdded            = "customers.contact_added"
	CustomersContactVerified         = "customers.contact_verified"
	CustomersContactRemoved          = "customers.contact_removed"
	CustomersWorkspaceAdded          = "customers.workspace_added"
	CustomersWorkspaceRemoved        = "customers.workspace_removed"
	CustomersAssigned                = "customers.assigned"
	CustomersUnassigned              = "customers.unassigned"
	CustomersTransferred             = "customers.transferred"
	CustomersFlagAdded               = "customers.flag_added"
	CustomersFlagRemoved             = "customers.flag_removed"
	CustomersNoteAdded               = "customers.note_added"
	CustomersNoteRemoved             = "customers.note_removed"
	CustomersExternalAccountLinked   = "customers.external_account_linked"
	CustomersExternalAccountUnlinked = "customers.external_account_unlinked"
)

// Inbox domain events (INBOX_DOMAIN_ADR catalog), inherited from the boilerplate.
// New ClickFix code publishes the typed singular names in domain.go
// (conversation.created, message.received, ...).
const (
	ConversationsCreated        = "conversations.created"
	ConversationsUpdated        = "conversations.updated"
	ConversationsAssigned       = "conversations.assigned"
	ConversationsResolved       = "conversations.resolved"
	ConversationsReopened       = "conversations.reopened"
	MessagesReceived            = "messages.received"
	MessagesSent                = "messages.sent"
	MessagesStatusChanged       = "messages.status_changed"
	ConversationsTyping         = "conversations.typing"
	ChannelAccountsConnected    = "channel_accounts.connected"
	ChannelAccountsDisconnected = "channel_accounts.disconnected"
	ChannelAccountsError        = "channel_accounts.error"
)

// Commerce domain events (COMMERCE_DOMAIN_ADR catalog).
const (
	CommerceAppsUpdated                 = "commerce.apps.updated"
	CommerceConnectionsCreated          = "commerce.connections.created"
	CommerceConnectionsUpdated          = "commerce.connections.updated"
	CommerceConnectionsDisconnected     = "commerce.connections.disconnected"
	CommerceConnectionsWorkspaceAdded   = "commerce.connections.workspace_added"
	CommerceConnectionsWorkspaceRemoved = "commerce.connections.workspace_removed"
	CommerceOrdersUpserted              = "commerce.orders.upserted"
	CommerceOrdersLinkedToCustomer      = "commerce.orders.linked_to_customer"
	CommerceRefundsUpdated              = "commerce.refunds.updated"
	CommerceCategoriesUpserted          = "commerce.categories.upserted"
	CommerceCategoriesDeleted           = "commerce.categories.deleted"
	CommerceProductsUpserted            = "commerce.products.upserted"
	CommerceProductsDeleted             = "commerce.products.deleted"
	CommerceCatalogSyncCompleted        = "commerce.catalog.sync_completed"
	CommerceCatalogSyncFailed           = "commerce.catalog.sync_failed"
)

// AI domain events (AI_DOMAIN_ADR catalog). Names only in 022; publish/handlers in 023+.
const (
	AIProfilesCreated           = "ai.profiles.created"
	AIProfilesUpdated           = "ai.profiles.updated"
	AIProfilesDeleted           = "ai.profiles.deleted"
	AIProfilesWorkspaceAdded    = "ai.profiles.workspace_added"
	AIProfilesWorkspaceRemoved  = "ai.profiles.workspace_removed"
	AIProfilesChannelAttached   = "ai.profiles.channel_attached"
	AIProfilesChannelDetached   = "ai.profiles.channel_detached"
	AICredentialsCreated        = "ai.credentials.created"
	AICredentialsUpdated        = "ai.credentials.updated"
	AICredentialsDeleted        = "ai.credentials.deleted"
	KnowledgeArticlePublished   = "knowledge.article_published"
	KnowledgeArticleUnpublished = "knowledge.article_unpublished"
	AIRunCompleted              = "ai.run_completed"
	AIPipelineCompleted         = "ai.pipeline.completed"
	AIPipelineEscalated         = "ai.pipeline.escalated"
	AIPipelineGuardFailed       = "ai.pipeline.guard_failed"
	AIDraftCreated              = "ai.draft.created"
	AIConversationTakeover      = "ai.conversation.takeover"
	AIConversationReleased      = "ai.conversation.released"
)

// Reports domain events (REPORTS_DOMAIN_ADR).
const (
	ReportsDefinitionsCreated = "reports.definitions.created"
	ReportsDashboardsCreated  = "reports.dashboards.created"
	ReportsExportCompleted    = "reports.export.completed"
)

// Tickets domain events (TICKETS_DOMAIN_ADR catalog), inherited from the boilerplate.
// New ClickFix code publishes ticket.* / sla.* from domain.go.
const (
	TicketsCreated                      = "tickets.created"
	TicketsUpdated                      = "tickets.updated"
	TicketsStatusChanged                = "tickets.status_changed"
	TicketsDepartmentsCreated           = "tickets.departments.created"
	TicketsDepartmentsUpdated           = "tickets.departments.updated"
	TicketsDepartmentsDeleted           = "tickets.departments.deleted"
	TicketsDepartmentsMemberAdded       = "tickets.departments.member_added"
	TicketsDepartmentsMemberRemoved     = "tickets.departments.member_removed"
	TicketsCategoriesCreated            = "tickets.categories.created"
	TicketsCategoriesUpdated            = "tickets.categories.updated"
	TicketsCategoriesDeleted            = "tickets.categories.deleted"
	TicketsRulesTriggered               = "tickets.rules.triggered"
	TicketsRulesActionApplied           = "tickets.rules.action_applied"
	TicketsRemindersCreated             = "tickets.reminders.created"
	TicketsRemindersSent                = "tickets.reminders.sent"
	TicketsRemindersEscalated           = "tickets.reminders.escalated"
	TicketsSLAWarning                   = "tickets.sla.warning"
	TicketsSLABreached                  = "tickets.sla.breached"
	TicketsAssignmentsAssigned          = "tickets.assignments.assigned"
	TicketsAssignmentsReassigned        = "tickets.assignments.reassigned"
	TicketsAssignmentsUnassigned        = "tickets.assignments.unassigned"
	TicketsMessagesCreated              = "tickets.messages.created"
	TicketsConversationsTouched         = "tickets.conversations.touched"
	TicketsAttachmentsCreated           = "tickets.attachments.created"
	TicketsAttachmentsRead              = "tickets.attachments.read"
	TicketsAttachmentsAIDenied          = "tickets.attachments.ai_read_denied"
	TicketsEvaluationsLinkGenerated     = "tickets.evaluations.link_generated"
	TicketsEvaluationsFeedbackSubmitted = "tickets.evaluations.feedback_submitted"
	TicketsEvaluationsFinalized         = "tickets.evaluations.finalized"
	TicketsAISuggestionCreated          = "tickets.ai.suggestion_created"
	TicketsAIAssistEscalated            = "tickets.ai.assist_escalated"
	TicketsAIAssistGuardFailed          = "tickets.ai.assist_guard_failed"
)

// Auth / tenant notification source events (existing Notification Center templates).
const (
	AuthWelcome            = "auth.welcome"
	AuthEmailVerification  = "auth.email_verification"
	AuthPasswordReset      = "auth.password_reset"
	AuthProfileUpdated     = "auth.profile_updated"
	TenantMemberAdded      = "tenant.member_added"
	NotificationsTest      = "notifications.test"
	NotificationsQueued    = "notifications.queued"
	NotificationsSent      = "notifications.sent"
	NotificationsFailed    = "notifications.failed"
	NotificationsRead      = "notifications.read"
	NotificationsCancelled = "notifications.cancelled"
)

// ValidateEventName checks the naming standard without requiring catalog membership.
func ValidateEventName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("events: empty name")
	}
	if strings.Contains(name, " ") || strings.Contains(name, "/") {
		return fmt.Errorf("events: name %q must not contain spaces or slashes", name)
	}
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return fmt.Errorf("events: name %q must match %s", name, NamingStandard)
	}
	for _, p := range parts {
		if p == "" {
			return fmt.Errorf("events: name %q has empty segment", name)
		}
		for _, r := range p {
			if unicode.IsUpper(r) {
				return fmt.Errorf("events: name %q must be lowercase", name)
			}
			if !unicode.IsLower(r) && !unicode.IsDigit(r) && r != '_' {
				return fmt.Errorf("events: name %q has invalid character in segment %q", name, p)
			}
		}
	}
	return nil
}

// AllKnownEvents returns every catalog constant (deduplicated, sorted).
func AllKnownEvents() []string {
	set := map[string]struct{}{}
	for _, n := range catalogConstants() {
		if n == "" {
			continue
		}
		set[n] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// IsKnownEvent reports whether name is in the platform catalog.
func IsKnownEvent(name string) bool {
	_, ok := knownEventSet()[name]
	return ok
}

func knownEventSet() map[string]struct{} {
	set := map[string]struct{}{}
	for _, n := range catalogConstants() {
		set[n] = struct{}{}
	}
	return set
}

func catalogConstants() []string {
	out := append(legacyCatalogConstants(), supportCatalogEvents()...)
	out = append(out, "engineering.created", "engineering.updated", "engineering.linked", "engineering.commented", "engineering.released", "engineering.sync_requested", "engineering.synced", "engineering.escalated", "engineering.error_attached")
	for name := range domainDefinitions {
		out = append(out, name)
	}
	return out
}

func legacyCatalogConstants() []string {
	return []string{
		BillingBankStatementImported, BillingBankStatementReviewed, AITriageCompleted, AIReplySuggested, AIHandoff,

		KnowledgeArticlePublished, KnowledgeArticleUnpublished, AIRunCompleted,
		PaymentSucceeded, PaymentFailed, BillingRenewalDue, BillingRenewalReminder, BillingPaymentRequired, BillingTransferReported, BillingTransferApproved, BillingTransferRejected, BillingReconciliationMismatch, BillingRefundSucceeded, BillingRefundFailed, BillingRefundUnknown,
		BillingUsageRecorded, BillingUsageClosed, BillingUsageDenied, BillingTrialReminder,
		BillingInvoiceCreated, BillingInvoiceIssued, BillingCreditNoteCreated, BillingStatementOrphan, BillingCardExpiring, BillingWalletChanged, BillingWalletLow, BillingCouponAttached, BillingReferralAwarded, BillingPlanChangeScheduled, BillingPlanChangeAwaiting, BillingPlanChangeApplied,
		BillingPlanPublished,
		SubscriptionPendingPayment,
		SubscriptionTrialing,
		SubscriptionActive,
		SubscriptionPastDue,
		SubscriptionGrace,
		SubscriptionSuspended,
		SubscriptionPaused,
		SubscriptionCanceled,
		SubscriptionExpired,
		SubscriptionRenewed,
		SubscriptionPlanScheduled,
		SubscriptionCancelScheduled,
		EntitlementsQuotaWarning,
		EntitlementsOverrideChanged,

		CustomersCreated,
		CustomersUpdated,
		CustomersDeleted,
		CustomersRestored,
		CustomersStatusChanged,
		CustomersContactAdded,
		CustomersContactVerified,
		CustomersContactRemoved,
		CustomersWorkspaceAdded,
		CustomersWorkspaceRemoved,
		CustomersAssigned,
		CustomersUnassigned,
		CustomersTransferred,
		CustomersFlagAdded,
		CustomersFlagRemoved,
		CustomersNoteAdded,
		CustomersNoteRemoved,
		CustomersExternalAccountLinked,
		CustomersExternalAccountUnlinked,
		ConversationsCreated,
		ConversationsUpdated,
		ConversationsAssigned,
		ConversationsResolved,
		ConversationsReopened,
		MessagesReceived,
		MessagesSent,
		MessagesStatusChanged,
		ConversationsTyping,
		ChannelAccountsConnected,
		ChannelAccountsDisconnected,
		ChannelAccountsError,
		CommerceAppsUpdated,
		CommerceConnectionsCreated,
		CommerceConnectionsUpdated,
		CommerceConnectionsDisconnected,
		CommerceConnectionsWorkspaceAdded,
		CommerceConnectionsWorkspaceRemoved,
		CommerceOrdersUpserted,
		CommerceOrdersLinkedToCustomer,
		CommerceRefundsUpdated,
		CommerceCategoriesUpserted,
		CommerceCategoriesDeleted,
		CommerceProductsUpserted,
		CommerceProductsDeleted,
		CommerceCatalogSyncCompleted,
		CommerceCatalogSyncFailed,
		AIProfilesCreated,
		AIProfilesUpdated,
		AIProfilesDeleted,
		AIProfilesWorkspaceAdded,
		AIProfilesWorkspaceRemoved,
		AIProfilesChannelAttached,
		AIProfilesChannelDetached,
		AICredentialsCreated,
		AICredentialsUpdated,
		AICredentialsDeleted,
		AIPipelineCompleted,
		AIPipelineEscalated,
		AIPipelineGuardFailed,
		AIDraftCreated,
		AIConversationTakeover,
		AIConversationReleased,
		ReportsDefinitionsCreated,
		ReportsDashboardsCreated,
		ReportsExportCompleted,
		TicketsCreated,
		TicketsUpdated,
		TicketsStatusChanged,
		TicketsDepartmentsCreated,
		TicketsDepartmentsUpdated,
		TicketsDepartmentsDeleted,
		TicketsDepartmentsMemberAdded,
		TicketsDepartmentsMemberRemoved,
		TicketsCategoriesCreated,
		TicketsCategoriesUpdated,
		TicketsCategoriesDeleted,
		TicketsRulesTriggered,
		TicketsRulesActionApplied,
		TicketsRemindersCreated,
		TicketsRemindersSent,
		TicketsRemindersEscalated,
		TicketsSLAWarning,
		TicketsSLABreached,
		TicketsAssignmentsAssigned,
		TicketsAssignmentsReassigned,
		TicketsAssignmentsUnassigned,
		TicketsMessagesCreated,
		TicketsConversationsTouched,
		TicketsAttachmentsCreated,
		TicketsAttachmentsRead,
		TicketsAttachmentsAIDenied,
		TicketsEvaluationsLinkGenerated,
		TicketsEvaluationsFeedbackSubmitted,
		TicketsEvaluationsFinalized,
		TicketsAISuggestionCreated,
		TicketsAIAssistEscalated,
		TicketsAIAssistGuardFailed,
		AuthWelcome,
		AuthEmailVerification,
		AuthPasswordReset,
		AuthProfileUpdated,
		TenantMemberAdded,
		NotificationsTest,
		NotificationsQueued,
		NotificationsSent,
		NotificationsFailed,
		NotificationsRead,
		NotificationsCancelled,
	}
}
