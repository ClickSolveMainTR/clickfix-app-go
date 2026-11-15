package rbac

// Role slugs (seeded). Workspace access comes from organization
// membership; the old cms_user role was removed in CF-143.
const (
	RoleSuperAdmin = "super_admin"
)

// Permission slugs (seeded).
const (
	PermPlatformPlansRead          = "platform.plans.read"
	PermPlatformPlansWrite         = "platform.plans.write"
	PermPlatformSubscriptionsRead  = "platform.subscriptions.read"
	PermPlatformSubscriptionsWrite = "platform.subscriptions.write"
	PermPlatformEntitlementsRead   = "platform.entitlements.read"
	PermPlatformEntitlementsWrite  = "platform.entitlements.write"
	PermTenantBillingRead          = "tenant.billing.read"
	PermTenantBillingWrite         = "tenant.billing.write"
)

const (
	PermPlatformUsersRead                 = "platform.users.read"
	PermPlatformUsersWrite                = "platform.users.write"
	PermPlatformUsersExport               = "platform.users.export"
	PermPlatformUsersImport               = "platform.users.import"
	PermPlatformUsersBulkDisable          = "platform.users.bulk.disable"
	PermPlatformUsersBulkEnable           = "platform.users.bulk.enable"
	PermPlatformUsersImpersonate          = "platform.users.impersonate"
	PermPlatformRolesRead                 = "platform.roles.read"
	PermPlatformRolesWrite                = "platform.roles.write"
	PermPlatformRolesExport               = "platform.roles.export"
	PermPlatformRolesImport               = "platform.roles.import"
	PermPlatformRolesBulkDelete           = "platform.roles.bulk.delete"
	PermPlatformOrganizationsRead         = "platform.organizations.read"
	PermPlatformOrganizationsWrite        = "platform.organizations.write"
	PermPlatformOrganizationsExport       = "platform.organizations.export"
	PermPlatformOrganizationsBulkActivate = "platform.organizations.bulk.activate"
	PermPlatformOrganizationsBulkSuspend  = "platform.organizations.bulk.suspend"
	PermPlatformBulkRead                  = "platform.bulk.read"
	PermPlatformNotificationsRead         = "platform.notifications.read"
	PermPlatformNotificationsReadAll      = "platform.notifications.read_all"
	PermPlatformNotificationsExport       = "platform.notifications.export"
	PermPlatformSettingsRead              = "platform.settings.read"
	PermPlatformSettingsWrite             = "platform.settings.write"
	PermPlatformActivityRead              = "platform.activity.read"
	PermPlatformImportsRead               = "platform.imports.read"
	PermPlatformExportsRead               = "platform.exports.read"
	PermPlatformStorageRead               = "platform.storage.read"
	PermPlatformStorageWrite              = "platform.storage.write"
	PermPlatformLogsRead                  = "platform.logs.read"
	PermPlatformLogsWrite                 = "platform.logs.write"
	PermPlatformAccessRead                = "platform.access.read"
	PermPlatformAccessWrite               = "platform.access.write"
	PermPlatformIntegrationsGitHubRead    = "platform.integrations.github.read"
	PermPlatformIntegrationsGitHubWrite   = "platform.integrations.github.write"
	PermPlatformIntegrationsGoogleRead    = "platform.integrations.google.read"
	PermPlatformIntegrationsGoogleWrite   = "platform.integrations.google.write"
	PermPlatformIntegrationsFacebookRead  = "platform.integrations.facebook.read"
	PermPlatformIntegrationsFacebookWrite = "platform.integrations.facebook.write"
	PermPlatformIntegrationsAppleRead     = "platform.integrations.apple.read"
	PermPlatformIntegrationsAppleWrite    = "platform.integrations.apple.write"
	PermPlatformAuthSettingsRead          = "platform.auth.settings.read"
	PermPlatformAuthSettingsWrite         = "platform.auth.settings.write"
	PermAuthSession                       = "auth.session"
	PermNotificationsRead                 = "notifications.read"
	PermNotificationsManage               = "notifications.manage"
	PermTenantSettingsRead                = "tenant.settings.read"
	PermTenantSettingsWrite               = "tenant.settings.write"
	PermTenantMembersRead                 = "tenant.members.read"
	PermTenantMembersWrite                = "tenant.members.write"
	PermTenantRolesRead                   = "tenant.roles.read"
	PermTenantRolesWrite                  = "tenant.roles.write"
	PermPlatformSupportAccess             = "platform.support.access"
	PermTenantSupportAccessRead           = "tenant.support_access.read"
	PermTenantSupportAccessManage         = "tenant.support_access.manage"
	PermTenantAuditRead                   = "tenant.audit.read"
	PermTenantAuditExport                 = "tenant.audit.export"
	PermTenantChannelsManage              = "tenant.channels.manage"
	// Helpdesk catalog (CF-4). `.read` = own teams / assignments,
	// `.read_all` = the whole organization (orgctx.Scope.CanSeeAll).
	PermTenantReplaysRead = "tenant.replays.read"
	// PermTenantWidgetsPublish makes web widget settings live (CF-150);
	// editing the draft needs tenant.channels.manage.
	PermTenantWidgetsPublish       = "tenant.widgets.publish"
	PermTenantCobrowseUse          = "tenant.cobrowse.use"
	PermTenantCobrowseControl      = "tenant.cobrowse.control"
	PermTenantConversationsRead    = "tenant.conversations.read"
	PermTenantConversationsReadAll = "tenant.conversations.read_all"
	PermTenantConversationsReply   = "tenant.conversations.reply"
	PermTenantConversationsNote    = "tenant.conversations.note"
	PermTenantConversationsAssign  = "tenant.conversations.assign"
	PermTenantConversationsManage  = "tenant.conversations.manage"
	PermTenantConversationsDelete  = "tenant.conversations.delete"
	PermTenantConversationsExport  = "tenant.conversations.export"
	PermTenantTicketsRead          = "tenant.tickets.read"
	PermTenantTicketsReadAll       = "tenant.tickets.read_all"
	PermTenantTicketsWrite         = "tenant.tickets.write"
	PermTenantTicketsAssign        = "tenant.tickets.assign"
	PermTenantTicketsDelete        = "tenant.tickets.delete"
	PermTenantTicketsExport        = "tenant.tickets.export"
	PermTenantContactsRead         = "tenant.contacts.read"
	PermTenantContactsWrite        = "tenant.contacts.write"
	PermTenantContactsDelete       = "tenant.contacts.delete"
	PermTenantContactsExport       = "tenant.contacts.export"
	PermTenantContactsImport       = "tenant.contacts.import"
	PermTenantChannelsRead         = "tenant.channels.read"
	PermTenantTeamsRead            = "tenant.teams.read"
	PermTenantTeamsWrite           = "tenant.teams.write"
	PermTenantReportsRead          = "tenant.reports.read"
	PermTenantReportsExport        = "tenant.reports.export"
	PermTenantAiUse                = "tenant.ai.use"
	PermTenantAiRead               = "tenant.ai.read"
	PermTenantAiWrite              = "tenant.ai.write"
	PermTenantAutomationsRead      = "tenant.automations.read"
	PermTenantAutomationsWrite     = "tenant.automations.write"
	PermTenantKnowledgeRead        = "tenant.knowledge.read"
	PermTenantKnowledgeWrite       = "tenant.knowledge.write"
	PermTenantKnowledgePublish     = "tenant.knowledge.publish"
	PermTenantMacrosRead           = "tenant.macros.read"
	PermTenantMacrosWrite          = "tenant.macros.write"
	PermTenantSlaRead              = "tenant.sla.read"
	PermTenantSlaWrite             = "tenant.sla.write"
	PermTenantAttachmentsRead      = "tenant.attachments.read"
	PermTenantAttachmentsWrite     = "tenant.attachments.write"
	PermTenantCustomFieldsRead     = "tenant.custom_fields.read"
	PermTenantCustomFieldsManage   = "tenant.custom_fields.manage"
	PermTenantCustomObjectsRead    = "tenant.custom_objects.read"
	PermTenantCustomObjectsWrite   = "tenant.custom_objects.write"
	PermTenantOnboardingManage     = "tenant.onboarding.manage"
)

