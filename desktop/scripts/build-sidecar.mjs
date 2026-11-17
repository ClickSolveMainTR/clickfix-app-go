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

// macOS'ta ekran yakalama (ScreenCaptureKit + VideoToolbox) ve girdi
// (CGEvent) cgo ister; derleme yalnız macOS'ta, Xcode komut satırı
// araçlarıyla yapılır. Öbür hedefler saf Go'dur (CGO_ENABLED=0) ve yakalama
// orada "desteklenmiyor" döner.
const env = { ...process.env, GOOS: os, GOARCH: arch };
if (os === "darwin") {
  if (process.platform !== "darwin") {
    console.error("macOS sidecar'ı yalnız macOS'ta derlenir (cgo, ScreenCaptureKit)");
    process.exit(1);
  }
  // ScreenCaptureKit macOS 12.3 ister.
  const minOS = process.env.MACOSX_DEPLOYMENT_TARGET || "12.3";
  const clangArch = { amd64: "x86_64", arm64: "arm64" }[arch];
  const flags = `-arch ${clangArch} -mmacosx-version-min=${minOS}`;
  Object.assign(env, {
    CGO_ENABLED: "1",
    CC: process.env.CC || "clang",
    MACOSX_DEPLOYMENT_TARGET: minOS,
    CGO_CFLAGS: `${process.env.CGO_CFLAGS || "-O2"} ${flags}`,
    CGO_LDFLAGS: `${process.env.CGO_LDFLAGS || ""} ${flags}`.trim(),
  });
} else {
  env.CGO_ENABLED = "0";
}

mkdirSync(dirname(first), { recursive: true });
execFileSync(
  "go",
  ["build", "-trimpath", "-ldflags", `-s -w -X main.version=${version}`, "-o", first, "./cmd/clickfix-sidecar"],
  { cwd: join(root, "sidecar"), stdio: "inherit", env },
);
console.log(`sidecar: ${first}`);
for (const out of rest) {
  mkdirSync(dirname(out), { recursive: true });
  copyFileSync(first, out);
  console.log(`sidecar: ${out}`);
}
