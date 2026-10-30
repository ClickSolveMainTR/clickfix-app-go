import type {
  Appointment,
  AppointmentBookInput,
  AppointmentRescheduleInput,
  AppointmentSettings,
  AppointmentSettingsInput,
  AppointmentSlot,
  AppointmentStatusInput,
  AppointmentType,
  AppointmentTypeInput,
  CalendarFeed,
  CalendarFeedItem,
  CalendarMeta,
  CalendarMoveInput,
} from "@/features/calendar/types";
import { tenantRequest } from "@/lib/api/tenant-request";

type Query = Record<string, string | number | undefined>;

/** Organization calendar (ported from otopoly-go, `/v1/tenant/calendar`). */
export const calendarService = {
  meta(slug: string) {
    return tenantRequest<CalendarMeta>(slug, "GET", "/v1/tenant/calendar/meta");
  },
  feed(slug: string, query: Query) {
    return tenantRequest<CalendarFeed>(
      slug,
      "GET",
      "/v1/tenant/calendar/feed",
      {
        query,
        silent: true,
      },
    );
  },
  move(slug: string, body: CalendarMoveInput) {
    return tenantRequest<CalendarFeedItem>(
      slug,
      "POST",
      "/v1/tenant/calendar/items/move",
      { body },
    );
  },
};

const BASE = "/v1/tenant/appointments";

/** Appointment types, settings and bookings (CF-138). */
export const appointmentsService = {
  settings(slug: string) {
    return tenantRequest<AppointmentSettings>(slug, "GET", `${BASE}/settings`, {
      silent: true,
    });
  },
  saveSettings(slug: string, body: AppointmentSettingsInput) {
    return tenantRequest<AppointmentSettings>(slug, "PUT", `${BASE}/settings`, {
      body,
    });
  },
  types(slug: string) {
    return tenantRequest<{ items: AppointmentType[] }>(
      slug,
      "GET",
      "/v1/tenant/appointment-types",
      { silent: true },
    );
  },
  createType(slug: string, body: AppointmentTypeInput) {
    return tenantRequest<AppointmentType>(
      slug,
      "POST",
      "/v1/tenant/appointment-types",
      {
        body,
      },
    );
  },
  updateType(slug: string, uuid: string, body: AppointmentTypeInput) {
    return tenantRequest<AppointmentType>(
      slug,
      "PUT",
      `/v1/tenant/appointment-types/${uuid}`,
      {
        body,
      },
    );
  },
  deleteType(slug: string, uuid: string) {
    return tenantRequest<{ deleted: boolean }>(
      slug,
      "DELETE",
      `/v1/tenant/appointment-types/${uuid}`,
    );
  },
  slots(slug: string, typeUuid: string, query: Query) {
    return tenantRequest<{ items: AppointmentSlot[] }>(
      slug,
      "GET",
      `/v1/tenant/appointment-types/${typeUuid}/slots`,
      { query, silent: true },
    );
  },
  list(slug: string, query: Query) {
    return tenantRequest<{ items: Appointment[] }>(slug, "GET", BASE, {
      query,
      silent: true,
    });
  },
  upcoming(slug: string, query: Query) {
    return tenantRequest<{ items: Appointment[] }>(
      slug,
      "GET",
      `${BASE}/upcoming`,
      { query, silent: true },
    );
  },
  get(slug: string, uuid: string) {
    return tenantRequest<Appointment>(slug, "GET", `${BASE}/${uuid}`, {
      silent: true,
    });
  },
  book(slug: string, body: AppointmentBookInput) {
    return tenantRequest<Appointment>(slug, "POST", BASE, { body });
  },
  reschedule(slug: string, uuid: string, body: AppointmentRescheduleInput) {
    return tenantRequest<Appointment>(
      slug,
      "POST",
      `${BASE}/${uuid}/reschedule`,
      { body },
    );
  },
  setStatus(slug: string, uuid: string, body: AppointmentStatusInput) {
    return tenantRequest<Appointment>(slug, "POST", `${BASE}/${uuid}/status`, {
      body,
    });
  },
  updateDetails(
    slug: string,
    uuid: string,
    body: { title?: string; notes?: string; location_value?: string },
  ) {
    return tenantRequest<Appointment>(slug, "PATCH", `${BASE}/${uuid}`, {
      body,
    });
  },
};
