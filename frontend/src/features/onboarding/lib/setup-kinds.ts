import {
  BookOpen,
  Bot,
  CalendarClock,
  ClipboardList,
  Clock,
  Columns3,
  Filter,
  FileText,
  FolderOpen,
  Gauge,
  Bell,
  MessageSquareQuote,
  Palette,
  Route,
  ShieldCheck,
  Smile,
  Sparkles,
  Tag,
  UsersRound,
  Wand2,
  Workflow,
  type LucideIcon,
} from "lucide-react";

import type { SetupItemKind } from "../services/setup.service";

/**
 * Plan item kinds in display order with their icon. Copy lives under
 * `onboarding.setup.kinds.<kind>` (plural label for the preview groups).
 */
export const SETUP_KINDS: { kind: SetupItemKind; icon: LucideIcon }[] = [
  { kind: "branding", icon: Palette },
  { kind: "business_hours", icon: Clock },
  { kind: "team", icon: UsersRound },
  { kind: "tag", icon: Tag },
  { kind: "status", icon: Columns3 },
  { kind: "sla_policy", icon: Gauge },
  { kind: "custom_field", icon: ClipboardList },
  { kind: "canned_reply", icon: MessageSquareQuote },
  { kind: "macro", icon: Wand2 },
  { kind: "form", icon: FileText },
  { kind: "kb_collection", icon: FolderOpen },
  { kind: "kb_article", icon: BookOpen },
  { kind: "widget", icon: Sparkles },
  { kind: "ai_persona", icon: Bot },
  { kind: "ai_guardrails", icon: ShieldCheck },
  { kind: "ai_procedure", icon: Workflow },
  { kind: "routing_rule", icon: Route },
  { kind: "view", icon: Filter },
  { kind: "survey", icon: Smile },
  { kind: "appointment_type", icon: CalendarClock },
  { kind: "notifications", icon: Bell },
];

export function kindIcon(kind: SetupItemKind): LucideIcon {
  return SETUP_KINDS.find((k) => k.kind === kind)?.icon ?? Sparkles;
}

/** Statuses that count as "done" on the build screen. */
export const FINISHED_ITEM = new Set([
  "created",
  "updated",
  "skipped",
  "unavailable",
  "failed",
]);
