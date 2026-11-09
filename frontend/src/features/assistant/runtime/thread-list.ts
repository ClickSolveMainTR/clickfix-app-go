import type { RemoteThreadListAdapter } from "@assistant-ui/react";
import { createAssistantStream } from "assistant-stream";

import {
  assistantService,
  type AssistantContext,
  type AssistantThread,
} from "../services/assistant.service";

export type ThreadListDeps = {
  slug: string;
  /** Context of a thread created now (route, guide key of the question). */
  context: () => AssistantContext;
};

const toMetadata = (t: AssistantThread) => ({
  status: "regular" as const,
  remoteId: t.uuid,
  title: t.title || undefined,
  lastMessageAt: new Date(t.updated_at),
});

const unsupported = (what: string) => () =>
  Promise.reject(new Error(`Assistant threads cannot be ${what}`));

/**
 * assistant-ui `RemoteThreadListAdapter` over `GET/POST threads`. The API
 * titles a thread itself after the first answer, so `generateTitle` only
 * waits for that title and streams it back. Threads are never renamed,
 * archived or deleted from the panel (the API has no such routes).
 */
export function createThreadListAdapter(
  deps: ThreadListDeps,
): RemoteThreadListAdapter {
  return {
    async list() {
      const { items } = await assistantService.threads(deps.slug);
      return { threads: items.map(toMetadata) };
    },
    async initialize() {
      const created = await assistantService.createThread(
        deps.slug,
        deps.context(),
      );
      return { remoteId: created.uuid };
    },
    async fetch(threadId) {
      const { items } = await assistantService.threads(deps.slug);
      const found = items.find((t) => t.uuid === threadId);
      if (!found) throw new Error("ASSISTANT_NOT_FOUND");
      return toMetadata(found);
    },
    async generateTitle(remoteId) {
      return createAssistantStream(async (controller) => {
        for (let attempt = 0; attempt < 8; attempt++) {
          const { items } = await assistantService
            .threads(deps.slug)
            .catch(() => ({ items: [] as AssistantThread[] }));
          const title = items.find((t) => t.uuid === remoteId)?.title;
          if (title) {
            controller.appendText(title);
            return;
          }
          await new Promise((resolve) => setTimeout(resolve, 1500));
        }
      });
    },
    rename: unsupported("renamed"),
    archive: unsupported("archived"),
    delete: unsupported("deleted"),
  };
}
