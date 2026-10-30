import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { orgKey, useOrgSlug } from '@/core/api/org';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';
import { clientId } from '@/features/inbox/helpers';

/**
 * The team (CF-79): who is online (`/v1/tenant/presence`), team chat
 * (`/v1/tenant/team-chat`, CF-106) and on-call (`/v1/tenant/oncall/*`).
 */
export type PresenceMember = components['schemas']['PresenceMember'];
export type Thread = components['schemas']['InternalThread'];
export type ThreadMessage = components['schemas']['InternalMessage'];
export type OncallRota = components['schemas']['OncallRota'];
export type OncallAlert = components['schemas']['OncallAlert'];

export const teamKeys = {
  all: (org: string) => orgKey(org, 'team'),
  presence: (org: string) => [...teamKeys.all(org), 'presence'] as const,
  threads: (org: string) => [...teamKeys.all(org), 'threads'] as const,
  messages: (org: string, uuid: string) => [...teamKeys.all(org), 'messages', uuid] as const,
  rotas: (org: string) => [...teamKeys.all(org), 'rotas'] as const,
  current: (org: string, uuid: string) => [...teamKeys.all(org), 'oncall', uuid] as const,
  alerts: (org: string) => [...teamKeys.all(org), 'alerts'] as const,
};

export function useTeamPresence() {
  const org = useOrgSlug();
  return useQuery({
    queryKey: teamKeys.presence(org),
    enabled: !!org,
    staleTime: 15_000,
    refetchInterval: 30_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/presence', { params: { header: orgHeader() } })),
  });
}

export function useThreads() {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: teamKeys.threads(org),
    enabled: !!org && can(P.teamChat),
    staleTime: 15_000,
    refetchInterval: 30_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/team-chat', { params: { header: orgHeader(), query: { limit: 100 } } })),
  });
}

/** Unread team chat messages (the More hub badge). */
export function useTeamChatUnread() {
  const threads = useThreads();
  return (threads.data?.items ?? []).reduce((n, th) => n + (th.unread ?? 0), 0);
}

export function useThreadMessages(uuid: string) {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: teamKeys.messages(org, uuid),
    enabled: !!org && !!uuid && can(P.teamChat),
    staleTime: 5_000,
    refetchInterval: 10_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/team-chat/{uuid}/messages', {
          params: { header: orgHeader(), path: { uuid }, query: { limit: 100 } },
        }),
      ),
  });
}

export function useSendThreadMessage(uuid: string) {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { body: string; mentions?: string[] }) =>
      unwrap(
        api.POST('/v1/tenant/team-chat/{uuid}/messages', {
          params: { header: orgHeader(), path: { uuid } },
          body: { body: v.body, client_id: clientId(), ...(v.mentions?.length ? { mentions: v.mentions } : {}) },
        }),
      ),
    onSuccess: (m) =>
      qc.setQueryData<ThreadMessage[]>(teamKeys.messages(org, uuid), (cur) =>
        cur && !cur.some((x) => x.uuid === m.uuid) ? [...cur, m] : cur,
      ),
    onSettled: () => void qc.invalidateQueries({ queryKey: teamKeys.threads(org) }),
  });
}

export function useAskAi(uuid: string) {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (question: string) =>
      unwrap(
        api.POST('/v1/tenant/team-chat/{uuid}/ask-ai', {
          params: { header: orgHeader(), path: { uuid } },
          body: { question },
        }),
      ),
    onSettled: () => void qc.invalidateQueries({ queryKey: teamKeys.messages(org, uuid) }),
  });
}

export function useMarkThreadRead(uuid: string) {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      unwrap(api.POST('/v1/tenant/team-chat/{uuid}/read', { params: { header: orgHeader(), path: { uuid } } })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: teamKeys.threads(org) }),
  });
}

export function useCreateThread() {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: components['schemas']['InternalThreadInput']) =>
      unwrap(api.POST('/v1/tenant/team-chat', { params: { header: orgHeader() }, body })),
    onSettled: () => void qc.invalidateQueries({ queryKey: teamKeys.threads(org) }),
  });
}

export function useRotas() {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: teamKeys.rotas(org),
    enabled: !!org && can(P.schedulesRead),
    staleTime: 5 * 60_000,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/oncall/rotas', { params: { header: orgHeader(), query: { limit: 50 } } })),
  });
}

export function useOncallNow(rota: string) {
  const org = useOrgSlug();
  return useQuery({
    queryKey: teamKeys.current(org, rota),
    enabled: !!org && !!rota,
    staleTime: 60_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/oncall/rotas/{uuid}/current', { params: { header: orgHeader(), path: { uuid: rota } } }),
      ),
  });
}

export function useOncallAlerts() {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: teamKeys.alerts(org),
    enabled: !!org && can(P.schedulesRead),
    staleTime: 15_000,
    refetchInterval: 30_000,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/oncall/alerts', { params: { header: orgHeader(), query: { limit: 50 } } })),
  });
}

export function useAcknowledgeAlert() {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (uuid: string) =>
      unwrap(
        api.POST('/v1/tenant/oncall/alerts/{uuid}/acknowledge', { params: { header: orgHeader(), path: { uuid } } }),
      ),
    onSettled: () => void qc.invalidateQueries({ queryKey: teamKeys.alerts(org) }),
  });
}
