import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { useSession } from '@/core/auth/session';

/**
 * The member's in-app notification inbox (`/v1/notifications`, channel
 * `inapp`, the same list as the panel's bell). Notifications belong to the
 * user, not the organization, so the keys are per user.
 */
export type AppNotification = components['schemas']['Notification'];

const PAGE = 30;
const CHANNEL = 'inapp';

export const notificationKeys = {
  all: (user: string) => ['notifications', user] as const,
  list: (user: string, unread: boolean) => [...notificationKeys.all(user), 'list', unread] as const,
  unread: (user: string) => [...notificationKeys.all(user), 'unread'] as const,
};

function useUser() {
  return useSession((s) => s.me?.user?.uuid ?? '');
}

export function useNotifications(unreadOnly: boolean) {
  const user = useUser();
  return useInfiniteQuery({
    queryKey: notificationKeys.list(user, unreadOnly),
    enabled: !!user,
    initialPageParam: 0,
    staleTime: 15_000,
    refetchInterval: 60_000,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/v1/notifications', {
          params: {
            query: {
              limit: PAGE,
              offset: pageParam,
              sort: '-created_at',
              channel: CHANNEL,
              ...(unreadOnly ? { unread: true } : {}),
            },
          },
        }),
      ),
    getNextPageParam: (last) =>
      last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined,
  });
}

/** Unread badge for the Notifications tab; refreshed every minute and on focus. */
export function useUnreadNotifications(): number {
  const user = useUser();
  const q = useQuery({
    queryKey: notificationKeys.unread(user),
    enabled: !!user,
    staleTime: 15_000,
    refetchInterval: 60_000,
    queryFn: () => unwrap(api.GET('/v1/notifications/unread-count')),
  });
  return Number(q.data?.count ?? 0);
}

export function useMarkNotificationRead() {
  const user = useUser();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (uuid: string) => unwrap(api.POST('/v1/notifications/{uuid}/read', { params: { path: { uuid } } })),
    onSettled: () => void qc.invalidateQueries({ queryKey: notificationKeys.all(user) }),
  });
}

export function useMarkAllNotificationsRead() {
  const user = useUser();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap(api.POST('/v1/notifications/read-all')),
    onSettled: () => void qc.invalidateQueries({ queryKey: notificationKeys.all(user) }),
  });
}
