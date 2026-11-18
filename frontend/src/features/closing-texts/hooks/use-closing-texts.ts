"use client";

import { useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { useAppMutation } from "@/lib/query/mutation";

import {
  closingTextsService,
  type ClosingTextInput,
  type ClosingTextSettings,
} from "../services/closing-texts.service";

export const closingTextKeys = {
  all: (org: string) => ["closing-texts", org] as const,
  list: (org: string) => [...closingTextKeys.all(org), "list"] as const,
  conversation: (org: string, uuid: string) =>
    [...closingTextKeys.all(org), "conversation", uuid] as const,
};

export function useClosingTextScope() {
  const { active } = useActiveOrganization();
  const perms = useMemo(
    () => new Set<string>(active?.permissions ?? []),
    [active?.permissions],
  );
  return {
    org: active?.slug ?? "",
    canRead: perms.has(Permission.TenantConversationsRead),
    canWrite: perms.has(Permission.TenantMacrosWrite),
  };
}

export function useClosingTexts() {
  const { org, canRead } = useClosingTextScope();
  return useQuery({
    queryKey: closingTextKeys.list(org),
    queryFn: () => closingTextsService.list(),
    enabled: Boolean(org) && canRead,
  });
}

/** Picker options of one conversation (rendered for its contact). */
export function useConversationClosingTexts(uuid: string, enabled = true) {
  const { org, canRead } = useClosingTextScope();
  return useQuery({
    queryKey: closingTextKeys.conversation(org, uuid),
    queryFn: () => closingTextsService.forConversation(uuid),
    enabled: enabled && Boolean(org) && canRead,
    staleTime: 60_000,
  });
}

export function useClosingTextMutations() {
  const { org } = useClosingTextScope();
  const qc = useQueryClient();
  const done = () =>
    void qc.invalidateQueries({ queryKey: closingTextKeys.all(org) });
  return {
    save: useAppMutation({
      mutationFn: (v: { uuid?: string; body: ClosingTextInput }) =>
        v.uuid
          ? closingTextsService.update(v.uuid, v.body)
          : closingTextsService.create(v.body),
      onSuccess: done,
    }),
    remove: useAppMutation({
      mutationFn: (uuid: string) => closingTextsService.remove(uuid),
      onSuccess: done,
    }),
    settings: useAppMutation({
      mutationFn: (body: ClosingTextSettings) =>
        closingTextsService.saveSettings(body),
      onSuccess: done,
    }),
  };
}
