import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  attachmentKind,
  clientId,
  contactLabel,
  eventText,
  fallbackFileName,
  formatBytes,
  formatDuration,
  initials,
  slaBadge,
  snoozeUntil,
  STATUS_QUERY,
  upsertMessagePages,
  VIEW_QUERY,
} from './helpers.ts';

const t = (key: string, params?: Record<string, string>) => (params ? `${key} ${JSON.stringify(params)}` : key);

test('initials take the first and last word, Turkish upper case', () => {
  assert.equal(initials('ilkay şahin'), 'İŞ');
  assert.equal(initials('Ayşe'), 'A');
  assert.equal(initials('  '), '?');
  assert.equal(initials(null), '?');
});

test('contact label falls back from name to e-mail to phone', () => {
  assert.equal(contactLabel({ contact: { name: 'Ali', email: 'a@x.co', phone: null } }), 'Ali');
  assert.equal(contactLabel({ contact: { name: '', email: 'a@x.co', phone: '+90' } }), 'a@x.co');
  assert.equal(contactLabel({ contact: { name: '', email: null, phone: '+90' } }), '+90');
  assert.equal(contactLabel({ contact: { name: '', email: null, phone: null } }), '—');
});

test('list filters match the panel', () => {
  assert.equal(STATUS_QUERY.open, 'open,pending');
  assert.equal(STATUS_QUERY.all, undefined);
  assert.deepEqual(VIEW_QUERY.mine, { assignee: 'me' });
  assert.deepEqual(VIEW_QUERY.mentions, { mentioned: '1' });
});

test('event lines name the actor and the change', () => {
  assert.equal(
    eventText({ event: 'assigned', actor_name: 'Ece', to: 'Can' }, t),
    'inbox:events.assigned {"actor":"Ece","to":"Can"}',
  );
  assert.equal(eventText({ event: 'assigned', actor_name: 'Ece', to: 'Ece' }, t), 'inbox:events.took {"actor":"Ece"}');
  assert.equal(eventText({ event: 'assigned', actor_name: 'Ece' }, t), 'inbox:events.unassigned {"actor":"Ece"}');
  assert.equal(
    eventText({ event: 'status_changed', to: 'resolved|x' }, t),
    'inbox:events.status_changed {"actor":"inbox:sender.system","status":"inbox:status.resolved"}',
  );
  assert.equal(eventText({ event: 'custom' }, t), 'conversation:events.updated');
  assert.equal(eventText(null, t), '');
});

test('upsert replaces by uuid or client_id and appends new messages to the newest page', () => {
  type M = { uuid: string; client_id: string | null; status: string };
  const pages = [
    { items: [{ uuid: 'b', client_id: 'cb', status: 'sent' }] as M[] },
    { items: [{ uuid: 'a', client_id: null, status: 'read' }] as M[] },
  ];
  const confirmed = upsertMessagePages(pages, { uuid: 'srv-b', client_id: 'cb', status: 'queued' });
  assert.deepEqual(confirmed[0].items, [{ uuid: 'srv-b', client_id: 'cb', status: 'queued' }]);
  const updated = upsertMessagePages(pages, { uuid: 'a', client_id: null, status: 'failed' });
  assert.equal(updated[1].items[0].status, 'failed');
  const added = upsertMessagePages(pages, { uuid: 'c', client_id: null, status: 'received' });
  assert.deepEqual(
    added[0].items.map((m) => m.uuid),
    ['b', 'c'],
  );
  assert.deepEqual(upsertMessagePages([], { uuid: 'x', client_id: null, status: 'sent' }), []);
});

test('client ids are v4 uuids', () => {
  assert.match(clientId(), /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
});

test('slaBadge picks the nearest deadline and its tone', () => {
  const now = Date.parse('2026-10-28T10:00:00Z');
  const base = { status: 'open', first_response_due_at: null, next_response_due_at: null, resolution_due_at: null };
  assert.equal(slaBadge(base, now), null);
  assert.equal(slaBadge({ ...base, status: 'resolved', resolution_due_at: '2026-10-28T09:00:00Z' }, now), null);
  const soon = slaBadge(
    { ...base, first_response_due_at: '2026-10-28T10:20:00Z', resolution_due_at: '2026-10-29T10:00:00Z' },
    now,
  );
  assert.deepEqual(soon && { metric: soon.metric, tone: soon.tone }, { metric: 'first_response', tone: 'soon' });
  assert.equal(slaBadge({ ...base, next_response_due_at: '2026-10-28T09:59:00Z' }, now)?.tone, 'overdue');
  assert.equal(slaBadge({ ...base, resolution_due_at: '2026-10-28T12:00:00Z' }, now)?.tone, 'running');
});

test('formatDuration shows the two largest units', () => {
  assert.equal(formatDuration(8100, 'tr'), '2 sa 15 dk');
  assert.equal(formatDuration(-8100, 'en'), '2h 15m');
  assert.equal(formatDuration(90_000, 'en'), '1d 1h');
  assert.equal(formatDuration(10, 'tr'), '0 dk');
});

test('snoozeUntil follows the panel presets', () => {
  const wed = new Date(2026, 9, 28, 14, 30); // Wednesday
  assert.equal(snoozeUntil('one_hour', wed).getTime() - wed.getTime(), 3_600_000);
  assert.equal(snoozeUntil('three_hours', wed).getTime() - wed.getTime(), 3 * 3_600_000);
  const tomorrow = snoozeUntil('tomorrow', wed);
  assert.deepEqual([tomorrow.getDate(), tomorrow.getHours(), tomorrow.getMinutes()], [29, 9, 0]);
  const monday = snoozeUntil('next_week', wed);
  assert.deepEqual([monday.getDay(), monday.getDate(), monday.getHours()], [1, 2, 9]);
  const sunday = new Date(2026, 10, 1, 20, 0);
  assert.equal(snoozeUntil('next_week', sunday).getDate(), 2);
});

test('attachmentKind, formatBytes and fallbackFileName', () => {
  assert.equal(attachmentKind('image/jpeg'), 'image');
  assert.equal(attachmentKind('audio/ogg; codecs=opus'), 'audio');
  assert.equal(attachmentKind('application/pdf'), 'file');
  assert.equal(attachmentKind(null), 'file');
  assert.equal(formatBytes(512, 'en'), '512 B');
  assert.equal(formatBytes(1536, 'en'), '1.5 KB');
  assert.equal(formatBytes(1536, 'tr'), '1,5 KB');
  assert.equal(fallbackFileName('image/jpeg', new Date(2026, 9, 28, 14, 30, 5)), 'photo-20261028-143005.jpg');
  assert.equal(fallbackFileName(undefined, new Date(2026, 9, 28, 14, 30, 5)), 'file-20261028-143005.bin');
});
