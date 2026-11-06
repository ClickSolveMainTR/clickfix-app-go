import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  conditionLines,
  countConditions,
  decodeRule,
  describeAction,
  describeNode,
  emptyCondition,
  hasUnpublishedChanges,
  lastExecution,
  orderedNodes,
  patchRow,
  type RawRule,
  type RuleExecution,
  runTone,
  type T,
  toRuleInput,
  triggerLabel,
  type Workflow,
  workflowState,
} from './core.ts';

/** Echoes the key and its params, so the tests read what would be translated. */
const t: T = (key, params) =>
  params
    ? `${key}(${Object.entries(params)
        .map(([k, v]) => `${k}=${v}`)
        .join(',')})`
    : key;

const raw = (over: Partial<RawRule> = {}): RawRule => ({
  id: 7,
  uuid: 'r-1',
  name: 'İade etiketi',
  trigger_event: 'conversation.created',
  conditions: { all: [{ field: 'status', operator: 'eq', value: 'open' }] },
  actions: [{ type: 'set_priority', value: 'high' }],
  position: 2,
  enabled: true,
  stop_on_match: false,
  schedule_minutes: 0,
  next_run_at: null,
  created_at: '2026-11-01T10:00:00Z',
  updated_at: '2026-11-02T10:00:00Z',
  ...over,
});

test('decodeRule reads base64 JSON the API sends, Turkish characters included', () => {
  const conditions = { any: [{ field: 'subject', operator: 'contains', value: 'iade şikâyeti' }] };
  const actions = [{ type: 'add_note', content: 'Müşteri aranacak' }];
  const b64 = (v: unknown) => Buffer.from(JSON.stringify(v), 'utf8').toString('base64');
  const rule = decodeRule(raw({ conditions: b64(conditions), actions: b64(actions) }));
  assert.deepEqual(rule.conditions, conditions);
  assert.deepEqual(rule.actions, actions);
});

test('decodeRule keeps objects and falls back on unreadable values', () => {
  assert.deepEqual(decodeRule(raw()).actions, [{ type: 'set_priority', value: 'high' }]);
  const broken = decodeRule(raw({ conditions: '%%%', actions: '%%%' }));
  assert.deepEqual(broken.conditions, { all: [] });
  assert.deepEqual(broken.actions, []);
});

test('toRuleInput sends the whole rule back, so a toggle changes only `enabled`', () => {
  const rule = decodeRule(raw({ stop_on_match: true, schedule_minutes: 15, trigger_event: 'scheduled' }));
  assert.deepEqual(
    { ...toRuleInput(rule), enabled: false },
    {
      name: 'İade etiketi',
      trigger: 'scheduled',
      conditions: rule.conditions,
      actions: rule.actions,
      position: 2,
      enabled: false,
      stop_on_match: true,
      schedule_minutes: 15,
    },
  );
});

test('countConditions counts comparisons, not groups', () => {
  assert.equal(countConditions({ all: [] }), 0);
  assert.equal(
    countConditions({
      all: [{ field: 'status', operator: 'eq', value: 'open' }, { any: [{ field: 'priority' }, { field: 'subject' }] }],
    }),
    3,
  );
});

test('conditionLines indents members under their group and labels known values', () => {
  const names = new Map([['tag-1', 'VIP']]);
  const lines = conditionLines(
    t,
    {
      all: [
        { field: 'status', operator: 'eq', value: 'open' },
        {
          any: [
            { field: 'tag_uuids', operator: 'in', value: ['tag-1', 'tag-2'] },
            { field: 'sla_breached', operator: 'eq', value: true },
          ],
        },
        { field: 'custom_fields.plan', operator: 'exists' },
      ],
    },
    names,
  );
  assert.deepEqual(lines, [
    { depth: 0, text: 'automations.group_all(count=3)' },
    { depth: 1, text: 'automations.field.status automations.operator.eq automations.status_value.open' },
    { depth: 1, text: 'automations.group_any(count=2)' },
    { depth: 2, text: 'automations.field.tag_uuids automations.operator.in VIP, tag-2' },
    { depth: 2, text: 'automations.field.sla_breached automations.operator.eq automations.yes' },
    { depth: 1, text: 'plan automations.operator.exists' },
  ]);
});

test('an empty root group says what it matches', () => {
  assert.equal(emptyCondition({ all: [] }), 'all');
  assert.equal(emptyCondition({ any: [] }), 'any');
  assert.equal(emptyCondition({ all: [{ field: 'status' }] }), null);
  assert.deepEqual(conditionLines(t, { all: [] }), []);
});

test('triggerLabel translates known triggers and shows the interval of a scheduled one', () => {
  assert.equal(triggerLabel(t, 'message.received'), 'automations.trigger.message_received');
  assert.equal(triggerLabel(t, 'scheduled', 30), 'automations.trigger.scheduled · automations.every_minutes(count=30)');
  assert.equal(triggerLabel(t, 'custom.event'), 'custom.event');
  assert.equal(triggerLabel(t, undefined), 'automations.trigger_none');
});

