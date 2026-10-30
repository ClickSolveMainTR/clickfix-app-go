import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';

/** The member's signed-in devices (`/v1/auth/sessions`), as on the panel's security page. */

export type AuthSession = components['schemas']['AuthSession'];

const key = ['auth', 'sessions'] as const;

export function useSessions() {
  return useQuery({ queryKey: key, staleTime: 0, queryFn: () => unwrap(api.GET('/v1/auth/sessions')) });
}

export function useRevokeSession() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (uuid: string) => unwrap(api.DELETE('/v1/auth/sessions/{uuid}', { params: { path: { uuid } } })),
    onSettled: () => void qc.invalidateQueries({ queryKey: key }),
  });
}

export function useRevokeOtherSessions() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap(api.POST('/v1/auth/sessions/revoke-others')),
    onSettled: () => void qc.invalidateQueries({ queryKey: key }),
  });
}
