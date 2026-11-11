import { notFound } from "next/navigation";

import { pageMarkdown } from "@/features/docs/lib/markdown";
import { source } from "@/lib/docs/source";

export const dynamic = "force-static";
export const dynamicParams = false;

/**
 * Raw Markdown of a docs page. Public URL is `/docs/<slug>.md` (and
 * `?lang=en`, or `/en/docs/<slug>.md`); the proxy and next.config rewrite
 * it to `/llms.mdx/<locale>/<slug>` (`_index` for the docs home).
 */
export function generateStaticParams() {
  return (["tr", "en"] as const).flatMap((locale) =>
    source.getPages(locale).map((page) => ({
      locale,
      slug: page.slugs.length ? page.slugs : ["_index"],
    })),
  );
}

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ locale: string; slug?: string[] }> },
) {
  const { locale, slug = [] } = await params;
  const lang = locale === "en" ? "en" : "tr";
  // "_index" stands for the docs home; a segment literally named "index"
  // is not routable in Next.
  const slugs = slug[0] === "_index" && slug.length === 1 ? [] : slug;
  const page = source.getPage(slugs, lang);
  if (!page) notFound();
  return new Response(await pageMarkdown(page, lang), {
    headers: { "content-type": "text/markdown; charset=utf-8" },
  });
}
