import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type PublicBookingOrganization = Schemas["PublicBookingOrganization"];
export type PublicBookingType = Schemas["PublicBookingType"];
export type PublicBookingTypeDetail = Schemas["PublicBookingTypeDetail"];
export type PublicBookingSlot = Schemas["PublicBookingSlot"];
export type PublicBookingInput = Schemas["PublicBookingInput"];
export type PublicAppointment = Schemas["PublicAppointment"];
export type PublicField = Schemas["PublicRequestFormField"];

const enc = encodeURIComponent;

/**
 * Public booking API (CF-138, no session) through the same-origin BFF.
 * Errors are rendered by the pages (silent).
 */
export const bookingService = {
  index(org: string) {
    return platformRequest<PublicBookingOrganization>(
      "GET",
      `/v1/public/booking/${enc(org)}`,
      { silent: true },
    );
  },
  type(org: string, type: string) {
    return platformRequest<PublicBookingTypeDetail>(
      "GET",
      `/v1/public/booking/${enc(org)}/${enc(type)}`,
      { silent: true },
    );
  },
  slots(org: string, type: string, from: string, to: string, duration: number) {
    return platformRequest<{ items: PublicBookingSlot[] }>(
      "GET",
      `/v1/public/booking/${enc(org)}/${enc(type)}/slots`,
      { query: { from, to, duration: duration || undefined }, silent: true },
    );
  },
  book(org: string, type: string, body: PublicBookingInput) {
    return platformRequest<PublicAppointment>(
      "POST",
      `/v1/public/booking/${enc(org)}/${enc(type)}`,
      { body, silent: true },
    );
  },
  manage(token: string) {
    return platformRequest<PublicAppointment>(
      "GET",
      `/v1/public/appointments/${enc(token)}`,
      { silent: true },
    );
  },
  manageSlots(token: string, from: string, to: string) {
    return platformRequest<{ items: PublicBookingSlot[] }>(
      "GET",
      `/v1/public/appointments/${enc(token)}/slots`,
      { query: { from, to }, silent: true },
    );
  },
  cancel(token: string, reason: string) {
    return platformRequest<PublicAppointment>(
      "POST",
      `/v1/public/appointments/${enc(token)}/cancel`,
      { body: { reason }, silent: true },
    );
  },
  reschedule(token: string, startsAt: string) {
    return platformRequest<PublicAppointment>(
      "POST",
      `/v1/public/appointments/${enc(token)}/reschedule`,
      { body: { starts_at: startsAt }, silent: true },
    );
  },
};
