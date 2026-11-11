import { createOpenAPI } from "fumadocs-openapi/server";

/**
 * Both language copies of the public API spec (`pnpm docs:sync`). The
 * playground calls the API through the same-origin proxy route, which only
 * forwards to api.getclickfix.com.
 */
export const openapi = createOpenAPI({
  input: ["./content/api/public-api.yaml", "./content/api/public-api.tr.yaml"],
  proxyUrl: "/docs/api-proxy",
});
