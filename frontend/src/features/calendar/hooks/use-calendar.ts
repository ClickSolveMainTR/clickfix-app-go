"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import {
  appointmentsService,
  calendarService,
} from "@/features/calendar/services/calendar.service";
import type {
  AppointmentBookInput,
  AppointmentRescheduleInput,
  AppointmentSettingsInput,
  AppointmentStatusInput,
  AppointmentTypeInput,
  CalendarFeed,
  CalendarFilters,
  CalendarMoveInput,
} from "@/features/calendar/types";
import { useActiveOrganization } from "@/features/organizations";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useChannel } from "@/hooks/use-realtime";
import { RealtimeChannels } from "@/lib/realtime";

/** Query keys carry the organization slug. */
export const calendarQueryKeys = {
  all: (slug: string) => ["tenant", slug, "calendar"] as const,
  meta: (slug: string) => ["tenant", slug, "calendar", "meta"] as const,
  feeds: (slug: string) => ["tenant", slug, "calendar", "feed"] as const,
  feed: (slug: string, from: string, to: string, key: string) =>
    ["tenant", slug, "calendar", "feed", from, to, key] as const,
  appointments: (slug: string) =>
    ["tenant", slug, "calendar", "appointments"] as const,
  appointment: (slug: string, uuid: string) =>
    ["tenant", slug, "calendar", "appointments", "detail", uuid] as const,
  list: (slug: string, query: Record<string, string | undefined>) =>
    ["tenant", slug, "calendar", "appointments", "list", query] as const,
  upcoming: (slug: string, scope: string) =>
    ["tenant", slug, "calendar", "appointments", "upcoming", scope] as const,
  types: (slug: string) => ["tenant", slug, "calendar", "types"] as const,
  settings: (slug: string) => ["tenant", slug, "calendar", "settings"] as const,
  slots: (
    slug: string,
    type: string,
    from: string,
    to: string,
    extra: string,
  ) => ["tenant", slug, "calendar", "slots", type, from, to, extra] as const,
};

/** Layers the member may see / move, colours, statuses, types, time zone. */
export function useCalendarMeta(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: calendarQueryKeys.meta(slug),
    queryFn: () => calendarService.meta(slug),
    enabled: enabled && Boolean(slug),
    staleTime: 5 * 60_000,
    retry: false,
  });
}

function filterQuery(filters?: Partial<CalendarFilters>) {
  return {
    host_uuid: filters?.hostUuids?.length
      ? filters.hostUuids.join(",")
      : undefined,
    team_uuid: filters?.teamUuid || undefined,
    type_uuid: filters?.typeUuids?.length
      ? filters.typeUuids.join(",")
      : undefined,
    status: filters?.statuses?.length ? filters.statuses.join(",") : undefined,
  };
}

export function useCalendarFeed(
  from: string,
  to: string,
  layers: string[],
  enabled = true,
  filters?: Partial<CalendarFilters>,
) {
  const { slug } = useTenant();
  const query = {
    from,
    to,
    layers: [...layers].sort().join(","),
    ...filterQuery(filters),
  };
  const key = JSON.stringify(query);
  return useQuery({
    queryKey: calendarQueryKeys.feed(slug, from, to, key),
    queryFn: () => calendarService.feed(slug, query),
    enabled: enabled && Boolean(slug) && layers.length > 0,
    placeholderData: keepPreviousData,
    staleTime: 30_000,
  });
}

/**
 * Live calendar: `appointment.changed` on `org:{uuid}` invalidates every
 * calendar query of the organization (feed, lists, upcoming, types).
 */
export function useCalendarLive(enabled = true) {
  const { active } = useActiveOrganization();
  const { slug } = useTenant();
  const qc = useQueryClient();
  useChannel(
    enabled && active ? RealtimeChannels.org(active.uuid) : null,
    (message) => {
      if (message.type !== "appointment.changed") return;
      void qc.invalidateQueries({ queryKey: calendarQueryKeys.all(slug) });
    },
  );
}

/** Optimistically places a moved item in every cached feed. */
function placeInFeeds(
  feed: CalendarFeed | undefined,
  id: string,
  body: CalendarMoveInput,
): CalendarFeed | undefined {
  if (!feed) return feed;
  return {
    ...feed,
    items: feed.items.map((item) =>
      item.id === id
        ? { ...item, starts_at: body.starts_at, ends_at: body.ends_at }
        : item,
    ),
  };
}

