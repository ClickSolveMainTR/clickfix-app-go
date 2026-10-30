/**
 * Centrifugo realtime — infrastructure kept; disable/stub if upstream unavailable.
 */
export const realtimeConfig = {
  wsUrl:
    process.env.NEXT_PUBLIC_CENTRIFUGO_URL ??
    process.env.NEXT_PUBLIC_WS_URL ??
    "ws://127.0.0.1:8000/connection/websocket",
  reconnect: {
    minDelayMs: 500,
    maxDelayMs: 10_000,
  },
  /** Roles that may subscribe to system notification channels. */
  platformRoles: ["super_admin"] as const,
  channels: {
    systemNotifications: "system.notifications",
    tenant: (tenantUuid: string) => `tenant:${tenantUuid}`,
    user: (userId: number | string) => `user:${userId}`,
    workspace: (workspaceUuid: string) => `workspace:${workspaceUuid}`,
    conversation: (conversationUuid: string) =>
      `conversation:${conversationUuid}`,
    /** Organization-wide events: agent presence (CF-10). */
    org: (orgUuid: string) => `org:${orgUuid}`,
    /** Inbox events of an organization (CF-43); read_all members only. */
    inbox: (orgUuid: string) => `inbox:${orgUuid}`,
    /** Inbox events one member may see without read_all (CF-44). */
    inboxUser: (orgUuid: string, userUuid: string) =>
      `inbox:${orgUuid}:${userUuid}`,
  },
  presenceEnabled: true,
  /** When false, RealtimeProvider stays idle (no connect attempts). */
  enabled: process.env.NEXT_PUBLIC_REALTIME_ENABLED !== "false",
} as const;
