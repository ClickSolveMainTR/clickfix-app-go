import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { orgKey, useOrgSlug } from '@/core/api/org';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';
import { inboxKeys } from '@/features/inbox/cache';

/**
 * Contacts and companies (`/v1/tenant/contacts`, `/companies`, CF-85 account
 * timeline, customer health). Reads need `tenant.contacts.read`, edits and
 * notes `tenant.contacts.write`.
 */
export type ContactRecord = components['schemas']['Contact'];
type ContactPage = components['schemas']['ContactPage'];
type CompanyPage = components['schemas']['CompanyPage'];
export type ContactInput = components['schemas']['ContactInput'];
export type Company = components['schemas']['Company'];
export type CompanyOverview = components['schemas']['CompanyOverview'];
export type CompanyTimelineItem = components['schemas']['CompanyTimelineItem'];
export type CompanyHealth = components['schemas']['CompanyHealth'];
export type CustomFieldDefinition = components['schemas']['CustomFieldDefinition'];

const PAGE = 30;
export const peopleKeys = {
  all: (org: string) => orgKey(org, 'people'),
  contacts: (org: string, q: string) => [...peopleKeys.all(org), 'contacts', q] as const,
  contact: (org: string, uuid: string) => [...peopleKeys.all(org), 'contact', uuid] as const,
  companies: (org: string, q: string) => [...peopleKeys.all(org), 'companies', q] as const,
  company: (org: string, uuid: string) => [...peopleKeys.all(org), 'company', uuid] as const,
  overview: (org: string, uuid: string) => [...peopleKeys.all(org), 'overview', uuid] as const,
  timeline: (org: string, uuid: string) => [...peopleKeys.all(org), 'timeline', uuid] as const,
  health: (org: string, uuid: string) => [...peopleKeys.all(org), 'health', uuid] as const,
  fields: (org: string, entity: string) => [...peopleKeys.all(org), 'fields', entity] as const,
};

export function usePeopleScope() {
  const can = useCan();
  return { canRead: can(P.contactsRead), canWrite: can(P.contactsWrite), canConversations: can(P.conversationsRead) };
}

/** `getNextPageParam` for offset pages. */
export function nextOffset<T extends { items: unknown[]; total: number; offset: number }>(last: T) {
  return last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined;
}

export function useContactList(q: string) {
  const org = useOrgSlug();
  const { canRead } = usePeopleScope();
  const query = q.trim();
  return useInfiniteQuery({
    queryKey: peopleKeys.contacts(org, query),
    enabled: !!org && canRead,
    initialPageParam: 0,
    staleTime: 30_000,
    placeholderData: (prev) => prev,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/v1/tenant/contacts', {
          params: {
            header: orgHeader(),
            query: { limit: PAGE, offset: pageParam, sort: '-last_seen_at', ...(query ? { q: query } : {}) },
          },
        }),
      ),
    getNextPageParam: (last: ContactPage) => nextOffset(last),
  });
}

export function useCompanyList(q: string) {
  const org = useOrgSlug();
  const { canRead } = usePeopleScope();
  const query = q.trim();
  return useInfiniteQuery({
    queryKey: peopleKeys.companies(org, query),
    enabled: !!org && canRead,
    initialPageParam: 0,
    staleTime: 30_000,
    placeholderData: (prev) => prev,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/v1/tenant/companies', {
          params: { header: orgHeader(), query: { limit: PAGE, offset: pageParam, ...(query ? { q: query } : {}) } },
        }),
      ),
    getNextPageParam: (last: CompanyPage) => nextOffset(last),
  });
}

export function useContactRecord(uuid: string) {
  const org = useOrgSlug();
  const { canRead } = usePeopleScope();
  return useQuery({
    queryKey: peopleKeys.contact(org, uuid),
    enabled: !!org && !!uuid && canRead,
    staleTime: 30_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/contacts/{uuid}', { params: { header: orgHeader(), path: { uuid } } })),
  });
}

export function useUpdateContact(uuid: string) {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: ContactInput) =>
      unwrap(api.PATCH('/v1/tenant/contacts/{uuid}', { params: { header: orgHeader(), path: { uuid } }, body })),
    onSuccess: (c) => {
      qc.setQueryData(peopleKeys.contact(org, uuid), c);
      qc.setQueryData(inboxKeys.contact(org, uuid), c);
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: peopleKeys.all(org) }),
  });
}

/** Conversations of a contact or company (the panel's "conversations of …" filter). */
export function useRecordConversations(filter: { contact?: string; company?: string }) {
  const org = useOrgSlug();
  const { canConversations } = usePeopleScope();
  const id = filter.contact ?? filter.company ?? '';
  return useQuery({
    queryKey: [...inboxKeys.all(org), 'record', filter] as const,
    enabled: !!org && !!id && canConversations,
    staleTime: 30_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/conversations', {
          params: { header: orgHeader(), query: { ...filter, sort: '-last_message_at', limit: 20 } },
        }),
      ),
  });
}

export function useCompany(uuid: string) {
  const org = useOrgSlug();
  const { canRead } = usePeopleScope();
  return useQuery({
    queryKey: peopleKeys.overview(org, uuid),
    enabled: !!org && !!uuid && canRead,
    staleTime: 30_000,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/companies/{uuid}/overview', { params: { header: orgHeader(), path: { uuid } } })),
  });
}

export function useCompanyTimeline(uuid: string) {
  const org = useOrgSlug();
  const { canRead } = usePeopleScope();
  return useInfiniteQuery({
    queryKey: peopleKeys.timeline(org, uuid),
    enabled: !!org && !!uuid && canRead,
    initialPageParam: undefined as string | undefined,
    staleTime: 30_000,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/v1/tenant/companies/{uuid}/timeline', {
          params: {
            header: orgHeader(),
            path: { uuid },
            query: { limit: 20, ...(pageParam ? { before: pageParam } : {}) },
          },
        }),
      ),
    getNextPageParam: (last) => last.next_before ?? undefined,
  });
}

export function useCompanyHealth(uuid: string) {
  const org = useOrgSlug();
  const { canRead } = usePeopleScope();
  return useQuery({
    queryKey: peopleKeys.health(org, uuid),
    enabled: !!org && !!uuid && canRead,
    staleTime: 5 * 60_000,
    retry: false,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/companies/{uuid}/health', { params: { header: orgHeader(), path: { uuid } } })),
  });
}

export function useAddCompanyNote(uuid: string) {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: string) =>
      unwrap(
        api.POST('/v1/tenant/companies/{uuid}/notes', {
          params: { header: orgHeader(), path: { uuid } },
          body: { body },
        }),
      ),
    onSettled: () => void qc.invalidateQueries({ queryKey: peopleKeys.timeline(org, uuid) }),
  });
}

/** Custom field definitions of an entity (labels, types, options) for the detail and edit screens. */
export function useCustomFields(entity: 'contact' | 'company') {
  const org = useOrgSlug();
  return useQuery({
    queryKey: peopleKeys.fields(org, entity),
    enabled: !!org,
    staleTime: 10 * 60_000,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/custom-fields', { params: { header: orgHeader(), query: { entity_type: entity } } })),
  });
}
