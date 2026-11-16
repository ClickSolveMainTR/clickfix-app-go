#!/usr/bin/env node
// Go sidecar'ı Tauri bundle.externalBin adlandırmasıyla derler:
//   apps/<app>/src-tauri/binaries/clickfix-sidecar-<target-triple>[.exe]
//
// Kullanım: node scripts/build-sidecar.mjs [target-triple]
// Hedef verilmezse TAURI_ENV_TARGET_TRIPLE (tauri CLI'nin before* komutlarına
// verdiği değer), o da yoksa `rustc -vV` host'u kullanılır. Node ile yazıldı ki
// macOS, Linux ve Windows'ta aynı betik çalışsın.
import { execFileSync } from "node:child_process";
import { copyFileSync, mkdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const apps = ["agent", "support"];

const triple =
  process.argv[2] ||
  process.env.TAURI_ENV_TARGET_TRIPLE ||
  /^host: (.+)$/m.exec(execFileSync("rustc", ["-vV"], { encoding: "utf8" }))?.[1];
if (!triple) {
  console.error("hedef üçlüsü bulunamadı");
  process.exit(1);
}

const arch = { x86_64: "amd64", aarch64: "arm64" }[triple.split("-")[0]];
let os;
if (triple.endsWith("-apple-darwin")) os = "darwin";
else if (triple.includes("-windows-")) os = "windows";
else if (triple.includes("-linux-")) os = "linux";
if (!arch || !os) {
  console.error(`desteklenmeyen hedef: ${triple}`);
  process.exit(1);
}

const version = readFileSync(join(root, "VERSION"), "utf8").trim();
const name = `clickfix-sidecar-${triple}${os === "windows" ? ".exe" : ""}`;
const [first, ...rest] = apps.map((app) => join(root, "apps", app, "src-tauri", "binaries", name));

mkdirSync(dirname(first), { recursive: true });
execFileSync(
  "go",
  ["build", "-trimpath", "-ldflags", `-s -w -X main.version=${version}`, "-o", first, "./cmd/clickfix-sidecar"],
  {
    cwd: join(root, "sidecar"),
    stdio: "inherit",
    env: { ...process.env, CGO_ENABLED: "0", GOOS: os, GOARCH: arch },
  },
);
console.log(`sidecar: ${first}`);
for (const out of rest) {
  mkdirSync(dirname(out), { recursive: true });
  copyFileSync(first, out);
  console.log(`sidecar: ${out}`);
}
