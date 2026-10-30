import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';
import { useSession } from '@/core/auth/session';
import { useChannel } from '@/features/realtime/client';

/**
 * Appointments (CF-138): the upcoming agenda and one appointment, the same
 * endpoints as the panel's calendar. `appointment.changed` on
 * `org:{uuid}` refreshes them.
 */

export type Appointment = components['schemas']['Appointment'];
export type AppointmentStatus = Appointment['status'];
export type AgendaScope = 'mine' | 'all';

export const appointmentKeys = {
  all: (org: string) => ['appointments', org] as const,
  upcoming: (org: string, scope: AgendaScope) => ['appointments', org, 'upcoming', scope] as const,
  detail: (org: string, uuid: string) => ['appointments', org, 'detail', uuid] as const,
};

export function useAppointmentScope() {
  const org = useSession((s) => s.organization);
  const can = useCan();
  return {
    org: org?.slug ?? '',
    orgUuid: org?.uuid ?? '',
    canRead: can(P.appointmentsRead),
    canWrite: can(P.appointmentsWrite),
  };
}

/** Refetch appointment queries when the organization's calendar changes. */
export function useAppointmentsLive() {
  const { org, orgUuid } = useAppointmentScope();
  const qc = useQueryClient();
  useChannel(orgUuid ? `org:${orgUuid}` : null, (message) => {
    if ((message as { type?: string }).type !== 'appointment.changed') return;
    void qc.invalidateQueries({ queryKey: appointmentKeys.all(org) });
  });
}

export function useUpcomingAppointments(scope: AgendaScope) {
  const { org, canRead } = useAppointmentScope();
  return useQuery({
    queryKey: appointmentKeys.upcoming(org, scope),
    enabled: !!org && canRead,
    staleTime: 30_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/appointments/upcoming', {
          params: { header: orgHeader(), query: { scope, limit: 50 } },
        }),
      ),
  });
}

export function useAppointment(uuid: string) {
  const { org, canRead } = useAppointmentScope();
  return useQuery({
    queryKey: appointmentKeys.detail(org, uuid),
    enabled: !!org && !!uuid && canRead,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/appointments/{uuid}', {
          params: { header: orgHeader(), path: { uuid } },
        }),
      ),
  });
}

export function useSetAppointmentStatus(uuid: string) {
  const { org } = useAppointmentScope();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { status: AppointmentStatus; reason?: string }) =>
      unwrap(
        api.POST('/v1/tenant/appointments/{uuid}/status', {
          params: { header: orgHeader(), path: { uuid } },
          body,
        }),
      ),
    onSuccess: (a) => {
      qc.setQueryData(appointmentKeys.detail(org, uuid), a);
      void qc.invalidateQueries({ queryKey: appointmentKeys.all(org) });
    },
  });
}

/** Status pill tone: never color alone (StatusPill also prints the label). */
export function statusTone(status: AppointmentStatus): 'neutral' | 'progress' | 'done' | 'problem' {
  switch (status) {
    case 'scheduled':
      return 'progress';
    case 'confirmed':
    case 'completed':
      return 'done';
    default:
      return 'problem';
  }
}

export function useAppointmentTypes(enabled = true) {
  const { org, canRead } = useAppointmentScope();
  return useQuery({
    queryKey: [...appointmentKeys.all(org), 'types'],
    enabled: !!org && canRead && enabled,
    queryFn: () => unwrap(api.GET('/v1/tenant/appointment-types', { params: { header: orgHeader() } })),
  });
}
export function useBookAppointment() {
  const { org } = useAppointmentScope();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: components['schemas']['AppointmentBookInput']) =>
      unwrap(api.POST('/v1/tenant/appointments', { params: { header: orgHeader() }, body })),
    onSuccess: (appointment) => {
      qc.setQueryData(appointmentKeys.detail(org, appointment.uuid), appointment);
      void qc.invalidateQueries({ queryKey: appointmentKeys.all(org) });
    },
  });
}

/** Calendar uses the bounded range endpoint, including past events in the visible month. */
export function useCalendarAppointments(scope: AgendaScope, from: string, to: string) {
  const { org, canRead } = useAppointmentScope();
  const me = useSession((s) => s.me?.user?.uuid);
  return useQuery({
    queryKey: [...appointmentKeys.all(org), 'calendar', scope, from, to],
    enabled: !!org && canRead && (scope === 'all' || !!me),
    staleTime: 30_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/appointments', {
          params: {
            header: orgHeader(),
            query: { from, to, ...(scope === 'mine' ? { host_uuid: me } : {}), order: 'asc', limit: 500 },
          },
        }),
      ),
  });
}

export function useRescheduleAppointment(uuid: string) {
  const { org } = useAppointmentScope();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: components['schemas']['AppointmentRescheduleInput']) =>
      unwrap(
        api.POST('/v1/tenant/appointments/{uuid}/reschedule', {
          params: { header: orgHeader(), path: { uuid } },
          body,
        }),
      ),
    onSuccess: (appointment) => {
      qc.setQueryData(appointmentKeys.detail(org, uuid), appointment);
      void qc.invalidateQueries({ queryKey: appointmentKeys.all(org) });
    },
  });
}
export function useAppointmentSlots(type: string | null, day: Date, duration: number) {
  const { org, canRead } = useAppointmentScope();
  const date = (value: Date) =>
    `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, '0')}-${String(value.getDate()).padStart(2, '0')}`;
  const from = date(day);
  const end = new Date(day);
  end.setDate(end.getDate() + 1);
  const to = date(end);
  return useQuery({
    queryKey: [...appointmentKeys.all(org), 'slots', type, from, duration],
    enabled: !!org && canRead && !!type,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/appointment-types/{uuid}/slots', {
          params: {
            header: orgHeader(),
            path: { uuid: type! },
            query: { from, to, ...(duration ? { duration } : {}) },
          },
        }),
      ),
  });
}
