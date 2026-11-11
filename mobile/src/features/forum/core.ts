/**
 * Community forum moderation (CF-171): how a queued post and a topic read.
 * Rules and wording follow the panel (`frontend/src/features/forum/`). Free
 * of app imports so it runs under `node --test`.
 */

/** `t` of the `admin` namespace; every key here starts with `forum.`. */
export type T = (key: string, params?: Record<string, string | number>) => string;

/**
 * A pending post of the moderation queue. The OpenAPI item is an open object
 * (the backend returns the stored forum post); only the fields the queue
 * shows are read. `topic_id` is null for a topic and set for a reply.
 */
export type ModerationItem = {
  uuid: string;
  title: string;
  body: string;
  flag_reason: string;
  topic_id: number | null;
  created_at: string;
};

const text = (v: unknown) => (typeof v === 'string' ? v : '');

/** One queue row from the open API object; a row without an id cannot be acted on and is dropped. */
export function readModerationItem(raw: Record<string, unknown> | null | undefined): ModerationItem | null {
  const uuid = text(raw?.uuid);
  if (!raw || !uuid) return null;
  return {
    uuid,
    title: text(raw.title),
    body: text(raw.body),
    flag_reason: text(raw.flag_reason),
    topic_id: typeof raw.topic_id === 'number' ? raw.topic_id : null,
    created_at: text(raw.created_at),
  };
}

export function isReply(item: Pick<ModerationItem, 'topic_id'>) {
  return item.topic_id != null;
}

const KNOWN_FLAGS = new Set(['unsafe_instructions', 'link_spam', 'awaiting_review', 'ai_reviewed']);
const AI_PREFIX = 'ai: ';

/** Why the post waits: a known reason translated, an AI note quoted, anything else shown as it is. */
export function flagLabel(t: T, reason: string): string | null {
  if (!reason) return null;
  if (KNOWN_FLAGS.has(reason)) return t(`forum.flag.${reason}`);
  if (reason.startsWith(AI_PREFIX)) return t('forum.flag.ai', { reason: reason.slice(AI_PREFIX.length) });
  return reason.replace(/_+/g, ' ');
}

/** Flags that warn the moderator (the panel's red badge). */
export function isRisky(reason: string) {
  return reason === 'unsafe_instructions' || reason === 'link_spam' || reason.startsWith(AI_PREFIX);
}

export type Outcome = 'approve' | 'reject' | 'spam';

/** The API has two outcomes: reject and "spam" both hide the post. */
export function outcomeStatus(outcome: Outcome): 'published' | 'rejected' {
  return outcome === 'approve' ? 'published' : 'rejected';
}

/** Hiding a post cannot be undone from the app, so it asks first; publishing does not (like the panel). */
export function needsConfirm(outcome: Outcome) {
  return outcome !== 'approve';
}

/** The category's name for its slug; an unknown slug is shown readable. */
export function categoryName(categories: { slug: string; name: string }[] | undefined, slug: string) {
  return categories?.find((c) => c.slug === slug)?.name ?? slug.replace(/[-_]+/g, ' ');
}

/** The accepted reply of a topic: the detail's answer wins over the list row (the list may be older). */
export function acceptedReply(
  detail: { accepted_answer: { uuid: string } | null } | undefined,
  summary?: { accepted_reply_uuid: string | null },
) {
  if (detail) return detail.accepted_answer?.uuid ?? null;
  return summary?.accepted_reply_uuid ?? null;
}

/** Is the post still in the pages of the queue (which lists pending posts only)? */
export function stillPending(pages: { items: { uuid: string }[] }[] | undefined, uuid: string) {
  return (pages ?? []).some((p) => p.items.some((i) => i.uuid === uuid));
}

/** Thrown before a decision is sent when the post left the queue meanwhile (another moderator decided). */
export class AlreadyHandled extends Error {
  constructor() {
    super('forum post already handled');
    this.name = 'AlreadyHandled';
  }
}

/**
 * Why a decision was refused, as a message key; null leaves it to the API's
 * own error text. 404: the post is gone. 409: a reply cannot be published
 * before its topic (`forum/usecase/service.go`, `Moderate`).
 */
export function decisionProblem(
  error: unknown,
  status: number | null,
): 'forum.already_handled' | 'forum.topic_first' | null {
  if (error instanceof AlreadyHandled || status === 404) return 'forum.already_handled';
  if (status === 409) return 'forum.topic_first';
  return null;
}

/** Long enough that the queue row cuts it: the row then offers "show all". */
export function isLongBody(body: string, maxChars = 320, maxLines = 8) {
  return body.length > maxChars || body.split('\n').length > maxLines;
}

export type PostKind = 'topic' | 'reply';
export type VisibilityAction = 'unpublish' | 'republish';

/**
 * Taking a published post down and publishing it again (CF-182). Both ask
 * first: a takedown hides the post from every visitor at once, and
 * republishing shows it again. The wording names the kind of post.
 */
export function visibilityCopy(action: VisibilityAction, kind: PostKind) {
  return {
    title: `forum.${action}_${kind}_title`,
    body: `forum.${action}_${kind}_body`,
    action: `forum.${action}`,
    destructive: action === 'unpublish',
  };
}

/**
 * Why a takedown or republish was refused, as a message key; null leaves it
 * to the API's own text. 404: the post is gone. 409: it is no longer in the
 * status the action needs (someone else acted, or a removed reply's topic
 * is down).
 */
export function visibilityProblem(
  status: number | null,
  action: VisibilityAction,
): 'forum.already_handled' | 'forum.unpublish_conflict' | 'forum.republish_conflict' | null {
  if (status === 404) return 'forum.already_handled';
  if (status === 409) return action === 'republish' ? 'forum.republish_conflict' : 'forum.unpublish_conflict';
  return null;
}

/** The line under a removed post: who took it down, and why when a reason was given. */
export function removedLine(t: T, item: { removed_by_name: string | null; removed_reason: string }, when: string) {
  const by = item.removed_by_name ? t('forum.removed_by', { name: item.removed_by_name, when }) : when;
  return item.removed_reason ? `${by} · ${t('forum.removed_reason', { reason: item.removed_reason })}` : by;
}