// IsSystemRole reports whether slug is a protected system role.
func IsSystemRole(slug string) bool {
	return slug == RoleSuperAdmin
}

// Support operations permissions.
const (
	PermTenantFeedbackRead    = "tenant.feedback.read"
	PermTenantFeedbackWrite   = "tenant.feedback.write"
	PermTenantFeedbackManage  = "tenant.feedback.manage"
	PermTenantSchedulesRead   = "tenant.schedules.read"
	PermTenantSchedulesManage = "tenant.schedules.manage"
	// Appointments and calendar (CF-138).
	PermTenantAppointmentsRead   = "tenant.appointments.read"
	PermTenantAppointmentsWrite  = "tenant.appointments.write"
	PermTenantAppointmentsManage = "tenant.appointments.manage"
	PermTenantTeamChatUse        = "tenant.team_chat.use"
	PermTenantQualityRead        = "tenant.quality.read"
	PermTenantQualityManage      = "tenant.quality.manage"
	PermPlatformRevenueRead      = "platform.revenue.read"
	PermPlatformQuotesRead       = "platform.quotes.read"
	PermPlatformQuotesWrite      = "platform.quotes.write"
)

// Video rooms (CF-139).
const (
	PermTenantVideoUse      = "tenant.video.use"
	PermTenantVideoManage   = "tenant.video.manage"
	PermPlatformVideoManage = "platform.video.manage"
)

// Module guides and the ClickFix assistant (CF-141).
const (
	PermTenantAssistantUse          = "tenant.assistant.use"
	PermPlatformAssistantManage     = "platform.assistant.manage"
	PermPlatformGuidesManage        = "platform.guides.manage"
	PermPlatformProductFeedbackRead = "platform.product_feedback.read"
)

// Connected system, "Sistemini bağla" (CF-197).
const (
	PermTenantIntegrationsRead   = "tenant.integrations.read"
	PermTenantIntegrationsManage = "tenant.integrations.manage"
	PermTenantIntegrationsUse    = "tenant.integrations.use"
)

// Category taxonomy and the label store (CF-218).
const (
	PermTenantCategoriesRead   = "tenant.categories.read"
	PermTenantCategoriesManage = "tenant.categories.manage"
	PermTenantCategoriesLabel  = "tenant.categories.label"
	PermTenantCategoriesExport = "tenant.categories.export"
)
