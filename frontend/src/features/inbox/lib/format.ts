import type { Conversation } from "../types";

/** Two-letter initials for avatars. */
export function initials(name: string | null | undefined): string {
  const parts = (name ?? "").trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  const first = parts[0]?.[0] ?? "";
  const last = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? "") : "";
  return (first + last).toLocaleUpperCase("tr");
}

/** Display name of a conversation's contact. */
export function contactLabel(c: Pick<Conversation, "contact">): string {
  return c.contact.name || c.contact.email || c.contact.phone || "—";
}

/** Short relative time: "şimdi", "5 dk", "3 sa", "2 g", else date. */
export function shortAgo(iso: string, locale: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const min = Math.floor(diff / 60_000);
  const tr = locale === "tr";
  if (min < 1) return tr ? "şimdi" : "now";
  if (min < 60) return tr ? `${min} dk` : `${min}m`;
  const h = Math.floor(min / 60);
  if (h < 24) return tr ? `${h} sa` : `${h}h`;
  const d = Math.floor(h / 24);
  if (d < 7) return tr ? `${d} g` : `${d}d`;
  return new Date(iso).toLocaleDateString(tr ? "tr-TR" : "en-US", {
    day: "numeric",
    month: "short",
  });
}

/** Compact "until" label: time today, else day + time. */
export function shortUntil(iso: string, locale: string): string {
  const d = new Date(iso);
  const tag = locale === "tr" ? "tr-TR" : "en-US";
  const sameDay = d.toDateString() === new Date().toDateString();
  return sameDay
    ? d.toLocaleTimeString(tag, { hour: "2-digit", minute: "2-digit" })
    : d.toLocaleString(tag, {
        day: "numeric",
        month: "short",
        hour: "2-digit",
        minute: "2-digit",
      });
}

export const PRIORITY_ORDER = ["urgent", "high", "normal", "low"] as const;

/** Tailwind classes of a priority marker. */
export const PRIORITY_TONE: Record<string, string> = {
  urgent: "text-red-600 dark:text-red-400",
  high: "text-amber-600 dark:text-amber-400",
  normal: "text-muted-foreground",
  low: "text-muted-foreground/60",
};

/** The customer waits for an answer only on an open or pending conversation. */
export function isWaitingStatus(status: string): boolean {
  return status === "open" || status === "pending";
}
