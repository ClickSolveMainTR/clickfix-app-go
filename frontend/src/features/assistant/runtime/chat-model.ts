import type {
  ChatModelAdapter,
  ChatModelRunResult,
  ThreadAssistantMessagePart,
  ThreadMessage,
} from "@assistant-ui/react";

import {
  actionPart,
  assistantStatus,
  type ActionResult,
  type AssistantCustom,
  type DecisionQueue,
} from "../lib/messages";
import {
  assistantService,
  type AssistantAction,
  type AssistantContext,
  type AssistantMessage,
} from "../services/assistant.service";

export type ChatModelDeps = {
  slug: string;
  locale: string;
  /** Surface, route and the guide key of the question that opened the chat. */
  context: () => AssistantContext;
  /** The backend thread of the current assistant-ui thread, created on demand. */
  ensureThread: () => Promise<string>;
  /** Card decisions taken through the approval gate, consumed by the continuation run. */
  decisions: DecisionQueue;
};

type Event = { name: string; data: unknown };

/** Text of the user's message: the text parts joined. */
function userText(message: ThreadMessage | undefined): string {
  if (!message || message.role !== "user") return "";
  return message.content
    .map((p) => (p.type === "text" ? p.text : ""))
    .join("\n")
    .trim();
}

/** Turns the callback-based SSE reader into an async iterator. */
function eventQueue() {
  const buffer: Event[] = [];
  let waiter: (() => void) | null = null;
  let finished = false;
  let failure: unknown;
  const wake = () => {
    waiter?.();
    waiter = null;
  };
  return {
    emit(name: string, data: unknown) {
      buffer.push({ name, data });
      wake();
    },
    done(err?: unknown) {
      finished = true;
      failure = err;
      wake();
    },
    async *[Symbol.asyncIterator]() {
      for (;;) {
        if (buffer.length) {
          yield buffer.shift()!;
          continue;
        }
        if (finished) {
          if (failure) throw failure;
          return;
        }
        await new Promise<void>((resolve) => {
          waiter = resolve;
        });
      }
    },
  };
}

/**
 * assistant-ui `ChatModelAdapter` over the ClickFix SSE stream
 * (`message.created`, `status`, `delta`, `suggestions`, `action`, `done`,
 * `error`). Every yield carries the accumulated text and cards, never a
 * delta. A run that follows an approval gate decision (confirm / reject)
 * does not stream: it calls the action endpoints and settles the cards.
 */
export function createChatModel(deps: ChatModelDeps): ChatModelAdapter {
  return {
    async *run({ messages, abortSignal }) {
      const thread = await deps.ensureThread();
      const decisions = deps.decisions.drain();
      if (decisions.length) {
        yield* settleDecisions(deps, thread, decisions);
        return;
      }
      const content = userText(messages.at(-1));
      if (!content) return;

      const queue = eventQueue();
      assistantService
        .stream(
          deps.slug,
          thread,
          content,
          deps.context(),
          deps.locale,
          abortSignal,
          queue.emit,
        )
        .then(
          () => queue.done(),
          (err: unknown) => queue.done(err),
        );

      let text = "";
      const actions: AssistantAction[] = [];
      let suggestions: string[] | undefined;
      let status = "";
      let reply: AssistantMessage | undefined;
      const snapshot = (): ChatModelRunResult => ({
        content: [
          ...(text
            ? ([{ type: "text", text }] as ThreadAssistantMessagePart[])
            : []),
          ...actions.map((a) => actionPart(a) as ThreadAssistantMessagePart),
        ],
        metadata: { custom: { status } },
      });
      for await (const { name, data } of queue) {
        if (name === "delta") {
          text += (data as { text: string }).text;
          status = "";
        } else if (name === "status") {
          status = (data as { text: string }).text;
        } else if (name === "action") {
          actions.push(data as AssistantAction);
        } else if (name === "suggestions") {
          suggestions = data as string[];
        } else if (name === "done") {
          reply = (data as { reply: AssistantMessage }).reply;
          continue;
        } else continue;
        yield snapshot();
      }
      if (reply) {
        text = reply.content || text;
        if (reply.actions?.length)
          actions.splice(0, actions.length, ...reply.actions);
        suggestions = reply.suggestions ?? suggestions;
      }
      const custom: AssistantCustom = {
        uuid: reply?.uuid,
        role: "assistant",
        ...(suggestions?.length ? { suggestions } : {}),
      };
      yield {
        content: snapshot().content,
        status: assistantStatus(actions),
        metadata: { custom },
      };
    },
  };
}

/** Confirm / reject the decided cards; yields the settled tool-call parts. */
async function* settleDecisions(
  deps: ChatModelDeps,
  thread: string,
  decisions: ReturnType<DecisionQueue["drain"]>,
): AsyncGenerator<ChatModelRunResult, void> {
  const parts: ThreadAssistantMessagePart[] = [];
  for (const d of decisions) {
    if (!d.approved) {
      await assistantService.reject(deps.slug, thread, d.actionId);
      continue;
    }
    const out = await assistantService.confirm(
      deps.slug,
      thread,
      d.actionId,
      deps.locale,
      d.params,
    );
    const action = out.actions?.find((a) => a.id === d.actionId);
    if (!action) continue;
    const result: ActionResult = {
      action,
      ...(out.secret_once && Object.keys(out.secret_once).length
        ? { secrets: out.secret_once }
        : {}),
    };
    parts.push({
      type: "tool-call",
      toolCallId: action.id,
      toolName: "propose_action",
      args: {},
      argsText: "{}",
      artifact: action,
      result,
      isError: action.status === "failed",
    });
  }
  yield { content: parts, status: { type: "complete", reason: "stop" } };
}
