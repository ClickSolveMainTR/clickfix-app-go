import assert from "node:assert/strict";
import { test } from "node:test";

import { focusHref, readValue, toText } from "./card.ts";

test("focusHref keeps the user inside the app", () => {
  assert.equal(focusHref("/settings/sla"), "/settings/sla");
  assert.equal(
    focusHref("/settings/sla", "sla.new-policy"),
    "/settings/sla?focus=sla.new-policy",
  );
  assert.equal(focusHref("/channels", "Bad Target!"), "/channels");
  for (const bad of [
    "https://evil.example",
    "//evil.example",
    "/\\evil.example",
    "javascript:alert(1)",
    "/x?next=https://evil.example",
    "relative/path",
    "/\t/evil.example",
    "/\n/evil.example",
    "/ /evil.example",
  ]) {
    assert.equal(focusHref(bad, "a"), null, bad);
  }
});

test("readValue types the card fields", () => {
  assert.equal(readValue("string", "  Ad  "), "Ad");
  assert.equal(readValue("string", "   "), undefined);
  assert.equal(readValue("number", "42"), 42);
  assert.equal(readValue("boolean", true), true);
  assert.deepEqual(readValue("json", '{"all":[]}'), { all: [] });
  assert.equal(readValue("json", "{broken"), null);
  assert.equal(toText({ a: 1 }), '{\n  "a": 1\n}');
  assert.equal(toText(null), "");
});
