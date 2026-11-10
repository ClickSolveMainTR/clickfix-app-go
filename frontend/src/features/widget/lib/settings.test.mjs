import assert from "node:assert/strict";
import test from "node:test";
import { build } from "esbuild";

const out = await build({
  entryPoints: [new URL("./settings.ts", import.meta.url).pathname],
  bundle: true,
  format: "esm",
  platform: "neutral",
  write: false,
  logLevel: "silent",
  tsconfig: new URL("../../../../tsconfig.json", import.meta.url).pathname,
});
const lib = await import(
  `data:text/javascript;base64,${Buffer.from(out.outputFiles[0].text).toString("base64")}`
);

test("setPath copies along the path and creates missing objects", () => {
  const s = { a: { b: 1, keep: { x: 1 } }, list: [{ v: 1 }, { v: 2 }] };
  const next = lib.setPath(s, "a.b", 2);
  assert.equal(next.a.b, 2);
  assert.equal(s.a.b, 1, "original untouched");
  assert.equal(next.a.keep, s.a.keep, "untouched branches are shared");
  const listed = lib.setPath(s, "list.1.v", 9);
  assert.ok(Array.isArray(listed.list));
  assert.deepEqual(listed.list, [{ v: 1 }, { v: 9 }]);
  assert.deepEqual(lib.setPath({}, "x.y.z", true), { x: { y: { z: true } } });
  assert.equal(lib.getPath(listed, "list.1.v"), 9);
});

test("readSettings keeps replay and drops legacy hours", () => {
  const s = lib.readSettings({
    replay: { replay_enabled: true, retention_days: 99 },
    business_hours: { enabled: true, days: { mon: [] } },
  });
  assert.equal(s.replay.replay_enabled, true);
  assert.equal(s.replay.retention_days, 7);
  // Working hours come from the SLA schedule (CF-198), not the widget.
  assert.equal("business_hours" in s, false);
  assert.equal(s.schema_version, 2);
});

test("quick checks report errors and warnings", () => {
  const s = lib.readSettings({ color: "#ffff00" });
  s.appearance.launcher.icon = "custom";
  s.prechat.consent.enabled = true;
  s.triggers = [
    {
      id: "t",
      name: "",
      enabled: true,
      message: {},
      delay_s: 0,
      frequency: "once",
      open: false,
      conditions: {},
    },
  ];
  s.appearance.custom_css = "@import url(x); .a{}";
  const issues = lib.quickChecks(s);
  const paths = Object.fromEntries(issues.map((i) => [i.path, i.level]));
  assert.equal(paths["appearance.launcher.image_url"], "error");
  assert.equal(paths["prechat.consent.text"], "error");
  assert.equal(paths["triggers.0.message"], "error");
  assert.equal(paths["appearance.custom_css"], "warning");
  assert.equal(paths["color"], "warning", "yellow on white is low contrast");
  assert.equal(lib.publishable(issues), false);
  assert.equal(
    lib.publishable(issues.filter((i) => i.level === "warning")),
    true,
  );
  assert.equal(lib.quickChecks(lib.readSettings({})).length, 0);
});

test("section search folds Turkish characters and counts issues", () => {
  const titles = Object.fromEntries(lib.SECTIONS.map((s) => [s.id, s.id]));
  assert.ok(lib.searchSections("CEREZ", titles).includes("privacy"));
  assert.ok(lib.searchSections("whatsapp", titles).includes("channels"));
  // Working hours are no longer a widget section (CF-198); the search
  // still lands on the section that explains where they come from.
  assert.ok(lib.searchSections("çalışma", titles).includes("conversation"));
  assert.equal(
    lib.SECTIONS.some((s) => s.id === "hours"),
    false,
  );
  assert.equal(lib.searchSections("", titles).length, lib.SECTIONS.length);
  const issues = [
    { path: "appearance.launcher.size", code: "out_of_range", message: "" },
    { path: "triggers.0.message", code: "required", message: "" },
    { path: "color", code: "contrast", message: "", level: "warning" },
  ];
  assert.equal(lib.sectionIssues(issues, "launcher"), 1);
  assert.equal(lib.sectionIssues(issues, "triggers"), 1);
  assert.equal(lib.sectionIssues(issues, "appearance"), 0);
  assert.deepEqual(lib.issuesAt(issues, "triggers.0").length, 1);
});

test("install snippets for every platform carry the widget id", () => {
  for (const p of lib.PLATFORMS) {
    const code = lib.snippet(p, "https://app.test", "abc-123");
    assert.ok(code.length > 20, p);
    if (p !== "identify") assert.ok(code.includes("abc-123"), p);
  }
  assert.equal(
    lib.installSnippet("https://app.test", "w1"),
    '<script src="https://app.test/widget.js" data-widget="w1" async></script>',
  );
  for (const l of ["node", "go", "php", "python"])
    assert.ok(lib.hashSnippet(l).includes("CLICKFIX_IDENTITY_SECRET"));
});

test("fingerprint ignores key order", () => {
  assert.equal(
    lib.fingerprint({ a: 1, b: { c: 2, d: 3 } }),
    lib.fingerprint({ b: { d: 3, c: 2 }, a: 1 }),
  );
  assert.notEqual(lib.fingerprint({ a: 1 }), lib.fingerprint({ a: 2 }));
});

test("install freshness and helpers", () => {
  const now = Date.parse("2026-10-29T12:00:00Z");
  assert.equal(lib.installFreshness("2026-10-29T11:55:00Z", now), "live");
  assert.equal(lib.installFreshness("2026-10-27T12:00:00Z", now), "recent");
  assert.equal(lib.installFreshness("2026-09-01T12:00:00Z", now), "stale");
  assert.deepEqual(lib.parseDomains("A.com, b.com\nA.com"), ["a.com", "b.com"]);
  assert.match(lib.newId("t"), /^t_[0-9a-f]{8}$/);
});

test("identity warning follows the last rejected identify (CF-176)", () => {
  const at = "2026-11-02T10:00:00Z";
  const disabled = { code: "WIDGET_IDENTITY_DISABLED", rejected_at: at };
  const invalid = { code: "WIDGET_IDENTITY_INVALID", rejected_at: at };
  assert.equal(lib.identityWarning(null, false), null);
  assert.equal(lib.identityWarning(undefined, true), null);
  assert.equal(lib.identityWarning(disabled, false), "disabled");
  // The secret was generated after the rejection: nothing failed since.
  assert.equal(lib.identityWarning(disabled, true), null);
  assert.equal(lib.identityWarning(invalid, true), "invalid");
  assert.equal(lib.identityWarning({ code: "OTHER" }, false), null);
});

test("basic set-up steps are a subset of the advanced editor", () => {
  assert.deepEqual(
    lib.BASIC_STEPS.map((s) => s.id),
    ["appearance", "identity", "channel"],
  );
  for (const step of lib.BASIC_STEPS)
    for (const path of step.paths)
      assert.ok(
        lib.advancedSectionFor(path),
        `${path} has no advanced section`,
      );
  const issues = [
    { path: "appearance.launcher.size", code: "out_of_range", message: "" },
    { path: "prechat.form_uuid", code: "invalid", message: "" },
    { path: "color", code: "contrast", message: "", level: "warning" },
  ];
  assert.equal(lib.basicStepIssues(issues, "appearance"), 1);
  assert.equal(lib.basicStepIssues(issues, "channel"), 1);
  assert.equal(lib.basicStepIssues(issues, "identity"), 0);
});
