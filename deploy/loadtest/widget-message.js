// Website visitors: bootstrap the widget, open a conversation, send a follow-up and poll messages.
// Every iteration creates a conversation in the demo organization (refresh it with seed-demo).
import http from "k6/http";
import { fail, sleep } from "k6";
import { API, ok, options as base } from "./lib.js";

const WIDGET = __ENV.WIDGET_ID || "";
const ORIGIN = __ENV.WIDGET_ORIGIN || "";

export const options = base({
  "http_req_duration{name:widget.bootstrap}": ["p(95)<400"],
  "http_req_duration{name:widget.send}": ["p(95)<700"],
  "http_req_duration{name:widget.messages}": ["p(95)<400"],
});

export function setup() {
  if (!WIDGET) fail("WIDGET_ID is required (printed by seed-demo as 'widget public id')");
}

export default function () {
  const base = `${API}/widget/${WIDGET}`;
  const h = { "Content-Type": "application/json" };
  if (ORIGIN) h.Origin = ORIGIN;
  const boot = http.post(`${base}/bootstrap`, "{}", { headers: h, tags: { name: "widget.bootstrap" } });
  if (!ok(boot, "widget.bootstrap")) return;
  const auth = { ...h, Authorization: `Bearer ${boot.json("data.visitor.token")}` };
  const first = http.post(
    `${base}/conversations`,
    JSON.stringify({ text: "Merhaba, fiyat teklifi almak istiyorum (k6)", name: `k6 ziyaretçi ${__VU}`, email: `k6-${__VU}-${__ITER}@example.com` }),
    { headers: auth, tags: { name: "widget.send" } },
  );
  if (!ok(first, "widget.send")) return;
  const conv = first.json("data.conversation_uuid");
  sleep(1);
  ok(http.post(`${base}/conversations/${conv}/messages`, JSON.stringify({ text: "Ek bilgi: 25 kullanıcı" }), { headers: auth, tags: { name: "widget.send" } }), "widget.send");
  for (let i = 0; i < 3; i++) {
    sleep(2);
    ok(http.get(`${base}/conversations/${conv}/messages`, { headers: auth, tags: { name: "widget.messages" } }), "widget.messages");
  }
}
