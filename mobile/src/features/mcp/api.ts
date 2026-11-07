import { useQuery } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { orgKey, useOrgSlug } from '@/core/api/org';
import { P, useCan } from '@/core/auth/permissions';

/**
 * The MCP connection guide of the member in the active organization
 * (`GET /v1/tenant/mcp/guide`, CF-173). `tenant.settings.read` is the API's
 * MCP connect permission; without it nothing is requested. The answer holds
 * addresses and tool names only, never a key or a token.
 */
export function useMcpScope() {
  const org = useOrgSlug();
  const can = useCan();
  return { org, canConnect: can(P.settingsRead) };
}

export function useMcpGuide() {
  const { org, canConnect } = useMcpScope();
  return useQuery({
    queryKey: orgKey(org, 'mcp', 'guide'),
    enabled: !!org && canConnect,
    staleTime: 60_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/mcp/guide', { params: { header: orgHeader() } })),
  });
}