export function useCalendarMutations() {
  const { slug } = useTenant();
  const qc = useQueryClient();
  const invalidate = () =>
    void qc.invalidateQueries({ queryKey: calendarQueryKeys.all(slug) });
  return {
    move: useMutation({
      mutationFn: ({ body }: { id: string; body: CalendarMoveInput }) =>
        calendarService.move(slug, body),
      onMutate: async ({ id, body }) => {
        await qc.cancelQueries({ queryKey: calendarQueryKeys.feeds(slug) });
        const snapshot = qc.getQueriesData<CalendarFeed>({
          queryKey: calendarQueryKeys.feeds(slug),
        });
        qc.setQueriesData<CalendarFeed>(
          { queryKey: calendarQueryKeys.feeds(slug) },
          (feed) => placeInFeeds(feed, id, body),
        );
        return { snapshot };
      },
      onError: (_err, _vars, ctx) => {
        for (const [key, data] of ctx?.snapshot ?? []) {
          qc.setQueryData(key, data);
        }
      },
      onSettled: invalidate,
    }),
    book: useMutation({
      mutationFn: (body: AppointmentBookInput) =>
        appointmentsService.book(slug, body),
      onSuccess: invalidate,
    }),
    reschedule: useMutation({
      mutationFn: ({
        uuid,
        body,
      }: {
        uuid: string;
        body: AppointmentRescheduleInput;
      }) => appointmentsService.reschedule(slug, uuid, body),
      onSuccess: invalidate,
    }),
    setStatus: useMutation({
      mutationFn: ({
        uuid,
        body,
      }: {
        uuid: string;
        body: AppointmentStatusInput;
      }) => appointmentsService.setStatus(slug, uuid, body),
      onSuccess: invalidate,
    }),
    updateNotes: useMutation({
      mutationFn: ({ uuid, notes }: { uuid: string; notes: string }) =>
        appointmentsService.updateDetails(slug, uuid, { notes }),
      onSuccess: invalidate,
    }),
  };
}

export function useAppointment(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: calendarQueryKeys.appointment(slug, uuid ?? ""),
    queryFn: () => appointmentsService.get(slug, uuid ?? ""),
    enabled: Boolean(slug && uuid),
    retry: false,
  });
}

/** Bookings linked to a contact, company or conversation. */
export function useLinkedAppointments(
  link: { contact?: string; company?: string; conversation?: string },
  enabled = true,
) {
  const { slug } = useTenant();
  const query = {
    contact_uuid: link.contact,
    company_uuid: link.company,
    conversation_uuid: link.conversation,
    order: "desc",
    limit: "20",
  };
  return useQuery({
    queryKey: calendarQueryKeys.list(slug, query),
    queryFn: () => appointmentsService.list(slug, query),
    enabled:
      enabled &&
      Boolean(slug && (link.contact || link.company || link.conversation)),
    retry: false,
  });
}

export function useUpcomingAppointments(scope: "mine" | "all", enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: calendarQueryKeys.upcoming(slug, scope),
    queryFn: () => appointmentsService.upcoming(slug, { scope, limit: 8 }),
    enabled: enabled && Boolean(slug),
    retry: false,
    staleTime: 60_000,
  });
}

export function useAppointmentTypes(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: calendarQueryKeys.types(slug),
    queryFn: () => appointmentsService.types(slug),
    enabled: enabled && Boolean(slug),
    retry: false,
  });
}

export function useAppointmentSettings(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: calendarQueryKeys.settings(slug),
    queryFn: () => appointmentsService.settings(slug),
    enabled: enabled && Boolean(slug),
    retry: false,
  });
}

export function useAppointmentSlots(
  typeUuid: string,
  from: string,
  to: string,
  duration: number,
  hostUuid: string,
) {
  const { slug } = useTenant();
  const extra = `${duration}:${hostUuid}`;
  return useQuery({
    queryKey: calendarQueryKeys.slots(slug, typeUuid, from, to, extra),
    queryFn: () =>
      appointmentsService.slots(slug, typeUuid, {
        from,
        to,
        duration: duration || undefined,
        host_uuid: hostUuid || undefined,
      }),
    enabled: Boolean(slug && typeUuid && from && to),
    retry: false,
  });
}

export function useAppointmentTypeMutations() {
  const { slug } = useTenant();
  const qc = useQueryClient();
  const invalidate = () =>
    void qc.invalidateQueries({ queryKey: calendarQueryKeys.all(slug) });
  return {
    create: useMutation({
      mutationFn: (body: AppointmentTypeInput) =>
        appointmentsService.createType(slug, body),
      onSuccess: invalidate,
    }),
    update: useMutation({
      mutationFn: ({
        uuid,
        body,
      }: {
        uuid: string;
        body: AppointmentTypeInput;
      }) => appointmentsService.updateType(slug, uuid, body),
      onSuccess: invalidate,
    }),
    remove: useMutation({
      mutationFn: (uuid: string) => appointmentsService.deleteType(slug, uuid),
      onSuccess: invalidate,
    }),
    saveSettings: useMutation({
      mutationFn: (body: AppointmentSettingsInput) =>
        appointmentsService.saveSettings(slug, body),
      onSuccess: invalidate,
    }),
  };
}
