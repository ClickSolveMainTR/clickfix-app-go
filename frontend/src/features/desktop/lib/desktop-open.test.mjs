import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { transform } from "esbuild";

const { code } = await transform(
  await readFile(new URL("./desktop-open.ts", import.meta.url), "utf8"),
  { loader: "ts", format: "esm" },
);
const { desktopOpenPath, desktopAppLink } = await import(
  `data:text/javascript;base64,${Buffer.from(code).toString("base64")}`
);

const id = "0b9c8a1e-1111-4222-8333-444455556666";

test("desktopOpenPath keeps in-app paths only", () => {
  assert.equal(desktopOpenPath(`/inbox?c=${id}`), `/inbox?c=${id}`);
  assert.equal(desktopOpenPath(" /booking "), "/booking");
  for (const bad of [
    null,
    undefined,
    "",
    "inbox",
    "//evil.test",
    "/\\evil.test",
    "https://evil.test/",
    "javascript:alert(1)",
    "/a\nb",
    `/${"a".repeat(2048)}`,
  ]) {
    assert.equal(desktopOpenPath(bad), null, String(bad));
  }
});

test("desktopAppLink uses the conversation form when it can", () => {
  assert.equal(
    desktopAppLink(`/inbox?c=${id.toUpperCase()}`),
    `clickfix://conversation/${id}`,
  );
  assert.equal(
    desktopAppLink(`/inbox?c=${id}&tab=notes`),
    `clickfix://open?path=${encodeURIComponent(`/inbox?c=${id}&tab=notes`)}`,
  );
  assert.equal(
    desktopAppLink("/inbox?c=../x"),
    "clickfix://open?path=%2Finbox%3Fc%3D..%2Fx",
  );
  assert.equal(
    desktopAppLink("/booking?a=1"),
    "clickfix://open?path=%2Fbooking%3Fa%3D1",
  );
});
