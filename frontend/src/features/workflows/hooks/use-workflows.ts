"use client";

import { useQuery } from "@tanstack/react-query";

import type { components } from "@/generated/api";
import { aiSettingsService } from "@/features/ai-settings/services/ai-settings.service";
import { automationIntegrationsService } from "@/features/automation-integrations/services/automation-integrations.service";
import { inboxService } from "@/features/inbox/services/inbox.service";
import { macrosService } from "@/features/macros/services/macros.service";
import { useActiveOrganization } from "@/features/organizations";
import { teamsService } from "@/features/teams/services/teams.service";
import { apiClient, unwrap } from "@/lib/api";

import type { TargetKind } from "../lib/graph";
import { workflowsService } from "../services/workflows.service";

export const workflowKeys = {
  all: (org: string | undefined) => ["tenant", org, "workflows"] as const,
  list: (org: string | undefined) =>
    [...workflowKeys.all(org), "list"] as const,
  detail: (org: string | undefined, uuid: string) =>
    [...workflowKeys.all(org), "detail", uuid] as const,
  versions: (org: string | undefined, uuid: string) =>
    [...workflowKeys.all(org), "versions", uuid] as const,
  runs: (org: string | undefined, uuid: string, limit: number) =>
    [...workflowKeys.all(org), "runs", uuid, limit] as const,
  run: (org: string | undefined, run: string) =>
    [...workflowKeys.all(org), "run", run] as const,
  lastRun: (org: string | undefined, uuid: string) =>
    [...workflowKeys.all(org), "last-run", uuid] as const,
  options: (org: string | undefined, kind: string) =>
    [...workflowKeys.all(org), "options", kind] as const,
};

export function useWorkflow(uuid: string) {
  const { active: org } = useActiveOrganization();
  return useQuery({
    queryKey: workflowKeys.detail(org?.slug, uuid),
    queryFn: () => workflowsService.get(uuid),
    enabled: Boolean(org),
    // The editor owns the draft once loaded; refetching would clobber edits.
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });
}

export function useWorkflowVersions(uuid: string) {
  const { active: org } = useActiveOrganization();
  return useQuery({
    queryKey: workflowKeys.versions(org?.slug, uuid),
    queryFn: () => workflowsService.versions(uuid),
    enabled: Boolean(org),
  });
}

export type PickerOption = { value: string; label: string; hint?: string };

/**
 * Choices for action targets. Each source keeps its own permission; when the
 * list is forbidden the panel falls back to a raw UUID field.
 */
export function useTargetOptions(kind: TargetKind | "team" | null) {
  const { active: org } = useActiveOrganization();
  const slug = org?.slug ?? "";
  return useQuery({
    queryKey: workflowKeys.options(org?.slug, kind ?? "none"),
    enabled: Boolean(org) && kind !== null,
    staleTime: 60_000,
    retry: false,
    queryFn: async (): Promise<PickerOption[]> => {
      switch (kind) {
        case "agent":
        case "user":
          return (await inboxService.agents()).map((a) => ({
            value: a.uuid,
            label: a.name || a.email,
            hint: a.email,
          }));
        case "tag":
          return (await inboxService.tags()).map((t) => ({
            value: t.uuid,
            label: t.name,
          }));
        case "canned":
          return (await macrosService.canned()).map((c) => ({
            value: c.uuid,
            label: c.title,
            hint: c.shortcut ? `/${c.shortcut}` : undefined,
          }));
        case "webhook":
          return (await automationIntegrationsService.hooks(slug)).map((h) => ({
            value: h.uuid,
            label: h.name,
            hint: h.url,
          }));
        case "procedure":
          return ((await aiSettingsService.procedures(slug)).items ?? [])
            .filter((p) => p.published_revision !== null)
            .map((p) => ({ value: p.uuid, label: p.title, hint: p.intent }));
        case "team":
          return (
            (await teamsService.list(slug, { limit: 100, offset: 0 })).items ??
            []
          ).map((t) => ({ value: t.uuid, label: t.name }));
        default:
          return [];
      }
    },
  });
}

type Conversation = components["schemas"]["Conversation"];

/** Recent conversations for the dry-run picker (current visibility applies). */
export function useSampleConversations(q: string) {
  const { active: org } = useActiveOrganization();
  return useQuery({
    queryKey: [...workflowKeys.all(org?.slug), "sample", q],
    enabled: Boolean(org),
    staleTime: 30_000,
    queryFn: async () => {
      const page = await unwrap<{ items: Conversation[] }>(
        await apiClient.GET("/v1/tenant/conversations", {
          params: {
            header: { "X-Organization": org?.slug ?? "" },
            query: { limit: 10, ...(q ? { q } : {}) },
          },
        }),
      );
      return page.items ?? [];
    },
  });
}
