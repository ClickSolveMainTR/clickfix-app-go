import type { components } from "@/generated/api";

export type Conversation = components["schemas"]["Conversation"];
export type ConversationDetail = components["schemas"]["ConversationDetail"];
export type ConversationPage = components["schemas"]["ConversationPage"];
export type ConversationCounts = components["schemas"]["ConversationCounts"];
export type ConversationStatus = components["schemas"]["ConversationStatus"];
export type ConversationPriority =
  components["schemas"]["ConversationPriority"];
export type ConversationUpdate = components["schemas"]["ConversationUpdate"];
export type Message = components["schemas"]["Message"];
export type MessagePage = components["schemas"]["MessagePage"];
export type MessageCreate = components["schemas"]["MessageCreate"];
export type Tag = components["schemas"]["Tag"];
export type CustomStatus = components["schemas"]["ConversationCustomStatus"];
export type InboxAgent = components["schemas"]["InboxAgent"];
export type Viewer = components["schemas"]["ConversationViewer"];
export type ConversationView = components["schemas"]["ConversationView"];
export type ConversationViewFilters =
  components["schemas"]["ConversationViewFilters"];
export type ConversationViewInput =
  components["schemas"]["ConversationViewInput"];
export type ConversationLinks = components["schemas"]["ConversationLinks"];
export type LinkedConversation = components["schemas"]["LinkedConversation"];
export type LinkRelation =
  components["schemas"]["ConversationLinkRequest"]["relation"];
export type SideConversation = components["schemas"]["SideConversation"];
export type SideConversationList =
  components["schemas"]["SideConversationList"];
export type SideConversationCreate =
  components["schemas"]["SideConversationCreate"];

/** Inbox views (sidebar + 1/2/3/4 shortcuts). */
export type InboxView =
  | "all"
  | "mine"
  | "teams"
  | "unassigned"
  | "mentions"
  | "sla_soon"
  | "breached";

/** Status tabs: open covers open + pending. */
export type InboxStatusTab = "open" | "snoozed" | "resolved" | "all";

export type InboxFilters = {
  view: InboxView;
  status: InboxStatusTab;
  q: string;
  unread: boolean;
  sort: "-last_message_at" | "last_message_at" | "waiting_since" | "-priority";
  /** Contact / company uuid (palette hits deep-link with ?contact, ?company). */
  contact?: string;
  company?: string;
  /** Advanced filters (csv in the URL: ?pr, ?ch, ?tg). */
  priorities?: string[];
  channelTypes?: string[];
  tags?: string[];
  /** Original brand uuid of the conversation (?br, CF-124). */
  brand?: string;
  /** Contact identity state (?id, CF-195). */
  identity?: "verified" | "anonymous" | "self_named";
  /** A saved view's stored filters, applied on top (CF-47). */
  saved?: ConversationViewFilters;
};

/** A message the composer sent that the server has not confirmed yet. */
export type OptimisticMessage = Message & { optimistic?: true };

/** Realtime event on `inbox:{org_uuid}` or `inbox:{org_uuid}:{user_uuid}` (backend model.LiveEvent). */
export type InboxLiveEvent = {
  type:
    | "conversation.created"
    | "conversation.updated"
    | "message.created"
    | "message.updated"
    | "conversation.viewers"
    | "ai.job_finished";
  conversation_uuid: string;
  message?: Message;
  viewers?: Viewer[];
  actor_uuid?: string;
};

export type BulkResult = components["schemas"]["ConversationBulkResult"];
export type BulkAction =
  components["schemas"]["ConversationBulkInput"]["action"];
export type Reminder = components["schemas"]["Reminder"];
export type ReminderInput = components["schemas"]["ReminderInput"];
