// Agent replies. Written as INTERNAL NOTES so nothing is delivered to a customer channel; the
// write path (message insert, conversation update, outbox, realtime) is the same as a reply.
import http from "k6/http";
import { sleep } from "k6";
import { API, conversationIds, headers, login, ok, options as base, pick } from "./lib.js";

export const options = base({ "http_req_duration{name:conversation.reply}": ["p(95)<800"] });

export function setup() {
  const data = login();
  return { ...data, ids: conversationIds(data) };
}

export default function (data) {
  const body = JSON.stringify({ content: `k6 yük testi notu ${__VU}-${__ITER}`, private: true });
  const res = http.post(`${API}/tenant/conversations/${pick(data.ids)}/messages`, body, {
    headers: headers(data),
    tags: { name: "conversation.reply" },
  });
  ok(res, "conversation.reply");
  sleep(3 + Math.random() * 4);
}
