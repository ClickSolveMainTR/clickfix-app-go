import { readJSON, storage, writeJSON } from '@/core/storage';

/**
 * Unsent composer text per conversation (CF-81), kept on the phone so leaving
 * a conversation, a call or an app kill never loses a half-written reply
 * (Front, Superhuman and WhatsApp keep drafts the same way). The mode (reply
 * or note) is kept with it. Sign-out clears them with the send queue.
 */
export type Draft = { text: string; mode: 'reply' | 'note' };

const KEY = 'inbox.drafts';
const id = (org: string, conversation: string) => `${org}/${conversation}`;

export function readDraft(org: string, conversation: string): Draft | null {
  return readJSON<Record<string, Draft>>(KEY)?.[id(org, conversation)] ?? null;
}

export function writeDraft(org: string, conversation: string, draft: Draft) {
  const all = readJSON<Record<string, Draft>>(KEY) ?? {};
  if (draft.text.trim()) all[id(org, conversation)] = draft;
  else delete all[id(org, conversation)];
  writeJSON(KEY, all);
}

export function clearDrafts() {
  storage.remove(KEY);
}
