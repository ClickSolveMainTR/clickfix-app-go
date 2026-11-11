import assert from "node:assert/strict";
import { test } from "node:test";

import { messageTimePattern, showCharCount } from "./time.ts";

test("messageTimePattern shortens with distance", () => {
  const now = new Date(2026, 10, 11, 15, 0);
  assert.equal(messageTimePattern(new Date(2026, 10, 11, 9, 5), now), "HH:mm");
  assert.equal(
    messageTimePattern(new Date(2026, 10, 10, 23, 59), now),
    "d MMM HH:mm",
  );
  assert.equal(
    messageTimePattern(new Date(2025, 11, 31, 10, 0), now),
    "d MMM yyyy HH:mm",
  );
});

test("showCharCount appears from 80% of the limit", () => {
  assert.equal(showCharCount(0, 4000), false);
  assert.equal(showCharCount(3199, 4000), false);
  assert.equal(showCharCount(3200, 4000), true);
  assert.equal(showCharCount(4000, 4000), true);
  assert.equal(showCharCount(10, 0), false);
});
