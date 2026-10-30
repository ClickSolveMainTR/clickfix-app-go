"use client";

import { useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { useAppMutation } from "@/lib/query/mutation";

import {
  macrosService,
  type CannedInput,
  type MacroInput,
} from "../services/macros.service";

export const macrosKeys = {
  canned: (org: string) => ["macros", org, "canned"] as const,
  macros: (org: string) => ["macros", org, "macros"] as const,
};

export function useMacrosScope() {
  const { active } = useActiveOrganization();
  const perms = useMemo(
    () => new Set<string>(active?.permissions ?? []),
    [active?.permissions],
  );
  return {
    org: active?.slug ?? "",
    canRead: perms.has(Permission.TenantMacrosRead),
    canWrite: perms.has(Permission.TenantMacrosWrite),
  };
}

/** Every usable canned response (filtered client-side for instant "/"). */
export function useCannedResponses() {
  const { org, canRead } = useMacrosScope();
  return useQuery({
    queryKey: macrosKeys.canned(org),
    queryFn: () => macrosService.canned(),
    enabled: Boolean(org) && canRead,
    staleTime: 60_000,
  });
}

export function useMacros() {
  const { org, canRead } = useMacrosScope();
  return useQuery({
    queryKey: macrosKeys.macros(org),
    queryFn: () => macrosService.macros(),
    enabled: Boolean(org) && canRead,
    staleTime: 60_000,
  });
}

export function useCannedMutations() {
  const { org } = useMacrosScope();
  const qc = useQueryClient();
  const done = () =>
    void qc.invalidateQueries({ queryKey: macrosKeys.canned(org) });
  return {
    create: useAppMutation({
      mutationFn: (b: CannedInput) => macrosService.createCanned(b),
      onSuccess: done,
    }),
    update: useAppMutation({
      mutationFn: (v: { uuid: string; body: CannedInput }) =>
        macrosService.updateCanned(v.uuid, v.body),
      onSuccess: done,
    }),
    remove: useAppMutation({
      mutationFn: (uuid: string) => macrosService.deleteCanned(uuid),
      onSuccess: done,
    }),
  };
}

export function useMacroMutations() {
  const { org } = useMacrosScope();
  const qc = useQueryClient();
  const done = () =>
    void qc.invalidateQueries({ queryKey: macrosKeys.macros(org) });
  return {
    create: useAppMutation({
      mutationFn: (b: MacroInput) => macrosService.createMacro(b),
      onSuccess: done,
    }),
    update: useAppMutation({
      mutationFn: (v: { uuid: string; body: MacroInput }) =>
        macrosService.updateMacro(v.uuid, v.body),
      onSuccess: done,
    }),
    remove: useAppMutation({
      mutationFn: (uuid: string) => macrosService.deleteMacro(uuid),
      onSuccess: done,
    }),
  };
}

/** Runs a macro and refreshes the inbox caches. */
export function useApplyMacro(conversation: string) {
  const { org } = useMacrosScope();
  const qc = useQueryClient();
  return useAppMutation({
    mutationFn: (macro: string) => macrosService.apply(conversation, macro),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ["inbox", org] });
      void qc.invalidateQueries({ queryKey: macrosKeys.macros(org) });
    },
  });
}
