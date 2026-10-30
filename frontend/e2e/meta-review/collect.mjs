#!/usr/bin/env node
// Gathers the recordings under out/raw into out/<use-case>.{webm,vtt,mp4}. The mp4 (H.264 with the
// captions as a soft subtitle track) needs a full ffmpeg on PATH; without it only webm + vtt are kept.
import { execFileSync } from "node:child_process";
import {
  copyFileSync,
  existsSync,
  readdirSync,
  readFileSync,
  statSync,
} from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const raw = join(here, "out", "raw");
const out = join(here, "out");
const expected = (process.env.REVIEW_ONLY || "messenger,instagram,whatsapp")
  .split(",")
  .map((s) => s.trim());

function manifests(dir) {
  if (!existsSync(dir)) return [];
  return readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) return manifests(full);
    return entry === "meta-review.json"
      ? [{ dir, ...JSON.parse(readFileSync(full, "utf8")) }]
      : [];
  });
}

function hasFfmpeg() {
  try {
    execFileSync("ffmpeg", ["-version"], { stdio: "ignore" });
    return true;
  } catch {
    return false;
  }
}

const found = manifests(raw);
const ffmpeg = hasFfmpeg();
let missing = 0;
for (const id of expected) {
  const m = found.find((item) => item.id === id);
  const webm = m && join(m.dir, "video.webm");
  if (!webm || !existsSync(webm)) {
    console.log(`MISSING  ${id}`);
    missing++;
    continue;
  }
  copyFileSync(webm, join(out, `${id}.webm`));
  copyFileSync(join(m.dir, "captions.vtt"), join(out, `${id}.vtt`));
  let mp4 = "";
  if (ffmpeg) {
    mp4 = join(out, `${id}.mp4`);
    execFileSync(
      "ffmpeg",
      [
        "-y",
        "-loglevel",
        "error",
        "-i",
        webm,
        "-i",
        join(out, `${id}.vtt`),
        "-c:v",
        "libx264",
        "-pix_fmt",
        "yuv420p",
        "-c:s",
        "mov_text",
        "-metadata:s:s:0",
        "language=eng",
        "-movflags",
        "+faststart",
        mp4,
      ],
      { stdio: "inherit" },
    );
  }
  console.log(
    `OK       ${id.padEnd(10)} ${m.permissions.join(", ")}  ${mp4 || "(mp4 skipped: no ffmpeg)"}`,
  );
}
process.exit(missing ? 1 : 0);
