import { useTeamPresence } from './api';

/** Member names by uuid, from the presence list every member may read. */
export function useMemberNames(): (uuid: string | null | undefined) => string {
  const presence = useTeamPresence();
  const map = new Map((presence.data?.items ?? []).map((m) => [m.user_uuid, m.name]));
  return (uuid) => (uuid ? (map.get(uuid) ?? '—') : '—');
}

export const PRESENCE_DOT: Record<string, string> = {
  online: 'bg-success',
  away: 'bg-warning',
  busy: 'bg-destructive',
  offline: 'bg-muted-foreground',
};
