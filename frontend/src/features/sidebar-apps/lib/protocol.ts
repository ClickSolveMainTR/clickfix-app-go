import {
  Activity,
  Bell,
  BookOpen,
  Box,
  ChartColumn,
  Calendar,
  CreditCard,
  Database,
  FileText,
  Globe,
  Link as LinkIcon,
  LifeBuoy,
  Map as MapIcon,
  MessageSquare,
  Package,
  Puzzle,
  Settings,
  ShoppingCart,
  Star,
  Truck,
  User,
  Zap,
  type LucideIcon,
} from "lucide-react";
import { z } from "zod";

import type { SidebarAppScope } from "../services/sidebar-apps.service";

/*
 * Host side of the iframe protocol in backend/docs/design/workflows-apps.md.
 * The iframe may only send `clickfix.ready` and `clickfix.action`; anything
 * else, from any other window or origin, is dropped.
 */

export const SCOPES: SidebarAppScope[] = [
  "conversation.read",
  "contact.read",
  "conversation.reply",
  "conversation.note",
  "conversation.status",
  "conversation.priority",
  "conversation.assign",
  "conversation.tag",
];

/** Scopes that let the iframe ask for a change (the rest are context only). */
export const ACTION_SCOPES = [
  "conversation.reply",
  "conversation.note",
  "conversation.status",
  "conversation.priority",
  "conversation.assign",
  "conversation.tag",
] as const;
export type AppAction = (typeof ACTION_SCOPES)[number];

/** Matches backend apps/model.Scopes: the staff permission each scope needs. */
export const SCOPE_PERMISSION: Record<SidebarAppScope, string> = {
  "conversation.read": "tenant.conversations.read",
  "contact.read": "tenant.contacts.read",
  "conversation.reply": "tenant.conversations.reply",
  "conversation.note": "tenant.conversations.note",
  "conversation.status": "tenant.conversations.manage",
  "conversation.priority": "tenant.conversations.manage",
  "conversation.assign": "tenant.conversations.assign",
  "conversation.tag": "tenant.conversations.manage",
};

const readyMessage = z.object({ type: z.literal("clickfix.ready") }).strict();

const actionMessage = z
  .object({
    type: z.literal("clickfix.action"),
    request_id: z.uuid(),
    action: z.enum(ACTION_SCOPES),
    value: z.string().max(200).optional(),
    content: z.string().max(10_000).optional(),
    target_uuid: z.uuid().optional(),
  })
  .strict();

export const inboundMessage = z.discriminatedUnion("type", [
  readyMessage,
  actionMessage,
]);
export type InboundMessage = z.infer<typeof inboundMessage>;
export type ActionMessage = z.infer<typeof actionMessage>;

/** Request id of a malformed action, so the frame still gets a rejection. */
export function looseRequestId(data: unknown): string | null {
  if (!data || typeof data !== "object") return null;
  const msg = data as { type?: unknown; request_id?: unknown };
  if (msg.type !== "clickfix.action") return null;
  return z.uuid().safeParse(msg.request_id).success
    ? (msg.request_id as string)
    : null;
}

/** Inert icon identifiers offered in the registry form. */
export const APP_ICONS: Record<string, LucideIcon> = {
  puzzle: Puzzle,
  box: Box,
  package: Package,
  "shopping-cart": ShoppingCart,
  "credit-card": CreditCard,
  truck: Truck,
  user: User,
  "file-text": FileText,
  database: Database,
  chart: ChartColumn,
  bell: Bell,
  calendar: Calendar,
  link: LinkIcon,
  globe: Globe,
  star: Star,
  zap: Zap,
  "message-square": MessageSquare,
  "life-buoy": LifeBuoy,
  book: BookOpen,
  settings: Settings,
  map: MapIcon,
  activity: Activity,
};

export function appIcon(id: string | undefined): LucideIcon {
  return (id && APP_ICONS[id]) || Puzzle;
}

/** Client mirror of apps/model.Input.Validate for inline form errors. */
export function validAppUrl(raw: string): boolean {
  if (raw.length > 2048) return false;
  try {
    const u = new URL(raw);
    return (
      u.protocol === "https:" &&
      Boolean(u.hostname) &&
      !u.username &&
      !u.password &&
      !u.hash &&
      !raw.includes("#")
    );
  } catch {
    return false;
  }
}
