"use client";

import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { useCallback } from "react";

import { useActiveOrganization } from "@/features/organizations";
import { useChannel } from "@/hooks/use-realtime";
import { RealtimeChannels } from "@/lib/realtime";

import {
  setupService,
  type SetupDiscoverRequest,
  type SetupProgressEvent,
  type SetupRun,
} from "../services/setup.service";

export const setupKeys = {
  templates: (slug: string) => ["onboarding", slug, "setup-templates"] as const,
  latest: (slug: string) => ["onboarding", slug, "setup-latest"] as const,
  run: (slug: string, uuid: string) =>
    ["onboarding", slug, "setup-run", uuid] as const,
};

const ACTIVE: SetupRun["status"][] = ["queued", "running"];

export function isApplying(run: SetupRun | null | undefined) {
  return Boolean(run && ACTIVE.includes(run.status));
}

function storeRun(qc: QueryClient, slug: string, run: SetupRun) {
  qc.setQueryData(setupKeys.run(slug, run.uuid), run);
  qc.setQueryData(setupKeys.latest(slug), run);
}

export function useSetupTemplates(slug: string, enabled = true) {
  return useQuery({
    queryKey: setupKeys.templates(slug),
    queryFn: () => setupService.templates(slug),
    enabled: Boolean(slug) && enabled,
    staleTime: 10 * 60_000,
  });
}

export function useLatestSetupRun(slug: string, enabled = true) {
  return useQuery({
    queryKey: setupKeys.latest(slug),
    queryFn: () => setupService.latest(slug),
    enabled: Boolean(slug) && enabled,
    retry: false,
    staleTime: 15_000,
  });
}

/**
 * One run. While it is being applied, progress arrives on the organization
 * channel (`onboarding.setup.progress`); a slow poll covers a missed event
 * or a realtime connection that is off.
 */
export function useSetupRun(slug: string, uuid: string | null) {
  return useQuery({
    queryKey: setupKeys.run(slug, uuid ?? ""),
    queryFn: () => setupService.run(slug, uuid ?? ""),
    enabled: Boolean(slug && uuid),
    refetchInterval: (query) => (isApplying(query.state.data) ? 4_000 : false),
  });
}

/** Patches the run cache with each progress event of `uuid`. */
export function useSetupProgress(
  slug: string,
  uuid: string | null,
  onEvent?: (event: SetupProgressEvent) => void,
) {
  const qc = useQueryClient();
  const { active } = useActiveOrganization();
  const handler = useCallback(
    (message: { type: string; data: unknown }) => {
      if (message.type !== "onboarding.setup.progress") return;
      const event = message.data as SetupProgressEvent;
      if (!uuid || event.run_uuid !== uuid) return;
      qc.setQueryData<SetupRun>(setupKeys.run(slug, uuid), (run) => {
        if (!run) return run;
        const items = event.item
          ? run.items.map((it) =>
              it.key === event.item?.key ? { ...it, ...event.item } : it,
            )
          : run.items;
        return {
          ...run,
          status: event.status,
          done: event.done,
          failed: event.failed,
          items,
        };
      });
      if (!ACTIVE.includes(event.status)) {
        void qc.invalidateQueries({ queryKey: setupKeys.run(slug, uuid) });
        void qc.invalidateQueries({ queryKey: setupKeys.latest(slug) });
      }
      onEvent?.(event);
    },
    [qc, slug, uuid, onEvent],
  );
  useChannel(
    active && uuid ? RealtimeChannels.org(active.uuid) : null,
    handler,
  );
}

export function useSetupActions(slug: string) {
  const qc = useQueryClient();
  const onRun = (run: SetupRun) => storeRun(qc, slug, run);
  const fromTemplate = useMutation({
    mutationFn: (key: string) => setupService.useTemplate(slug, key),
    onSuccess: onRun,
  });
  const discover = useMutation({
    mutationFn: (body: SetupDiscoverRequest) =>
      setupService.discover(slug, body),
    onSuccess: onRun,
  });
  const apply = useMutation({
    mutationFn: ({ uuid, selection }: { uuid: string; selection: string[] }) =>
      setupService.apply(slug, uuid, selection),
    onSuccess: onRun,
  });
  const undo = useMutation({
    mutationFn: (uuid: string) => setupService.undo(slug, uuid),
    onSuccess: onRun,
  });
  return { fromTemplate, discover, apply, undo };
}
