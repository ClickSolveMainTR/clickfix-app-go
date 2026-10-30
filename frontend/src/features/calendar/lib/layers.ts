import {
  CalendarClock,
  CalendarOff,
  Clock,
  type LucideIcon,
} from "lucide-react";

import {
  CALENDAR_COLORS,
  type CalendarColor,
} from "@/features/calendar/components/views/types";
import type {
  AppointmentHostMode,
  AppointmentLocationKind,
  AppointmentStatus,
  CalendarFeedItem,
  CalendarLayer,
} from "@/features/calendar/types";

type LayerDef = {
  color: CalendarColor;
  icon: LucideIcon;
  /** i18n key of the layer label (static so check-i18n can verify it). */
  labelKey: string;
};

/** Colour, icon and label of each calendar layer (ported from otopoly-go). */
export const LAYER_DEFS: Record<CalendarLayer, LayerDef> = {
  appointments: {
    color: "blue",
    icon: CalendarClock,
    labelKey: "calendar.layers.appointments",
  },
  shifts: {
    color: "slate",
    icon: Clock,
    labelKey: "calendar.layers.shifts",
  },
  time_off: {
    color: "rose",
    icon: CalendarOff,
    labelKey: "calendar.layers.time_off",
  },
};

const isColor = (c: unknown): c is CalendarColor =>
  typeof c === "string" && (CALENDAR_COLORS as readonly string[]).includes(c);

/** Appointments take their type's colour; other layers the layer colour. */
export function itemColor(item: CalendarFeedItem): CalendarColor {
  if (item.layer === "appointments" && isColor(item.color)) return item.color;
  return LAYER_DEFS[item.layer].color;
}

export const STATUS_KEYS: Record<AppointmentStatus, string> = {
  scheduled: "calendar.status.scheduled",
  confirmed: "calendar.status.confirmed",
  completed: "calendar.status.completed",
  no_show: "calendar.status.no_show",
  cancelled: "calendar.status.cancelled",
};

export function statusKey(status?: string | null): string | null {
  return (status && STATUS_KEYS[status as AppointmentStatus]) || null;
}

export function statusTone(
  status?: string | null,
): "default" | "success" | "warning" | "danger" {
  switch (status) {
    case "confirmed":
    case "completed":
      return "success";
    case "scheduled":
      return "warning";
    case "no_show":
    case "cancelled":
      return "danger";
    default:
      return "default";
  }
}

export const LOCATION_KEYS: Record<AppointmentLocationKind, string> = {
  video: "appointments.location.video",
  phone: "appointments.location.phone",
  in_person: "appointments.location.in_person",
  custom_url: "appointments.location.custom_url",
};

export const HOST_MODE_KEYS: Record<AppointmentHostMode, string> = {
  round_robin: "appointments.host_mode.round_robin",
  agents: "appointments.host_mode.agents",
  team: "appointments.host_mode.team",
};
