// Opening a conversation: detail + message history + read marker.
import http from "k6/http";
import { sleep } from "k6";
import { API, conversationIds, headers, login, ok, options as base, pick } from "./lib.js";

export const options = base({
  "http_req_duration{name:conversation.get}": ["p(95)<400"],
  "http_req_duration{name:conversation.messages}": ["p(95)<500"],
});

export function setup() {
  const data = login();
  return { ...data, ids: conversationIds(data) };
}

export default function (data) {
  const h = { headers: headers(data) };
  const id = pick(data.ids);
  ok(http.get(`${API}/tenant/conversations/${id}`, { ...h, tags: { name: "conversation.get" } }), "conversation.get");
  ok(http.get(`${API}/tenant/conversations/${id}/messages`, { ...h, tags: { name: "conversation.messages" } }), "conversation.messages");
  ok(http.post(`${API}/tenant/conversations/${id}/read`, null, { ...h, tags: { name: "conversation.read" } }), "conversation.read");
  sleep(2 + Math.random() * 3);
}
