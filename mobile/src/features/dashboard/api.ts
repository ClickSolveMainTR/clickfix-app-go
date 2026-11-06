import { useQuery } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { P, useCan } from '@/core/auth/permissions';
import { useSession } from '@/core/auth/session';
import { useInboxScope } from '@/features/inbox/api';
import { inboxKeys } from '@/features/inbox/cache';
import { notificationKeys } from '@/features/notifications/api';
import { type ApiConversation, SLA_WINDOW_MINUTES, slaAtRisk } from '@/features/widgets/snapshot-core';

type Page = { items: ApiConversation[] };

function list(query: Record<string, string | number>) {
  return unwrap(
    api.GET('/v1/tenant/conversations', { params: { header: orgHeader(), query: query as never } }),
  ) as Promise<Page>;
}

/**
 * My open conversations whose SLA is breached or due within the hour: the
 * two list calls the home-screen widget makes (`features/widgets/sync.ts`),
 * so the app and the widget always agree. It has its own key
 * (`inboxKeys.dashboardSla`), which the inbox invalidates next to its lists
 * (live events, replies, status changes, bulk actions); the screen also
 * refetches it when it comes back into focus.
 */
export function useSlaAtRisk() {
  const { org, canRead } = useInboxScope();
  return useQuery({
    queryKey: inboxKeys.dashboardSla(org),
    enabled: !!org && canRead,
    staleTime: 30_000,
    queryFn: async () => {
      const open = 'open,pending';
      const [breached, dueSoon] = await Promise.all([
        list({ status: open, assignee: 'me', sla_breached: '1', limit: 5 }),
        list({ status: open, assignee: 'me', sla_due_within: SLA_WINDOW_MINUTES, limit: 10 }),
      ]);
      return slaAtRisk([...breached.items, ...dueSoon.items], new Date());
    },
  });
}

/**
 * The organization's set-up checklist, the one the panel's home card reads
 * (`frontend/src/features/onboarding/hooks/use-onboarding.ts`): for members
 * with `tenant.settings.read` and for the owner. Shares its cache with the
 * settings set-up card.
 */
export function useOnboardingState() {
  const org = useSession((s) => s.organization);
  const can = useCan();
  const slug = org?.slug ?? '';
  const allowed = !!org && (can(P.settingsRead) || org.role.key === 'owner');
  return useQuery({
    queryKey: ['onboarding', slug, 'state'],
    enabled: !!slug && allowed,
    staleTime: 30_000,
    retry: false,
    queryFn: () => unwrap(api.GET('/v1/tenant/onboarding', { params: { header: orgHeader() } })),
  });
}

/** Unread notifications as a query (the tab badge's key), so the summary can show loading, failure and refresh. */
export function useUnreadCount() {
  const user = useSession((s) => s.me?.user?.uuid ?? '');
  return useQuery({
    queryKey: notificationKeys.unread(user),
    enabled: !!user,
    staleTime: 15_000,
    refetchInterval: 60_000,
    queryFn: () => unwrap(api.GET('/v1/notifications/unread-count')),
  });
}
