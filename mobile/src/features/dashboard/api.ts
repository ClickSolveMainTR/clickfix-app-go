import { useQuery } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { useInboxScope } from '@/features/inbox/api';
import { inboxKeys } from '@/features/inbox/cache';
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
 * so the app and the widget always agree. Under the inbox key, so a live
 * inbox event refreshes it with the lists.
 */
export function useSlaAtRisk() {
  const { org, canRead } = useInboxScope();
  return useQuery({
    queryKey: [...inboxKeys.all(org), 'dashboard-sla'] as const,
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
