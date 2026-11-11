"use client";

import { createOpenAPIPage } from "fumadocs-openapi/ui";

/**
 * API reference page. The playground sends requests through the docs proxy
 * (`/docs/api-proxy`), never from the browser to the Go host directly; the
 * reader pastes their own `cf_sk_…` key into the Authorization field.
 */
export const OpenAPIPage = createOpenAPIPage({
  playground: { enabled: true },
});
