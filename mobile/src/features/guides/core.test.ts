import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  autoStartGuide,
  autoSetupGuide,
  cardTop,
  type Guide,
  isVisibleRect,
  nextSetupGuide,
  resumeIndex,
  routePattern,
  screenGuide,
  spotlightRect,
} from './core.ts';

const step = { id: 's1', target: 'inbox.list', title: 'T', body: 'B', placement: 'auto' as const };
const guide = (over: Partial<Guide>): Guide => ({
  key: 'k',
  surface: 'mobile',
  route: '/inbox',
  title: 'Guide',
  summary: '',
  kind: 'tour',
  order: 1,
  steps: [step, { ...step, id: 's2' }],
  progress: { status: 'new', step_index: 0 },
  ...over,
});

test('routePattern drops groups and index, keeps dynamic segments', () => {
  assert.equal(routePattern(['(app)', '(org)', '(tabs)', 'inbox']), '/inbox');
  assert.equal(routePattern(['(app)', '(org)', '(tabs)', 'more', 'index']), '/more');
  assert.equal(routePattern(['(app)', '(org)', 'conversation', '[uuid]']), '/conversation/[uuid]');
  assert.equal(routePattern([]), '/');
});

test('autoStartGuide picks the first new tour with steps', () => {
  const a = guide({ key: 'a', order: 2 });
  const b = guide({ key: 'b', order: 1 });
  const seen = guide({ key: 'c', order: 0, progress: { status: 'completed', step_index: 1 } });
  const page = guide({ key: 'p', order: 0, kind: 'page' });
  const empty = guide({ key: 'e', order: 0, steps: [] });
  assert.equal(autoStartGuide([a, b, seen, page, empty])?.key, 'b');
  assert.equal(autoStartGuide([seen, page]), null);
});

test('screenGuide prefers the page guide, else the first tour', () => {
  assert.equal(screenGuide([guide({ key: 't' }), guide({ key: 'p', kind: 'page', order: 5 })])?.key, 'p');
  assert.equal(screenGuide([guide({ key: 't' })])?.key, 't');
  assert.equal(screenGuide([]), null);
});

test('resumeIndex continues an unfinished guide inside its steps', () => {
  assert.equal(resumeIndex(guide({ progress: { status: 'in_progress', step_index: 1 } })), 1);
  assert.equal(resumeIndex(guide({ progress: { status: 'in_progress', step_index: 9 } })), 1);
  assert.equal(resumeIndex(guide({ progress: { status: 'completed', step_index: 1 } })), 0);
});

test('nextSetupGuide skips finished items in order', () => {
  const done = guide({ key: 'd', order: 1, progress: { status: 'completed', step_index: 0 } });
  const skipped = guide({ key: 's', order: 2, progress: { status: 'skipped', step_index: 0 } });
  const open = guide({ key: 'o', order: 3, progress: { status: 'in_progress', step_index: 1 } });
  assert.equal(nextSetupGuide([open, skipped, done])?.key, 'o');
  assert.equal(nextSetupGuide([done, skipped]), null);
});

const win = { width: 390, height: 844 };
const insets = { top: 47, bottom: 34, left: 0, right: 0 };

test('spotlightRect pads the target and stays inside the window', () => {
  assert.deepEqual(spotlightRect({ x: 2, y: 100, width: 50, height: 40 }, win), {
    x: 0,
    y: 94,
    width: 58,
    height: 52,
  });
});

test('isVisibleRect rejects missing, tiny and off-screen views', () => {
  assert.equal(isVisibleRect(null, win), false);
  assert.equal(isVisibleRect({ x: 0, y: 0, width: 1, height: 40 }, win), false);
  assert.equal(isVisibleRect({ x: 0, y: 900, width: 40, height: 40 }, win), false);
  assert.equal(isVisibleRect({ x: 10, y: 10, width: 40, height: 40 }, win), true);
});

test('cardTop puts the card on the roomier side and inside the safe area', () => {
  const nearTop = { x: 0, y: 100, width: 390, height: 44 };
  const nearBottom = { x: 0, y: 760, width: 390, height: 50 };
  assert.deepEqual(cardTop(nearTop, 200, win, insets, 'auto'), { top: 156, side: 'bottom' });
  assert.equal(cardTop(nearBottom, 200, win, insets, 'auto').side, 'top');
  // "bottom" asked but no room below: falls back to above.
  assert.equal(cardTop(nearBottom, 200, win, insets, 'bottom').side, 'top');
  // "top" asked but no room above: below instead, never under the status bar.
  const r = cardTop(nearTop, 200, win, insets, 'top');
  assert.equal(r.side, 'bottom');
  assert.ok(r.top >= insets.top);
});

test('automatic setup never resumes a seen sequence or ignores disabled preference', () => {
  const fresh = guide({ key: 'one', progress: { status: 'new', step_index: 0 } });
  assert.equal(autoSetupGuide([fresh], true)?.key, 'one');
  assert.equal(autoSetupGuide([fresh], false), null);
  assert.equal(autoStartGuide([fresh], false), null);
  for (const status of ['in_progress', 'completed', 'skipped'] as const) {
    const seen = guide({ key: 'seen', progress: { status, step_index: 1 } });
    assert.equal(autoSetupGuide([fresh, seen], true), null);
    assert.equal(autoStartGuide([seen]), null);
  }
});
