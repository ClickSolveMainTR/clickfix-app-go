// Cmd+K search (Meilisearch behind /v1/tenant/search). Run search-reindex after seed-demo.
import http from "k6/http";
import { sleep } from "k6";
import { API, headers, login, ok, options as base, pick } from "./lib.js";

export const options = base({ "http_req_duration{name:search}": ["p(95)<400"] });

export const setup = login;

const terms = ["fatura", "Kuzey", "webhook", "giriş", "Ege Gıda", "iade", "rezervasyon", "SMS", "Ahmet"];

export default function (data) {
  const q = encodeURIComponent(pick(terms));
  ok(http.get(`${API}/tenant/search?q=${q}&limit=8`, { headers: headers(data), tags: { name: "search" } }), "search");
  sleep(1 + Math.random() * 2);
}
