import { createFromSource } from "fumadocs-core/search/server";

import { source } from "@/lib/docs/source";

export const dynamic = "force-static";

/**
 * Static search index per language. Turkish pages are tokenized with the
 * Turkish splitter (keeps ç, ğ, ı, ö, ş, ü whole), English with the
 * English one (CF-208).
 */
export const { staticGET: GET } = createFromSource(source, {
  localeMap: { tr: "turkish", en: "english" },
});
