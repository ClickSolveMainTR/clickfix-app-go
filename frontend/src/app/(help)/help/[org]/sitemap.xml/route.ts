import { routes } from "@/config/routes";
import {
  helpApi,
  HelpNotFoundError,
  siteOrigin,
} from "@/features/help-center/lib/api";

type RouteContext = { params: Promise<{ org: string }> };

function xml(value: string) {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

/** Sitemap cap for forum topics (pages of 100 from the API). */
const FORUM_SITEMAP_MAX = 2000;

/**
 * Published forum topics (CF-78). Optional: a forum failure leaves the
 * article sitemap intact.
 */
async function forumTopics(org: string) {
  const out: { slug: string; updated_at: string }[] = [];
  try {
    for (let offset = 0; offset < FORUM_SITEMAP_MAX; offset += 100) {
      const page = await helpApi.forumSitemap(org, 100, offset);
      out.push(...page.items);
      if (offset + 100 >= page.total || page.items.length === 0) break;
    }
  } catch {
    // Keep what was collected.
  }
  return out;
}

/**
 * Help center sitemap of one organization (CF-74): home per language,
 * collections and published public articles.
 */
export async function GET(_request: Request, context: RouteContext) {
  const { org } = await context.params;
  let data;
  try {
    data = await helpApi.sitemap(org);
  } catch (err) {
    if (err instanceof HelpNotFoundError) {
      return new Response("Not found", { status: 404 });
    }
    throw err;
  }
  const base = siteOrigin();
  const slug = data.organization.slug;
  const forum = await forumTopics(org);
  const urls: { loc: string; lastmod?: string; priority: string }[] = [
    ...data.organization.locales.map((l) => ({
      loc: base + routes.help.home(slug, l.locale),
      priority: "1.0",
    })),
    ...data.collections.map((c) => ({
      loc: base + routes.help.collection(slug, c.slug, c.locale),
      lastmod: c.updated_at,
      priority: "0.7",
    })),
    ...data.articles.map((a) => ({
      loc: base + routes.help.article(slug, a.slug, a.locale),
      lastmod: a.updated_at,
      priority: "0.8",
    })),
    ...(forum.length > 0
      ? [{ loc: base + routes.help.forum(slug), priority: "0.6" }]
      : []),
    ...forum.map((t) => ({
      loc: base + routes.help.forumTopic(slug, t.slug),
      lastmod: t.updated_at,
      priority: "0.5",
    })),
  ];
  const body = [
    '<?xml version="1.0" encoding="UTF-8"?>',
    '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">',
    ...urls.map(
      (u) =>
        `<url><loc>${xml(u.loc)}</loc>${
          u.lastmod && !u.lastmod.startsWith("0001")
            ? `<lastmod>${xml(u.lastmod)}</lastmod>`
            : ""
        }<priority>${u.priority}</priority></url>`,
    ),
    "</urlset>",
  ].join("\n");
  return new Response(body, {
    headers: {
      "Content-Type": "application/xml; charset=utf-8",
      "Cache-Control": "public, max-age=3600",
    },
  });
}
