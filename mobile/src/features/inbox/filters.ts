/**
 * Inbox list filters (CF-79), pure so they run under `node --test`: the
 * quick view + status tab, the advanced filter sheet (team, assignee,
 * priority, channel, tag, unread, SLA, sort) and saved views
 * (`/v1/tenant/conversation-views`), turned into `/v1/tenant/conversations`
 * query parameters the way the panel's filter bar does.
 */
import { STATUS_QUERY, VIEW_QUERY, type InboxStatusTab, type InboxView } from './helpers.ts';

export type InboxSort = '-last_message_at' | '-created_at' | 'waiting_since' | '-priority';
export const SORTS: InboxSort[] = ['-last_message_at', 'waiting_since', '-priority', '-created_at'];

export type AdvancedFilters = {
  priorities: string[];
  channels: string[];
  tags: string[];
  /** Team uuid or `mine` (my teams). */
  team: string | null;
  /** Member uuid; only applies to the "All" view (the other views name the assignee). */
  assignee: string | null;
  unread: boolean;
  slaBreached: boolean;
  sort: InboxSort;
};

export const NO_FILTERS: AdvancedFilters = {
  priorities: [],
  channels: [],
  tags: [],
  team: null,
  assignee: null,
  unread: false,
  slaBreached: false,
  sort: '-last_message_at',
};

/** A saved view's stored filters (OpenAPI `ConversationViewFilters`). */
export type SavedViewFilters = {
  statuses?: string[];
  priorities?: string[];
  channel_types?: string[];
  inboxes?: string[];
  assignee?: string;
  tags?: string[];
  unread?: boolean;
  mentioned?: boolean;
  sla_breached?: boolean;
  q?: string;
  sort?: string;
};

export type InboxFilters = {
  view: InboxView;
  status: InboxStatusTab;
  q: string;
  adv: AdvancedFilters;
  /** A saved view replaces the view, tab and sheet filters; the search box still narrows it. */
  saved: { uuid: string; filters: SavedViewFilters } | null;
};

type Query = Record<string, string>;

const csv = (v: string[] | undefined) => (v && v.length ? v.join(',') : undefined);

function put(q: Query, key: string, v: string | undefined | null | false) {
  if (v) q[key] = v;
}

/** `/v1/tenant/conversations` query (without limit / offset) for the current filters. */
export function listQuery(f: InboxFilters): Query {
  const q: Query = {};
  const search = f.q.trim();
  if (f.saved) {
    const s = f.saved.filters;
    put(q, 'status', csv(s.statuses));
    put(q, 'priority', csv(s.priorities));
    put(q, 'channel_type', csv(s.channel_types));
    put(q, 'inbox', csv(s.inboxes));
    put(q, 'assignee', s.assignee);
    put(q, 'tag', csv(s.tags));
    put(q, 'unread', s.unread && '1');
    put(q, 'mentioned', s.mentioned && '1');
    put(q, 'sla_breached', s.sla_breached && '1');
    put(q, 'q', search || s.q);
    q.sort = s.sort || '-last_message_at';
    return q;
  }
  const a = f.adv;
  const view = VIEW_QUERY[f.view];
  put(q, 'assignee', view.assignee ?? (f.view === 'all' ? a.assignee : null));
  put(q, 'mentioned', view.mentioned);
  put(q, 'status', STATUS_QUERY[f.status]);
  put(q, 'priority', csv(a.priorities));
  put(q, 'channel_type', csv(a.channels));
  put(q, 'tag', csv(a.tags));
  put(q, 'team', a.team);
  put(q, 'unread', a.unread && '1');
  put(q, 'sla_breached', a.slaBreached && '1');
  put(q, 'q', search);
  q.sort = a.sort;
  return q;
}

/** How many sheet filters are on (the badge on the filter button). */
export function activeFilterCount(a: AdvancedFilters): number {
  return (
    (a.priorities.length ? 1 : 0) +
    (a.channels.length ? 1 : 0) +
    (a.tags.length ? 1 : 0) +
    (a.team ? 1 : 0) +
    (a.assignee ? 1 : 0) +
    (a.unread ? 1 : 0) +
    (a.slaBreached ? 1 : 0) +
    (a.sort !== NO_FILTERS.sort ? 1 : 0)
  );
}

/** Adds or removes `v` from a multi-choice filter. */
export function toggle(list: string[], v: string): string[] {
  return list.includes(v) ? list.filter((x) => x !== v) : [...list, v];
}
