import assert from "node:assert/strict";
import { test } from "node:test";

import { supportWidgetInit } from "./support-widget.ts";

const data = {
  available: true,
  widget_id: "0b9f3c3e-6a4f-4a4e-9a51-2b3c4d5e6f70",
  user_id: "member:org:user",
  user_hash: "ab".repeat(32),
};
const user = { name: "Ayşe", email: "ayse@example.com" };

test("the widget loads identified, without a launcher and without screen capture", () => {
  assert.deepEqual(supportWidgetInit(data, user, "tr"), {
    widgetId: data.widget_id,
    locale: "tr",
    user: {
      user_id: data.user_id,
      user_hash: data.user_hash,
      name: "Ayşe",
      email: "ayse@example.com",
    },
    hideLauncher: true,
    capture: false,
  });
});

test("nothing loads without a complete, well-formed answer", () => {
  assert.equal(supportWidgetInit({ available: false }, user, "tr"), null);
  assert.equal(supportWidgetInit(data, null, "tr"), null);
  assert.equal(
    supportWidgetInit({ ...data, widget_id: "../x" }, user, "tr"),
    null,
  );
  assert.equal(
    supportWidgetInit({ ...data, user_hash: "short" }, user, "tr"),
    null,
  );
  assert.equal(supportWidgetInit(undefined, user, "tr"), null);
});
