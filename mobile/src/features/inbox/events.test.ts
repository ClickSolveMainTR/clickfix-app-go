import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import { createInstance } from 'i18next';

import { eventText } from './helpers.ts';

const kinds = [
  'assigned',
  'team_assigned',
  'status_changed',
  'priority_changed',
  'tag_added',
  'tag_removed',
  'subject_changed',
  'merged_from',
  'merged_into',
  'split_to',
  'split_from',
  'linked',
  'unlinked',
  'side_conversation_started',
  'side_conversation_sent',
  'side_conversation_reply',
  'appointment_booked',
  'appointment_rescheduled',
  'appointment_confirmed',
  'appointment_completed',
  'appointment_no_show',
  'appointment_cancelled',
  'snoozed',
  'unsnoozed',
  'sla_applied',
  'sla_breached',
  'sla_paused',
  'sla_resumed',
];

for (const lang of ['tr', 'en']) {
  test(`${lang}: all backend timeline event kinds resolve to readable text`, () => {
    const read = (url: string) => JSON.parse(readFileSync(new URL(url, import.meta.url), 'utf8'));
    const conversation = read(`../../core/i18n/locales/${lang}/conversation.json`);
    const inbox = read(`../../../../frontend/src/locales/${lang}/inbox.json`);
    const i18n = createInstance();
    i18n.init({
      lng: lang,
      resources: { [lang]: { inbox, conversation } },
      keySeparator: false,
      initAsync: false,
      interpolation: { escapeValue: false },
    });
    for (const event of kinds) {
      const text = eventText(
        {
          event,
          actor_name: 'Ece',
          to: event === 'priority_changed' ? 'high' : event === 'status_changed' ? 'snoozed|2026-10-30' : '#8',
          subject: 'Delivery',
          type: 'Call',
          relation: 'related',
        },
        (key, params) => i18n.t(key, params),
      );
      assert.ok(text.length > 0, event);
      assert.doesNotMatch(text, /(?:inbox|conversation):[\w.]|{{|merged_from|status_changed/, event);
    }
    assert.equal(
      eventText({ event: 'future_kind' }, (key) => i18n.t(key)),
      conversation['events.updated'],
    );
    assert.equal(
      i18n.t('conversation:events.merged_count', { count: 5 }),
      lang === 'tr' ? '5 konuşma bu konuşmaya birleştirildi' : '5 conversations merged into this conversation',
    );
  });
}
