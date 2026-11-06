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

const { CONSENT_PATH, consentHeaders, headerRules } = await load(
  "./security-headers.ts",
);
const value = (headers, key) =>
  headers.find((h) => h.key.toLowerCase() === key.toLowerCase())?.value;

test("the OAuth consent screen can never be framed or cached", () => {
  assert.equal(CONSENT_PATH, "/oauth/consent");
  assert.match(
    value(consentHeaders, "Content-Security-Policy"),
    /frame-ancestors 'none'/,
  );
  assert.equal(value(consentHeaders, "X-Frame-Options"), "DENY");
  assert.equal(value(consentHeaders, "Cache-Control"), "no-store");
  assert.equal(value(consentHeaders, "Referrer-Policy"), "no-referrer");
});

test("the consent rule comes after the baseline so it wins", () => {
  for (const production of [true, false]) {
    const rules = headerRules(production);
    assert.equal(rules[0].source, "/:path*");
    assert.equal(rules.at(-1).source, CONSENT_PATH);
    assert.deepEqual(rules.at(-1).headers, consentHeaders);
    assert.match(
      value(rules[0].headers, "Content-Security-Policy"),
      /frame-ancestors 'self'/,
    );
    assert.equal(value(rules[0].headers, "X-Content-Type-Options"), "nosniff");
    assert.equal(
      Boolean(value(rules[0].headers, "Strict-Transport-Security")),
      production,
    );
  }
});
