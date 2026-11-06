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
export type EscalationPolicy = components['schemas']['EscalationPolicy'];
export type EscalationStep = components['schemas']['EscalationStep'];
export type UptimeMonitor = components['schemas']['UptimeMonitor'];
export type UptimeResult = components['schemas']['UptimeResult'];
export type StatusComponent = components['schemas']['StatusComponent'];
export type Team = components['schemas']['Team'];
export type TeamMember = components['schemas']['TeamMember'];
export type TeamRole = TeamMember['role'];

export const teamKeys = {
  all: (org: string) => orgKey(org, 'team'),
  presence: (org: string) => [...teamKeys.all(org), 'presence'] as const,
  threads: (org: string) => [...teamKeys.all(org), 'threads'] as const,
  messages: (org: string, uuid: string) => [...teamKeys.all(org), 'messages', uuid] as const,
  rotas: (org: string) => [...teamKeys.all(org), 'rotas'] as const,
  current: (org: string, uuid: string) => [...teamKeys.all(org), 'oncall', uuid] as const,
  alerts: (org: string) => [...teamKeys.all(org), 'alerts'] as const,
  policies: (org: string) => [...teamKeys.all(org), 'policies'] as const,
  monitors: (org: string) => [...teamKeys.all(org), 'monitors'] as const,
  results: (org: string, uuid: string) => [...teamKeys.all(org), 'monitors', uuid, 'results'] as const,
  components: (org: string) => [...teamKeys.all(org), 'status-components'] as const,
  team: (org: string, uuid: string) => [...teamKeys.all(org), 'teams', uuid] as const,
  skills: (org: string) => [...teamKeys.all(org), 'skills'] as const,
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

export function useRotas(enabled = true) {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: teamKeys.rotas(org),
    enabled: !!org && enabled && can(P.schedulesRead),
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

/**
 * What the member may read of the on-call setup (CF-168), the panel's own
 * checks (`frontend/src/features/oncall/components/oncall-page.tsx`):
 * escalation policies and uptime monitors need `tenant.schedules.read` and
 * organization-wide conversation access (`oncall/routes.go`), status
 * components `tenant.conversations.read` with the same access
 * (`statuspage/routes.go`). A hook below never calls an endpoint the member
 * cannot read.
 */
export function useOncallScope() {
  const can = useCan();
  const all = can(P.conversationsReadAll);
  return {
    canSetup: all && can(P.schedulesRead),
    canComponents: all && can(P.conversationsRead),
  };
}

/** Escalation policies: who is notified, in which order (`GET /v1/tenant/oncall/policies`). */
export function useEscalationPolicies(enabled = true) {
  const org = useOrgSlug();
  const { canSetup } = useOncallScope();
  return useQuery({
    queryKey: teamKeys.policies(org),
    enabled: !!org && enabled && canSetup,
    staleTime: 5 * 60_000,
    // The API's default page is 20; the panel asks for 100 too.
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/oncall/policies', { params: { header: orgHeader(), query: { limit: 100 } } })),
  });
}

/** Uptime monitors with their last check (`GET /v1/tenant/uptime-monitors`), refreshed every minute like the panel. */
export function useUptimeMonitors(enabled = true) {
  const org = useOrgSlug();
  const { canSetup } = useOncallScope();
  return useQuery({
    queryKey: teamKeys.monitors(org),
    enabled: !!org && enabled && canSetup,
    staleTime: 30_000,
    refetchInterval: 60_000,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/uptime-monitors', { params: { header: orgHeader(), query: { limit: 100 } } })),
  });
}

/**
 * Recent checks of one monitor, newest first. The endpoint takes no paging
 * parameters and answers with its default page (20 checks). Read again every
 * minute, like the monitor list, keeping what is on screen meanwhile.
 */
export function useUptimeResults(uuid: string) {
  const org = useOrgSlug();
  const { canSetup } = useOncallScope();
  return useQuery({
    queryKey: teamKeys.results(org, uuid),
    enabled: !!org && !!uuid && canSetup,
    staleTime: 30_000,
    refetchInterval: 60_000,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/uptime-monitors/{uuid}/results', { params: { header: orgHeader(), path: { uuid } } })),
  });
}

/** Status page components and their current status (`GET /v1/tenant/status/components`). */
export function useStatusComponents(enabled = true) {
  const org = useOrgSlug();
  const { canComponents } = useOncallScope();
  return useQuery({
    queryKey: teamKeys.components(org),
    enabled: !!org && enabled && canComponents,
    staleTime: 60_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/status/components', { params: { header: orgHeader() } })),
  });
}

/**
 * Team management (CF-166), the phone version of the panel's Teams pages
 * (`frontend/src/features/teams`): `/v1/tenant/teams`. The list itself is
 * `useTeams` (`features/inbox/api.ts`), the one the pickers already read.
 */
export function useTeam(uuid: string) {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: teamKeys.team(org, uuid),
    enabled: !!org && !!uuid && can(P.teamsRead),
    staleTime: 30_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/teams/{uuid}', { params: { header: orgHeader(), path: { uuid } } })),
  });
}

