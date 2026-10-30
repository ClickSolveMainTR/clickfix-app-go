import assert from 'node:assert/strict';
import { test } from 'node:test';

import { activeFilterCount, type InboxFilters, listQuery, NO_FILTERS, toggle } from './filters.ts';

const base: InboxFilters = { view: 'all', status: 'open', q: '', adv: NO_FILTERS, saved: null };

test('quick view and status tab map like the panel', () => {
  assert.deepEqual(listQuery({ ...base, view: 'mine' }), {
    assignee: 'me',
    status: 'open,pending',
    sort: '-last_message_at',
  });
  assert.deepEqual(listQuery({ ...base, view: 'mentions', status: 'all' }), {
    mentioned: '1',
    sort: '-last_message_at',
  });
});

test('sheet filters become comma lists and flags', () => {
  const q = listQuery({
    ...base,
    q: '  iade ',
    adv: {
      ...NO_FILTERS,
      priorities: ['urgent', 'high'],
      channels: ['whatsapp'],
      tags: ['t1', 't2'],
      team: 'mine',
      assignee: 'u1',
      unread: true,
      slaBreached: true,
      sort: 'waiting_since',
    },
  });
  assert.deepEqual(q, {
    assignee: 'u1',
    status: 'open,pending',
    priority: 'urgent,high',
    channel_type: 'whatsapp',
    tag: 't1,t2',
    team: 'mine',
    unread: '1',
    sla_breached: '1',
    q: 'iade',
    sort: 'waiting_since',
  });
});

test('the assignee filter only applies to the All view', () => {
  assert.equal(
    listQuery({ ...base, view: 'unassigned', adv: { ...NO_FILTERS, assignee: 'u1' } }).assignee,
    'unassigned',
  );
});

test('a saved view replaces the tab and sheet filters; search still narrows', () => {
  const q = listQuery({
    ...base,
    view: 'mine',
    q: 'kargo',
    adv: { ...NO_FILTERS, unread: true },
    saved: {
      uuid: 'v1',
      filters: { statuses: ['open'], tags: ['t9'], sla_breached: true, q: 'x', sort: '-created_at' },
    },
  });
  assert.deepEqual(q, { status: 'open', tag: 't9', sla_breached: '1', q: 'kargo', sort: '-created_at' });
});

test('activeFilterCount and toggle', () => {
  assert.equal(activeFilterCount(NO_FILTERS), 0);
  assert.equal(activeFilterCount({ ...NO_FILTERS, tags: ['a'], unread: true, sort: '-priority' }), 3);
  assert.deepEqual(toggle(['a'], 'b'), ['a', 'b']);
  assert.deepEqual(toggle(['a', 'b'], 'a'), ['b']);
});
