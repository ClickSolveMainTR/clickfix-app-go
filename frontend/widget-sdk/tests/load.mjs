// Bundles a TypeScript module of the SDK for node:test (no DOM needed by
// the shared modules).
import { build } from "esbuild";

export async function load(path) {
  const out = await build({
    entryPoints: [new URL(path, import.meta.url).pathname],
    bundle: true,
    format: "esm",
    platform: "neutral",
    write: false,
    logLevel: "silent",
  });
  const code = out.outputFiles[0].text;
  return import(
    `data:text/javascript;base64,${Buffer.from(code).toString("base64")}`
  );
}

export const fixture = (name) =>
  new URL(
    `../../../backend/internal/platform/channels/widget/testdata/${name}`,
    import.meta.url,
  );
