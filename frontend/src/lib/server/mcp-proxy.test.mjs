import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createServer } from "node:http";
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

const {
  apiOrigin,
  forwardMcp,
  mcpUpstreamPath,
  MCP_MAX_BODY_BYTES,
  MCP_PUBLIC_PATHS,
  MCP_REQUEST_HEADERS,
  MCP_RESPONSE_HEADERS,
} = await load("./mcp-proxy-core.ts");

/** A stand-in for the Go API that records what reached it. */
async function fakeApi(handler) {
  const seen = [];
  const server = createServer((req, res) => {
    const chunks = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => {
      const call = {
        method: req.method,
        url: req.url,
        headers: req.headers,
        body: Buffer.concat(chunks).toString("utf8"),
      };
      seen.push(call);
      handler(call, res);
    });
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const origin = `http://127.0.0.1:${server.address().port}`;
  return { origin, seen, close: () => new Promise((r) => server.close(r)) };
}

const WEB = "https://app.example.test";

test("the public surface is exactly the listed paths", () => {
  assert.deepEqual([...MCP_PUBLIC_PATHS].sort(), [
    "/.well-known/oauth-authorization-server",
    "/.well-known/oauth-protected-resource/mcp/business",
    "/mcp/business",
    "/oauth/authorize",
    "/oauth/register",
    "/oauth/revoke",
    "/oauth/token",
  ]);
  for (const path of MCP_PUBLIC_PATHS) {
    const method =
      path.startsWith("/oauth/") && path !== "/oauth/authorize"
        ? "POST"
        : "GET";
    assert.equal(mcpUpstreamPath(path, method), path, path);
  }
  assert.equal(mcpUpstreamPath("/mcp/business", "POST"), "/mcp/business");
  assert.equal(mcpUpstreamPath("/mcp/business/", "DELETE"), "/mcp/business");
  assert.equal(
    mcpUpstreamPath("/oauth/register", "OPTIONS"),
    "/oauth/register",
  );
});

test("never forwards other paths, layers or methods", () => {
  for (const [path, method] of [
    ["/mcp/platform", "POST"],
    ["/mcp/customer", "POST"],
    ["/mcp/business/extra", "POST"],
    ["/mcp/../v1/auth/me", "GET"],
    ["/mcp/%2e%2e/v1/auth/me", "GET"],
    ["/oauth/consent", "GET"],
    ["/oauth/authorize", "POST"],
    ["/oauth/token", "GET"],
    ["/oauth/internal", "POST"],
    ["/oauth/token/extra", "POST"],
    ["/v1/auth/me", "GET"],
    ["/v1/mcp/grants", "GET"],
    ["/.well-known/oauth-protected-resource/mcp/platform", "GET"],
    ["/.well-known/openid-configuration", "GET"],
    ["/.well-known/oauth-authorization-server/extra", "GET"],
    ["/mcp/business", "PUT"],
    ["/mcp/business", "PATCH"],
  ]) {
    assert.equal(mcpUpstreamPath(path, method), null, `${method} ${path}`);
  }
});

test("a path that is not listed never reaches the API", async () => {
  const api = await fakeApi((_c, res) => res.end("{}"));
  try {
    for (const path of ["/v1/auth/me", "/oauth/consent", "/mcp/platform"]) {
      const res = await forwardMcp(new Request(`${WEB}${path}`), {
        origin: api.origin,
      });
      assert.equal(res.status, 404);
    }
    assert.equal(api.seen.length, 0);
  } finally {
    await api.close();
  }
});

test("forwards the body and the client's Authorization, never the session cookie", async () => {
  const api = await fakeApi((_c, res) => {
    res.writeHead(200, {
      "Content-Type": "application/json",
      "Set-Cookie": "leak=1",
      "X-Internal": "secret",
    });
    res.end('{"jsonrpc":"2.0","id":1,"result":{}}');
  });
  try {
    const body = '{"jsonrpc":"2.0","id":1,"method":"tools/list"}';
    const res = await forwardMcp(
      new Request(`${WEB}/mcp/business?x=1`, {
        method: "POST",
        headers: {
          Authorization: "Bearer client-token",
          Cookie: "session=panel-session",
          "Content-Type": "application/json",
          "X-Organization": "acme",
          "X-Forwarded-For": "6.6.6.6",
        },
        body,
      }),
      { origin: api.origin, clientIp: "203.0.113.9" },
    );
    assert.equal(res.status, 200);
    assert.equal(await res.text(), '{"jsonrpc":"2.0","id":1,"result":{}}');
    const call = api.seen[0];
    assert.equal(call.method, "POST");
    assert.equal(call.url, "/mcp/business?x=1");
    assert.equal(call.body, body);
    assert.equal(call.headers.authorization, "Bearer client-token");
    assert.equal(call.headers.cookie, undefined);
    assert.equal(call.headers["x-organization"], undefined);
    // The client address comes from the edge, not from the client's own header.
    assert.equal(call.headers["x-forwarded-for"], "203.0.113.9");
    // Nothing of the API leaks back beyond the listed headers.
    assert.equal(res.headers.get("set-cookie"), null);
    assert.equal(res.headers.get("x-internal"), null);
    assert.equal(res.headers.get("cache-control"), "no-store");
  } finally {
    await api.close();
  }
});

test("redirects are returned unchanged, never followed", async () => {
  const consent = `${WEB}/oauth/consent?request=5b0a7d0e-6f0a-4f0e-9a7e-2f4d6c1b9e11`;
  const api = await fakeApi((call, res) => {
    if (call.url.startsWith("/oauth/authorize")) {
      res.writeHead(302, { Location: consent, "Cache-Control": "no-store" });
      res.end();
      return;
    }
    res.writeHead(500);
    res.end("followed");
  });
  try {
    const res = await forwardMcp(
      new Request(`${WEB}/oauth/authorize?client_id=c&response_type=code`),
      { origin: api.origin },
    );
    assert.equal(res.status, 302);
    assert.equal(res.headers.get("location"), consent);
    assert.equal(api.seen.length, 1);
    assert.equal(
      api.seen[0].url,
      "/oauth/authorize?client_id=c&response_type=code",
    );
  } finally {
    await api.close();
  }
});

test("the 401 challenge of the MCP endpoint reaches the client", async () => {
  const challenge =
    'Bearer resource_metadata="https://app.example.test/.well-known/oauth-protected-resource/mcp/business"';
  const api = await fakeApi((_c, res) => {
    res.writeHead(401, { "WWW-Authenticate": challenge });
    res.end();
  });
  try {
    const res = await forwardMcp(
      new Request(`${WEB}/mcp/business`, { method: "POST", body: "{}" }),
      { origin: api.origin },
    );
    assert.equal(res.status, 401);
    assert.equal(res.headers.get("www-authenticate"), challenge);
  } finally {
    await api.close();
  }
});

test("a body over the limit is refused before it is sent", async () => {
  const api = await fakeApi((_c, res) => res.end("{}"));
  try {
    const res = await forwardMcp(
      new Request(`${WEB}/oauth/token`, {
        method: "POST",
        body: "a".repeat(MCP_MAX_BODY_BYTES + 1),
      }),
      { origin: api.origin },
    );
    assert.equal(res.status, 413);
    assert.equal(api.seen.length, 0);
  } finally {
    await api.close();
  }
});

test("an unreachable API answers 503 without detail", async () => {
  const res = await forwardMcp(
    new Request(`${WEB}/oauth/token`, { method: "POST", body: "x" }),
    {
      origin: "http://127.0.0.1:1",
    },
  );
  assert.equal(res.status, 503);
  assert.deepEqual(Object.keys(await res.json()).sort(), [
    "error",
    "error_description",
  ]);
});

test("header lists never include a cookie", () => {
  assert.equal(MCP_REQUEST_HEADERS.includes("cookie"), false);
  assert.equal(MCP_REQUEST_HEADERS.includes("authorization"), true);
  assert.equal(MCP_RESPONSE_HEADERS.includes("set-cookie"), false);
});

test("apiOrigin strips the /v1 suffix of the upstream base", () => {
  assert.equal(apiOrigin("http://api:8080/v1"), "http://api:8080");
  assert.equal(apiOrigin("http://api:8080/v1/"), "http://api:8080");
  assert.equal(
    apiOrigin("https://api.example.test"),
    "https://api.example.test",
  );
});
