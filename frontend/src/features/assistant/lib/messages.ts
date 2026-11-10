/**
 * Pure mapping between the ClickFix assistant API and assistant-ui messages.
 *
 * - A proposed action (confirmation card) is a `tool-call` part of the
 *   assistant message named `PROPOSE_TOOL`. The card data travels in the
 *   part's `artifact` (UI-only); `result` is set only once the server has
 *   answered (done / failed / rejected). While the card waits, the part
 *   carries an assistant-ui approval gate (`approval.id` = action id) and the
 *   message status is `requires-action`, so the Thread pauses on it.
 * - A `tool` role note from the API becomes an assistant message with
 *   `metadata.custom.role = "tool"`; the Thread renders it as a marker.
 * - Quick replies travel in `metadata.custom.suggestions`; the suggestion
 *   adapter reads them from the last assistant message.
 */
import type { ThreadMessageLike } from "@assistant-ui/react";

import type {
  AssistantAction,
  AssistantMessage,
} from "../services/assistant.service";

export const PROPOSE_TOOL = "propose_action";

export type ActionResult = {
  action: AssistantAction;
  /** Secrets returned once by confirm; kept in memory only. */
  secrets?: Record<string, string>;
};

export type AssistantCustom = {
  uuid?: string;
  role?: "assistant" | "tool";
  suggestions?: string[];
  interrupted?: boolean;
  /** Stream error code of a turn that failed; absent when the user stopped it. */
  errorCode?: string;
};

type Part = Exclude<ThreadMessageLike["content"], string>[number];

export const isPending = (a: AssistantAction) => a.status === "proposed";

/** The tool-call part of one action. */
export function actionPart(action: AssistantAction): Part {
  const pending = isPending(action);
  return {
    type: "tool-call",
    toolCallId: action.id,
    toolName: PROPOSE_TOOL,
    args: {},
    artifact: action,
    ...(pending
      ? { approval: { id: action.id } }
      : {
          result: { action } satisfies ActionResult,
          isError: action.status === "failed",
          approval:
            action.status === "rejected"
              ? { id: action.id, approved: false }
              : { id: action.id, approved: true },
        }),
  };
}

export function assistantStatus(
  actions: readonly AssistantAction[] | undefined,
): NonNullable<ThreadMessageLike["status"]> {
  return actions?.some(isPending)
    ? { type: "requires-action", reason: "tool-calls" }
    : { type: "complete", reason: "stop" };
}

/** One API message as an assistant-ui message. */
export function toThreadMessage(m: AssistantMessage): ThreadMessageLike {
  const createdAt = new Date(m.created_at);
  if (m.role === "user") {
    return { id: m.uuid, role: "user", content: m.content, createdAt };
  }
  const content: Part[] = [];
  if (m.content) content.push({ type: "text", text: m.content });
  for (const a of m.actions ?? []) content.push(actionPart(a));
  const custom: AssistantCustom = {
    uuid: m.uuid,
    role: m.role === "tool" ? "tool" : "assistant",
    ...(m.suggestions?.length ? { suggestions: m.suggestions } : {}),
    ...(m.interrupted ? { interrupted: true } : {}),
    ...(m.error_code ? { errorCode: m.error_code } : {}),
  };
  return {
    id: m.uuid,
    role: "assistant",
    content,
    createdAt,
    status:
      m.role === "tool"
        ? { type: "complete", reason: "stop" }
        : assistantStatus(m.actions),
    metadata: { custom },
  };
}

export const toThreadMessages = (items: readonly AssistantMessage[]) =>
  items.map(toThreadMessage);

/**
 * Decisions taken on a card (approve with edited fields, or reject) wait here
 * between the click and the continuation run the approval gate starts; the
 * chat model adapter consumes them and calls confirm / reject.
 */
export type Decision = {
  actionId: string;
  approved: boolean;
  params?: Record<string, unknown>;
};

export class DecisionQueue {
  private items = new Map<string, Decision>();
  push(d: Decision) {
    this.items.set(d.actionId, d);
  }
  remove(actionId: string) {
    this.items.delete(actionId);
  }
  /** Removes and returns every waiting decision. */
  drain(): Decision[] {
    const out = [...this.items.values()];
    this.items.clear();
    return out;
  }
  get size() {
    return this.items.size;
  }
}
