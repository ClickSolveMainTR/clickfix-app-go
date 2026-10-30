import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { orgKey, useOrgSlug } from '@/core/api/org';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';

/**
 * Admin-lite (CF-79): what an organization admin checks or flips from a
 * phone: channel health, members and invitations, AI on/off and credits,
 * subscription and invoices. Definitions (rules, roles, SLAs) stay on the panel.
 */
export type Channel = components['schemas']['ChannelInstance'];
export type Member = components['schemas']['TenantMember'];
export type Invitation = components['schemas']['Invitation'];
export type AIPolicy = components['schemas']['AIPolicy'];

const keys = {
  all: (org: string) => orgKey(org, 'admin'),
  channels: (org: string) => [...keys.all(org), 'channels'] as const,
  members: (org: string, q: string) => [...keys.all(org), 'members', q] as const,
  invitations: (org: string) => [...keys.all(org), 'invitations'] as const,
  roles: (org: string) => [...keys.all(org), 'roles'] as const,
  ai: (org: string) => [...keys.all(org), 'ai'] as const,
  wallet: (org: string) => [...keys.all(org), 'wallet'] as const,
  subscription: (org: string) => [...keys.all(org), 'subscription'] as const,
  invoices: (org: string) => [...keys.all(org), 'invoices'] as const,
  paymentAvailability: (org: string) => [...keys.all(org), 'payment-availability'] as const,
};

export function useChannels() {
  const qc = useQueryClient();
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: keys.channels(org),
    enabled: !!org && can(P.channelsRead),
    staleTime: 60_000,
    queryFn: async () => {
      const page = await unwrap(api.GET('/v1/tenant/channels', { params: { header: orgHeader() } }));
      for (const channel of page.items) recordChannelHealth(qc, org, channel);
      return page;
    },
  });
}

export function useCheckChannel() {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (uuid: string) =>
      unwrap(api.POST('/v1/tenant/channels/{uuid}/check', { params: { header: orgHeader(), path: { uuid } } })),
    onSuccess: (channel) => recordChannelHealth(qc, org, channel),
    onSettled: () => void qc.invalidateQueries({ queryKey: keys.channels(org) }),
  });
}

type MemberPage = components['schemas']['TenantMemberPage'];

export function useMembers(q: string) {
  const org = useOrgSlug();
  const can = useCan();
  const query = q.trim();
  return useInfiniteQuery({
    queryKey: keys.members(org, query),
    enabled: !!org && can(P.membersRead),
    initialPageParam: 0,
    staleTime: 60_000,
    placeholderData: (prev) => prev,
    queryFn: ({ pageParam }): Promise<MemberPage> =>
      unwrap(
        api.GET('/v1/tenant/members', {
          params: {
            header: orgHeader(),
            query: { limit: 30, offset: pageParam, ...(query ? { q: query } : {}) },
          },
        }),
      ),
    getNextPageParam: (last: MemberPage) =>
      last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined,
  });
}

export function useInvitations() {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: keys.invitations(org),
    enabled: !!org && can(P.membersRead),
    staleTime: 60_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/invitations', {
          params: { header: orgHeader(), query: { limit: 50, status: 'pending' } },
        }),
      ),
  });
}

export function useRoles(enabled: boolean) {
  const org = useOrgSlug();
  return useQuery({
    queryKey: keys.roles(org),
    enabled: !!org && enabled,
    staleTime: 10 * 60_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/roles', { params: { header: orgHeader(), query: { limit: 100 } } })),
  });
}

export function useInvite() {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: components['schemas']['CreateInvitationRequest']) =>
      unwrap(api.POST('/v1/tenant/invitations', { params: { header: orgHeader() }, body })),
    onSettled: () => void qc.invalidateQueries({ queryKey: keys.invitations(org) }),
  });
}

export function useAIPolicy() {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: keys.ai(org),
    enabled: !!org && can(P.settingsRead),
    staleTime: 60_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/ai/policy', { params: { header: orgHeader() } })),
  });
}

