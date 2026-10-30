// Shared helpers for the ClickFix k6 load tests (CF-131). See README.md before running against
// staging or production.
import http from "k6/http";
import { check, fail } from "k6";

export const API = (__ENV.API_URL || "http://127.0.0.1:8080/v1").replace(/\/$/, "");
export const ORG = __ENV.ORG_SLUG || "demo-anadolu-yazilim";
const EMAIL = __ENV.DEMO_EMAIL || "burak.sahin@anadolu-yazilim.example.com";
const PASSWORD = __ENV.DEMO_PASSWORD || "";

// Low by default: a run must never be the biggest customer of the system it measures.
export function options(thresholds, overrides = {}) {
  return {
    scenarios: {
      default: {
        executor: "ramping-vus",
        startVUs: 0,
        stages: [
          { duration: __ENV.RAMP || "30s", target: Number(__ENV.VUS || 5) },
          { duration: __ENV.DURATION || "1m", target: Number(__ENV.VUS || 5) },
          { duration: "15s", target: 0 },
        ],
        gracefulRampDown: "10s",
      },
    },
    thresholds: Object.assign(
      {
        http_req_failed: ["rate<0.01"],
        checks: ["rate>0.99"],
      },
      thresholds,
    ),
    // Stop early instead of hammering a struggling system.
    noConnectionReuse: false,
    userAgent: "clickfix-loadtest/1.0 (k6)",
    ...overrides,
  };
}

// setup(): one login for the whole run; the auth endpoint is rate limited and not under test.
export function login() {
  if (!PASSWORD) fail("DEMO_PASSWORD is required (printed by seed-demo)");
  const res = http.post(`${API}/auth/login`, JSON.stringify({ email: EMAIL, password: PASSWORD }), {
    headers: { "Content-Type": "application/json" },
    tags: { name: "auth.login" },
  });
  if (res.status !== 200) fail(`login failed: ${res.status} ${res.body}`);
  return { token: res.json("data.access_token") };
}

export function headers(data) {
  return {
    Authorization: `Bearer ${data.token}`,
    "X-Organization": ORG,
    "Content-Type": "application/json",
  };
}

export function ok(res, name) {
  return check(res, { [`${name} 2xx`]: (r) => r.status >= 200 && r.status < 300 });
}

// Open demo conversations to read and reply to.
export function conversationIds(data, limit = 25) {
  const res = http.get(`${API}/tenant/conversations?status=open&limit=${limit}`, {
    headers: headers(data),
    tags: { name: "inbox.list" },
  });
  if (res.status !== 200) fail(`conversation list failed: ${res.status}`);
  const items = res.json("data.items") || [];
  if (!items.length) fail("no open conversations; run seed-demo first");
  return items.map((c) => c.uuid);
}

export function pick(list) {
  return list[Math.floor(Math.random() * list.length)];
}
