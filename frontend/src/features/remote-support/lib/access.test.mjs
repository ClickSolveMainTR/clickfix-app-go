import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { transform } from "esbuild";

async function load(name) {
  const { code } = await transform(
    await readFile(new URL(name, import.meta.url), "utf8"),
    { loader: "ts", format: "esm" },
  );
  return import(
    `data:text/javascript;base64,${Buffer.from(code).toString("base64")}`
  );
}

const a = await load("./access.ts");
const live = {
  status: "active",
  mode_requested: "control",
  control_active: true,
};

test("members without tenant.remote_support.use do not see the action", () => {
  assert.deepEqual(a.remoteSupportAction({ use: false, control: true }), {
    visible: false,
    modes: [],
  });
  assert.deepEqual(a.remoteSupportAction({ use: true, control: false }).modes, [
    "view",
  ]);
  assert.deepEqual(a.remoteSupportAction({ use: true, control: true }).modes, [
    "view",
    "control",
  ]);
});

test("control needs the permission, the consent, a live session and the data grant", () => {
  assert.equal(
    a.controlAllowed({ use: true, control: true }, live, true),
    true,
  );
  assert.equal(
    a.controlAllowed({ use: true, control: false }, live, true),
    false,
  );
  assert.equal(
    a.controlAllowed({ use: false, control: true }, live, true),
    false,
  );
  assert.equal(
    a.controlAllowed({ use: true, control: true }, live, false),
    false,
  );
  assert.equal(
    a.controlAllowed(
      { use: true, control: true },
      { ...live, control_active: false },
      true,
    ),
    false,
  );
  assert.equal(
    a.controlAllowed(
      { use: true, control: true },
      { ...live, mode_requested: "view" },
      true,
    ),
    false,
  );
  assert.equal(
    a.controlAllowed(
      { use: true, control: true },
      { ...live, status: "ended" },
      true,
    ),
    false,
  );
  assert.equal(
    a.controlAllowed({ use: true, control: true }, null, true),
    false,
  );
});

test("status steps follow the session", () => {
  assert.equal(a.sessionStep({ status: "pending" }), "waiting");
  assert.equal(a.sessionStep({ status: "joined" }), "joined");
  assert.equal(
    a.sessionStep({
      status: "joined",
      view_consent_at: "2026-11-16T10:00:00Z",
    }),
    "consented",
  );
  assert.equal(a.sessionStep({ status: "active" }), "active");
  assert.equal(a.sessionStep({ status: "expired" }), "ended");
});

test("formatting and error keys", () => {
  assert.equal(a.formatCode("123456789"), "123 456 789");
  assert.equal(a.formatDuration(65), "1:05");
  assert.equal(a.formatDuration(3725), "1:02:05");
  assert.equal(
    a.errorKey({ code: "REMOTE_SUPPORT_DISABLED" }),
    "REMOTE_SUPPORT_DISABLED",
  );
  assert.equal(a.errorKey({ code: "WHATEVER" }), "UNKNOWN");
  assert.equal(a.errorKey(null), "UNKNOWN");
});
