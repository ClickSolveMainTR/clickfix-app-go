import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { fixture, load } from "./load.mjs";

const targeting = await load("../src/shared/targeting.ts");
const { sanitizeCSS } = await load("../src/shared/css.ts");
const schema = await load("../src/shared/schema.ts");
const markdown = await load("../src/shared/markdown.ts");
const theme = await load("../src/shared/theme.ts");
const i18n = await load("../src/shared/i18n.ts");
const cases = JSON.parse(
  await readFile(fixture("targeting_cases.json"), "utf8"),
);

test("URL rules match like the API (shared fixture)", () => {
  for (const c of cases.url)
    assert.equal(
      targeting.matchURL(c.rule, c.url),
      c.match,
      `${JSON.stringify(c.rule)} ${c.url}`,
    );
});

test("page targeting matches the API (shared fixture)", () => {
  for (const [i, c] of cases.visible.entries()) {
    const got = targeting.visible(c.targeting, c.ctx);
    assert.deepEqual(
      got,
      { visible: c.visible, reason: c.reason ?? "" },
      `visible[${i}]`,
    );
  }
});

test("trigger page conditions match the API (shared fixture)", () => {
  for (const c of cases.triggers) {
    const t = {
      ...c.trigger,
      conditions: { url: [], ...c.trigger.conditions },
    };
    assert.equal(targeting.eligible(t, c.ctx), c.eligible, `trigger ${t.id}`);
  }
});

test("custom CSS is sanitized like the API (shared fixture)", () => {
  for (const c of cases.css) {
    const out = sanitizeCSS(c.in);
    assert.equal(out.css, c.out, c.in);
    assert.equal(out.problems.length, c.problems, c.in);
  }
});

test("defaults equal the API defaults (Go testdata/defaults.json)", async () => {
  const go = JSON.parse(await readFile(fixture("defaults.json"), "utf8"));
  assert.deepEqual(schema.defaults(), go);
  assert.deepEqual(schema.normalize({}), go);
  assert.deepEqual(schema.normalize(go), go, "normalize is idempotent");
});

test("normalize keeps v1 settings and drops unsafe values", () => {
  const s = schema.normalize({
    color: "#FF0000",
    position: "left",
    greeting_title: "Selam",
    appearance: {
      logo_url: "javascript:alert(1)",
      radius: 99,
      launcher: { icon: "custom" },
    },
    channels: { whatsapp: "+90 555 111 22 33", instagram: "@acme" },
    privacy: { cookieless: true },
    triggers: [
      { id: "t", message: "Hi" },
      { id: "t", message: "dup" },
      { id: "x" },
    ],
  });
  assert.equal(s.color, "#ff0000");
  assert.equal(s.position, "left");
  assert.equal(s.appearance.logo_url, "");
  assert.equal(s.appearance.radius, 16);
  assert.equal(s.appearance.launcher.icon, "chat");
  assert.equal(s.channels.whatsapp, "905551112233");
  assert.equal(s.channels.instagram, "acme");
  assert.equal(s.privacy.persistence, "memory");
  assert.equal(s.triggers.length, 1);
  assert.deepEqual(s.triggers[0].message, { tr: "Hi", en: "Hi" });
  assert.equal(schema.text(s, "tr", "greeting_title"), "Selam");
});

test("attachment groups and accept list", () => {
  assert.equal(
    schema.attachmentKind("application/octet-stream", "a.PDF"),
    "pdf",
  );
  assert.equal(schema.attachmentKind("image/png", "x"), "image");
  assert.equal(
    schema.attachmentKind("application/x-msdownload", "setup.exe"),
    "",
  );
  assert.equal(schema.acceptList(["pdf", "text"]), ".pdf,.txt,.log,.json");
});

test("markdown subset is safe and structured", () => {
  const blocks = markdown.parse(
    "**Merhaba** *dünya* `kod`\n- bir\n- iki\n[site](https://x.test) javascript:alert(1)",
  );
  assert.equal(blocks[0].t, "p");
  assert.deepEqual(blocks[0].c[0], {
    t: "strong",
    c: [{ t: "text", v: "Merhaba" }],
  });
  assert.equal(blocks[1].t, "list");
  assert.equal(blocks[1].items.length, 2);
  const link = blocks[2].c.find((x) => x.t === "link");
  assert.equal(link.href, "https://x.test");
  assert.ok(!JSON.stringify(blocks).includes('"href":"javascript'));
  // [x](javascript:...) never becomes a link.
  assert.ok(
    !markdown
      .parse("[x](javascript:alert(1))")
      .flatMap((b) => b.c ?? [])
      .some((i) => i.t === "link"),
  );
  // Markdown off: only links are recognised.
  const raw = markdown.parse("**a** https://x.test", false);
  assert.equal(raw[0].c[0].v, "**a** ");
  assert.equal(
    markdown.plain("**Kalın** [link](https://x.test)"),
    "Kalın link",
  );
});

test("contrast helpers pick readable text", () => {
  assert.equal(theme.onColor("#ffffff"), "#111827");
  assert.equal(theme.onColor("#4f46e5"), "#ffffff");
  assert.ok(theme.contrast("#000000", "#ffffff") > 20);
});

test("i18n catalogs have the same keys and locale detection", () => {
  assert.deepEqual(
    Object.keys(i18n.STRINGS.tr).sort(),
    Object.keys(i18n.STRINGS.en).sort(),
  );
  assert.equal(i18n.detectLocale(["de-DE", "tr-TR"]), "tr");
  assert.equal(i18n.detectLocale(["fr"]), "en");
  assert.equal(i18n.format("{time} sonra", { time: "09:00" }), "09:00 sonra");
});
