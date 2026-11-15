"use client";

import {
  keepPreviousData,
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import { useAppMutation } from "@/lib/query/mutation";

import {
  contactProfileService,
  type ContactTimelineQuery,
} from "../services/profile.service";
import { contactsKeys, useContactsScope } from "./use-contacts";

export const contactProfileKeys = {
  root: (org: string, uuid: string) =>
    [...contactsKeys.all(org), "profile", uuid] as const,
  overview: (org: string, uuid: string) =>
    [...contactProfileKeys.root(org, uuid), "overview"] as const,
  timeline: (org: string, uuid: string, q: ContactTimelineQuery) =>
    [...contactProfileKeys.root(org, uuid), "timeline", q] as const,
};

/** Contact page header and sections (CF-221). */
export function useContactOverview(uuid: string) {
  const { org, canRead } = useContactsScope();
  return useQuery({
    queryKey: contactProfileKeys.overview(org, uuid),
    queryFn: () => contactProfileService.overview(uuid),
    enabled: Boolean(org && uuid) && canRead,
  });
}

/** Unified contact timeline, paged backwards with `next_before`. */
export function useContactTimeline(uuid: string, q: ContactTimelineQuery) {
  const { org, canRead } = useContactsScope();
  return useInfiniteQuery({
    queryKey: contactProfileKeys.timeline(org, uuid, q),
    queryFn: ({ pageParam }) =>
      contactProfileService.timeline(uuid, { ...q, before: pageParam }),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => last.next_before ?? undefined,
    enabled: Boolean(org && uuid) && canRead,
    placeholderData: keepPreviousData,
  });
}

function useInvalidateProfile(uuid: string) {
  const { org } = useContactsScope();
  const qc = useQueryClient();
  return () =>
    qc.invalidateQueries({ queryKey: contactProfileKeys.root(org, uuid) });
}

export function useContactNoteMutations(contactUuid: string) {
  const invalidate = useInvalidateProfile(contactUuid);
  return {
    add: useAppMutation({
      mutationFn: (body: string) =>
        contactProfileService.addNote(contactUuid, body),
      onSuccess: invalidate,
    }),
    remove: useAppMutation({
      mutationFn: (uuid: string) => contactProfileService.deleteNote(uuid),
      onSuccess: invalidate,
    }),
  };
}

/** Generates and caches the AI summary; the page reads it from the overview. */
export function useGenerateContactSummary(contactUuid: string) {
  const invalidate = useInvalidateProfile(contactUuid);
  return useAppMutation({
    mutationFn: (locale: "tr" | "en") =>
      contactProfileService.generateSummary(contactUuid, locale),
    onSuccess: invalidate,
    meta: { silent: true },
  });
}
