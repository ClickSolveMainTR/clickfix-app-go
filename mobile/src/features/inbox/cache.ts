import type { InfiniteData, QueryClient } from '@tanstack/react-query';

import type { components } from '@/core/api/schema';

import type { InboxFilters } from './filters';
import { upsertMessagePages } from './helpers';

/** Inbox query keys and cache writers, shared by the hooks (`api.ts`) and the send queue (`outbox.ts`). */

export type Conversation = components['schemas']['Conversation'];
export type ConversationDetail = components['schemas']['ConversationDetail'];
export type ConversationPage = components['schemas']['ConversationPage'];
export type Message = components['schemas']['Message'];
export type MessagePage = components['schemas']['MessagePage'];
export type ConversationUpdate = components['schemas']['ConversationUpdate'];

/**
 * A message the composer sent that the server has not confirmed yet.
 * `queued`: waiting in the send queue for the network.
 */
export type OptimisticMessage = Message & { optimistic?: true; queued?: boolean };

export type { InboxFilters } from './filters';

export const inboxKeys = {
  all: (org: string) => ['inbox', org] as const,
  lists: (org: string) => [...inboxKeys.all(org), 'list'] as const,
  list: (org: string, f: InboxFilters) => [...inboxKeys.lists(org), f] as const,
  counts: (org: string) => [...inboxKeys.all(org), 'counts'] as const,
  agents: (org: string) => [...inboxKeys.all(org), 'agents'] as const,
  detail: (org: string, uuid: string) => [...inboxKeys.all(org), 'detail', uuid] as const,
  sla: (org: string, uuid: string) => [...inboxKeys.all(org), 'sla', uuid] as const,
  contact: (org: string, uuid: string) => [...inboxKeys.all(org), 'contact', uuid] as const,
  messages: (org: string, uuid: string) => [...inboxKeys.all(org), 'messages', uuid] as const,
};

export type MessagesData = InfiniteData<MessagePage, string | undefined>;

export function upsertMessage(qc: QueryClient, key: readonly unknown[], msg: OptimisticMessage) {
  qc.setQueryData<MessagesData>(key, (data) => (data ? { ...data, pages: upsertMessagePages(data.pages, msg) } : data));
}

/** A cached message by its composer `client_id`. */
export function findByClientId(qc: QueryClient, key: readonly unknown[], clientId: string) {
  return qc
    .getQueryData<MessagesData>(key)
    ?.pages.flatMap((p) => p.items)
    .find((m) => m.client_id === clientId) as OptimisticMessage | undefined;
}
