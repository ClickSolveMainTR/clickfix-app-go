import { parseISO } from "date-fns";

import type { CalendarViewItem } from "@/features/calendar/components/views/types";
import { itemColor, LAYER_DEFS } from "@/features/calendar/lib/layers";
import type { CalendarFeedItem } from "@/features/calendar/types";

/** YYYY-MM-DD → local midnight (all-day items follow the wall calendar). */
export function localDay(day: string): Date {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(y, (m ?? 1) - 1, d ?? 1);
}

export function itemStart(item: CalendarFeedItem): Date {
  return parseISO(item.starts_at);
}

export function itemEnd(item: CalendarFeedItem): Date {
  return parseISO(item.ends_at);
}

/** Feed item → generic calendar view item (ported from otopoly-go). */
export function toViewItem(item: CalendarFeedItem): CalendarViewItem {
  return {
    id: item.id,
    title: item.title,
    subtitle: item.subtitle || null,
    start: itemStart(item),
    end: itemEnd(item),
    allDay: item.all_day,
    color: itemColor(item),
    editable: item.movable,
    // Appointments have a duration; shifts / time off are read-only.
    resizable: item.layer === "appointments",
    muted: item.status === "cancelled" || item.status === "no_show",
    icon: LAYER_DEFS[item.layer].icon,
  };
}
