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

const { mcpClientConfigs, splitTools } = await load("./clients.ts");
const URL_ = "https://app.example.test/mcp/business";

test("every client configuration points at the reported endpoint", () => {
  const configs = mcpClientConfigs(URL_);
  assert.deepEqual(
    configs.map((c) => c.id),
    ["claude", "claude_code", "cursor", "vscode", "other"],
  );
  for (const c of configs) assert.ok(c.value.includes(URL_), c.id);
  assert.equal(configs[0].value, URL_);
  assert.equal(
    configs[1].value,
    `claude mcp add --transport http clickfix ${URL_}`,
  );
});

test("the JSON configurations parse and carry no credential", () => {
  for (const c of mcpClientConfigs(URL_).filter((c) => c.kind === "json")) {
    const parsed = JSON.parse(c.value);
    assert.equal(typeof parsed, "object");
    assert.doesNotMatch(
      c.value,
      /token|secret|authorization|bearer|api[_-]?key/i,
    );
  }
  const cursor = JSON.parse(mcpClientConfigs(URL_)[2].value);
  assert.deepEqual(cursor, { mcpServers: { clickfix: { url: URL_ } } });
  const vscode = JSON.parse(mcpClientConfigs(URL_)[3].value);
  assert.deepEqual(vscode, {
    servers: { clickfix: { type: "http", url: URL_ } },
  });
});

test("splitTools separates what the member gets, read tools first", () => {
  const tools = [
    { name: "reply", description: "", write: true, available: true },
    { name: "list_contacts", description: "", write: false, available: false },
    {
      name: "search_conversations",
      description: "",
      write: false,
      available: true,
    },
    { name: "set_status", description: "", write: true, available: false },
  ];
  const { available, unavailable } = splitTools(tools);
  assert.deepEqual(
    available.map((t) => t.name),
    ["search_conversations", "reply"],
  );
  assert.deepEqual(
    unavailable.map((t) => t.name),
    ["list_contacts", "set_status"],
  );
});
