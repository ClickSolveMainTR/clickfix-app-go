/**
 * Where a push or a panel link opens in the app. Pushes carry the panel path
 * of what they are about (`data.url`, backend `notifications/usecase/expo.go`):
 * a conversation is `/inbox?c={uuid}` (mentions, assignments); the QR sign-in
 * page is `{panel}/login/qr/{session_id}`. Contacts, companies, knowledge
 * articles, billing, channels and team status open their mobile screens. Kept free of app imports so it runs
 * under `node --test`.
 */
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
/** QR session ids: 43 URL-safe characters (OpenAPI `QRSessionID`). */
const SESSION_ID = /^[A-Za-z0-9_-]{43}$/;
/** Signed video join codes: 51 URL-safe characters (backend videorooms/link.go). */
const VIDEO_CODE = /^[A-Za-z0-9_-]{51}$/;

/** An app route for a panel path or URL, else null (the push then only opens the app). */
export function appRoute(link: unknown): string | null {
  if (typeof link !== 'string' || !link.trim()) return null;
  let url: URL;
  try {
    url = new URL(link, 'https://panel.invalid');
  } catch {
    return null;
  }
  const parts = url.pathname.split('/').filter(Boolean);
  const uuidAt = (i: number) => (parts[i] && UUID.test(parts[i]) && parts.length === i + 1 ? parts[i] : null);
  if (parts[0] === 'inbox' && parts.length === 1) {
    const c = url.searchParams.get('c');
    if (c && UUID.test(c)) return `/conversation/${c}`;
    // The panel's "conversations of this contact / company" links open the record.
    const contact = url.searchParams.get('contact');
    if (contact && UUID.test(contact)) return `/contact/${contact}`;
    const company = url.searchParams.get('company');
    if (company && UUID.test(company)) return `/company/${company}`;
    return '/';
  }
  // Video rooms (CF-139): guest join links and the member room page.
  if (parts[0] === 'r' && parts.length === 2 && VIDEO_CODE.test(parts[1])) return `/r/${parts[1]}`;
  if (parts[0] === 'video' && parts.length === 2 && UUID.test(parts[1])) return `/video/${parts[1]}`;
  if (parts[0] === 'companies' && uuidAt(1)) return `/company/${parts[1]}`;
  if (parts[0] === 'contacts' && uuidAt(1)) return `/contact/${parts[1]}`;
  if (parts[0] === 'settings' && parts[1] === 'knowledge' && uuidAt(2)) return `/article/${parts[2]}`;
  if (parts[0] === 'billing' && parts.length === 1) return '/more/billing';
  if (parts[0] === 'channels') return '/more/channels';
  if (parts[0] === 'team-status' && parts.length === 1) return '/more/team';
  if (parts[0] === 'login' && parts[1] === 'qr' && parts[2] && parts.length === 3) {
    const id = decodeURIComponent(parts[2]);
    return SESSION_ID.test(id) ? `/qr-login/${id}` : null;
  }
  return null;
}

/** What a push is about: where it opens, in which organization, and the conversation a quick reply answers. */
export type PushTarget = { route: string | null; organization: string | null; conversation: string | null };

const SLUG = /^[a-z0-9][a-z0-9-]{0,62}$/;

/**
 * Reads a push's `data` (backend `notifications/usecase/expo.go`):
 * `url`, `organization` (slug, CF-81) and `conversation_uuid`. Anything
 * malformed is dropped, so a push never opens or answers the wrong thing.
 */
export function pushTarget(data: unknown): PushTarget {
  const d = (data && typeof data === 'object' ? data : {}) as Record<string, unknown>;
  const org = typeof d.organization === 'string' && SLUG.test(d.organization) ? d.organization : null;
  let conversation =
    typeof d.conversation_uuid === 'string' && UUID.test(d.conversation_uuid) ? d.conversation_uuid : null;
  const route = appRoute(d.url) ?? (conversation ? `/conversation/${conversation}` : null);
  if (!conversation && route?.startsWith('/conversation/')) conversation = route.slice('/conversation/'.length);
  return { route, organization: org, conversation };
}
