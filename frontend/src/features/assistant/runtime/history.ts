import {
  ExportedMessageRepository,
  type ThreadHistoryAdapter,
} from "@assistant-ui/react";

import { toThreadMessages } from "../lib/messages";
import { assistantService } from "../services/assistant.service";

/**
 * assistant-ui `ThreadHistoryAdapter` over `GET threads/{uuid}/messages`.
 * The API persists every message itself (user, reply, cards, notes), so
 * `append` / `update` only acknowledge. A thread that has no backend id yet
 * (a new chat) loads empty; the id is read when `load` runs.
 */
export function createHistoryAdapter(
  slug: string,
  localId: string,
  remoteId: () => string | undefined,
): ThreadHistoryAdapter {
  return {
    scopeId: `${slug}:${localId}`,
    async load() {
      const id = remoteId();
      if (!id) return { messages: [] };
      const { items } = await assistantService.messages(slug, id);
      return ExportedMessageRepository.fromArray(toThreadMessages(items));
    },
    async append() {},
    async update() {},
  };
}
