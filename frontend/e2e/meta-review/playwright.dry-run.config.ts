import { defineConfig } from "@playwright/test";

/** Dry run: checks every composition (captions, permission coverage, length) without a browser or servers. */
export default defineConfig({
  testDir: ".",
  testMatch: "**/dry-run.check.ts",
  outputDir: "./out/dry-run",
  workers: 1,
  reporter: [["list"]],
});
