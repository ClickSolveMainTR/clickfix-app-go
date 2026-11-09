"use client";

import {
  useAuiState,
  useLocalRuntime,
  useRemoteThreadListRuntime,
  type AssistantRuntime,
  type AttachmentAdapter,
  type ThreadHistoryAdapter,
} from "@assistant-ui/react";
import { useLayoutEffect, useMemo, useState } from "react";

import { DecisionQueue } from "../lib/messages";
import type { AssistantContext } from "../services/assistant.service";

import { createAttachmentAdapter } from "./attachments";
import { createChatModel } from "./chat-model";
import { createHistoryAdapter } from "./history";
import { suggestionAdapter } from "./suggestions";
import { createThreadListAdapter } from "./thread-list";

export type AssistantRuntimeOptions = {
  slug: string;
  locale: string;
  /** Current panel route; sent as the thread / message context. */
  route: string;
  /** Guide key of the question that opened the current chat, if any. */
  guideKey?: string | undefined;
  onDocumentUploaded: () => void;
  translate: (key: string) => string;
};

class Holder<T> {
  constructor(public value: T) {}
  set(value: T) {
    this.value = value;
  }
}

export type AssistantRuntimeHandle = {
  runtime: AssistantRuntime;
  decisions: DecisionQueue;
  /** Backend id of the visible thread, created when it has none yet. */
  ensureThread: () => Promise<string>;
};

/**
 * The assistant-ui runtime of one organization: a remote thread list over
 * the API's threads and, per thread, a LocalRuntime whose chat model reads
 * the SSE stream. History, attachments and suggestions are the adapters in
 * `runtime/`. The hook passed to the thread list runs inside assistant-ui's
 * own hook context, so it only uses assistant-ui hooks.
 */
export function useAssistantRuntime(
  options: AssistantRuntimeOptions,
): AssistantRuntimeHandle {
  // Holders read by the adapters outside render (event handlers, network
  // callbacks); they follow the latest render in a layout effect.
  const [latest] = useState(() => new Holder(options));
  const [runtimeRef] = useState(
    () => new Holder<AssistantRuntime | null>(null),
  );
  const { slug } = options;

  const shared = useMemo(() => {
    const decisions = new DecisionQueue();
    const context = (): AssistantContext => ({
      surface: "web",
      route: latest.value.route,
      ...(latest.value.guideKey ? { guide_key: latest.value.guideKey } : {}),
    });
    const ensureThread = async () => {
      const item = runtimeRef.value?.threads.mainItem;
      if (!item) throw new Error("ASSISTANT_INTERNAL");
      const { remoteId } = item.getState();
      if (remoteId) return remoteId;
      return (await item.initialize()).remoteId;
    };
    // One history adapter per assistant-ui thread, keyed by its local id:
    // the backend id arrives on the first message, and keying by it would
    // reload history mid-stream and drop the live reply. The adapter reads
    // the current backend id when it loads.
    const threads = new Map<
      string,
      { remoteId: string | undefined; history: ThreadHistoryAdapter }
    >();
    const threadState = (localId: string, remoteId: string | undefined) => {
      let entry = threads.get(localId);
      if (!entry) {
        const state = {
          remoteId,
          history: undefined as unknown as ThreadHistoryAdapter,
        };
        state.history = createHistoryAdapter(
          slug,
          localId,
          () => state.remoteId,
        );
        entry = state;
        threads.set(localId, entry);
      }
      entry.remoteId = remoteId;
      return entry;
    };
    const attachments: AttachmentAdapter = createAttachmentAdapter({
      slug,
      ensureThread,
      onUploaded: () => latest.value.onDocumentUploaded(),
      translate: (key) => latest.value.translate(key),
    });
    const chatModel = createChatModel({
      slug,
      get locale() {
        return latest.value.locale;
      },
      context,
      ensureThread,
      decisions,
    });
    const threadList = createThreadListAdapter({ slug, context });
    const useThreadRuntime = () => {
      // Runs in assistant-ui's hook context: only assistant-ui hooks here.
      const localId = useAuiState((s) => s.threadListItem.id);
      const remoteId = useAuiState((s) => s.threadListItem.remoteId);
      return useLocalRuntime(chatModel, {
        adapters: {
          history: threadState(localId, remoteId).history,
          attachments,
          suggestion: suggestionAdapter,
        },
      });
    };
    return { decisions, ensureThread, threadList, useThreadRuntime };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- holders are stable
  }, [slug]);

  const runtime = useRemoteThreadListRuntime({
    runtimeHook: shared.useThreadRuntime,
    adapter: shared.threadList,
  });
  useLayoutEffect(() => {
    latest.set(options);
    runtimeRef.set(runtime);
  });

  return {
    runtime,
    decisions: shared.decisions,
    ensureThread: shared.ensureThread,
  };
}
