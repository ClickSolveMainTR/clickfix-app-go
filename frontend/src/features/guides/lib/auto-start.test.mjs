import assert from "node:assert/strict";
import { test } from "node:test";
import { canAutoStart } from "./auto-start.ts";

test("automatic guides only request a claim when fresh and enabled", () => {
  assert.equal(canAutoStart("new", true, 2), true);
  for (const status of ["in_progress", "skipped", "completed"])
    assert.equal(canAutoStart(status, true, 2), false);
  assert.equal(canAutoStart("new", false, 2), false);
  assert.equal(canAutoStart("new", true, 0), false);
});
