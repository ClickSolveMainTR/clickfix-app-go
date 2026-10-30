/**
 * ClickFix Asistan's pure rules (CF-142 mobile, CF-141 API contract). Free
 * of app imports so they run under `node --test`.
 */

import type { components } from '../../core/api/schema';
export type ProposedAction = components['schemas']['AssistantProposedAction'];
export type ActionStatus = ProposedAction['status'];
export type AssistantMessage = components['schemas']['AssistantMessage'];
export type AssistantThread = components['schemas']['AssistantThread'];
export type AssistantStatus = components['schemas']['AssistantStatus'];

/** What the member sees: tool messages are the assistant's own bookkeeping. */
export function visibleMessages(messages: readonly AssistantMessage[]): AssistantMessage[] {
  return messages.filter((m) => m.role !== 'tool');
}

/** Quick replies of the assistant's last answer, only while it is the last word. */
export function lastSuggestions(messages: readonly AssistantMessage[]): string[] {
  const shown = visibleMessages(messages);
  const last = shown[shown.length - 1];
  if (!last || last.role !== 'assistant') return [];
  return (last.suggestions ?? [])
    .map((s) => s.trim())
    .filter(Boolean)
    .slice(0, 6);
}

/** Appends messages the server returned, skipping any the list already has. */
export function appendMessages(list: readonly AssistantMessage[] | undefined, add: AssistantMessage[]) {
  const have = new Set((list ?? []).map((m) => m.uuid));
  return [...(list ?? []), ...add.filter((m) => !have.has(m.uuid))];
}

/** Sets one action's status wherever it appears (after confirm / reject). */
export function withActionStatus(
  list: readonly AssistantMessage[] | undefined,
  actionId: string,
  status: ActionStatus,
): AssistantMessage[] {
  return (list ?? []).map((m) =>
    m.actions?.some((a) => a.id === actionId)
      ? { ...m, actions: m.actions.map((a) => (a.id === actionId ? { ...a, status } : a)) }
      : m,
  );
}

/** Messages the member may still send today; null when there is no limit. */
export function messagesLeft(status: AssistantStatus | undefined): number | null {
  if (!status || status.daily_limit <= 0) return null;
  return Math.max(status.daily_limit - status.used_today, 0);
}

/** A short line per action parameter for the card ("email: ayse@…"), at most `max`. */
export function paramLines(params: Record<string, unknown> | undefined, max = 4): string[] {
  const lines: string[] = [];
  for (const [k, v] of Object.entries(params ?? {})) {
    if (v === null || v === undefined || v === '') continue;
    const text = typeof v === 'object' ? JSON.stringify(v) : String(v);
    lines.push(`${k.replace(/_/g, ' ')}: ${text.length > 60 ? `${text.slice(0, 59)}…` : text}`);
    if (lines.length >= max) break;
  }
  return lines;
}
