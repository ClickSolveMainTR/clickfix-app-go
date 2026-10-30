"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import { useAppMutation } from "@/lib/query/mutation";

import { inboxService } from "../services/inbox.service";
import type { LinkRelation, SideConversationCreate } from "../types";
import { inboxKeys, useInboxScope } from "./use-inbox";

/*
 * CF-98 queries live under the conversation's detail key, so the realtime
 * handler that invalidates a detail on every change refreshes them too.
 */
const linkKeys = {
  links: (org: string, uuid: string) =>
    [...inboxKeys.detail(org, uuid), "links"] as const,
  side: (org: string, uuid: string) =>
    [...inboxKeys.detail(org, uuid), "side"] as const,
  thread: (org: string, conv: string, side: string) =>
    [...inboxKeys.detail(org, conv), "side", side] as const,
};

export function useConversationLinks(uuid: string) {
  const { org, canRead } = useInboxScope();
  return useQuery({
    queryKey: linkKeys.links(org, uuid),
    queryFn: () => inboxService.links(uuid),
    enabled: Boolean(org && uuid && canRead),
  });
}

/** Refreshes everything a merge / split / link touched. */
function useRefresh() {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  return (...uuids: string[]) => {
    for (const u of uuids) {
      void qc.invalidateQueries({ queryKey: inboxKeys.detail(org, u) });
      void qc.invalidateQueries({ queryKey: inboxKeys.messages(org, u) });
    }
    void qc.invalidateQueries({ queryKey: inboxKeys.lists(org) });
    void qc.invalidateQueries({ queryKey: inboxKeys.counts(org) });
  };
}

export function useMergeConversation(uuid: string) {
  const refresh = useRefresh();
  return useAppMutation({
    mutationFn: (source: string) => inboxService.merge(uuid, source),
    onSuccess: (_d, source) => refresh(uuid, source),
  });
}

export function useSplitConversation(uuid: string) {
  const refresh = useRefresh();
  return useAppMutation({
    mutationFn: (body: {
      message_uuid: string;
      following: boolean;
      subject?: string;
    }) => inboxService.split(uuid, body),
    onSuccess: (child) => refresh(uuid, child.uuid),
  });
}

export function useLinkConversation(uuid: string) {
  const refresh = useRefresh();
  return useAppMutation({
    mutationFn: (v: { other: string; relation: LinkRelation }) =>
      inboxService.link(uuid, v.other, v.relation),
    onSuccess: (_d, v) => refresh(uuid, v.other),
  });
}

export function useUnlinkConversation(uuid: string) {
  const refresh = useRefresh();
  return useAppMutation({
    mutationFn: (other: string) => inboxService.unlink(uuid, other),
    onSuccess: (_d, other) => refresh(uuid, other),
  });
}

export function useSideConversations(uuid: string) {
  const { org, canRead } = useInboxScope();
  return useQuery({
    queryKey: linkKeys.side(org, uuid),
    queryFn: () => inboxService.sideList(uuid),
    enabled: Boolean(org && uuid && canRead),
  });
}

export function useSideThread(conv: string, side: string | null) {
  const { org } = useInboxScope();
  return useQuery({
    queryKey: linkKeys.thread(org, conv, side ?? ""),
    queryFn: () => inboxService.sideGet(side as string),
    enabled: Boolean(org && side),
  });
}

export function useStartSideConversation(uuid: string) {
  const refresh = useRefresh();
  return useAppMutation({
    mutationFn: (body: SideConversationCreate) =>
      inboxService.sideStart(uuid, body),
    onSuccess: () => refresh(uuid),
  });
}

export function useReplySideConversation(conv: string, side: string) {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  const refresh = useRefresh();
  return useAppMutation({
    mutationFn: (body: string) => inboxService.sideReply(side, body),
    onSuccess: (data) => {
      qc.setQueryData(linkKeys.thread(org, conv, side), data);
      refresh(conv);
    },
  });
}

export function useSideStatus(conv: string, side: string) {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  return useAppMutation({
    mutationFn: (status: "open" | "closed") =>
      inboxService.sideStatus(side, status),
    onSuccess: (data) => {
      qc.setQueryData(linkKeys.thread(org, conv, side), data);
      void qc.invalidateQueries({ queryKey: linkKeys.side(org, conv) });
    },
  });
}
