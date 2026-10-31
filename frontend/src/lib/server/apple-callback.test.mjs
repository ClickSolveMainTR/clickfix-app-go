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

const { isCrossSiteAppleCallback, resubmitSameSite } = await load(
  "./apple-callback.ts",
);

const req = (path, headers = {}, method = "POST") => ({
  method,
  headers: new Headers(headers),
  nextUrl: { pathname: path },
});

test("Apple's cross-site form_post callback is re-posted", () => {
  const path = "/api/auth/callback/apple";
  assert.equal(
    isCrossSiteAppleCallback(req(path, { "sec-fetch-site": "cross-site" })),
    true,
  );
  assert.equal(
    isCrossSiteAppleCallback(
      req(path, { origin: "https://appleid.apple.com" }),
    ),
    true,
  );
});

test("the re-posted request and other routes go straight to Auth.js", () => {
  const path = "/api/auth/callback/apple";
  assert.equal(
    isCrossSiteAppleCallback(req(path, { "sec-fetch-site": "same-origin" })),
    false,
  );
  // Sec-Fetch-Site wins over a spoofable Origin.
  assert.equal(
    isCrossSiteAppleCallback(
      req(path, {
        "sec-fetch-site": "same-origin",
        origin: "https://appleid.apple.com",
      }),
    ),
    false,
  );
  assert.equal(
    isCrossSiteAppleCallback(
      req("/api/auth/callback/github", { "sec-fetch-site": "cross-site" }),
    ),
    false,
  );
  assert.equal(
    isCrossSiteAppleCallback(
      req(path, { "sec-fetch-site": "cross-site" }, "GET"),
    ),
    false,
  );
});

test("the resubmit page posts every field back, escaped", async () => {
  const form = new FormData();
  form.set("state", "s1");
  form.set("code", 'c"><script>x</script>');
  form.set("user", '{"name":{"firstName":"A"}}');
  const res = resubmitSameSite(form, "/api/auth/callback/apple");
  assert.equal(res.status, 200);
  assert.equal(res.headers.get("cache-control"), "no-store");
  const html = await res.text();
  assert.match(
    html,
    /<form id="f" method="post" action="\/api\/auth\/callback\/apple">/,
  );
  assert.match(html, /name="state" value="s1"/);
  assert.match(
    html,
    /name="code" value="c&quot;&gt;&lt;script&gt;x&lt;\/script&gt;"/,
  );
  assert.match(html, /name="user" value="\{&quot;name&quot;/);
  assert.doesNotMatch(html, /<script>x<\/script>/);
});
