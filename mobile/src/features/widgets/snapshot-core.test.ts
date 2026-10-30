import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  type ApiAppointment,
  type ApiConversation,
  buildSnapshot,
  nextDue,
  oneLine,
  type SnapshotInput,
  slaAtRisk,
  snapshotKey,
  spotlightItems,
  todayAppointments,
} from './snapshot-core.ts';

// Local 09:00, so "today" holds in any time zone the tests run in.
const now = new Date(2026, 9, 30, 9, 0, 0);
const at = (min: number) => new Date(now.getTime() + min * 60_000).toISOString();

function conv(over: Partial<ApiConversation> = {}): ApiConversation {
  return {
    uuid: '00000000-0000-4000-8000-000000000001',
    channel_type: 'whatsapp',
    contact: { name: 'Mehmet Kaya', email: null, phone: null },
    subject: 'Sipariş',
    status: 'open',
    unread_count: 2,
    last_message_preview: 'Siparişim\nne zaman gelir?',
    last_message_direction: 'in',
    last_message_at: at(-2),
    first_response_at: null,
    first_response_due_at: null,
    next_response_due_at: null,
    resolution_due_at: null,
    sla_breached_at: null,
    ...over,
  };
}

function appt(over: Partial<ApiAppointment> = {}): ApiAppointment {
  return {
    uuid: '00000000-0000-4000-8000-0000000000a1',
    status: 'confirmed',
    title: 'Kurulum görüşmesi',
    starts_at: at(30),
    ends_at: at(60),
    location_kind: 'video',
    video_room_uuid: '00000000-0000-4000-8000-0000000000f1',
    contact: { name: 'Ayşe' },
    ...over,
  };
}

const base: SnapshotInput = {
  now,
  locale: 'tr',
  signedIn: true,
  org: { slug: 'acme', name: 'Acme' },
  user: { name: 'Ajan' },
  can: { inbox: true, appointments: true, presence: true },
  presence: 'online',
  counts: { open: 23, mine: 7, breached: 1 },
  recent: [conv()],
  unreadTotal: 4,
  atRisk: [],
  appointments: [appt()],
};

test('signed out, no organization and no permission are explicit states without data', () => {
  const out = buildSnapshot({ ...base, signedIn: false });
  assert.equal(out.state, 'signed_out');
  assert.equal(out.org, null);
  assert.equal(out.user, null);
  assert.equal(out.inbox, null);
  assert.deepEqual(out.can, { inbox: false, appointments: false, presence: false });

  assert.equal(buildSnapshot({ ...base, org: null }).state, 'no_org');
  const none = buildSnapshot({ ...base, can: { inbox: false, appointments: false, presence: false } });
  assert.equal(none.state, 'no_permission');
  assert.equal(none.today, null);
});

test('ready snapshot carries counts, one-line previews and the version', () => {
  const out = buildSnapshot(base);
  assert.equal(out.v, 1);
  assert.equal(out.state, 'ready');
  assert.equal(out.generated_at, now.toISOString());
  assert.deepEqual(
    { unread: out.inbox?.unread, mine: out.inbox?.mine, open: out.inbox?.open, breached: out.inbox?.breached },
    { unread: 4, mine: 7, open: 23, breached: 1 },
  );
  assert.equal(out.inbox?.top[0].preview, 'Siparişim ne zaman gelir?');
  assert.equal(out.inbox?.top[0].contact, 'Mehmet Kaya');
  assert.equal(out.presence, 'online');
});

test('a section the member may not see is null, the rest stays', () => {
  const out = buildSnapshot({ ...base, can: { inbox: false, appointments: true, presence: false } });
  assert.equal(out.state, 'ready');
  assert.equal(out.inbox, null);
  assert.equal(out.sla, null);
  assert.equal(out.presence, null);
  assert.equal(out.today?.length, 1);
});

