import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { AppState } from 'react-native';

import { api, orgHeader, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { useSession } from '@/core/auth/session';
import { scheduleWidgetSync } from '@/features/widgets/sync';

import { presenceKeys } from './keys';

export { presenceKeys };

/**
 * Agent presence from the phone (CF-10 / CF-54): the app heartbeats while it
 * is in the foreground, so an agent working only on mobile counts as
 * available for routing, and the status can be changed from settings.
 */

export type PresenceMe = components['schemas']['PresenceMe'];
export type PresenceStatus = PresenceMe['status'];

export const PRESENCE_STATUSES: PresenceStatus[] = ['online', 'away', 'busy', 'offline'];

const HEARTBEAT_FALLBACK_MS = 30_000;


function useForeground() {
  const [active, setActive] = useState(AppState.currentState === 'active');
  useEffect(() => {
    const sub = AppState.addEventListener('change', (s) => setActive(s === 'active'));
    return () => sub.remove();
  }, []);
  return active;
}

/** The caller's presence; heartbeats every interval the server asks for while foregrounded. */
export function useMyPresence() {
  const org = useSession((s) => s.organization)?.slug ?? '';
  const foreground = useForeground();
  return useQuery({
    queryKey: presenceKeys.me(org),
    queryFn: () => unwrap<PresenceMe>(api.POST('/v1/tenant/presence/heartbeat', { params: { header: orgHeader() } })),
    enabled: !!org && foreground,
    refetchInterval: (q) => (q.state.data?.heartbeat_seconds ?? 0) * 1000 || HEARTBEAT_FALLBACK_MS,
    refetchIntervalInBackground: false,
  });
}

/** Changes the caller's status. */
export function useSetPresence() {
  const org = useSession((s) => s.organization)?.slug ?? '';
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (status: PresenceStatus) =>
      unwrap<PresenceMe>(api.PUT('/v1/tenant/presence/me', { params: { header: orgHeader() }, body: { status } })),
    onSuccess: (me) => {
      qc.setQueryData(presenceKeys.me(org), me);
      scheduleWidgetSync(true);
    },
  });
}
