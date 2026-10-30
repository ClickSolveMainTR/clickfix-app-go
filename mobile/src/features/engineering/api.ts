import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { orgKey, useOrgSlug } from '@/core/api/org';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';

/**
 * Engineering issues (`/v1/tenant/engineering/issues`, `tenant.tickets.*`):
 * bugs and requests support hands to engineering, with the conversations
 * that reported them (Plain / Linear style).
 */
export type Issue = components['schemas']['EngineeringIssue'];
export type IssueStatus = Issue['status'];
export type IssueCreate = components['schemas']['EngineeringCreateIssue'];

export const ISSUE_STATUSES: IssueStatus[] = ['backlog', 'todo', 'in_progress', 'done', 'canceled'];

const PAGE = 30;
export const engKeys = {
  all: (org: string) => orgKey(org, 'eng'),
  list: (org: string, status: string | null, q: string) => [...engKeys.all(org), 'list', status, q] as const,
  issue: (org: string, uuid: string) => [...engKeys.all(org), 'issue', uuid] as const,
  links: (org: string, uuid: string) => [...engKeys.all(org), 'links', uuid] as const,
};

export function useEngineeringScope() {
  const can = useCan();
  return { canRead: can(P.ticketsRead), canWrite: can(P.ticketsWrite) };
}

type IssuePage = { items: Issue[]; total: number; limit: number; offset: number };

export function useIssues(status: IssueStatus | null, q: string, enabled = true) {
  const org = useOrgSlug();
  const { canRead } = useEngineeringScope();
  const query = q.trim();
  return useInfiniteQuery({
    queryKey: engKeys.list(org, status, query),
    enabled: !!org && canRead && enabled,
    initialPageParam: 0,
    staleTime: 30_000,
    placeholderData: (prev) => prev,
    queryFn: ({ pageParam }): Promise<IssuePage> =>
      unwrap(
        api.GET('/v1/tenant/engineering/issues', {
          params: {
            header: orgHeader(),
            query: {
              limit: PAGE,
              offset: pageParam,
              sort: '-updated_at',
              ...(status ? { status } : {}),
              ...(query ? { q: query } : {}),
            },
          },
        }),
      ),
    getNextPageParam: (last: IssuePage) =>
      last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined,
  });
}

export function useIssue(uuid: string) {
  const org = useOrgSlug();
  const { canRead } = useEngineeringScope();
  return useQuery({
    queryKey: engKeys.issue(org, uuid),
    enabled: !!org && !!uuid && canRead,
    staleTime: 30_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/engineering/issues/{issue}', { params: { header: orgHeader(), path: { issue: uuid } } }),
      ),
  });
}

export function useIssueConversations(uuid: string) {
  const org = useOrgSlug();
  const can = useCan();
  return useQuery({
    queryKey: engKeys.links(org, uuid),
    enabled: !!org && !!uuid && can(P.ticketsRead) && can(P.conversationsRead),
    staleTime: 30_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/engineering/issues/{issue}/conversations', {
          params: { header: orgHeader(), path: { issue: uuid } },
        }),
      ),
  });
}

/** Opens an issue, optionally from a conversation (linked at once; `ai_summary` drafts the text). */
export function useCreateIssue() {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: IssueCreate) =>
      unwrap(api.POST('/v1/tenant/engineering/issues', { params: { header: orgHeader() }, body })),
    onSettled: () => void qc.invalidateQueries({ queryKey: engKeys.all(org) }),
  });
}

export function useLinkIssue() {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { issue: string; conversation: string; notifyCustomer: boolean }) =>
      unwrap(
        api.POST('/v1/tenant/engineering/issues/{issue}/conversations', {
          params: { header: orgHeader(), path: { issue: v.issue } },
          body: { conversation_uuid: v.conversation, notify_customer: v.notifyCustomer },
        }),
      ),
    onSettled: () => void qc.invalidateQueries({ queryKey: engKeys.all(org) }),
  });
}

export function useUpdateIssue(uuid: string) {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { status?: IssueStatus; priority?: Issue['priority']; revision: number }) =>
      unwrap(
        api.PATCH('/v1/tenant/engineering/issues/{issue}', {
          params: { header: orgHeader(), path: { issue: uuid } },
          body,
        }),
      ),
    onSuccess: (i) => qc.setQueryData(engKeys.issue(org, uuid), i),
    onSettled: () => void qc.invalidateQueries({ queryKey: engKeys.all(org) }),
  });
}

export type IssueComment = components['schemas']['EngineeringComment'];

export function useIssueComments(uuid: string) {
  const org = useOrgSlug();
  const { canRead } = useEngineeringScope();
  return useQuery({
    queryKey: [...engKeys.issue(org, uuid), 'comments'] as const,
    enabled: !!org && !!uuid && canRead,
    staleTime: 30_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/engineering/issues/{issue}/comments', {
          params: { header: orgHeader(), path: { issue: uuid } },
        }),
      ),
  });
}

export function useAddIssueComment(uuid: string) {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: string) =>
      unwrap(
        api.POST('/v1/tenant/engineering/issues/{issue}/comments', {
          params: { header: orgHeader(), path: { issue: uuid } },
          body: { body },
        }),
      ),
    onSettled: () => void qc.invalidateQueries({ queryKey: [...engKeys.issue(org, uuid), 'comments'] }),
  });
}
