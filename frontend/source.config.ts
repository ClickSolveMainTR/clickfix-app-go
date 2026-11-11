import { defineConfig, defineDocs } from "fumadocs-mdx/config";

import lastModified from "./content/docs/last-modified.json";

/**
 * Customer docs. "Last updated" comes from the committed snapshot of git
 * dates (`pnpm docs:generate` refreshes it from a full clone; the Docker
 * build context has no `.git`, and CI clones shallowly, so git is not asked
 * at build time). The processed Markdown of every page is kept for
 * `/docs/<slug>.md`, `llms-full.txt` and the copy-as-Markdown action.
 */
export const docs = defineDocs({
  dir: "content/docs",
  docs: {
    lastModified: async (file) => {
      const key = file.replace(/\\/g, "/").replace(/^.*?content\/docs\//, "");
      const date = (lastModified as Record<string, string>)[key];
      return date ? new Date(date) : null;
    },
    postprocess: { includeProcessedMarkdown: true },
  },
});
export default defineConfig();
