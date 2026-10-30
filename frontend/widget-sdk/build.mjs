// Bundles the widget SDK (public/widget.js) and the lazily loaded session
// recorder (public/widget-recorder.js). Both are IIFEs without runtime
// dependencies on the host page.
import { build } from "esbuild";

const common = {
  bundle: true,
  format: "iife",
  minify: true,
  target: "es2018",
  legalComments: "none",
};

await build({
  ...common,
  entryPoints: ["widget-sdk/src/widget.ts"],
  outfile: "public/widget.js",
  metafile: false,
});
await build({
  ...common,
  entryPoints: ["widget-sdk/src/recorder.ts"],
  globalName: "ClickFixRecorder",
  outfile: "public/widget-recorder.js",
});
