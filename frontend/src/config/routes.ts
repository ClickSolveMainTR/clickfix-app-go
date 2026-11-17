function helpLang(locale: string) {
  return locale === "tr" ? "" : `?lang=${locale}`;
}

export const routes = {
  public: {
    root: "/",
    /** Public home for every visitor, including signed-in users. */
    landing: "/",
    legal: (slug: string) => `/legal/${slug}`,
    health: "/health",
    /** Public staff invitation page (`/v1/invitations/{token}`). */
    invite: (token: string) => `/invite/${encodeURIComponent(token)}`,
    share: {
      slug: (slug: string) => `/share/${slug}`,
      signed: (token: string) => `/share/s/${token}`,
    },
  },
  /**
   * Public help center of an organization (CF-74). English pages carry
   * `?lang=en`; Turkish is the default.
   */
  help: {
    home: (org: string, locale = "tr") =>
      `/help/${encodeURIComponent(org)}${helpLang(locale)}`,
    collection: (org: string, slug: string, locale = "tr") =>
      `/help/${encodeURIComponent(org)}/${encodeURIComponent(slug)}${helpLang(locale)}`,
    article: (org: string, slug: string, locale = "tr") =>
      `/help/${encodeURIComponent(org)}/article/${encodeURIComponent(slug)}${helpLang(locale)}`,
    search: (org: string, query: string, locale = "tr") =>
      `/help/${encodeURIComponent(org)}/search?q=${encodeURIComponent(query)}${locale === "tr" ? "" : `&lang=${locale}`}`,
    sitemap: (org: string) => `/help/${encodeURIComponent(org)}/sitemap.xml`,
    /** Public community forum under the help center (CF-78). */
    forum: (org: string, locale = "tr") =>
      `/help/${encodeURIComponent(org)}/community${helpLang(locale)}`,
    forumCategory: (org: string, category: string, locale = "tr") =>
      `/help/${encodeURIComponent(org)}/community/category/${encodeURIComponent(category)}${helpLang(locale)}`,
    forumTopic: (org: string, slug: string, locale = "tr") =>
      `/help/${encodeURIComponent(org)}/community/topic/${encodeURIComponent(slug)}${helpLang(locale)}`,
  },
  /** Public booking pages (CF-138, no session). */
  booking: {
    index: (org: string) => `/book/${encodeURIComponent(org)}`,
    type: (org: string, type: string) =>
      `/book/${encodeURIComponent(org)}/${encodeURIComponent(type)}`,
    manage: (token: string) => `/book/manage/${encodeURIComponent(token)}`,
  },
  guest: {
    login: "/login",
    /** Organization SSO sign-in (CF-128); `?organization=<uuid>&next=`. */
    loginSSO: "/login/sso",
    /**
     * Self-serve sign-up (CF-143): account, e-mail verification, first
     * organization. `?plan=` carries the price picked on the pricing page.
     */
    signup: "/register",
    /** Canonical sign-up URL, shared with `signup`. */
    register: "/register",
    forgotPassword: "/platform/forgot-password",
    resetPassword: "/platform/reset-password",
    verifyEmail: "/platform/verify-email",
  },
  cms: {
    root: "/dashboard",
    home: "/dashboard",
    /** Set-up wizard of the active organization (CF-13). */
    onboarding: "/setup",
    /**
     * Full-screen AI-assisted set-up (CF-140): discovery or template
     * gallery, preview, live build, invite the team. `?calisma=` resumes a run.
     */
    smartSetup: "/getting-started",
    /** Shared inbox. */
    inbox: "/inbox",
    /** Contacts and their detail page (CF-221); companies keep their own. */
    contacts: {
      root: "/contacts",
      detail: (uuid: string) => `/contacts/${encodeURIComponent(uuid)}`,
    },
    companies: {
      root: "/companies",
      detail: (uuid: string) => `/companies/${encodeURIComponent(uuid)}`,
    },
    /** Channel management (CF-61); Meta logins return to connect/{type}. */
    channels: "/channels",
    /** ClickFix Asistan as a full page (CF-200). */
    assistant: "/assistant",
    /** Customer billing portal (CF-33); gateway success/fail URL. */
    billing: "/billing",
    /** Video rooms (CF-139). */
    video: {
      root: "/video",
      room: (uuid: string) => `/video/${uuid}`,
    },
    profile: {
      root: "/profile",
      password: "/profile/password",
      preferences: "/profile/preferences",
    },
    /**
     * Settings of the ACTIVE organization (sidebar switcher, CF-15); API
     * calls carry its slug as `X-Organization`.
     */
    /** Teams of the active organization (CF-5). */
    teamStatus: "/team-status",
    /** Team chat: direct, group and swarm threads (CF-161); thread in `?thread=`. */
    teamChat: "/team-chat",
    /** Agent scorecards, custom and scheduled reports (CF-112). */
    reports: "/reports",
    /** Read-only service incidents for agents (CF-162). */
    incidents: "/incidents",
    /** On-call rotas, escalation, uptime monitors and alerts (CF-104). */
    oncall: "/oncall",
    /** Volume forecast, staffing gaps and timesheets (CF-107). */
    workforce: "/workforce",
    /** Calibration, coaching and gamification (CF-109). */
    coaching: "/coaching",
    /** Organization calendar and appointments (CF-138). */
    calendar: {
      root: "/calendar",
      appointment: (uuid: string) =>
        `/calendar?appointment=${encodeURIComponent(uuid)}`,
    },
    /** Community forum moderation, categories and topics (CF-78); tabs in `?tab=`. */
    forum: "/forum",
    /** Visual workflow list and canvas editor (CF-96). */
    workflows: {
      root: "/workflows",
      detail: (uuid: string) => `/workflows/${encodeURIComponent(uuid)}`,
    },
    /** Automation rules and their run log (CF-94); `new` creates one. */
    rules: {
      root: "/rules",
      detail: (uuid: string) => `/rules/${encodeURIComponent(uuid)}`,
    },
    /** Sidebar apps placed in the navigation (CF-118); `?app=` selects one. */
    apps: {
      root: "/apps",
      detail: (uuid: string) => `/apps?app=${encodeURIComponent(uuid)}`,
    },
    /** Product requests handed to engineering (CF-159). */
    engineering: {
      root: "/engineering",
      detail: (uuid: string) => `/engineering/${encodeURIComponent(uuid)}`,
    },
    teams: {
      root: "/teams",
      create: "/teams/create",
      detail: (uuid: string) => `/teams/${uuid}`,
    },
    departments: {
      root: "/departments",
      create: "/departments/create",
      detail: (uuid: string) => `/departments/${uuid}`,
    },
    settings: {
      organization: "/settings/organization",
      members: "/settings/members",
      audit: "/settings/audit",
      supportAccess: "/settings/support-access",
      /** KVKK / GDPR tools (CF-127); tabs in `?tab=`, subject in `?contact=`. */
      privacy: "/settings/privacy",
      /** Organization SSO, SCIM and IP allow-list (CF-128). */
      security: "/settings/security",
      sla: "/settings/sla",
      /** API keys and REST hooks for Zapier / Make / n8n (CF-97). */
      integrations: "/settings/integrations",
      /** Connected system: concepts, endpoints, actions, AI playbooks (CF-197). */
      integrationsConnect: "/settings/integrations/connect",
      /** MCP connections: connected apps and the connection guide (CF-173). */
      mcp: "/settings/mcp",
      /** Jira / Linear / GitHub connections of product requests (CF-160). */
      engineering: {
        root: "/settings/engineering",
        /** OAuth return leg; this path is the provider's redirect URI. */
        callback: (provider: string) =>
          `/settings/engineering/callback/${encodeURIComponent(provider)}`,
      },
      video: "/settings/video",
      /** Remote support settings (CF-237). */
      remoteSupport: "/settings/remote-support",
      customerHealth: "/settings/customer-health",
      roles: {
        root: "/settings/roles",
        create: "/settings/roles/create",
        detail: (uuid: string) => `/settings/roles/${uuid}`,
      },
      customFields: "/settings/custom-fields",
      /** Fields tab of one entity (`contact`, …) or custom object type uuid. */
      customFieldsTab: (tab: string) =>
        `/settings/custom-fields?tab=${encodeURIComponent(tab)}`,
      customObjects: "/settings/custom-objects",
      requestForms: "/settings/request-forms",
      /** Appointment types and module settings (CF-138). */
      appointments: "/settings/appointments",
      /** Organization brands: identity, sender, help center, widget (CF-124). */
      brands: "/settings/brands",
      /** Sidebar app registry: iframe URL, placement, scopes (CF-118). */
      apps: "/settings/apps",
      /** Organization AI settings; tabs in `?tab=` (agent, procedures, tools, runs, ...). */
      ai: "/settings/ai",
      /** Knowledge base admin (CF-73). */
      knowledge: {
        root: "/settings/knowledge",
        tab: (tab: string) =>
          `/settings/knowledge?tab=${encodeURIComponent(tab)}`,
        create: "/settings/knowledge/new",
        detail: (uuid: string) => `/settings/knowledge/${uuid}`,
      },
      /** Category taxonomy, review queue and label export (CF-218). */
      categories: "/settings/categories",
    },
    objects: {
      records: (typeUuid: string) => `/objects/${typeUuid}`,
    },
  },
  platform: {
    root: "/platform",
    home: "/platform",
    /**
     * System Settings shell (`/platform/settings/*`). Platform admin pages
     * live here behind one grouped sub-sidebar (`settingsNav`). The legacy
     * top-level URLs redirect permanently via `next.config.ts`.
     */
    settings: {
      root: "/platform/settings",
      system: "/platform/settings/system",
      environment: "/platform/settings/environment",
      general: "/platform/settings/general",
      maintenance: "/platform/settings/maintenance",
      integrations: "/platform/settings/integrations",
      roles: "/platform/settings/roles",
      activity: "/platform/settings/activity",
      logs: "/platform/settings/logs",
      storage: "/platform/settings/storage",
      access: "/platform/settings/access",
      auth: "/platform/settings/auth",
      imports: "/platform/settings/imports",
      exports: "/platform/settings/exports",
      /** Marketing site tags, SEO defaults and sign-up sources (CF-144). */
      marketing: "/platform/settings/marketing",
      marketingSources: "/platform/settings/marketing/sources",
    },
    roles: {
      root: "/platform/settings/roles",
      create: "/platform/settings/roles/create",
      detail: (uuid: string) => `/platform/settings/roles/${uuid}`,
      edit: (uuid: string) => `/platform/settings/roles/${uuid}/edit`,
    },
    /** Platform video module (CF-139). */
    video: "/platform/video",
    /** Remote support kill switch and reports (CF-237). */
    remoteSupport: "/platform/remote-support",
    /** ClickFix Asistan settings, module guides, product feedback (CF-141). */
    assistant: "/platform/assistant",
    guides: "/platform/guides",
    productFeedback: "/platform/product-feedback",
    /** Platform billing management (CF-39). */
    billing: {
      root: "/platform/billing",
      tab: (tab: string) => `/platform/billing?tab=${tab}`,
      plan: (uuid: string) => `/platform/billing/plans/${uuid}`,
    },
    organizations: {
      root: "/platform/organizations",
      create: "/platform/organizations/create",
      detail: (uuid: string) => `/platform/organizations/${uuid}`,
      edit: (uuid: string) => `/platform/organizations/${uuid}/edit`,
    },
    /** Organizations open to ClickFix support (CF-14). */
    supportAccess: "/platform/support-access",
    /** System health overview (CF-14). */
    health: "/platform/health",
    users: {
      root: "/platform/users",
      create: "/platform/users/create",
      detail: (uuid: string) => `/platform/users/${uuid}`,
      edit: (uuid: string) => `/platform/users/${uuid}/edit`,
    },
    notifications: {
      root: "/platform/notifications",
      detail: (uuid: string) => `/platform/notifications/${uuid}`,
    },
    imports: {
      root: "/platform/settings/imports",
      detail: (uuid: string) => `/platform/settings/imports/${uuid}`,
    },
    exports: {
      root: "/platform/settings/exports",
      detail: (uuid: string) => `/platform/settings/exports/${uuid}`,
    },
    access: {
      root: "/platform/settings/access",
    },
    authSettings: {
      root: "/platform/settings/auth",
    },
    integrations: {
      root: "/platform/settings/integrations",
      github: "/platform/settings/integrations?tab=github",
      google: "/platform/settings/integrations?tab=google",
      facebook: "/platform/settings/integrations?tab=facebook",
      apple: "/platform/settings/integrations?tab=apple",
    },
    activity: {
      root: "/platform/settings/activity",
    },
    logs: {
      root: "/platform/settings/logs",
    },
    storage: {
      root: "/platform/settings/storage",
    },
    profile: {
      root: "/platform/profile",
      password: "/platform/profile/password",
      preferences: "/platform/profile/preferences",
    },
  },
  errors: {
    unauthorized: "/unauthorized",
    forbidden: "/forbidden",
  },
} as const;

type NestedRoutes<T> = T extends string
  ? T
  : T extends (...args: never[]) => infer R
    ? R extends string
      ? R
      : never
    : T extends Record<string, unknown>
      ? NestedRoutes<T[keyof T]>
      : never;

export type AppRoute = NestedRoutes<typeof routes>;
