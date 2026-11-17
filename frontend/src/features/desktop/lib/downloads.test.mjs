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

const { detectOS, recommended, sortDownloads, appOrder, isApiDownloadPath } =
  await load("./downloads.ts");

const item = (app, os, arch) => ({
  app,
  os,
  arch,
  target: `${os}-${arch}`,
  version: "0.2.0",
  channel: "stable",
  os_signed: true,
  file: {
    name: "f",
    size: 1,
    sha256: "0".repeat(64),
    url: "/api/v1/public/desktop/files/x/installer/f",
  },
  published_at: "2026-11-16T00:00:00Z",
});

test("detectOS", () => {
  const mac =
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15";
  const win =
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36";
  const iphone =
    "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148";
  const android =
    "Mozilla/5.0 (Linux; Android 15) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Mobile Safari/537.36";
  assert.equal(detectOS(mac), "darwin");
  assert.equal(detectOS(win), "windows");
  assert.equal(detectOS("Mozilla/5.0 (X11; Linux x86_64)"), "linux");
  assert.equal(detectOS(iphone), null);
  assert.equal(detectOS(android), null);
  assert.equal(detectOS("", "macOS"), "darwin");
  assert.equal(detectOS("curl/8"), null);
});

test("recommended prefers Apple silicon and the right app", () => {
  const items = [
    item("support", "darwin", "x86_64"),
    item("support", "darwin", "aarch64"),
    item("support", "windows", "x86_64"),
    item("agent", "windows", "x86_64"),
  ];
  assert.equal(recommended(items, "support", "darwin").arch, "aarch64");
  assert.equal(recommended(items, "agent", "windows").app, "agent");
  assert.equal(recommended(items, "agent", "darwin"), null);
  assert.equal(recommended(items, "support", null), null);
  assert.deepEqual(
    sortDownloads(items.filter((i) => i.app === "support")).map(
      (i) => i.target,
    ),
    ["darwin-aarch64", "darwin-x86_64", "windows-x86_64"],
  );
});

test("appOrder and download paths", () => {
  assert.deepEqual(appOrder(null), ["support", "agent"]);
  assert.deepEqual(appOrder("support"), ["support", "agent"]);
  assert.deepEqual(appOrder("agent"), ["agent", "support"]);
  assert.ok(
    isApiDownloadPath("/api/v1/public/desktop/files/u/installer/a.dmg"),
  );
  assert.ok(!isApiDownloadPath("https://s3.example.test/bucket/a.dmg"));
});
