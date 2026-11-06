import { useQuery } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { orgKey, useOrgSlug } from '@/core/api/org';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';

/**
 * Sidebar apps (CF-172): the registry only (`GET /v1/tenant/apps`, which
 * needs `tenant.conversations.read` like on the panel). The app does NOT
 * launch them: no context token is requested and no app URL is loaded. See
 * `apps-screen.tsx` for why.
 */
export type SidebarApp = components['schemas']['SidebarApp'];

export function useSidebarApps(enabled = true) {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: [...orgKey(org, 'apps'), 'list'],
    enabled: !!org && enabled && can(P.conversationsRead),
    staleTime: 60_000,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/apps', { params: { header: orgHeader(), query: { limit: 100, offset: 0 } } })),
  });
}
