"use client";

import { createOpenAPIPage } from "fumadocs-openapi/ui";

// Documentation only: browsers keep using the same-origin application BFF.
export const OpenAPIPage = createOpenAPIPage({
  playground: { enabled: false },
});
