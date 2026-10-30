"use client";

import { useQuery } from "@tanstack/react-query";

import { useActiveOrganization } from "@/features/organizations";

import { rulesService } from "../services/rules.service";

export const ruleKeys = {
  all: (org: string | undefined) =>
    ["tenant", org, "automation-rules"] as const,
  list: (org: string | undefined) => [...ruleKeys.all(org), "list"] as const,
  detail: (org: string | undefined, uuid: string) =>
    [...ruleKeys.all(org), "detail", uuid] as const,
  triggers: (org: string | undefined) =>
    [...ruleKeys.all(org), "triggers"] as const,
};

/** All rules in evaluation order (position, then creation). */
export function useRules(enabled = true) {
  const { active: org } = useActiveOrganization();
  return useQuery({
    queryKey: ruleKeys.list(org?.slug),
    queryFn: () => rulesService.list(org?.slug ?? "", 100, 0),
    enabled: Boolean(org) && enabled,
  });
}

export function useRule(uuid: string) {
  const { active: org } = useActiveOrganization();
  return useQuery({
    queryKey: ruleKeys.detail(org?.slug, uuid),
    queryFn: () => rulesService.get(org?.slug ?? "", uuid),
    enabled: Boolean(org) && uuid !== "new",
    // The editor owns the form once loaded; refetching would clobber edits.
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });
}

export function useRuleTriggers() {
  const { active: org } = useActiveOrganization();
  return useQuery({
    queryKey: ruleKeys.triggers(org?.slug),
    queryFn: () => rulesService.triggers(org?.slug ?? ""),
    enabled: Boolean(org),
    staleTime: 5 * 60_000,
  });
}
