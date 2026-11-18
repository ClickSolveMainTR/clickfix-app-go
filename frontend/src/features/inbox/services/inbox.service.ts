import type { components, operations } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";
import { apiConfig } from "@/config/api";
import { getActiveOrganizationSlug } from "@/features/organizations/lib/active-organization-store";

import type {
  ConversationCounts,
  ConversationDetail,
  ConversationLinks,
  ConversationPage,
  ConversationUpdate,
  ConversationView,
  ConversationViewFilters,
  ConversationViewInput,
  InboxAgent,
  InboxFilters,
  Message,
  MessageCreate,
  MessagePage,
  LinkRelation,
  SideConversation,
  SideConversationCreate,
  SideConversationList,
  Tag,
  Viewer,
  BulkAction,
  BulkResult,
  Reminder,
  ReminderInput,
} from "../types";

const STATUS_QUERY: Record<InboxFilters["status"], string | undefined> = {
  open: "open,pending",
  snoozed: "snoozed",
  resolved: "resolved,closed",
  all: undefined,
};

const VIEW_QUERY: Record<
  InboxFilters["view"],
  {
    assignee?: string;
    mentioned?: "1";
    team?: string;
    sla_due_within?: number;
    sla_breached?: "1";
  }
> = {
  all: {},
  mine: { assignee: "me" },
  teams: { team: "mine" },
  unassigned: { assignee: "unassigned" },
  mentions: { mentioned: "1" },
  sla_soon: { sla_due_within: 60 },
  breached: { sla_breached: "1" },
};

/** Query parameters of a saved view's filters (they win over the tabs). */
function savedQuery(f: ConversationViewFilters): ListQuery {
  const q: Record<string, string | number> = {};
  if (f.statuses?.length) q.status = f.statuses.join(",");
  if (f.priorities?.length) q.priority = f.priorities.join(",");
  if (f.channel_types?.length) q.channel_type = f.channel_types.join(",");
  if (f.inboxes?.length) q.inbox = f.inboxes.join(",");
  if (f.tags?.length) q.tag = f.tags.join(",");
  if (f.assignee) q.assignee = f.assignee;
  if (f.unread) q.unread = "1";
  if (f.mentioned) q.mentioned = "1";
  if (f.sla_breached != null) q.sla_breached = f.sla_breached ? "1" : "0";
  if (f.sla_due_within_minutes)
    q.sla_due_within = String(f.sla_due_within_minutes);
  if (f.q) q.q = f.q;
  if (f.sort) q.sort = f.sort;
  return q as ListQuery;
}

type ListQuery = NonNullable<
  operations["listTenantConversations"]["parameters"]["query"]
>;

export type ConversationListQuery = InboxFilters & {
  limit: number;
  offset: number;
};

/** The org header is added by the client middleware; typed here for openapi-fetch. */
export type WidgetDiagnostic = components["schemas"]["WidgetDiagnostic"];
export type WidgetTechnicalContext =
  components["schemas"]["WidgetTechnicalContext"];

function header() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

