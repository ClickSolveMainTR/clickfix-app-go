"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  aiSettingsService,
  type AgentConfigState,
} from "../services/ai-settings.service";

export const agentKey = (slug: string) =>
  ["tenant", slug, "ai", "agent"] as const;

/** The organization's AI agent config (`GET /v1/tenant/ai/agent`). */
export function useAgentConfig(slug: string, enabled = true) {
  return useQuery({
    queryKey: agentKey(slug),
    queryFn: () => aiSettingsService.agent(slug),
    enabled: enabled && Boolean(slug),
  });
}

/** Channel types with `type` added (`enabled`) or removed, order kept. */
export function withChannel(
  channels: string[],
  type: string,
  enabled: boolean,
): string[] {
  if (enabled) return channels.includes(type) ? channels : [...channels, type];
  return channels.filter((c) => c !== type);
}

/**
 * Turns the AI agent on/off for a channel type (CF-243). The agent's scope
 * is its `channels` list of types, so this affects every instance of the
 * type. The config endpoint has no server-side revision check: the latest
 * config is read right before writing so only `channels` changes.
 */
export function useAgentChannelToggle(slug: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({
      type,
      enabled,
    }: {
      type: string;
      enabled: boolean;
    }): Promise<AgentConfigState> => {
      const current = await aiSettingsService.agent(slug);
      const channels = withChannel(current.config.channels, type, enabled);
      if (channels === current.config.channels) return current;
      return aiSettingsService.saveAgent(slug, {
        ...current.config,
        channels,
      });
    },
    onSuccess: (saved) => qc.setQueryData(agentKey(slug), saved),
  });
}
