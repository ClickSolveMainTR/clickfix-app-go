import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type CalendarLayerInfo = Schemas["CalendarLayerInfo"];
export type CalendarLayer = CalendarLayerInfo["key"];
export type CalendarMeta = Schemas["CalendarMeta"];
export type CalendarMoveInput = Schemas["CalendarMoveInput"];
export type CalendarFeedItem = Schemas["CalendarFeedItem"];
export type CalendarFeed = Schemas["CalendarFeed"];

export type Appointment = Schemas["Appointment"];
export type AppointmentStatus = Appointment["status"];
export type AppointmentType = Schemas["AppointmentType"];
export type AppointmentTypeInput = Schemas["AppointmentTypeInput"];
export type AppointmentSettings = Schemas["AppointmentSettings"];
export type AppointmentSettingsInput = Schemas["AppointmentSettingsInput"];
export type AppointmentSlot = Schemas["AppointmentSlot"];
export type AppointmentBookInput = Schemas["AppointmentBookInput"];
export type AppointmentRescheduleInput = Schemas["AppointmentRescheduleInput"];
export type AppointmentStatusInput = Schemas["AppointmentStatusInput"];
export type AppointmentLocationKind = AppointmentType["location_kind"];
export type AppointmentHostMode = AppointmentType["host_mode"];

/** Every layer in display order (mirrors the API enum). */
export const CALENDAR_LAYERS: readonly CalendarLayer[] = [
  "appointments",
  "shifts",
  "time_off",
];

export const APPOINTMENT_STATUSES: readonly AppointmentStatus[] = [
  "scheduled",
  "confirmed",
  "completed",
  "no_show",
  "cancelled",
];

export const LOCATION_KINDS: readonly AppointmentLocationKind[] = [
  "video",
  "phone",
  "in_person",
  "custom_url",
];

export const HOST_MODES: readonly AppointmentHostMode[] = [
  "round_robin",
  "agents",
  "team",
];

/** Feed / list filters of the calendar. */
export type CalendarFilters = {
  hostUuids: string[];
  teamUuid: string;
  typeUuids: string[];
  statuses: AppointmentStatus[];
};