test('unread falls back to the rows when the total is unknown; the contact falls back to phone', () => {
  const out = buildSnapshot({
    ...base,
    unreadTotal: null,
    recent: [conv(), conv({ uuid: 'b', unread_count: 0, contact: { name: '', phone: '+90555' } })],
  });
  assert.equal(out.inbox?.unread, 1);
  assert.equal(out.inbox?.top[1].contact, '+90555');
});

test('top conversations are capped at five', () => {
  const many = Array.from({ length: 9 }, (_, i) => conv({ uuid: `c${i}` }));
  assert.equal(buildSnapshot({ ...base, recent: many }).inbox?.top.length, 5);
});

test('nextDue skips the first response clock once answered and picks the earliest', () => {
  assert.equal(nextDue(conv({ first_response_due_at: at(5), next_response_due_at: at(20) })), at(5));
  assert.equal(
    nextDue(conv({ first_response_at: at(-1), first_response_due_at: at(5), next_response_due_at: at(20) })),
    at(20),
  );
  assert.equal(nextDue(conv()), null);
});

test('slaAtRisk keeps breached and soon-due rows, breached first, then nearest', () => {
  const rows = slaAtRisk(
    [
      conv({ uuid: 'late', next_response_due_at: at(45) }),
      conv({ uuid: 'far', next_response_due_at: at(240) }),
      conv({ uuid: 'soon', next_response_due_at: at(10) }),
      conv({ uuid: 'gone', sla_breached_at: at(-5), next_response_due_at: at(-5) }),
      conv({ uuid: 'done', status: 'resolved', next_response_due_at: at(1) }),
      conv({ uuid: 'none' }),
    ],
    now,
  );
  assert.deepEqual(
    rows.map((r) => [r.uuid, r.breached]),
    [
      ['gone', true],
      ['soon', false],
      ['late', false],
    ],
  );
});

test('todayAppointments keeps live ones of the local day and builds the join link', () => {
  const rows = todayAppointments(
    [
      appt({ uuid: 'later', starts_at: at(120), ends_at: at(150), location_kind: 'phone', video_room_uuid: null }),
      appt(),
      appt({ uuid: 'ended', starts_at: at(-120), ends_at: at(-60) }),
      appt({ uuid: 'cancelled', status: 'cancelled' }),
      appt({ uuid: 'tomorrow', starts_at: at(60 * 24 + 30), ends_at: at(60 * 24 + 60) }),
    ],
    now,
  );
  assert.deepEqual(
    rows.map((r) => r.uuid),
    ['00000000-0000-4000-8000-0000000000a1', 'later'],
  );
  assert.equal(rows[0].join, 'clickfix:///video/00000000-0000-4000-8000-0000000000f1');
  assert.equal(rows[0].with, 'Ayşe');
  assert.equal(rows[1].join, null);
});

test('oneLine flattens whitespace and truncates with an ellipsis', () => {
  assert.equal(oneLine('  a \n b\tc  '), 'a b c');
  assert.equal(oneLine('abcdef', 4), 'abc…');
  assert.equal(oneLine(null), '');
});

test('spotlight items merge recent and at-risk conversations without duplicates', () => {
  const snap = buildSnapshot({
    ...base,
    atRisk: [conv({ next_response_due_at: at(5) }), conv({ uuid: 'x', next_response_due_at: at(5) })],
  });
  const items = spotlightItems(snap);
  assert.deepEqual(
    items.map((i) => i.id),
    ['00000000-0000-4000-8000-000000000001', 'x'],
  );
  assert.equal(items[1].url, 'clickfix:///conversation/x');
});

test('snapshotKey ignores the generation time only', () => {
  const a = buildSnapshot(base);
  const b = buildSnapshot({ ...base, now: new Date(now.getTime() + 1000) });
  assert.equal(snapshotKey(a), snapshotKey({ ...b, generated_at: 'x' }));
  assert.notEqual(snapshotKey(a), snapshotKey(buildSnapshot({ ...base, presence: 'busy' })));
});