export const inboxService = {
  async list(p: ConversationListQuery) {
    const status = STATUS_QUERY[p.status];
    return unwrap<ConversationPage>(
      await apiClient.GET("/v1/tenant/conversations", {
        params: {
          header: header(),
          query: {
            ...VIEW_QUERY[p.view],
            ...(status ? { status } : {}),
            ...(p.q ? { q: p.q } : {}),
            ...(p.unread ? { unread: "1" as const } : {}),
            ...(p.contact ? { contact: p.contact } : {}),
            ...(p.company ? { company: p.company } : {}),
            ...(p.priorities?.length
              ? { priority: p.priorities.join(",") }
              : {}),
            ...(p.channelTypes?.length
              ? { channel_type: p.channelTypes.join(",") }
              : {}),
            ...(p.tags?.length ? { tag: p.tags.join(",") } : {}),
            ...(p.brand ? { brand_id: p.brand } : {}),
            ...(p.identity ? { identity: p.identity } : {}),
            ...(p.saved ? savedQuery(p.saved) : {}),
            sort: p.sort,
            limit: p.limit,
            offset: p.offset,
          } as ListQuery,
        },
      }),
    );
  },

  async counts() {
    return unwrap<ConversationCounts>(
      await apiClient.GET("/v1/tenant/conversations/counts", {
        params: { header: header() },
      }),
      { silent: true },
    );
  },

  async agents() {
    const res = await unwrap<{ items: InboxAgent[] }>(
      await apiClient.GET("/v1/tenant/conversations/agents", {
        params: { header: header() },
      }),
    );
    return res.items;
  },

  async get(uuid: string) {
    return unwrap<ConversationDetail>(
      await apiClient.GET("/v1/tenant/conversations/{uuid}", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },

  async messages(uuid: string, before?: string) {
    return unwrap<MessagePage>(
      await apiClient.GET("/v1/tenant/conversations/{uuid}/messages", {
        params: {
          header: header(),
          path: { uuid },
          query: { limit: 50, ...(before ? { before } : {}) },
        },
      }),
    );
  },

  async post(uuid: string, body: MessageCreate) {
    return unwrap<Message>(
      await apiClient.POST("/v1/tenant/conversations/{uuid}/messages", {
        params: { header: header(), path: { uuid } },
        body,
      }),
    );
  },

  async retry(uuid: string, message: string) {
    return unwrap(
      await apiClient.POST(
        "/v1/tenant/conversations/{uuid}/messages/{message}/retry",
        { params: { header: header(), path: { uuid, message } } },
      ),
    );
  },

  async update(uuid: string, body: ConversationUpdate) {
    return unwrap<ConversationDetail>(
      await apiClient.PATCH("/v1/tenant/conversations/{uuid}", {
        params: { header: header(), path: { uuid } },
        body,
      }),
    );
  },

  async markRead(uuid: string) {
    return unwrap(
      await apiClient.POST("/v1/tenant/conversations/{uuid}/read", {
        params: { header: header(), path: { uuid } },
      }),
      { silent: true },
    );
  },

  /** Marks the conversation unread for the caller again (CF-247). */
  async markUnread(uuid: string) {
    return unwrap(
      await apiClient.POST("/v1/tenant/conversations/{uuid}/unread", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },

  async tags() {
    const res = await unwrap<{ items: Tag[] }>(
      await apiClient.GET("/v1/tenant/tags", { params: { header: header() } }),
    );
    return res.items;
  },

  async addTag(uuid: string, tag: { uuid?: string; name?: string }) {
    return unwrap<Tag>(
      await apiClient.POST("/v1/tenant/conversations/{uuid}/tags", {
        params: { header: header(), path: { uuid } },
        body: tag,
      }),
    );
  },

  async removeTag(uuid: string, tag: string) {
    return unwrap<Tag>(
      await apiClient.DELETE("/v1/tenant/conversations/{uuid}/tags/{tag}", {
        params: { header: header(), path: { uuid, tag } },
      }),
    );
  },

  async presence(uuid: string, typing: boolean) {
    const res = await unwrap<{ viewers: Viewer[] }>(
      await apiClient.POST("/v1/tenant/conversations/{uuid}/presence", {
        params: { header: header(), path: { uuid } },
        body: { typing },
      }),
      { silent: true },
    );
    return res.viewers;
  },

  async leave(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/conversations/{uuid}/presence", {
        params: { header: header(), path: { uuid } },
      }),
      { silent: true },
    );
  },

  async views() {
    const res = await unwrap<{ items: ConversationView[] }>(
      await apiClient.GET("/v1/tenant/conversation-views", {
        params: { header: header() },
      }),
    );
    return res.items;
  },

  async viewCounts() {
    const res = await unwrap<{ counts: Record<string, number> }>(
      await apiClient.GET("/v1/tenant/conversation-views/counts", {
        params: { header: header() },
      }),
      { silent: true },
    );
    return res.counts;
  },

  async createView(body: ConversationViewInput) {
    return unwrap<ConversationView>(
      await apiClient.POST("/v1/tenant/conversation-views", {
        params: { header: header() },
        body,
      }),
    );
  },

  async deleteView(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/conversation-views/{uuid}", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },

  async bulk(conversations: string[], action: BulkAction, value = "") {
    return unwrap<BulkResult>(
      await apiClient.POST("/v1/tenant/conversations/bulk", {
        params: { header: header() },
        body: { conversations, action, value },
      }),
    );
  },

  async undoBulk(operation: string) {
    return unwrap<BulkResult>(
      await apiClient.POST("/v1/tenant/conversations/bulk/undo/{uuid}", {
        params: { header: header(), path: { uuid: operation } },
      }),
    );
  },

  async reminders(uuid: string) {
    const res = await unwrap<{ items: Reminder[] }>(
      await apiClient.GET("/v1/tenant/conversations/{uuid}/reminders", {
        params: { header: header(), path: { uuid } },
      }),
    );
    return res.items;
  },

  async addReminder(uuid: string, body: ReminderInput) {
    return unwrap<Reminder>(
      await apiClient.POST("/v1/tenant/conversations/{uuid}/reminders", {
        params: { header: header(), path: { uuid } },
        body,
      }),
    );
  },

  async cancelReminder(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/conversation-reminders/{uuid}", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },
  // CF-98: merge / split, links, side conversations.

  /** Folds `source` into `uuid`; returns the target conversation. */
  async merge(uuid: string, source: string) {
    return unwrap<ConversationDetail>(
      await apiClient.POST("/v1/tenant/conversations/{uuid}/merge", {
        params: { header: header(), path: { uuid } },
        body: { source_uuid: source },
      }),
    );
  },

  /** Moves a message (and optionally the later ones) into a new child conversation. */
  async split(
    uuid: string,
    body: { message_uuid: string; following: boolean; subject?: string },
  ) {
    return unwrap<ConversationDetail>(
      await apiClient.POST("/v1/tenant/conversations/{uuid}/split", {
        params: { header: header(), path: { uuid } },
        body,
      }),
    );
  },

  async links(uuid: string) {
    return unwrap<ConversationLinks>(
      await apiClient.GET("/v1/tenant/conversations/{uuid}/links", {
        params: { header: header(), path: { uuid } },
      }),
      { silent: true },
    );
  },

  async link(uuid: string, other: string, relation: LinkRelation) {
    return unwrap<ConversationLinks>(
      await apiClient.POST("/v1/tenant/conversations/{uuid}/links", {
        params: { header: header(), path: { uuid } },
        body: { conversation_uuid: other, relation },
      }),
    );
  },

  async unlink(uuid: string, other: string) {
    return unwrap<ConversationLinks>(
      await apiClient.DELETE("/v1/tenant/conversations/{uuid}/links/{other}", {
        params: { header: header(), path: { uuid, other } },
      }),
    );
  },

  async sideList(uuid: string) {
    return unwrap<SideConversationList>(
      await apiClient.GET(
        "/v1/tenant/conversations/{uuid}/side-conversations",
        {
          params: { header: header(), path: { uuid } },
        },
      ),
      { silent: true },
    );
  },

  async sideStart(uuid: string, body: SideConversationCreate) {
    return unwrap<SideConversation>(
      await apiClient.POST(
        "/v1/tenant/conversations/{uuid}/side-conversations",
        {
          params: { header: header(), path: { uuid } },
          body,
        },
      ),
    );
  },

  async sideGet(uuid: string) {
    return unwrap<SideConversation>(
      await apiClient.GET("/v1/tenant/side-conversations/{uuid}", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },

  async sideReply(uuid: string, body: string) {
    return unwrap<SideConversation>(
      await apiClient.POST("/v1/tenant/side-conversations/{uuid}/messages", {
        params: { header: header(), path: { uuid } },
        body: { body },
      }),
    );
  },

  async sideStatus(uuid: string, status: "open" | "closed") {
    return unwrap<SideConversation>(
      await apiClient.PATCH("/v1/tenant/side-conversations/{uuid}", {
        params: { header: header(), path: { uuid } },
        body: { status },
      }),
    );
  },

  /** Opens an attachment through a short-lived signed URL (links cannot send X-Organization). */
  async openAttachment(uuid: string) {
    const res = await unwrap<{ url: string }>(
      await apiClient.POST("/v1/tenant/attachments/{uuid}/signed-url", {
        params: { header: header(), path: { uuid } },
        body: {},
      }),
    );
    window.open(`${apiConfig.baseUrl}${res.url}`, "_blank", "noopener");
  },

  /** Widget screenshots, recordings and masked technical context (CF-125). */
  async diagnostics(uuid: string) {
    const data = await unwrap<{ items?: WidgetDiagnostic[] }>(
      await apiClient.GET("/v1/tenant/conversations/{uuid}/diagnostics", {
        params: { header: header(), path: { uuid } },
      }),
    );
    return data.items ?? [];
  },

  /**
   * Same-origin URL of an attachment for `<img>` / `<video>` (elements cannot
   * send X-Organization, so a short-lived signed URL is used).
   */
  async attachmentSrc(uuid: string, ttlSeconds = 900) {
    const res = await unwrap<{ url: string }>(
      await apiClient.POST("/v1/tenant/attachments/{uuid}/signed-url", {
        params: { header: header(), path: { uuid } },
        body: { ttl_seconds: ttlSeconds },
      }),
    );
    return `${apiConfig.baseUrl}${res.url}`;
  },

  /** Uploads a composer file; it is claimed by the message on send. */
  async upload(file: File) {
    const form = new FormData();
    form.append("file", file);
    return unwrap<{ uuid: string; filename: string; content_type: string }>(
      await apiClient.POST("/v1/tenant/attachments", {
        params: { header: header() },
        body: form as never,
        bodySerializer: (b: unknown) => b as FormData,
      }),
    );
  },
};
