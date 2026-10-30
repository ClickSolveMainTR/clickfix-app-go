import assert from 'node:assert/strict';
import { test } from 'node:test';
import { layoutEvents, monthDays, sameDay, startOfWeek } from './calendar-core.ts';
const day = new Date(2026, 9, 29);
const at = (hour: number, minute = 0) => new Date(2026, 9, 29, hour, minute).toISOString();

test('Turkish calendar starts on Monday and renders six complete month rows', () => {
  assert.equal(startOfWeek(day, true).getDay(), 1);
  assert.equal(startOfWeek(day, false).getDay(), 0);
  const days = monthDays(new Date(2028, 1, 15), true);
  assert.equal(days.length, 42);
  assert.equal(days[0].getDay(), 1);
  assert.ok(days.some((d) => sameDay(d, new Date(2028, 1, 29))));
});
test('overlap clusters get independent columns and nonoverlapping events regain full width', () => {
  const rows = layoutEvents(
    [
      { starts_at: at(9), ends_at: at(10) },
      { starts_at: at(9, 30), ends_at: at(10, 30) },
      { starts_at: at(11), ends_at: at(12) },
    ],
    day,
  );
  assert.deepEqual(
    rows.map((r) => [r.column, r.columns]),
    [
      [0, 2],
      [1, 2],
      [0, 1],
    ],
  );
  assert.equal(rows[0].top, 540);
  assert.equal(rows[0].height, 60);
});
test('day intervals are half open and spanning appointments are clipped to midnight', () => {
  const before = new Date(2026, 9, 28, 23, 30).toISOString();
  const rows = layoutEvents(
    [
      { starts_at: before, ends_at: at(0) },
      { starts_at: before, ends_at: at(0, 30) },
    ],
    day,
  );
  assert.equal(rows.length, 1);
  assert.equal(rows[0].top, 0);
  assert.equal(rows[0].height, 30);
});
test('short events receive distinct lanes when minimum visible heights overlap', () => {
  const rows = layoutEvents(
    [
      { starts_at: at(9), ends_at: at(9, 5) },
      { starts_at: at(9, 10), ends_at: at(9, 15) },
    ],
    day,
  );
  assert.deepEqual(
    rows.map((r) => r.columns),
    [2, 2],
  );
});
