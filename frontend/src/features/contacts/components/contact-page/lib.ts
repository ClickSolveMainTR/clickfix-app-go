import {
  CalendarClock,
  CheckCircle2,
  Cog,
  Lightbulb,
  Megaphone,
  MessageSquare,
  MessagesSquare,
  ShieldCheck,
  Smile,
  Star,
  StickyNote,
  UserCheck,
  Zap,
  type LucideIcon,
} from "lucide-react";

import type { ContactTimelineKind } from "../../services/profile.service";

/** Icon and accent of every contact timeline kind (CF-221). */
export const KIND_META: Record<
  ContactTimelineKind,
  { icon: LucideIcon; tone: string }
> = {
  conversation_opened: {
    icon: MessageSquare,
    tone: "bg-signal-soft text-signal-text",
  },
  conversation_resolved: {
    icon: CheckCircle2,
    tone: "bg-success/15 text-success",
  },
  csat: { icon: Star, tone: "bg-warning/15 text-warning" },
  survey: { icon: Smile, tone: "bg-warning/15 text-warning" },
  note: { icon: StickyNote, tone: "bg-muted text-foreground" },
  appointment: {
    icon: CalendarClock,
    tone: "bg-signal-soft text-signal-text",
  },
  consent: { icon: UserCheck, tone: "bg-muted text-foreground" },
  privacy_request: {
    icon: ShieldCheck,
    tone: "bg-destructive/10 text-destructive",
  },
  event: { icon: Zap, tone: "bg-muted text-foreground" },
  feedback: { icon: Lightbulb, tone: "bg-warning/15 text-warning" },
  forum: { icon: MessagesSquare, tone: "bg-muted text-foreground" },
  status_notice: {
    icon: Megaphone,
    tone: "bg-destructive/10 text-destructive",
  },
  system: { icon: Cog, tone: "bg-muted text-muted-foreground" },
};

/** Filter chips of the timeline; each maps to one or more kinds. */
export const TIMELINE_GROUPS = {
  conversations: ["conversation_opened", "conversation_resolved"],
  satisfaction: ["csat", "survey"],
  notes: ["note"],
  appointments: ["appointment"],
  compliance: ["consent", "privacy_request"],
  product: ["event", "feedback", "forum", "status_notice"],
  system: ["system"],
} as const satisfies Record<string, readonly ContactTimelineKind[]>;

export type TimelineGroup = keyof typeof TIMELINE_GROUPS;

export const TABS = [
  "activity",
  "conversations",
  "company",
  "fields",
  "appointments",
  "feedback",
  "compliance",
] as const;
export type ContactTab = (typeof TABS)[number];

export function isTab(v: string | null): v is ContactTab {
  return Boolean(v) && (TABS as readonly string[]).includes(v as string);
}

export function isClosed(status: string) {
  return status === "resolved" || status === "closed";
}

/** Local calendar day of an ISO time, `yyyy-MM-dd`. */
export function dayKey(iso: string) {
  const d = new Date(iso);
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${d.getFullYear()}-${m}-${day}`;
}

export function dayLabel(
  key: string,
  locale: string,
  t: (k: string) => string,
) {
  const today = dayKey(new Date().toISOString());
  const yesterday = dayKey(new Date(Date.now() - 86_400_000).toISOString());
  if (key === today) return t("contacts.profile.today");
  if (key === yesterday) return t("contacts.profile.yesterday");
  const [y, m, d] = key.split("-").map(Number);
  return new Date(y, (m ?? 1) - 1, d ?? 1).toLocaleDateString(locale, {
    day: "numeric",
    month: "long",
    year: "numeric",
    weekday: "long",
  });
}

export function timeOf(iso: string, locale: string) {
  return new Date(iso).toLocaleTimeString(locale, {
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function dateOf(iso: string, locale: string) {
  return new Date(iso).toLocaleDateString(locale, {
    day: "numeric",
    month: "short",
    year: "numeric",
  });
}

export function dateTimeOf(iso: string, locale: string) {
  return new Date(iso).toLocaleString(locale, {
    dateStyle: "medium",
    timeStyle: "short",
  });
}

/** Human "in 2 h" / "3 h ago" distance for SLA due times. */
export function relative(iso: string, locale: string) {
  const diff = new Date(iso).getTime() - Date.now();
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  const min = Math.round(diff / 60_000);
  if (Math.abs(min) < 60) return rtf.format(min, "minute");
  const h = Math.round(min / 60);
  if (Math.abs(h) < 48) return rtf.format(h, "hour");
  return rtf.format(Math.round(h / 24), "day");
}
