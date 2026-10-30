import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  clipboardChanged,
  consumeClipboard,
  initialClipboardOffer,
  shouldOfferClipboard,
  serializeStrokes,
  deserializeStrokes,
  strokePath,
  hitsStroke,
  type Stroke,
} from './image-editing.ts';

test('clipboard offer is focus gated and consumed image stays dismissed across focus and detection', () => {
  let state = { ...initialClipboardOffer, hasImage: true };
  assert.equal(shouldOfferClipboard(state, false), false);
  assert.equal(shouldOfferClipboard(state, true), true);
  state = consumeClipboard(state);
  assert.equal(shouldOfferClipboard(state, true), false);
  assert.equal(shouldOfferClipboard({ ...state, hasImage: true }, true), false);
  assert.equal(shouldOfferClipboard(state, false), false);
});
test('new clipboard revisions reoffer images but never text or an empty clipboard', () => {
  const consumed = consumeClipboard({ ...initialClipboardOffer, hasImage: true });
  assert.equal(shouldOfferClipboard(clipboardChanged(consumed, true), true), true);
  const text = clipboardChanged(consumed, false);
  assert.equal(shouldOfferClipboard(text, true), false);
  assert.equal(shouldOfferClipboard(clipboardChanged(text, true), true), true);
});
const strokes: Stroke[] = ['pen', 'highlighter', 'arrow', 'rectangle', 'text'].map((tool, i) => ({
  id: String(i),
  tool: tool as Stroke['tool'],
  color: '#FF3B30',
  width: 4,
  points: [
    { x: 10, y: 20 },
    { x: 80, y: 90 },
  ],
  ...(tool === 'text' ? { text: 'Türkçe 👋' } : {}),
}));
test('all markup tools round-trip in a versioned pixel coordinate format without mutation', () => {
  const saved = serializeStrokes(strokes);
  assert.deepEqual(deserializeStrokes(saved), strokes);
  assert.equal(serializeStrokes(deserializeStrokes(saved)), saved);
  const restored = deserializeStrokes(saved);
  restored[0].points[0].x = 900;
  assert.equal(strokes[0].points[0].x, 10);
  assert.deepEqual(deserializeStrokes(serializeStrokes([])), []);
});
test('invalid or incompatible saved strokes fail closed', () => {
  for (const value of [
    'oops',
    '{}',
    '{"version":2,"strokes":[]}',
    '{"version":1,"strokes":[null]}',
    serializeStrokes([{ ...strokes[0], width: -1 }]),
    serializeStrokes([{ ...strokes[0], points: [] }]),
    serializeStrokes([{ ...strokes[0], points: [{ x: NaN, y: 1 }] }]),
  ])
    assert.throws(() => deserializeStrokes(value));
});
test('eraser hits segment interiors and rectangle edges, but leaves empty areas intact', () => {
  assert.equal(hitsStroke(strokes[0], { x: 45, y: 55 }, 5), true);
  assert.equal(hitsStroke(strokes[0], { x: 90, y: 5 }, 5), false);
  assert.equal(hitsStroke(strokes[3], { x: 45, y: 20 }, 5), true);
  assert.equal(hitsStroke(strokes[3], { x: 45, y: 55 }, 5), false);
  assert.match(strokePath(strokes[2]), /M.*L.*M.*L.*L/);
  assert.match(strokePath(strokes[3]), /Z$/);
  assert.match(strokePath({ ...strokes[0], points: [{ x: 1, y: 2 }] }), /l0.1,0.1/);
});

test('finishing an old paste cannot consume a newer clipboard image', () => {
  const old = { ...initialClipboardOffer, hasImage: true };
  const next = clipboardChanged(old, true);
  assert.equal(shouldOfferClipboard(consumeClipboard(next, old.revision), true), true);
});
