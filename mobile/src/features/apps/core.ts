/**
 * Sidebar app registry rows (CF-172), with the panel's vocabulary
 * (`frontend/src/features/sidebar-apps/lib/protocol.ts`). Free of app
 * imports so it runs under `node --test`.
 */

const PLACEMENTS = ['conversation_sidebar', 'contact_panel', 'nav'] as const;
export type Placement = (typeof PLACEMENTS)[number];
export function placementKey(value: string): Placement | null {
  return (PLACEMENTS as readonly string[]).includes(value) ? (value as Placement) : null;
}

const SCOPES = [
  'conversation.read',
  'contact.read',
  'conversation.reply',
  'conversation.note',
  'conversation.status',
  'conversation.priority',
  'conversation.assign',
  'conversation.tag',
] as const;
/** `conversation.reply` → `conversation_reply`, the catalog key; `null` for a scope the app does not know. */
export function scopeKey(value: string) {
  return (SCOPES as readonly string[]).includes(value) ? value.replace('.', '_') : null;
}

/** `conversation.export` → `Conversation export`: an unknown value, shown as text instead of a key. */
export function readable(value: string | null | undefined) {
  const text = (value ?? '').replace(/[._-]+/g, ' ').trim();
  return text ? text.charAt(0).toUpperCase() + text.slice(1) : '—';
}

type App = { uuid: string; name: string; placement: string; enabled: boolean };

/** Enabled apps by placement, in registry order; an unknown placement gets its own group, last. */
export function groupApps<T extends App>(apps: T[]): { placement: string; apps: T[] }[] {
  const groups = new Map<string, T[]>();
  for (const app of apps) {
    if (!app.enabled) continue;
    groups.set(app.placement, [...(groups.get(app.placement) ?? []), app]);
  }
  const order = (p: string) => {
    const i = (PLACEMENTS as readonly string[]).indexOf(p);
    return i === -1 ? PLACEMENTS.length : i;
  };
  return [...groups.entries()]
    .sort(([a], [b]) => order(a) - order(b))
    .map(([placement, list]) => ({ placement, apps: list }));
}

/**
 * The host only (`crm.example.com`) of an app URL, so the member sees where
 * it lives without the full link. `null` unless it is a plain `https://`
 * address: no user info (`user:pass@`), which could dress up another host.
 */
export function appHost(url: string) {
  const match = /^https:\/\/([^/?#\s]+)/i.exec(url.trim());
  const authority = match?.[1];
  if (!authority || authority.includes('@')) return null;
  return authority.toLowerCase();
}
