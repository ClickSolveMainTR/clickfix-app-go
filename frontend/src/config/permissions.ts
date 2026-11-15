/**
 * Permission catalog aligned with platform RBAC seed permissions.
 */
export const Permission = {
  TenantFeedbackRead: "tenant.feedback.read",
  TenantFeedbackWrite: "tenant.feedback.write",
  TenantFeedbackManage: "tenant.feedback.manage",
  TenantSchedulesRead: "tenant.schedules.read",
  TenantSchedulesManage: "tenant.schedules.manage",
  TenantAppointmentsRead: "tenant.appointments.read",
  TenantAppointmentsWrite: "tenant.appointments.write",
  TenantAppointmentsManage: "tenant.appointments.manage",
  TenantTeamChatUse: "tenant.team_chat.use",
  TenantQualityRead: "tenant.quality.read",
  TenantQualityManage: "tenant.quality.manage",
  PlatformRevenueRead: "platform.revenue.read",
  PlatformQuotesRead: "platform.quotes.read",
  PlatformQuotesWrite: "platform.quotes.write",
  TenantVideoUse: "tenant.video.use",
  TenantVideoManage: "tenant.video.manage",
  PlatformVideoManage: "platform.video.manage",
  TenantAssistantUse: "tenant.assistant.use",
  PlatformAssistantManage: "platform.assistant.manage",
  PlatformGuidesManage: "platform.guides.manage",
  PlatformProductFeedbackRead: "platform.product_feedback.read",

  PlatformPlansRead: "platform.plans.read",
  PlatformPlansWrite: "platform.plans.write",
  PlatformSubscriptionsRead: "platform.subscriptions.read",
  PlatformSubscriptionsWrite: "platform.subscriptions.write",
  PlatformEntitlementsRead: "platform.entitlements.read",
  PlatformEntitlementsWrite: "platform.entitlements.write",
  TenantBillingRead: "tenant.billing.read",
  TenantBillingWrite: "tenant.billing.write",

  PlatformUsersRead: "platform.users.read",
  PlatformUsersWrite: "platform.users.write",
  PlatformUsersExport: "platform.users.export",
  PlatformUsersImport: "platform.users.import",
  PlatformRolesRead: "platform.roles.read",
  PlatformRolesWrite: "platform.roles.write",
  PlatformRolesExport: "platform.roles.export",
  PlatformRolesImport: "platform.roles.import",
  PlatformOrganizationsRead: "platform.organizations.read",
  PlatformOrganizationsWrite: "platform.organizations.write",
  PlatformOrganizationsExport: "platform.organizations.export",
  PlatformOrganizationsBulkActivate: "platform.organizations.bulk.activate",
  PlatformOrganizationsBulkSuspend: "platform.organizations.bulk.suspend",
  PlatformNotificationsRead: "platform.notifications.read",
  PlatformNotificationsReadAll: "platform.notifications.read_all",
  PlatformNotificationsExport: "platform.notifications.export",
  PlatformSettingsRead: "platform.settings.read",
  PlatformSettingsWrite: "platform.settings.write",
  PlatformActivityRead: "platform.activity.read",
  PlatformImportsRead: "platform.imports.read",
  PlatformExportsRead: "platform.exports.read",
  PlatformStorageRead: "platform.storage.read",
  PlatformStorageWrite: "platform.storage.write",
  PlatformLogsRead: "platform.logs.read",
  PlatformLogsWrite: "platform.logs.write",
  PlatformUsersBulkDisable: "platform.users.bulk.disable",
  PlatformUsersBulkEnable: "platform.users.bulk.enable",
  PlatformUsersImpersonate: "platform.users.impersonate",
  PlatformRolesBulkDelete: "platform.roles.bulk.delete",
  PlatformBulkRead: "platform.bulk.read",
  PlatformAccessRead: "platform.access.read",
  PlatformAccessWrite: "platform.access.write",
  PlatformIntegrationsGitHubRead: "platform.integrations.github.read",
  PlatformIntegrationsGitHubWrite: "platform.integrations.github.write",
  PlatformIntegrationsGoogleRead: "platform.integrations.google.read",
  PlatformIntegrationsGoogleWrite: "platform.integrations.google.write",
  PlatformIntegrationsFacebookRead: "platform.integrations.facebook.read",
  PlatformIntegrationsFacebookWrite: "platform.integrations.facebook.write",
  PlatformIntegrationsAppleRead: "platform.integrations.apple.read",
  PlatformIntegrationsAppleWrite: "platform.integrations.apple.write",
  PlatformAuthSettingsRead: "platform.auth.settings.read",
  PlatformAuthSettingsWrite: "platform.auth.settings.write",

  AuthSession: "auth.session",

  NotificationsRead: "notifications.read",
  NotificationsManage: "notifications.manage",

  TenantSettingsRead: "tenant.settings.read",
  TenantSettingsWrite: "tenant.settings.write",
  TenantMembersRead: "tenant.members.read",
  TenantMembersWrite: "tenant.members.write",
  TenantRolesRead: "tenant.roles.read",
  TenantRolesWrite: "tenant.roles.write",
  TenantAttachmentsRead: "tenant.attachments.read",
  TenantAttachmentsWrite: "tenant.attachments.write",
  TenantAuditRead: "tenant.audit.read",
  TenantAuditExport: "tenant.audit.export",
  TenantSupportAccessRead: "tenant.support_access.read",
  TenantSupportAccessManage: "tenant.support_access.manage",
  PlatformSupportAccess: "platform.support.access",
  TenantChannelsManage: "tenant.channels.manage",
  TenantConversationsRead: "tenant.conversations.read",
  TenantConversationsReadAll: "tenant.conversations.read_all",
  TenantConversationsReply: "tenant.conversations.reply",
  TenantConversationsNote: "tenant.conversations.note",
  TenantConversationsAssign: "tenant.conversations.assign",
  TenantReplaysRead: "tenant.replays.read",
  TenantWidgetsPublish: "tenant.widgets.publish",
  TenantCobrowseUse: "tenant.cobrowse.use",
  TenantCobrowseControl: "tenant.cobrowse.control",
  TenantConversationsManage: "tenant.conversations.manage",
  TenantConversationsDelete: "tenant.conversations.delete",
  TenantConversationsExport: "tenant.conversations.export",
  TenantTicketsRead: "tenant.tickets.read",
  TenantTicketsReadAll: "tenant.tickets.read_all",
  TenantTicketsWrite: "tenant.tickets.write",
  TenantTicketsAssign: "tenant.tickets.assign",
  TenantTicketsDelete: "tenant.tickets.delete",
  TenantTicketsExport: "tenant.tickets.export",
  TenantContactsRead: "tenant.contacts.read",
  TenantContactsWrite: "tenant.contacts.write",
  TenantContactsDelete: "tenant.contacts.delete",
  TenantContactsExport: "tenant.contacts.export",
  TenantContactsImport: "tenant.contacts.import",
  TenantChannelsRead: "tenant.channels.read",
  TenantTeamsRead: "tenant.teams.read",
  TenantTeamsWrite: "tenant.teams.write",
  TenantReportsRead: "tenant.reports.read",
  TenantReportsExport: "tenant.reports.export",
  TenantAiUse: "tenant.ai.use",
  TenantAiRead: "tenant.ai.read",
  TenantAiWrite: "tenant.ai.write",
  TenantAutomationsRead: "tenant.automations.read",
  TenantAutomationsWrite: "tenant.automations.write",
  TenantKnowledgeRead: "tenant.knowledge.read",
  TenantKnowledgeWrite: "tenant.knowledge.write",
  TenantKnowledgePublish: "tenant.knowledge.publish",
  TenantMacrosRead: "tenant.macros.read",
  TenantMacrosWrite: "tenant.macros.write",
  TenantSlaRead: "tenant.sla.read",
  TenantSlaWrite: "tenant.sla.write",
  TenantCustomFieldsRead: "tenant.custom_fields.read",
  TenantCustomFieldsManage: "tenant.custom_fields.manage",
  TenantCustomObjectsRead: "tenant.custom_objects.read",
  TenantCustomObjectsWrite: "tenant.custom_objects.write",
  TenantOnboardingManage: "tenant.onboarding.manage",
  /** Connected system, "Sistemini bağla" (CF-197). */
  TenantIntegrationsRead: "tenant.integrations.read",
  TenantIntegrationsManage: "tenant.integrations.manage",
  TenantIntegrationsUse: "tenant.integrations.use",
  /** AI knowledge layer measurement: panel and evaluation sets (CF-220). */
  TenantAIInsightsRead: "tenant.ai_insights.read",
  TenantAIInsightsManage: "tenant.ai_insights.manage",
} as const;

