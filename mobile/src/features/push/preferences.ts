import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';

import { applyUpdate } from './preferences-core';

export { applyUpdate, channelOn } from './preferences-core';

/** The member's notification preferences (`/v1/notification-preferences`, same as the panel's account page). */

export type NotificationPreferences = components['schemas']['NotificationPreferences'];
export type NotificationPreferencesUpdate = components['schemas']['NotificationPreferencesUpdate'];
export type NotificationEventType = components['schemas']['NotificationEventType'];

const keys = { prefs: ['notification-preferences'] as const, events: ['notification-preferences', 'events'] as const };

export function useNotificationPreferences() {
  return useQuery({
    queryKey: keys.prefs,
    queryFn: () => unwrap(api.GET('/v1/notification-preferences')),
  });
}

export function useNotificationEvents() {
  return useQuery({
    queryKey: keys.events,
    staleTime: 60 * 60_000,
    queryFn: () => unwrap(api.GET('/v1/notification-preferences/events')),
  });
}

/** Merge update; the screen shows the change at once and rolls back on failure. */
export function useUpdateNotificationPreferences() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: NotificationPreferencesUpdate) => unwrap(api.PUT('/v1/notification-preferences', { body })),
    onMutate: async (body) => {
      await qc.cancelQueries({ queryKey: keys.prefs });
      const prev = qc.getQueryData<NotificationPreferences>(keys.prefs);
      if (prev) qc.setQueryData<NotificationPreferences>(keys.prefs, applyUpdate(prev, body));
      return { prev };
    },
    onError: (_e, _b, ctx) => ctx?.prev && qc.setQueryData(keys.prefs, ctx.prev),
    onSuccess: (p) => qc.setQueryData(keys.prefs, p),
  });
}
