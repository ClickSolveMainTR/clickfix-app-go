import { useMutation } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';

/**
 * Agent copilot (CF-82) on the aicore API (`POST /v1/tenant/conversations/{uuid}/copilot`,
 * CF-62..CF-68): a suggestion is only text; nothing is sent or changed until the agent
 * puts it in the composer and sends it. Metered: 429 means the AI credits ran out.
 */

export type CopilotInput = components['schemas']['AICopilotInput'];
export type CopilotOperation = CopilotInput['operation'];
export type CopilotResult = components['schemas']['AIRunResult'];

export function useCopilot(conversation: string) {
  return useMutation({
    mutationFn: (input: CopilotInput) =>
      unwrap(
        api.POST('/v1/tenant/conversations/{uuid}/copilot', {
          // The route is organization-scoped; openapi does not list the header, the client adds it.
          params: { path: { uuid: conversation } },
          headers: orgHeader(),
          body: input,
        }),
      ),
  });
}
