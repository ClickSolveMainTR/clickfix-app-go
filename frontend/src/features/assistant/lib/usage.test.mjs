import assert from "node:assert/strict";
import { test } from "node:test";
import { bumpUsage, runSettled } from "./usage.ts";

const status = {
  enabled: true,
  reason: "",
  daily_limit: 100,
  used_today: 7,
  escalation_available: true,
};

test("the counter moves by one reply and never past the limit", () => {
  assert.equal(bumpUsage(undefined), undefined);
  assert.equal(bumpUsage(status).used_today, 8);
  assert.equal(bumpUsage({ ...status, used_today: 100 }).used_today, 100);
  // The cached object is reused when nothing changes (no re-render).
  const full = { ...status, used_today: 100 };
  assert.equal(bumpUsage(full), full);
});

test("only the end of a run counts", () => {
  assert.equal(runSettled(true, false), true);
  assert.equal(runSettled(false, false), false);
  assert.equal(runSettled(true, true), false);
});
