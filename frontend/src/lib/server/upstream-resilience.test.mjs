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

const {
  NULL_BODY_STATUSES,
  proxyResponse,
  isUpstreamNetworkError,
  withUpstreamRetry,
  UpstreamUnavailableError,
  upstreamUnavailableResponse,
} = await load("./upstream-resilience.ts");

function fetchFailed(code) {
  const cause = Object.assign(new Error(`connect ${code}`), { code });
  return new TypeError("fetch failed", { cause });
}

const noSleep = async () => {};

test("null-body statuses never get a body (CLICKFIX-FRONT-END-5)", async () => {
  // The upstream body of a 204 arrives as an empty ArrayBuffer; passing it
  // to the Response constructor is what threw in production.
  assert.throws(
    () => new Response(new ArrayBuffer(0), { status: 204 }),
    TypeError,
  );
  for (const status of NULL_BODY_STATUSES) {
    const res = proxyResponse(new ArrayBuffer(0), { status });
    assert.equal(res.status, status);
    assert.equal(res.body, null);
  }
  const ok = proxyResponse('{"success":true}', { status: 200 });
  assert.equal(await ok.text(), '{"success":true}');
});

test("network failures are recognised, aborts and bugs are not", () => {
  assert.equal(isUpstreamNetworkError(fetchFailed("ECONNREFUSED")), true);
  assert.equal(isUpstreamNetworkError(new TypeError("terminated")), true);
  const aggregate = new TypeError("fetch failed", {
    cause: { errors: [{ code: "ECONNRESET" }] },
  });
  assert.equal(isUpstreamNetworkError(aggregate), true);
  const abort = new DOMException("aborted", "AbortError");
  assert.equal(isUpstreamNetworkError(abort), false);
  assert.equal(isUpstreamNetworkError(new RangeError("bug")), false);
});

test("idempotent requests retry through a short outage", async () => {
  let calls = 0;
  const result = await withUpstreamRetry(
    async () => {
      calls++;
      if (calls < 3) throw fetchFailed("ECONNRESET");
      return "ok";
    },
    { method: "GET", sleep: noSleep },
  );
  assert.equal(result, "ok");
  assert.equal(calls, 3);
});

test("retries are bounded and end in UpstreamUnavailableError", async () => {
  let calls = 0;
  await assert.rejects(
    withUpstreamRetry(
      async () => {
        calls++;
        throw fetchFailed("ECONNREFUSED");
      },
      { method: "GET", delaysMs: [1, 1], sleep: noSleep },
    ),
    UpstreamUnavailableError,
  );
  assert.equal(calls, 3);
});

test("mutations only retry when the connection was refused", async () => {
  let calls = 0;
  await assert.rejects(
    withUpstreamRetry(
      async () => {
        calls++;
        throw fetchFailed("ECONNRESET");
      },
      { method: "POST", sleep: noSleep },
    ),
    UpstreamUnavailableError,
  );
  assert.equal(calls, 1, "a reset POST may have reached the API: no resend");

  calls = 0;
  const result = await withUpstreamRetry(
    async () => {
      calls++;
      if (calls === 1) throw fetchFailed("ECONNREFUSED");
      return "sent";
    },
    { method: "POST", sleep: noSleep },
  );
  assert.equal(result, "sent");
  assert.equal(calls, 2);
});

test("other errors are rethrown without retry", async () => {
  let calls = 0;
  const bug = new RangeError("bug");
  await assert.rejects(
    withUpstreamRetry(
      async () => {
        calls++;
        throw bug;
      },
      { method: "GET", sleep: noSleep },
    ),
    (err) => err === bug,
  );
  assert.equal(calls, 1);
});

test("unavailable response is a 503 API envelope", async () => {
  const res = upstreamUnavailableResponse();
  assert.equal(res.status, 503);
  assert.equal(res.headers.get("retry-after"), "5");
  const body = await res.json();
  assert.equal(body.success, false);
  assert.equal(body.error.code, "UPSTREAM_UNAVAILABLE");
});