export type PermissionSlug = (typeof Permission)[keyof typeof Permission];

export const ALL_PERMISSIONS: readonly PermissionSlug[] = Object.values(
  Permission,
).sort((a, b) => a.localeCompare(b));

const knownPermissionSet = new Set<string>(ALL_PERMISSIONS);

export function isKnownPermission(slug: string): slug is PermissionSlug {
  return knownPermissionSet.has(slug);
}

export const permissions = {
  video: {
    platformManage: Permission.PlatformVideoManage,
  },
  /** Module guides, ClickFix Asistan and product feedback (CF-141). */
  assistant: {
    use: Permission.TenantAssistantUse,
    platformManage: Permission.PlatformAssistantManage,
    guidesManage: Permission.PlatformGuidesManage,
    feedbackRead: Permission.PlatformProductFeedbackRead,
  },
  billing: {
    plansRead: Permission.PlatformPlansRead,
    plansWrite: Permission.PlatformPlansWrite,
    subscriptionsRead: Permission.PlatformSubscriptionsRead,
    subscriptionsWrite: Permission.PlatformSubscriptionsWrite,
    entitlementsRead: Permission.PlatformEntitlementsRead,
    entitlementsWrite: Permission.PlatformEntitlementsWrite,
    tenantRead: Permission.TenantBillingRead,
    tenantWrite: Permission.TenantBillingWrite,
  },
  users: {
    read: Permission.PlatformUsersRead,
    write: Permission.PlatformUsersWrite,
    export: Permission.PlatformUsersExport,
    import: Permission.PlatformUsersImport,
    bulkDisable: Permission.PlatformUsersBulkDisable,
    bulkEnable: Permission.PlatformUsersBulkEnable,
    impersonate: Permission.PlatformUsersImpersonate,
  },
  roles: {
    read: Permission.PlatformRolesRead,
    write: Permission.PlatformRolesWrite,
    export: Permission.PlatformRolesExport,
    import: Permission.PlatformRolesImport,
    bulkDelete: Permission.PlatformRolesBulkDelete,
  },
  organizations: {
    read: Permission.PlatformOrganizationsRead,
    write: Permission.PlatformOrganizationsWrite,
    export: Permission.PlatformOrganizationsExport,
    bulkActivate: Permission.PlatformOrganizationsBulkActivate,
    bulkSuspend: Permission.PlatformOrganizationsBulkSuspend,
  },
  support: {
    access: Permission.PlatformSupportAccess,
  },
  notifications: {
    read: Permission.NotificationsRead,
    manage: Permission.NotificationsManage,
    platformRead: Permission.PlatformNotificationsRead,
    platformReadAll: Permission.PlatformNotificationsReadAll,
    export: Permission.PlatformNotificationsExport,
  },
  settings: {
    read: Permission.PlatformSettingsRead,
    write: Permission.PlatformSettingsWrite,
  },
  activity: {
    read: Permission.PlatformActivityRead,
  },
  imports: {
    read: Permission.PlatformImportsRead,
  },
  exports: {
    read: Permission.PlatformExportsRead,
  },
  storage: {
    read: Permission.PlatformStorageRead,
    write: Permission.PlatformStorageWrite,
  },
  logs: {
    read: Permission.PlatformLogsRead,
    write: Permission.PlatformLogsWrite,
  },
  bulk: {
    read: Permission.PlatformBulkRead,
  },
  access: {
    read: Permission.PlatformAccessRead,
    write: Permission.PlatformAccessWrite,
  },
  integrations: {
    github: {
      read: Permission.PlatformIntegrationsGitHubRead,
      write: Permission.PlatformIntegrationsGitHubWrite,
    },
    google: {
      read: Permission.PlatformIntegrationsGoogleRead,
      write: Permission.PlatformIntegrationsGoogleWrite,
    },
    facebook: {
      read: Permission.PlatformIntegrationsFacebookRead,
      write: Permission.PlatformIntegrationsFacebookWrite,
    },
    apple: {
      read: Permission.PlatformIntegrationsAppleRead,
      write: Permission.PlatformIntegrationsAppleWrite,
    },
  },
  authSettings: {
    read: Permission.PlatformAuthSettingsRead,
    write: Permission.PlatformAuthSettingsWrite,
  },
  auth: {
    session: Permission.AuthSession,
  },
  tenant: {
    settings: {
      read: Permission.TenantSettingsRead,
      write: Permission.TenantSettingsWrite,
    },
    members: {
      read: Permission.TenantMembersRead,
      write: Permission.TenantMembersWrite,
    },
    roles: {
      read: Permission.TenantRolesRead,
      write: Permission.TenantRolesWrite,
    },
    audit: {
      read: Permission.TenantAuditRead,
      export: Permission.TenantAuditExport,
    },
    supportAccess: {
      read: Permission.TenantSupportAccessRead,
      manage: Permission.TenantSupportAccessManage,
    },
    conversations: {
      read: Permission.TenantConversationsRead,
      readAll: Permission.TenantConversationsReadAll,
      reply: Permission.TenantConversationsReply,
      note: Permission.TenantConversationsNote,
      assign: Permission.TenantConversationsAssign,
      manage: Permission.TenantConversationsManage,
      delete: Permission.TenantConversationsDelete,
      export: Permission.TenantConversationsExport,
    },
    replays: {
      read: Permission.TenantReplaysRead,
    },
    cobrowse: {
      use: Permission.TenantCobrowseUse,
      control: Permission.TenantCobrowseControl,
    },
    tickets: {
      read: Permission.TenantTicketsRead,
      readAll: Permission.TenantTicketsReadAll,
      write: Permission.TenantTicketsWrite,
      assign: Permission.TenantTicketsAssign,
      delete: Permission.TenantTicketsDelete,
      export: Permission.TenantTicketsExport,
    },
    contacts: {
      read: Permission.TenantContactsRead,
      write: Permission.TenantContactsWrite,
      delete: Permission.TenantContactsDelete,
      export: Permission.TenantContactsExport,
      import: Permission.TenantContactsImport,
    },
    channels: {
      read: Permission.TenantChannelsRead,
      manage: Permission.TenantChannelsManage,
    },
    teams: {
      read: Permission.TenantTeamsRead,
      write: Permission.TenantTeamsWrite,
    },
    appointments: {
      read: Permission.TenantAppointmentsRead,
      write: Permission.TenantAppointmentsWrite,
      manage: Permission.TenantAppointmentsManage,
    },
    reports: {
      read: Permission.TenantReportsRead,
      export: Permission.TenantReportsExport,
    },
    schedules: {
      read: Permission.TenantSchedulesRead,
      manage: Permission.TenantSchedulesManage,
    },
    quality: {
      read: Permission.TenantQualityRead,
      manage: Permission.TenantQualityManage,
    },
    /** Feedback, idea board and community forum moderation (CF-78). */
    feedback: {
      read: Permission.TenantFeedbackRead,
      write: Permission.TenantFeedbackWrite,
      manage: Permission.TenantFeedbackManage,
    },
    ai: {
      use: Permission.TenantAiUse,
      read: Permission.TenantAiRead,
      write: Permission.TenantAiWrite,
    },
    aiInsights: {
      read: Permission.TenantAIInsightsRead,
      manage: Permission.TenantAIInsightsManage,
    },
    automations: {
      read: Permission.TenantAutomationsRead,
      write: Permission.TenantAutomationsWrite,
    },
    knowledge: {
      read: Permission.TenantKnowledgeRead,
      write: Permission.TenantKnowledgeWrite,
      publish: Permission.TenantKnowledgePublish,
    },
    macros: {
      read: Permission.TenantMacrosRead,
      write: Permission.TenantMacrosWrite,
    },
    sla: {
      read: Permission.TenantSlaRead,
      write: Permission.TenantSlaWrite,
    },
    video: {
      use: Permission.TenantVideoUse,
      manage: Permission.TenantVideoManage,
    },
    teamChat: {
      use: Permission.TenantTeamChatUse,
    },
    customFields: {
      read: Permission.TenantCustomFieldsRead,
      manage: Permission.TenantCustomFieldsManage,
    },
    customObjects: {
      read: Permission.TenantCustomObjectsRead,
      write: Permission.TenantCustomObjectsWrite,
    },
    onboarding: {
      manage: Permission.TenantOnboardingManage,
    },
    integrations: {
      read: Permission.TenantIntegrationsRead,
      manage: Permission.TenantIntegrationsManage,
      use: Permission.TenantIntegrationsUse,
    },
  },
} as const;
