import { createFromSource } from "fumadocs-core/search/server";

import { source } from "@/lib/docs/source";

export const dynamic = "force-static";
export const { staticGET: GET } = createFromSource(source);
