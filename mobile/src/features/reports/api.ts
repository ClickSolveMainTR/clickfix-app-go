import { useQuery } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { orgKey, useOrgSlug } from '@/core/api/org';
import type { components } from '@/core/api/schema';
import { displayTimeZone } from '@/core/format';

import { type Period, periodRange } from './core';

/** Support analytics (`/v1/tenant/analytics/dashboard`, needs `tenant.conversations.read_all`). */
export type AnalyticsCards = components['schemas']['AnalyticsCards'];

export { delta, type Period } from './core';

export function useDashboard(days: Period, filter: { agent?: string; team?: string }) {
  const org = useOrgSlug();
  const range = periodRange(days);
  return useQuery({
    queryKey: orgKey(org, 'analytics', days, filter.agent ?? null, filter.team ?? null, range.from.slice(0, 10)),
    enabled: !!org,
    staleTime: 5 * 60_000,
    placeholderData: (prev) => prev,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/analytics/dashboard', {
          params: {
            header: orgHeader(),
            query: {
              ...range,
              timezone: displayTimeZone(),
              ...(filter.agent ? { agent: filter.agent } : {}),
              ...(filter.team ? { team: filter.team } : {}),
            },
          },
        }),
      ),
  });
}
