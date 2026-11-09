import type { SuggestionAdapter } from "@assistant-ui/react";

import type { AssistantCustom } from "../lib/messages";

/**
 * assistant-ui `SuggestionAdapter`: the quick replies the backend saved
 * with the last answer (`suggest_replies`, the `suggestions` stream event),
 * read from that message's metadata. Nothing is generated client-side.
 */
export const suggestionAdapter: SuggestionAdapter = {
  generate({ messages }) {
    const last = messages.at(-1);
    if (!last || last.role !== "assistant") return Promise.resolve([]);
    const custom = last.metadata.custom as AssistantCustom | undefined;
    return Promise.resolve(
      (custom?.suggestions ?? []).map((prompt) => ({ prompt })),
    );
  },
};