/** Flips AI on / off, keeping every other policy value as it is. */
export function useSetAIEnabled() {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: { policy: AIPolicy; enabled: boolean }) =>
      unwrap(
        api.PUT('/v1/tenant/ai/policy', {
          params: { header: orgHeader() },
          body: {
            enabled: p.enabled,
            mask_pii: p.policy.mask_pii,
            allow_unmask: p.policy.allow_unmask,
            daily_calls: p.policy.daily_calls,
            concurrency: p.policy.concurrency,
            daily_budget_micros: p.policy.daily_budget_micros,
          },
        }),
      ),
    onSuccess: (policy) => qc.setQueryData(keys.ai(org), policy),
  });
}

export function useWallet() {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: keys.wallet(org),
    enabled: !!org && can(P.billingRead),
    staleTime: 60_000,
    retry: false,
    queryFn: () => unwrap(api.GET('/v1/tenant/billing/wallet', { params: { query: { limit: 10 } } })),
  });
}

export function useSubscription() {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: keys.subscription(org),
    enabled: !!org && can(P.billingRead),
    staleTime: 5 * 60_000,
    retry: false,
    queryFn: () => unwrap(api.GET('/v1/tenant/billing/subscription', { params: { header: orgHeader() } })),
  });
}

export function useInvoices() {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: keys.invoices(org),
    enabled: !!org && can(P.billingRead),
    staleTime: 5 * 60_000,
    retry: false,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/billing/invoices', { params: { header: orgHeader(), query: { limit: 10 } } })),
  });
}

/** Whether payments can be taken right now (CF-154): the screen says so
 * before sending the user to the panel to pay. */
export function usePaymentAvailability() {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: keys.paymentAvailability(org),
    enabled: !!org && can(P.billingRead),
    staleTime: 60_000,
    retry: false,
    queryFn: () => unwrap(api.GET('/v1/tenant/billing/payment-availability', { params: { header: orgHeader() } })),
  });
}

export function useMember(uuid: string) {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: [...keys.all(org), 'member', uuid],
    enabled: !!org && !!uuid && can(P.membersRead),
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/members/{userUuid}', {
          params: { header: orgHeader(), path: { userUuid: uuid } },
        }),
      ),
  });
}
export function useSetMemberRoles(uuid: string) {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (roles: string[]) =>
      unwrap(
        api.PATCH('/v1/tenant/members/{userUuid}', {
          params: { header: orgHeader(), path: { userUuid: uuid } },
          body: { role_uuids: roles },
        }),
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.all(org) });
    },
  });
}
export function useMemberTeams(uuid: string) {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: [...keys.all(org), 'member-teams', uuid],
    enabled: !!org && !!uuid && can(P.teamsRead),
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/teams', { params: { header: orgHeader(), query: { member: uuid, limit: 100 } } })),
  });
}
export function usePricing() {
  return useQuery({
    queryKey: ['public-pricing'],
    staleTime: 300_000,
    queryFn: () => unwrap(api.GET('/v1/public/pricing', { params: { query: { limit: 100 } } })),
  });
}

type HealthObservation = { at: string; health: Channel['health']; error: string };
const healthKey = (org: string, uuid: string) => [...keys.all(org), 'channel-health', uuid];
function recordChannelHealth(qc: ReturnType<typeof useQueryClient>, org: string, c: Channel) {
  if (!c.health.checked_at) return;
  qc.setQueryData<HealthObservation[]>(healthKey(org, c.uuid), (previous = []) =>
    [
      { at: c.health.checked_at!, health: c.health, error: c.last_error },
      ...previous.filter((p) => p.at !== c.health.checked_at),
    ].slice(0, 30),
  );
}
export function useChannelHealthHistory(uuid: string) {
  const org = useOrgSlug();
  return useQuery<HealthObservation[]>({
    queryKey: healthKey(org, uuid),
    enabled: false,
    queryFn: async () => [],
  });
}
