import { onlineManager } from '@tanstack/react-query';
import { AppState } from 'react-native';
import { create } from 'zustand';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { queryClient } from '@/core/query';
import { readJSON, writeJSON } from '@/core/storage';
import { toast } from '@/ui/toast';

import { findByClientId, inboxKeys, type MessagesData, type OptimisticMessage, upsertMessage } from './cache';
import { backoffMs, dueItems, enqueue, type OutboxItem, outcomeOf, removeItem } from './outbox-core';

/**
 * The send queue (CF-80): every reply and note goes through here. It is
 * written to disk before the first try, so a message typed in a lift, in
 * airplane mode or right before the app is killed is sent as soon as the
 * phone is back online (network change, foreground, next launch). Each item
 * keeps its `client_id`, so a send whose answer was lost is never stored
 * twice. A refusal from the API (4xx) fails the bubble instead; it stays in
 * the queue (not sent again) until the agent retries or deletes it.
 */

const KEY = 'inbox.outbox';

type Outbox = { items: OutboxItem[]; sending: string | null };

export const useOutbox = create<Outbox>(() => ({ items: readJSON<OutboxItem[]>(KEY) ?? [], sending: null }));

function save(items: OutboxItem[]) {
  writeJSON(KEY, items);
  useOutbox.setState({ items });
}

/** The optimistic timeline bubble for a queued item. */
export function toOptimistic(item: OutboxItem, queued: boolean): OptimisticMessage {
  return {
    uuid: item.client_id,
    conversation_uuid: item.conversation,
    direction: item.private ? 'internal' : 'out',
    sender: { type: 'agent', uuid: item.sender.uuid, name: item.sender.name },
    kind: item.private ? 'note' : item.attachments.length && !item.content ? 'media' : 'text',
    content: item.content,
    attributes: {},
    private: item.private,
    status: item.failed ? 'failed' : 'pending',
    error: item.failed ?? '',
    client_id: item.client_id,
    reply_to: null,
    mentions: [],
    attachments: item.attachments.map((a) => ({ ...a, download_url: '', thumbnail_url: null })),
    sent_at: null,
    delivered_at: null,
    read_at: null,
    created_at: item.created_at,
    optimistic: true,
    queued,
  };
}

/** Queues a reply / note, shows it at once and tries to send it. */
export function queueMessage(item: Omit<OutboxItem, 'attempts' | 'next_at'>) {
  const full: OutboxItem = { ...item, attempts: 0, next_at: 0, failed: undefined };
  save(enqueue(useOutbox.getState().items, full));
  upsertMessage(queryClient, inboxKeys.messages(item.org, item.conversation), toOptimistic(full, false));
  void flushOutbox();
}

/** Sends a failed bubble again (a refusal that may have cleared, e.g. a re-opened window). */
export function requeueFailed(org: string, conversation: string, clientId: string) {
  const kept = useOutbox.getState().items.find((i) => i.client_id === clientId);
  if (kept) {
    queueMessage({ ...kept, failed: undefined, created_at: new Date().toISOString() });
    return;
  }
  const key = inboxKeys.messages(org, conversation);
  const m = findByClientId(queryClient, key, clientId);
  if (!m) return;
  queueMessage({
    org,
    conversation,
    client_id: clientId,
    content: m.content,
    private: m.private,
    attachments: m.attachments.map(({ uuid, filename, content_type, size_bytes }) => ({
      uuid,
      filename,
      content_type,
      size_bytes,
    })),
    sender: { uuid: m.sender.uuid, name: m.sender.name },
    created_at: new Date().toISOString(),
  });
}

/** Removes a failed bubble the agent gave up on (it never reached the server). */
export function discardFailed(org: string, conversation: string, clientId: string) {
  save(removeItem(useOutbox.getState().items, clientId));
  queryClient.setQueryData<MessagesData>(inboxKeys.messages(org, conversation), (data) =>
    data
      ? { ...data, pages: data.pages.map((p) => ({ ...p, items: p.items.filter((m) => m.client_id !== clientId) })) }
      : data,
  );
}

/** Drops everything (sign-out): queued messages belong to the signed-in member. */
export function clearOutbox() {
  save([]);
}

let flushing: Promise<void> | null = null;
let timer: ReturnType<typeof setTimeout> | null = null;

function schedule() {
  if (timer) clearTimeout(timer);
  const items = useOutbox.getState().items.filter((i) => !i.failed);
  if (!items.length) return;
  const wait = Math.max(500, Math.min(...items.map((i) => i.next_at)) - Date.now());
  timer = setTimeout(() => void flushOutbox(), wait);
}

async function sendOne(item: OutboxItem) {
  const key = inboxKeys.messages(item.org, item.conversation);
  try {
    const msg = await unwrap(
      api.POST('/v1/tenant/conversations/{uuid}/messages', {
        params: { header: { 'X-Organization': item.org }, path: { uuid: item.conversation } },
        body: {
          content: item.content,
          private: item.private,
          client_id: item.client_id,
          ...(item.attachments.length ? { attachments: item.attachments.map((a) => a.uuid) } : {}),
          ...(item.mentions?.length ? { mentions: item.mentions } : {}),
        },
      }),
    );
    save(removeItem(useOutbox.getState().items, item.client_id));
    upsertMessage(queryClient, key, { ...msg, optimistic: undefined, queued: false });
    void queryClient.invalidateQueries({ queryKey: inboxKeys.lists(item.org) });
  } catch (e) {
    const status = isApiError(e) ? e.status : 0;
    if (outcomeOf(status) === 'retry') {
      const attempts = item.attempts + 1;
      save(
        useOutbox
          .getState()
          .items.map((i) =>
            i.client_id === item.client_id ? { ...i, attempts, next_at: Date.now() + backoffMs(attempts) } : i,
          ),
      );
      upsertMessage(queryClient, key, toOptimistic(item, true));
      return false;
    }
    const code = (isApiError(e) ? e.code : undefined) ?? 'not_sent';
    const failed = { ...item, failed: code };
    save(useOutbox.getState().items.map((i) => (i.client_id === item.client_id ? failed : i)));
    upsertMessage(queryClient, key, toOptimistic(failed, false));
    toast.error(errorMessage(code));
  }
  return true;
}

/** Sends what is due, oldest first; stops at the first network failure (the rest would fail too). */
export function flushOutbox(): Promise<void> {
  flushing ??= (async () => {
    try {
      for (const item of dueItems(useOutbox.getState().items, Date.now())) {
        if (!onlineManager.isOnline()) {
          for (const i of useOutbox.getState().items.filter((x) => !x.failed))
            upsertMessage(queryClient, inboxKeys.messages(i.org, i.conversation), toOptimistic(i, true));
          break;
        }
        useOutbox.setState({ sending: item.client_id });
        const ok = await sendOne(item);
        if (!ok) break;
      }
    } finally {
      useOutbox.setState({ sending: null });
      flushing = null;
      schedule();
    }
  })();
  return flushing;
}

let started = false;

/** Flushes on launch, when the network returns and when the app comes to the foreground. */
export function startOutbox() {
  if (started) return;
  started = true;
  const now = () => {
    // A network change or foreground is a reason to try at once, whatever the backoff said.
    save(useOutbox.getState().items.map((i) => (i.failed ? i : { ...i, next_at: 0 })));
    void flushOutbox();
  };
  onlineManager.subscribe((online) => online && now());
  AppState.addEventListener('change', (s) => s === 'active' && now());
  void flushOutbox();
}
