import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { transform } from "esbuild";

const { code } = await transform(
  await readFile(new URL("./src/privacy.ts", import.meta.url), "utf8"),
  { loader: "ts", format: "esm" },
);
const { scrub } = await import(
  `data:text/javascript;base64,${Buffer.from(code).toString("base64")}`
);

test("replay masks initial snapshot, mutations, inputs and URLs before upload", () => {
  const event = {
    type: 2,
    timestamp: 123,
    data: {
      href: "https://example.test/?token=secret",
      node: {
        tagName: "div",
        attributes: {
          title: "secret@example.test",
          value: "password-secret",
          "data-account": "secret",
          src: "https://example.test/private-secret.png",
          width: "120px",
        },
        childNodes: [{ textContent: "secret@example.test" }],
      },
      texts: [{ id: 9, value: "secret", textContent: "secret" }],
      text: "typed-secret",
    },
  };
  const out = scrub(event);
  assert.ok(!JSON.stringify(out).includes("secret"));
  assert.equal(out.data.node.tagName, "div");
  assert.equal(out.data.node.attributes.width, "120px");
  assert.equal(out.data.href, "about:blank");
  assert.equal(out.timestamp, 123);
  assert.equal(event.data.text, "typed-secret", "must not mutate rrweb state");
});

test("replay strips arbitrary attribute strings and metadata", () => {
  assert.deepEqual(
    scrub({
      attributes: { width: "url(secret)", style: "background:url(secret)" },
      title: "secret",
      tagName: "script>secret",
    }),
    { attributes: {}, title: "", tagName: "" },
  );
});