/** Skill tags used in the organization (suggestions for the team form). */
export function useSkillCatalog(enabled: boolean) {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: teamKeys.skills(org),
    enabled: !!org && enabled && can(P.teamsRead),
    staleTime: 60_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/skills', { params: { header: orgHeader() } })),
  });
}

/**
 * Team writes (`tenant.teams.write`). Each one stores the returned team and
 * refreshes every team list in the app: the management list and the pickers
 * (`useTeams`), a member's teams and "my shift" all carry `'teams'` or
 * `'member-teams'` in their key.
 */
export function useTeamMutations() {
  const org = useOrgSlug();
  const qc = useQueryClient();
  const header = () => ({ header: orgHeader() });
  // `skip`: the team whose detail is already fresh (just stored) or gone (deleted, so a refetch would 404).
  const refresh = (team?: Team, skip = team?.uuid) => {
    if (team) qc.setQueryData(teamKeys.team(org, team.uuid), team);
    void qc.invalidateQueries({
      predicate: (q) =>
        q.queryKey.includes(org) &&
        (q.queryKey.includes('teams') || q.queryKey.includes('member-teams')) &&
        !(skip && q.queryKey.includes(skip)),
    });
    void qc.invalidateQueries({ queryKey: teamKeys.skills(org) });
  };
  return {
    create: useMutation({
      mutationFn: (body: components['schemas']['TeamInput']) =>
        unwrap(api.POST('/v1/tenant/teams', { params: header(), body })),
      onSuccess: (team) => refresh(team),
    }),
    update: useMutation({
      mutationFn: (v: { uuid: string; body: components['schemas']['TeamInput'] }) =>
        unwrap(api.PATCH('/v1/tenant/teams/{uuid}', { params: { ...header(), path: { uuid: v.uuid } }, body: v.body })),
      onSuccess: (team) => refresh(team),
    }),
    remove: useMutation({
      mutationFn: (uuid: string) =>
        unwrap(api.DELETE('/v1/tenant/teams/{uuid}', { params: { ...header(), path: { uuid } } })),
      onSuccess: (_data, uuid) => refresh(undefined, uuid),
    }),
    /** Adds the users in order, or changes one member's role; the last answer is the team. */
    setMembers: useMutation({
      mutationFn: async (v: { uuid: string; users: string[]; role: TeamRole }) => {
        let team: Team | undefined;
        for (const userUuid of v.users)
          team = await unwrap(
            api.PUT('/v1/tenant/teams/{uuid}/members/{userUuid}', {
              params: { ...header(), path: { uuid: v.uuid, userUuid } },
              body: { role: v.role },
            }),
          );
        return team;
      },
      onSuccess: (team) => refresh(team),
      // Some of several adds may have gone through: the team, its member count and every list are read again.
      onError: () => refresh(undefined, ''),
    }),
    removeMember: useMutation({
      mutationFn: (v: { uuid: string; userUuid: string }) =>
        unwrap(
          api.DELETE('/v1/tenant/teams/{uuid}/members/{userUuid}', {
            params: { ...header(), path: { uuid: v.uuid, userUuid: v.userUuid } },
          }),
        ),
      onSuccess: (team) => refresh(team),
    }),
  };
}
