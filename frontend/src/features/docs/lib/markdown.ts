import type { AppLocale } from "@/config/i18n";
import { siteBase } from "@/features/landing/lib/seo";
import { source } from "@/lib/docs/source";

type DocsPage = ReturnType<typeof source.getPage> & object;

/** Path of the raw Markdown route of a page (`/docs/<slug>.md`, index: `/docs/index.md`). */
export function markdownPath(page: DocsPage, locale: AppLocale) {
  const slug = page.slugs.length ? page.slugs.join("/") : "index";
  return `/docs/${slug}.md${locale === "en" ? "?lang=en" : ""}`;
}

/** Canonical absolute URL of a page (English keeps its `?lang=en`). */
export function pageUrl(page: DocsPage) {
  return `${siteBase()}${page.url}`;
}

/**
 * A page as Markdown for LLMs: title, description and the processed body
 * (`includeProcessedMarkdown`, with the included package table expanded).
 * Generated API reference pages carry no Markdown body; they list their
 * operations and point at the HTML page.
 */
export async function pageMarkdown(page: DocsPage, locale: AppLocale) {
  const data = page.data as {
    title: string;
    description?: string;
    _openapi?: {
      method?: string;
      structuredData?: {
        headings?: { content: string }[];
        contents?: { content: string }[];
      };
    };
    getText?: (type: "raw" | "processed") => Promise<string>;
  };
  const head = `# ${data.title}\n\n${data.description ?? ""}\n\nURL: ${pageUrl(page)}\n`;
  if (data._openapi) {
    const structured =
      (data as { structuredData?: { contents?: { content: string }[] } })
        .structuredData ?? data._openapi.structuredData;
    const text = (structured?.contents ?? [])
      .map((c) => c.content)
      .join("\n\n");
    const note =
      locale === "tr"
        ? "Bu sayfa OpenAPI şemasından üretilen API başvurusudur; istek ve yanıt şemaları, örnekler ve curl/Node.js/Go/PHP istekleri HTML sayfasındadır. Ortak kurallar: https://getclickfix.com/docs/api.md"
        : "This page is the API reference generated from the OpenAPI schema; request and response schemas, examples and curl/Node.js/Go/PHP requests are on the HTML page. Shared rules: https://getclickfix.com/docs/api.md?lang=en";
    return `${head}\n${text}\n\n${note}\n`;
  }
  const body = data.getText ? await data.getText("processed") : "";
  return `${head}\n${body.trim()}\n`;
}
