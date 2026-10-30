import type { components } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";

export type CopilotInput = components["schemas"]["AICopilotInput"];
export type CopilotOperation = CopilotInput["operation"];
export type CopilotResult = components["schemas"]["AIRunResult"];
export type AICitation = components["schemas"]["AICitation"];
export type AIAgentJob = components["schemas"]["AIAgentJob"];
export type AIAvailability = components["schemas"]["AIAvailability"];

/**
 * Agent copilot (CF-66). Results are suggestions only: nothing is sent or
 * executed server-side. Errors stay silent here because the copilot panel
 * renders them inline (AI disabled, limits, guard, provider). The client
 * middleware adds the active organization header.
 */
export const copilotService = {
  async run(uuid: string, body: CopilotInput, signal?: AbortSignal) {
    return unwrap<CopilotResult>(
      await apiClient.POST("/v1/tenant/conversations/{uuid}/copilot", {
        params: { path: { uuid } },
        body,
        signal,
      }),
      { silent: true },
    );
  },

  /**
   * Whether AI actions can run for the organization and, when not, why
   * (AI_NOT_CONFIGURED, AI_DISABLED, AI_NOT_ENTITLED).
   */
  async availability(org: string) {
    return unwrap<AIAvailability>(
      await apiClient.GET("/v1/tenant/ai/availability", {
        params: { header: { "X-Organization": org } },
      }),
      { silent: true },
    );
  },

  /** Autonomous agent jobs of the conversation (newest first). */
  async jobs(uuid: string) {
    const data = await unwrap<{ items: AIAgentJob[] }>(
      await apiClient.GET("/v1/tenant/conversations/{uuid}/ai", {
        params: { path: { uuid }, query: { limit: 5 } },
      }),
      { silent: true },
    );
    return data.items;
  },
};
