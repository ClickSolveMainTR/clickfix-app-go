import assert from "node:assert/strict";
import { test } from "node:test";
import { PendingMessages, shouldSendQueued } from "./queue.ts";

test("messages typed during a run are kept apart and sent in order", () => {
  const q = new PendingMessages();
  let notified = 0;
  q.subscribe(() => notified++);
  assert.equal(q.push("  "), false);
  assert.equal(q.push("Kanallarımı listele"), true);
  assert.equal(q.push(" Ekiplerimi de "), true);
  assert.equal(q.size, 2);
  assert.deepEqual(q.list(), ["Kanallarımı listele", "Ekiplerimi de"]);
  assert.equal(q.next(), "Kanallarımı listele");
  assert.equal(q.next(), "Ekiplerimi de");
  assert.equal(q.next(), undefined);
  assert.equal(notified, 4);
});

test("clear empties the queue once and tells the listeners", () => {
  const q = new PendingMessages();
  let notified = 0;
  q.subscribe(() => notified++);
  q.clear();
  q.push("a");
  q.clear();
  assert.equal(q.size, 0);
  assert.equal(notified, 2);
});

test("a queued message goes out only when the run settles", () => {
  assert.equal(shouldSendQueued(true, false, 1), true);
  assert.equal(shouldSendQueued(true, true, 1), false);
  assert.equal(shouldSendQueued(false, false, 1), false);
  assert.equal(shouldSendQueued(true, false, 0), false);
});
