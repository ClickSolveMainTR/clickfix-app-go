import { createOpenAPI } from "fumadocs-openapi/server";
import { generateFiles } from "fumadocs-openapi";
import { rm } from "node:fs/promises";

const output = "./content/docs/tr/api-reference";
// The schema is a private build input. It is never copied to public/ or
// exposed by a route. Each generated page describes one operation group.
const openapi = createOpenAPI({ input: ["./content/api/public-api.yaml"] });
await rm(output, { recursive: true, force: true });
const slugs = {
  Konuşmalar: "conversations",
  Kişiler: "contacts",
  Şirketler: "companies",
  Tetikleyiciler: "triggers",
  "Webhook'lar": "webhooks",
  "API kanalı": "api-channel",
};
await generateFiles({
  input: openapi,
  output,
  per: "tag",
  slugify(name) {
    if (!slugs[name]) throw new Error(`Missing English API slug: ${name}`);
    return slugs[name];
  },
  meta: true,
});
