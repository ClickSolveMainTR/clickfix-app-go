/**
 * The send queue's pure rules (CF-80): what is waiting, what a failed send
 * means, and how long to wait before the next try. Kept free of app imports
 * so they run under `node --test`; `outbox.ts` holds the store and the sender.
 */

/** A file already uploaded to `/v1/tenant/attachments` and waiting to go out with its message. */
export type OutboxAttachment = { uuid: string; filename: string; content_type: string; size_bytes: number };

/**
 * One reply or note the server has not confirmed. `client_id` makes every
 * try idempotent: the API answers `200` with the stored message when a lost
 * response is sent again.
 */
export type OutboxItem = {
  org: string;
  conversation: string;
  client_id: string;
  content: string;
  private: boolean;
  attachments: OutboxAttachment[];
  /** Members @mentioned in a note (uuids); they get a notification. */
  mentions?: string[];
  sender: { uuid: string | null; name: string };
  created_at: string;
  attempts: number;
  /** Epoch ms before which the sender leaves the item alone (backoff). */
  next_at: number;
  /**
   * Error code of a refusal (4xx). The item stays on the phone as a failed
   * bubble the agent can retry or delete, so a refetch never loses its text.
   */
  failed?: string;
};

/** Adds an item, or replaces the one with the same `client_id` (a manual retry). */
export function enqueue(list: OutboxItem[], item: OutboxItem): OutboxItem[] {
  return [...list.filter((i) => i.client_id !== item.client_id), item];
}

export function removeItem(list: OutboxItem[], clientId: string): OutboxItem[] {
  return list.filter((i) => i.client_id !== clientId);
}

/** Items of one conversation, oldest first. */
export function itemsFor(list: OutboxItem[], org: string, conversation: string): OutboxItem[] {
  return list
    .filter((i) => i.org === org && i.conversation === conversation)
    .sort((a, b) => a.created_at.localeCompare(b.created_at));
}

/**
 * What a failed send means: `retry` keeps the item (no network, timeouts,
 * rate limits, server errors); `drop` fails it for good (the API refused the
 * message itself, e.g. the reply window closed or the member lost access).
 * `0` is the client's NETWORK_ERROR.
 */
export function outcomeOf(status: number): 'retry' | 'drop' {
  if (status === 0 || status === 408 || status === 425 || status === 429 || status >= 500) return 'retry';
  return 'drop';
}

/** 2 s, 4 s, 8 s … capped at 2 minutes; a network change or foreground retries at once anyway. */
export function backoffMs(attempts: number): number {
  return Math.min(120_000, 1000 * 2 ** Math.max(1, Math.min(attempts, 10)));
}

/** Items that may be sent now, oldest first (one conversation keeps its order); failed ones wait for the agent. */
export function dueItems(list: OutboxItem[], now: number): OutboxItem[] {
  return [...list]
    .filter((i) => !i.failed && i.next_at <= now)
    .sort((a, b) => a.created_at.localeCompare(b.created_at));
}
