import {
  Bug,
  Blocks,
  Puzzle,
  Workflow,
  BarChart3,
  BookOpen,
  CalendarCog,
  CalendarDays,
  BookUser,
  MessageSquareText,
  BookOpenCheck,
  Bot,
  Activity,
  Bell,
  Building2,
  CalendarClock,
  ChartPie,
  Boxes,
  HeartPulse,
  LifeBuoy,
  ClipboardList,
  Cpu,
  CreditCard,
  Download,
  FileLock2,
  HardDrive,
  Inbox,
  MessagesSquare,
  RadioTower,
  Rocket,
  UserRoundCheck,
  UsersRound,
  KeyRound,
  LayoutDashboard,
  ListChecks,
  LogIn,
  Megaphone,
  MessageCircle,
  GitPullRequest,
  Palette,
  Cable,
  Plug,
  ScrollText,
  Settings2,
  Share2,
  Shield,
  Siren,
  SlidersHorizontal,
  Sparkles,
  Timer,
  TriangleAlert,
  Trophy,
  Upload,
  Users,
  MessageSquareHeart,
  Video,
  Zap,
} from "lucide-react";

import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { defineNav } from "@/features/nav-engine";
import type { NavCatalog } from "@/features/nav-engine";
import { CalendarNavAdornment } from "@/features/calendar/nav";
import { inboxNavAdornments } from "@/features/inbox/nav";
import { TeamChatNavAdornment } from "@/features/team-chat/nav";
import { usersNavItem } from "@/features/users/nav";

const integrationReadPermissions = [
  permissions.integrations.google.read,
  permissions.integrations.github.read,
  permissions.integrations.facebook.read,
  permissions.integrations.apple.read,
];

/**
 * System Settings sub-sidebar (`/platform/settings/*`). Rendered by
 * `features/settings` (not by `NavEngine`), but uses the same catalog shape
 * and `permission` / `anyPermission` gating.
 */
export const settingsNav = defineNav({
  id: "settings",
  groups: [
    {
      id: "system",
      labelKey: "settings.subnav.group_system",
      icon: Activity,
      defaultOpen: true,
      items: [
        {
          id: "system-overview",
          titleKey: "settings.subnav.system_overview",
          href: routes.platform.settings.root,
          icon: Activity,
          permission: permissions.settings.read,
        },
        {
          id: "system-environment",
          titleKey: "settings.subnav.system_environment",
          href: routes.platform.settings.environment,
          icon: Cpu,
          permission: permissions.settings.read,
        },
        {
          id: "logs",
          titleKey: "layout.nav_logs",
          href: routes.platform.settings.logs,
          icon: ScrollText,
          permission: permissions.logs.read,
        },
        {
          id: "activity",
          titleKey: "layout.nav_activity",
          href: routes.platform.settings.activity,
          icon: Activity,
          permission: permissions.activity.read,
        },
      ],
    },
    {
      id: "general",
      labelKey: "settings.subnav.group_general",
      icon: Settings2,
      defaultOpen: true,
      items: [
        {
          id: "general-branding",
          titleKey: "settings.subnav.general_branding",
          href: routes.platform.settings.general,
          icon: Palette,
          permission: permissions.settings.read,
        },
      ],
    },
    {
      id: "marketing",
      labelKey: "settings.subnav.group_marketing",
      icon: Megaphone,
      defaultOpen: true,
      items: [
        {
          id: "marketing-tracking",
          titleKey: "settings.subnav.marketing",
          href: routes.platform.settings.marketing,
          icon: Megaphone,
          permission: permissions.settings.read,
        },
        {
          id: "marketing-sources",
          titleKey: "settings.subnav.marketing_sources",
          href: routes.platform.settings.marketingSources,
          icon: ChartPie,
          permission: permissions.organizations.read,
        },
      ],
    },
    {
      id: "security",
      labelKey: "settings.subnav.group_security",
      icon: Shield,
      defaultOpen: true,
      items: [
        {
          id: "roles",
          titleKey: "layout.nav_roles",
          href: routes.platform.settings.roles,
          icon: Shield,
          permission: permissions.roles.read,
        },
        {
          id: "auth-settings",
          titleKey: "settings.subnav.auth_methods",
          href: routes.platform.settings.auth,
          icon: LogIn,
          permission: permissions.authSettings.read,
        },
        {
          id: "access-policy",
          titleKey: "settings.subnav.access_policy",
          href: routes.platform.settings.access,
          icon: KeyRound,
          permission: permissions.access.read,
        },
      ],
    },
    {
      id: "integrations",
      labelKey: "settings.subnav.group_integrations",
      icon: Share2,
      defaultOpen: true,
      items: [
        {
          id: "oauth-integrations",
          titleKey: "settings.subnav.oauth_providers",
          href: routes.platform.settings.integrations,
          icon: Share2,
          anyPermission: integrationReadPermissions,
        },
        {
          id: "storage",
          titleKey: "settings.subnav.storage",
          href: routes.platform.settings.storage,
          icon: HardDrive,
          permission: permissions.storage.read,
        },
      ],
    },
    {
      id: "io",
      labelKey: "settings.subnav.group_maintenance",
      icon: Upload,
      defaultOpen: true,
      items: [
        {
          id: "imports",
          titleKey: "layout.nav_imports",
          href: routes.platform.settings.imports,
          icon: Upload,
          permission: permissions.imports.read,
        },
        {
          id: "exports",
          titleKey: "layout.nav_exports",
          href: routes.platform.settings.exports,
          icon: Download,
          permission: permissions.exports.read,
        },
        {
          id: "purge-rules",
          titleKey: "settings.subnav.purge_rules",
          href: routes.platform.settings.maintenance,
          icon: Sparkles,
          permission: permissions.logs.read,
        },
      ],
    },
  ],
});

