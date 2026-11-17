#!/usr/bin/env node
// desktop-release iş akışının artefaktlarını API'ye yayınlar (CF-240).
//
// Kullanım:
//   CLICKFIX_API_URL=https://api.getclickfix.com \
//   CLICKFIX_EMAIL=... CLICKFIX_PASSWORD=... [CLICKFIX_TOTP=123456] \
//   node scripts/publish-release.mjs <artefakt-dizini>... [--notes "Sürüm notu"] [--allow-unsigned]
//
// Her dizin bir manifest.json (iş akışının yazdığı), updater paketi, .sig
// ve varsa .dmg içerir. Betik SHA256SUMS'u yerelde doğrular, giriş yapar
// (platform.desktop_releases.manage yetkisi gerekir) ve her hedefi
// POST /v1/platform/desktop/releases ile yükler. CLICKFIX_ACCESS_TOKEN
// verilirse giriş yapılmaz. İmzasız iç test paketleri --allow-unsigned
// olmadan reddedilir. Hiçbir gizli değer diske yazılmaz.
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const args = process.argv.slice(2);
const dirs = [];
let notes = "";
let allowUnsigned = false;
for (let i = 0; i < args.length; i++) {
  if (args[i] === "--notes") notes = args[++i] ?? "";
  else if (args[i] === "--allow-unsigned") allowUnsigned = true;
  else dirs.push(args[i]);
}
if (dirs.length === 0) {
  console.error("kullanım: publish-release.mjs <artefakt-dizini>... [--notes ...] [--allow-unsigned]");
  process.exit(2);
}
const api = (process.env.CLICKFIX_API_URL || "").replace(/\/+$/, "");
if (!/^https:\/\/|^http:\/\/(localhost|127\.0\.0\.1)(:\d+)?$/.test(api)) {
  console.error("CLICKFIX_API_URL https olmalı (yerelde http://localhost:8080 olabilir)");
  process.exit(2);
}

function sha256(path) {
  return createHash("sha256").update(readFileSync(path)).digest("hex");
}

function load(dir) {
  const manifest = JSON.parse(readFileSync(join(dir, "manifest.json"), "utf8"));
  const sums = Object.fromEntries(
    readFileSync(join(dir, "SHA256SUMS"), "utf8")
      .trim()
      .split("\n")
      .map((line) => line.trim().split(/\s+\*?/))
      .map(([sum, name]) => [name, sum]),
  );
  for (const name of [manifest.updater, manifest.installer].filter(Boolean)) {
    if (sums[name] !== sha256(join(dir, name))) {
      throw new Error(`${dir}: ${name} SHA-256 tutmuyor`);
    }
  }
  if (!manifest.os_signed && !allowUnsigned) {
    throw new Error(`${dir}: imzasız iç test paketi; yalnız --allow-unsigned ile yayınlanır`);
  }
  return { dir, manifest };
}

async function token() {
  if (process.env.CLICKFIX_ACCESS_TOKEN) return process.env.CLICKFIX_ACCESS_TOKEN;
  const res = await fetch(`${api}/v1/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      email: process.env.CLICKFIX_EMAIL,
      password: process.env.CLICKFIX_PASSWORD,
      totp_code: process.env.CLICKFIX_TOTP || undefined,
    }),
  });
  const json = await res.json().catch(() => ({}));
  if (!res.ok || !json.data?.access_token) {
    throw new Error(`giriş başarısız: ${res.status} ${json.error?.code ?? ""}`);
  }
  return json.data.access_token;
}

async function publish(bearer, { dir, manifest }) {
  const form = new FormData();
  for (const [k, v] of Object.entries({
    app: manifest.app,
    channel: manifest.channel,
    version: manifest.version,
    target: manifest.target,
    notes,
    signature: manifest.signature,
    os_signed: String(manifest.os_signed),
  })) {
    form.append(k, v);
  }
  form.append("updater", new Blob([readFileSync(join(dir, manifest.updater))]), manifest.updater);
  if (manifest.installer) {
    form.append("installer", new Blob([readFileSync(join(dir, manifest.installer))]), manifest.installer);
  }
  const res = await fetch(`${api}/v1/platform/desktop/releases`, {
    method: "POST",
    headers: { Authorization: `Bearer ${bearer}` },
    body: form,
  });
  const json = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(`${manifest.app} ${manifest.target} ${manifest.version}: ${res.status} ${json.error?.code ?? ""} ${json.error?.message ?? ""}`);
  }
  console.log(`yayınlandı: ${manifest.app} ${manifest.channel} ${manifest.version} ${manifest.target} (${json.data.uuid})`);
}

try {
  // Önce hepsi doğrulanır; biri bozuksa hiçbiri yüklenmez.
  const releases = dirs.map(load);
  const bearer = await token();
  for (const release of releases) await publish(bearer, release);
} catch (err) {
  console.error(err instanceof Error ? err.message : err);
  process.exit(1);
}
