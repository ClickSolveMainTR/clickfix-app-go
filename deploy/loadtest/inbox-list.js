// Inbox list: the agent's main screen (list + counters), with filter variations.
import http from "k6/http";
import { sleep } from "k6";
import { API, headers, login, ok, options as base, pick } from "./lib.js";

export const options = base({
  "http_req_duration{name:inbox.list}": ["p(95)<500"],
  "http_req_duration{name:inbox.counts}": ["p(95)<300"],
});

export const setup = login;

const filters = ["status=open", "status=open&assignee=me", "status=open&priority=urgent", "status=resolved", "status=all"];

export default function (data) {
  const h = { headers: headers(data) };
  ok(http.get(`${API}/tenant/conversations?${pick(filters)}&limit=25`, { ...h, tags: { name: "inbox.list" } }), "inbox.list");
  ok(http.get(`${API}/tenant/conversations/counts`, { ...h, tags: { name: "inbox.counts" } }), "inbox.counts");
  sleep(1 + Math.random() * 2);
}