/** Every slug that unlocks at least one settings subpage (OR-gate for the shell). */
function catalogPermissions(catalog: NavCatalog): string[] {
  const slugs = new Set<string>();
  for (const group of catalog.groups) {
    for (const item of group.items) {
      if (typeof item.permission === "string") slugs.add(item.permission);
      else if (item.permission) item.permission.forEach((p) => slugs.add(p));
      item.anyPermission?.forEach((p) => slugs.add(p));
    }
  }
  return [...slugs].sort();
}

export const settingsNavPermissions = catalogPermissions(settingsNav);

export const platformNav = defineNav({
  id: "platform",
  groups: [
    {
      id: "general",
      labelKey: "layout.nav_platform",
      icon: LayoutDashboard,
      defaultOpen: true,
      items: [
        {
          id: "home",
          titleKey: "layout.home",
          href: routes.platform.home,
          icon: LayoutDashboard,
        },
      ],
    },
    {
      id: "manage",
      labelKey: "layout.section_manage",
      icon: Shield,
      defaultOpen: true,
      items: [
        {
          id: "organizations",
          titleKey: "layout.nav_organizations",
          href: routes.platform.organizations.root,
          icon: Building2,
          permission: permissions.organizations.read,
        },
        {
          id: "billing",
          titleKey: "layout.nav_billing",
          href: routes.platform.billing.root,
          icon: CreditCard,
          anyPermission: [
            permissions.billing.subscriptionsRead,
            permissions.billing.plansRead,
          ],
        },
        {
          id: "platform-video",
          titleKey: "layout.nav_platform_video",
          href: routes.platform.video,
          icon: Video,
          permission: permissions.video.platformManage,
        },
        {
          id: "platform-assistant",
          titleKey: "layout.nav_platform_assistant",
          href: routes.platform.assistant,
          icon: Bot,
          permission: permissions.assistant.platformManage,
        },
        {
          id: "platform-guides",
          titleKey: "layout.nav_platform_guides",
          href: routes.platform.guides,
          icon: BookOpenCheck,
          permission: permissions.assistant.guidesManage,
        },
        {
          id: "platform-product-feedback",
          titleKey: "layout.nav_platform_product_feedback",
          href: routes.platform.productFeedback,
          icon: MessageSquareHeart,
          permission: permissions.assistant.feedbackRead,
        },
        usersNavItem,
        {
          id: "support-access",
          titleKey: "layout.nav_support_access",
          href: routes.platform.supportAccess,
          icon: LifeBuoy,
          permission: permissions.support.access,
        },
        {
          id: "health",
          titleKey: "layout.nav_platform_health",
          href: routes.platform.health,
          icon: HeartPulse,
          permission: permissions.organizations.read,
        },
        {
          id: "notifications",
          titleKey: "layout.nav_notifications",
          href: routes.platform.notifications.root,
          icon: Bell,
          permission: permissions.notifications.platformRead,
        },
        {
          id: "settings",
          titleKey: "layout.nav_settings",
          href: routes.platform.settings.root,
          icon: Settings2,
          anyPermission: settingsNavPermissions,
        },
      ],
    },
  ],
});