test('describeAction names the action and what it sets or targets', () => {
  const names = new Map([['u-1', 'Ayşe Yılmaz']]);
  assert.deepEqual(describeAction(t, { type: 'set_status', value: 'resolved' }), {
    title: 'automations.action.set_status',
    detail: 'automations.status_value.resolved',
  });
  assert.deepEqual(describeAction(t, { type: 'assign_user', target_uuid: 'u-1' }, names), {
    title: 'automations.action.assign_user',
    detail: 'Ayşe Yılmaz',
  });
  assert.equal(describeAction(t, { type: 'add_tag', target_uuid: 'unknown' }, names).detail, '');
  assert.equal(describeAction(t, { type: 'add_note', content: ' Not ' }).detail, 'Not');
  assert.equal(
    describeAction(t, { type: 'route', content: JSON.stringify({ settings: { strategy: 'load_balanced' } }) }).detail,
    'automations.route.load_balanced',
  );
  assert.equal(describeAction(t, { type: 'route', content: 'not json' }).detail, '');
});

const graph: Workflow['draft'] = {
  schema_version: 1,
  nodes: [
    { id: 'end', type: 'end' },
    { id: 'no', type: 'action', action: { type: 'add_note', content: 'Hayır' } },
    { id: 'wait', type: 'wait', delay_seconds: 7200 },
    { id: 'cond', type: 'condition', condition: { all: [{ field: 'priority', operator: 'eq', value: 'urgent' }] } },
    { id: 'start', type: 'trigger', trigger: 'conversation.created' },
    { id: 'orphan', type: 'action', action: { type: 'set_priority', value: 'low' } },
  ],
  edges: [
    { from: 'cond', port: 'false', to: 'no', input: 'in' },
    { from: 'start', port: 'next', to: 'cond', input: 'in' },
    { from: 'cond', port: 'true', to: 'wait', input: 'in' },
    { from: 'wait', port: 'next', to: 'cond', input: 'in', loop: true },
    { from: 'wait', port: 'next', to: 'end', input: 'in' },
    { from: 'no', port: 'next', to: 'end', input: 'in' },
  ],
};

test('orderedNodes follows the flow from the trigger, yes before no, each node once', () => {
  assert.deepEqual(
    orderedNodes(graph).map((n) => n.id),
    ['start', 'cond', 'wait', 'end', 'no', 'orphan'],
  );
});

test('describeNode puts each kind of step in words', () => {
  const by = (id: string) =>
    describeNode(
      t,
      graph.nodes.find((n) => n.id === id)!,
    );
  assert.deepEqual(by('start'), {
    title: 'automations.kind.trigger',
    lines: [{ depth: 0, text: 'automations.trigger.conversation_created' }],
  });
  assert.deepEqual(by('wait').lines, [{ depth: 0, text: 'automations.wait_hours(count=2)' }]);
  assert.deepEqual(by('no').lines, [
    { depth: 0, text: 'automations.action.add_note' },
    { depth: 1, text: 'Hayır' },
  ]);
  assert.deepEqual(by('end').lines, [{ depth: 0, text: 'automations.flow_ends' }]);
  assert.equal(by('cond').lines.length, 2);
  assert.deepEqual(describeNode(t, { id: 'w', type: 'wait', delay_seconds: 90 }).lines, [
    { depth: 0, text: 'automations.wait_minutes(count=2)' },
  ]);
  assert.deepEqual(describeNode(t, { id: 'b', type: 'branch', condition: { all: [] } }).lines, [
    { depth: 0, text: 'automations.empty_all' },
  ]);
});

test('a workflow that was never published is a draft, whatever `enabled` says', () => {
  assert.equal(workflowState({ published_version: null, enabled: true }), 'draft');
  assert.equal(workflowState({ published_version: 3, enabled: true }), 'enabled');
  assert.equal(workflowState({ published_version: 3, enabled: false }), 'disabled');
  assert.equal(hasUnpublishedChanges({ published_version: 3, revision: 4 }), true);
  assert.equal(hasUnpublishedChanges({ published_version: 3, revision: 3 }), false);
  assert.equal(hasUnpublishedChanges({ published_version: null, revision: 1 }), false);
});

test('lastExecution takes the newest log entry of the rule', () => {
  const entry = (uuid: string, rule_id: number) => ({ uuid, rule_id }) as RuleExecution;
  const log = [entry('a', 1), entry('b', 7), entry('c', 7)];
  assert.equal(lastExecution(log, 7)?.uuid, 'b');
  assert.equal(lastExecution(log, 9), undefined);
  assert.equal(lastExecution(undefined, 7), undefined);
});

test('runTone maps run and execution statuses', () => {
  assert.equal(runTone('succeeded'), 'done');
  assert.equal(runTone('blocked'), 'problem');
  assert.equal(runTone('waiting'), 'progress');
  assert.equal(runTone('skipped'), 'neutral');
});

test('patchRow changes one row and can be undone with the previous page', () => {
  const page = {
    items: [
      { uuid: 'a', enabled: false },
      { uuid: 'b', enabled: false },
    ],
    total: 2,
    limit: 100,
    offset: 0,
  };
  const next = patchRow(page, 'b', { enabled: true });
  assert.deepEqual(next?.items, [
    { uuid: 'a', enabled: false },
    { uuid: 'b', enabled: true },
  ]);
  assert.equal(page.items[1].enabled, false);
  assert.equal(patchRow(undefined, 'b', { enabled: true }), undefined);
});
