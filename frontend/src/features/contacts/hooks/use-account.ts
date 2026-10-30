"use client";

import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import { useAppMutation } from "@/lib/query/mutation";

import {
  accountService,
  type CompanyTimelineKind,
} from "../services/contacts.service";
import { contactsKeys, useContactsScope } from "./use-contacts";

export const accountKeys = {
  overview: (org: string, uuid: string) =>
    [...contactsKeys.all(org), "account", uuid, "overview"] as const,
  timeline: (org: string, uuid: string, kinds: CompanyTimelineKind[]) =>
    [...contactsKeys.all(org), "account", uuid, "timeline", kinds] as const,
};

/** Account screen header and conversations of a company (CF-85). */
export function useCompanyOverview(uuid: string) {
  const { org, canRead } = useContactsScope();
  return useQuery({
    queryKey: accountKeys.overview(org, uuid),
    queryFn: () => accountService.overview(uuid),
    enabled: Boolean(org && uuid) && canRead,
  });
}

/** Unified timeline, paged backwards with `next_before`. */
export function useCompanyTimeline(uuid: string, kinds: CompanyTimelineKind[]) {
  const { org, canRead } = useContactsScope();
  return useInfiniteQuery({
    queryKey: accountKeys.timeline(org, uuid, kinds),
    queryFn: ({ pageParam }) =>
      accountService.timeline(uuid, { kinds, before: pageParam }),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => last.next_before ?? undefined,
    enabled: Boolean(org && uuid) && canRead,
  });
}

export function useCompanyNoteMutations(companyUuid: string) {
  const { org } = useContactsScope();
  const qc = useQueryClient();
  const invalidate = () =>
    qc.invalidateQueries({
      queryKey: [...contactsKeys.all(org), "account", companyUuid],
    });
  return {
    add: useAppMutation({
      mutationFn: (body: string) => accountService.addNote(companyUuid, body),
      onSuccess: invalidate,
    }),
    remove: useAppMutation({
      mutationFn: (uuid: string) => accountService.deleteNote(uuid),
      onSuccess: invalidate,
    }),
  };
}
