import { useQuery } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { orgKey, useOrgSlug } from '@/core/api/org';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';

/** Status page incidents (`/v1/tenant/status/incidents`, `tenant.conversations.read`). */
export type Incident = components['schemas']['StatusIncident'];

export function useIncidents() {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: orgKey(org, 'incidents'),
    enabled: !!org && can(P.conversationsRead),
    staleTime: 60_000,
    refetchInterval: 2 * 60_000,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/status/incidents', { params: { header: orgHeader(), query: { limit: 30 } } })),
  });
}

/** Incidents still going on (not resolved). */
export function useActiveIncidents(): Incident[] {
  const q = useIncidents();
  return (q.data?.items ?? []).filter((i) => !i.resolved_at);
}