export const cmsNav = defineNav({
  id: "cms",
  groups: [
    {
      id: "cms",
      labelKey: "layout.section_cms",
      icon: LayoutDashboard,
      defaultOpen: true,
      items: [
        {
          id: "home",
          titleKey: "layout.home",
          href: routes.cms.home,
          icon: LayoutDashboard,
        },
        {
          id: "setup",
          titleKey: "layout.nav_setup",
          href: routes.cms.onboarding,
          icon: Rocket,
        },
      ],
    },
    /*
     * Workspace sections. Items stay `soon` until their route exists; live
     * ones are gated by the active organization's `tenant.*` permissions.
     */
    {
      id: "inbox",
      labelKey: "layout.section_inbox",
      icon: Inbox,
      defaultOpen: true,
      items: [
        {
          id: "inbox-all",
          titleKey: "layout.nav_inbox_all",
          href: "/inbox",
          icon: Inbox,
          Adornment: inboxNavAdornments.all,
        },
        {
          id: "inbox-mine",
          titleKey: "layout.nav_inbox_mine",
          href: "/inbox?view=mine",
          icon: UserRoundCheck,
          Adornment: inboxNavAdornments.mine,
        },
        {
          id: "inbox-unassigned",
          titleKey: "layout.nav_inbox_unassigned",
          href: "/inbox?view=unassigned",
          icon: MessagesSquare,
          Adornment: inboxNavAdornments.unassigned,
        },
        {
          id: "replies",
          titleKey: "layout.nav_replies",
          href: "/replies",
          icon: MessageSquareText,
        },
        {
          id: "engineering",
          titleKey: "layout.nav_engineering",
          href: routes.cms.engineering.root,
          icon: Bug,
          permission: permissions.tenant.tickets.read,
        },
        {
          id: "incidents",
          titleKey: "layout.nav_incidents",
          href: routes.cms.incidents,
          icon: TriangleAlert,
          permission: permissions.tenant.conversations.read,
        },
      ],
    },
    {
      id: "customers",
      labelKey: "layout.section_customers",
      icon: BookUser,
      defaultOpen: true,
      items: [
        {
          id: "contacts",
          titleKey: "layout.nav_contacts",
          href: "/contacts",
          icon: BookUser,
        },
        {
          id: "companies",
          titleKey: "layout.nav_companies",
          href: "/companies",
          icon: Building2,
        },
      ],
    },
    {
      id: "team",
      labelKey: "layout.section_team",
      icon: UsersRound,
      defaultOpen: true,
      items: [
        {
          id: "teams",
          titleKey: "layout.nav_teams",
          href: routes.cms.teams.root,
          icon: UsersRound,
          permission: permissions.tenant.teams.read,
        },
        {
          id: "calendar",
          titleKey: "layout.nav_calendar",
          href: routes.cms.calendar.root,
          icon: CalendarDays,
          permission: permissions.tenant.appointments.read,
          Adornment: CalendarNavAdornment,
        },
        {
          id: "video",
          titleKey: "layout.nav_video",
          href: routes.cms.video.root,
          icon: Video,
          permission: permissions.tenant.video.use,
        },
        {
          id: "team-chat",
          titleKey: "layout.nav_team_chat",
          href: routes.cms.teamChat,
          icon: MessagesSquare,
          permission: permissions.tenant.teamChat.use,
          Adornment: TeamChatNavAdornment,
        },
        {
          id: "team-status",
          titleKey: "layout.nav_team_status",
          href: routes.cms.teamStatus,
          icon: Activity,
          permission: permissions.tenant.teams.read,
        },
        {
          id: "reports",
          titleKey: "layout.nav_reports",
          href: routes.cms.reports,
          icon: BarChart3,
          permission: [
            permissions.tenant.conversations.read,
            permissions.tenant.conversations.readAll,
          ],
        },
        {
          id: "oncall",
          titleKey: "layout.nav_oncall",
          href: routes.cms.oncall,
          icon: Siren,
          permission: [
            permissions.tenant.schedules.read,
            permissions.tenant.conversations.readAll,
          ],
        },
        {
          id: "workforce",
          titleKey: "layout.nav_workforce",
          href: routes.cms.workforce,
          icon: CalendarClock,
          permission: permissions.tenant.schedules.read,
        },
        {
          id: "coaching",
          titleKey: "layout.nav_coaching",
          href: routes.cms.coaching,
          icon: Trophy,
          anyPermission: [
            permissions.tenant.schedules.read,
            permissions.tenant.quality.manage,
          ],
        },
        {
          id: "channels",
          titleKey: "layout.nav_channels",
          href: routes.cms.channels,
          icon: RadioTower,
          permission: permissions.tenant.channels.read,
        },
        {
          id: "widget",
          titleKey: "layout.nav_widget",
          href: "/widget",
          icon: MessageCircle,
        },
        {
          id: "forum",
          titleKey: "layout.nav_forum",
          href: routes.cms.forum,
          icon: MessagesSquare,
          permission: permissions.tenant.feedback.manage,
        },
        {
          id: "workflows",
          titleKey: "layout.nav_workflows",
          href: routes.cms.workflows.root,
          icon: Workflow,
          permission: permissions.tenant.automations.read,
        },
        {
          id: "rules",
          titleKey: "layout.nav_rules",
          href: routes.cms.rules.root,
          icon: Zap,
          permission: permissions.tenant.automations.read,
        },
        {
          id: "apps",
          titleKey: "layout.nav_apps",
          href: routes.cms.apps.root,
          icon: Blocks,
          permission: permissions.tenant.conversations.read,
        },
      ],
    },
    /*
     * Settings of the active organization; gated by its `tenant.*`
     * permissions (TenantProvider scopes usePermission() in this shell).
     */
    {
      id: "organization-settings",
      labelKey: "layout.section_tenant_settings",
      icon: Settings2,
      defaultOpen: true,
      items: [
        {
          id: "settings-organization",
          titleKey: "layout.nav_tenant_settings_general",
          href: routes.cms.settings.organization,
          icon: SlidersHorizontal,
          permission: permissions.tenant.settings.read,
        },
        {
          id: "settings-members",
          titleKey: "layout.nav_tenant_settings_members",
          href: routes.cms.settings.members,
          icon: Users,
          permission: permissions.tenant.members.read,
        },
        {
          id: "settings-roles",
          titleKey: "layout.nav_tenant_settings_roles",
          href: routes.cms.settings.roles.root,
          icon: Shield,
          permission: permissions.tenant.roles.read,
        },
        {
          id: "custom-fields",
          titleKey: "layout.nav_custom_fields",
          href: routes.cms.settings.customFields,
          icon: ListChecks,
          permission: permissions.tenant.customFields.read,
        },
        {
          id: "custom-objects",
          titleKey: "layout.nav_custom_objects",
          href: routes.cms.settings.customObjects,
          icon: Boxes,
          permission: permissions.tenant.customFields.read,
        },
        {
          id: "request-forms",
          titleKey: "layout.nav_request_forms",
          href: routes.cms.settings.requestForms,
          icon: ClipboardList,
          permission: permissions.tenant.customFields.read,
        },
        {
          id: "settings-appointments",
          titleKey: "layout.nav_appointment_settings",
          href: routes.cms.settings.appointments,
          icon: CalendarCog,
          permission: permissions.tenant.appointments.manage,
        },
        {
          id: "settings-brands",
          titleKey: "layout.nav_brands",
          href: routes.cms.settings.brands,
          icon: Palette,
          permission: permissions.tenant.channels.read,
        },
        {
          id: "settings-apps",
          titleKey: "layout.nav_settings_apps",
          href: routes.cms.settings.apps,
          icon: Puzzle,
          permission: permissions.tenant.settings.write,
        },
        {
          id: "settings-ai",
          titleKey: "layout.nav_ai_settings",
          href: routes.cms.settings.ai,
          icon: Bot,
          permission: permissions.tenant.settings.read,
        },
        {
          id: "settings-knowledge",
          titleKey: "layout.nav_knowledge",
          href: routes.cms.settings.knowledge.root,
          icon: BookOpen,
          permission: permissions.tenant.settings.read,
        },
        {
          id: "settings-billing",
          titleKey: "layout.nav_tenant_billing",
          href: routes.cms.billing,
          icon: CreditCard,
          permission: permissions.billing.tenantRead,
        },
        {
          id: "settings-audit",
          titleKey: "layout.nav_tenant_settings_audit",
          href: routes.cms.settings.audit,
          icon: ScrollText,
          permission: permissions.tenant.audit.read,
        },
        {
          id: "settings-support-access",
          titleKey: "layout.nav_tenant_settings_support_access",
          href: routes.cms.settings.supportAccess,
          icon: LifeBuoy,
          permission: permissions.tenant.supportAccess.read,
        },
        {
          id: "settings-privacy",
          titleKey: "layout.nav_tenant_settings_privacy",
          href: routes.cms.settings.privacy,
          icon: FileLock2,
          permission: permissions.tenant.settings.write,
        },
        {
          id: "settings-security",
          titleKey: "layout.nav_tenant_settings_security",
          href: routes.cms.settings.security,
          icon: KeyRound,
          permission: permissions.tenant.settings.write,
        },
        {
          id: "settings-sla",
          titleKey: "layout.nav_tenant_settings_sla",
          href: routes.cms.settings.sla,
          icon: Timer,
          permission: permissions.tenant.sla.read,
        },
        {
          id: "settings-integrations",
          titleKey: "layout.nav_tenant_settings_integrations",
          href: routes.cms.settings.integrations,
          icon: Plug,
          anyPermission: [
            permissions.tenant.settings.write,
            permissions.tenant.channels.read,
          ],
        },
        {
          id: "settings-mcp",
          titleKey: "layout.nav_tenant_settings_mcp",
          href: routes.cms.settings.mcp,
          icon: Cable,
          // The API's MCP connect permission (`rbac.PermTenantMCPConnect`).
          permission: permissions.tenant.settings.read,
        },
        {
          id: "settings-engineering",
          titleKey: "layout.nav_tenant_settings_engineering",
          href: routes.cms.settings.engineering.root,
          icon: GitPullRequest,
          permission: permissions.tenant.settings.write,
        },
        {
          id: "settings-video",
          titleKey: "layout.nav_tenant_settings_video",
          href: routes.cms.settings.video,
          icon: Video,
          permission: permissions.tenant.video.manage,
        },
        {
          id: "settings-customer-health",
          titleKey: "layout.nav_tenant_settings_customer_health",
          href: routes.cms.settings.customerHealth,
          icon: HeartPulse,
          permission: permissions.tenant.contacts.read,
        },
      ],
    },
  ],
});
