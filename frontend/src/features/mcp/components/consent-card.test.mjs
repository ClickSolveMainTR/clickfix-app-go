import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath, pathToFileURL } from "node:url";
import { build } from "esbuild";

const src = fileURLToPath(new URL("../../../", import.meta.url));

/**
 * Bundles the real consent card with one stub: the locale provider, whose
 * `t` echoes the key and its variables, so the markup can be read.
 */
async function load() {
  const { outputFiles } = await build({
    stdin: {
      contents: `
        export { createElement } from "react";
        export { renderToStaticMarkup } from "react-dom/server";
        export { ConsentCard } from "@/features/mcp/components/consent-card";
      `,
      resolveDir: src,
      loader: "ts",
    },
    bundle: true,
    write: false,
    format: "esm",
    platform: "node",
    jsx: "automatic",
    alias: { "@": src.replace(/\/$/, "") },
    banner: {
      js: 'import { createRequire as __cr } from "node:module"; const require = __cr(import.meta.url);',
    },
    define: { "process.env.NODE_ENV": '"production"' },
    logLevel: "silent",
    plugins: [
      {
        name: "stub-locale",
        setup(b) {
          b.onResolve({ filter: /^@\/providers\/locale-provider$/ }, () => ({
            path: "locale-stub",
            namespace: "stub",
          }));
          b.onLoad({ filter: /.*/, namespace: "stub" }, () => ({
            contents: `export function useLocale() {
              return {
                locale: "en",
                t: (key, vars) => (vars ? key + JSON.stringify(vars) : key),
              };
            }`,
            loader: "js",
          }));
        },
      },
    ],
  });
  const dir = await mkdtemp(join(tmpdir(), "consent-card-"));
  const file = join(dir, "bundle.mjs");
  await writeFile(file, outputFiles[0].text);
  try {
    return await import(pathToFileURL(file).href);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
}

const { createElement, renderToStaticMarkup, ConsentCard } = await load();

const consent = (over = {}) => ({
  request_uuid: "5b0a7d0e-6f0a-4f0e-9a7e-2f4d6c1b9e11",
  client_id: "mcp_abc",
  client_name: 'Claude <img src=x onerror="alert(1)"> <b>verified</b>',
  redirect_host: "xn--cl-ude-4ve.ai",
  redirect_loopback: false,
  resource: "/mcp/business",
  resource_url: "https://app.example.test/mcp/business",
  layer: "business",
  scopes: ["mcp:read", "mcp:write"],
  write_requested: true,
  organizations: [
    {
      uuid: "o-1",
      slug: "acme",
      name: "Acme <script>1</script>",
      tool_count: 6,
      can_write: true,
    },
  ],
  expires_at: "2026-11-06T12:00:00Z",
  access_ttl_seconds: 3600,
  refresh_ttl_seconds: 2592000,
  ...over,
});

const render = (props = {}) =>
  renderToStaticMarkup(
    createElement(ConsentCard, {
      consent: consent(),
      account: "selin@example.test",
      selected: "o-1",
      allowWrite: false,
      busy: false,
      errorKey: null,
      onSelect() {},
      onAllowWrite() {},
      onDecide() {},
      ...props,
    }),
  );

/** The opening tag of the button whose label is `key`. */
function button(html, key) {
  const at = html.indexOf(`>${key}</button>`);
  assert.notEqual(at, -1, `button ${key} not found`);
  return html.slice(html.lastIndexOf("<button", at), at + 1);
}

/** The `disabled` attribute itself (class names such as `disabled:opacity-50` do not count). */
const DISABLED = /\sdisabled(?:=""|\s|>)/;

test("what the client says about itself is text, never markup", () => {
  const html = render();
  assert.ok(
    html.includes(
      "Claude &lt;img src=x onerror=&quot;alert(1)&quot;&gt; &lt;b&gt;verified&lt;/b&gt;",
    ),
  );
  assert.ok(!html.includes("<img"));
  assert.ok(!html.includes("<b>verified"));
  assert.ok(html.includes("Acme &lt;script&gt;1&lt;/script&gt;"));
  assert.ok(!html.includes("<script"));
});

test("the name is marked as the client's own claim and the return host is shown as it is", () => {
  const html = render();
  assert.ok(html.includes("mcp.consent.unverified"));
  // Punycode, not the look-alike Unicode form.
  assert.ok(html.includes("xn--cl-ude-4ve.ai"));
  assert.ok(html.includes("mcp.consent.returns_remote"));
  assert.ok(!html.includes("mcp.consent.returns_local"));
  assert.ok(html.includes("selin@example.test"));
  assert.ok(html.includes("mcp.consent.duration"));
  assert.ok(html.includes("mcp.consent.hours"), "1 hour of access");
  assert.ok(html.includes("mcp.consent.days"), "30 days of renewal");
});

test("a loopback return is worded as a program on this computer", () => {
  const html = render({
    consent: consent({
      redirect_host: "127.0.0.1:7777",
      redirect_loopback: true,
    }),
  });
  assert.ok(html.includes("mcp.consent.returns_local"));
  assert.ok(!html.includes("mcp.consent.returns_remote"));
});

test("Deny has the focus; Allow is a separate, enabled button", () => {
  const html = render();
  assert.match(button(html, "mcp.consent.deny"), /autofocus/i);
  assert.doesNotMatch(button(html, "mcp.consent.approve"), /autofocus/i);
  assert.doesNotMatch(button(html, "mcp.consent.approve"), DISABLED);
});

test("while a decision is on its way nothing can be pressed again", () => {
  const html = render({ busy: true });
  assert.match(button(html, "mcp.consent.deny"), DISABLED);
  assert.match(button(html, "mcp.consent.approve"), DISABLED);
  assert.match(html, /<input[^>]*type="radio"[^>]*\sdisabled(?:=""|\s|>)/);
});

test("without an organization chosen, Allow is off but Deny works", () => {
  const html = render({ selected: undefined });
  assert.match(button(html, "mcp.consent.approve"), DISABLED);
  assert.doesNotMatch(button(html, "mcp.consent.deny"), DISABLED);
});

test("the write box is offered only when asked for and permitted there", () => {
  assert.ok(render().includes("mcp.consent.scope_write"));
  const noPermission = render({
    consent: consent({
      organizations: [
        {
          uuid: "o-1",
          slug: "acme",
          name: "Acme",
          tool_count: 3,
          can_write: false,
        },
      ],
    }),
  });
  assert.ok(!noPermission.includes("mcp.consent.scope_write"));
  assert.ok(noPermission.includes("mcp.consent.write_unavailable"));
  const notAsked = render({ consent: consent({ write_requested: false }) });
  assert.ok(!notAsked.includes("mcp.consent.scope_write"));
  assert.ok(!notAsked.includes("mcp.consent.write_unavailable"));
});

test("a refused decision is announced", () => {
  const html = render({ errorKey: "mcp.consent.expired" });
  assert.match(html, /role="alert"[^>]*>mcp\.consent\.expired/);
});
