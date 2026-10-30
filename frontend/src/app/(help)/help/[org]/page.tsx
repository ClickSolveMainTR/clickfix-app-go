import { MessagesSquare } from "lucide-react";
import type { Metadata } from "next";
import Link from "next/link";

import { HelpShell } from "@/features/help-center/components/help-shell";
import {
  ArticleList,
  CollectionGrid,
  JsonLd,
} from "@/features/help-center/components/help-parts";
import {
  helpApi,
  resolveHelpLocale,
  siteOrigin,
  type HelpSearchParams,
} from "@/features/help-center/lib/api";
import { orNotFound } from "@/features/help-center/lib/load";
import { helpMetadata, localized } from "@/features/help-center/lib/seo";
import { routes } from "@/config/routes";
import { translate } from "@/lib/i18n/messages";

type Props = {
  params: Promise<{ org: string }>;
  searchParams: HelpSearchParams;
};

export async function generateMetadata({
  params,
  searchParams,
}: Props): Promise<Metadata> {
  const [{ org }, locale] = await Promise.all([
    params,
    resolveHelpLocale(searchParams),
  ]);
  const home = await orNotFound(helpApi.home(org, locale));
  return helpMetadata({
    org: home.organization,
    locale,
    path: routes.help.home(home.organization.slug),
    title: translate(locale, "help_center.title"),
    description: translate(locale, "help_center.meta_description", {
      name: home.organization.name,
    }),
  });
}

/** Public help center home (CF-74): search, collections, latest articles. */
export default async function HelpHomePage({ params, searchParams }: Props) {
  const [{ org }, locale] = await Promise.all([
    params,
    resolveHelpLocale(searchParams),
  ]);
  const [home, forumCategories] = await Promise.all([
    orNotFound(helpApi.home(org, locale)),
    // The forum is optional: a failure must not break the help center.
    helpApi.forumCategories(org).catch(() => []),
  ]);
  const slug = home.organization.slug;
  const tc = (key: string) => translate(locale, `help_center.${key}`);
  const base = siteOrigin();
  const topLevel = home.collections.filter((c) => c.parent_slug == null);

  return (
    <HelpShell
      org={home.organization}
      locale={locale}
      path={routes.help.home(slug)}
      hero
    >
      <JsonLd
        data={{
          "@context": "https://schema.org",
          "@type": "WebSite",
          name: `${home.organization.name} · ${tc("title")}`,
          url: `${base}${localized(routes.help.home(slug), locale)}`,
          inLanguage: locale,
          potentialAction: {
            "@type": "SearchAction",
            target: `${base}/help/${encodeURIComponent(slug)}/search?q={search_term_string}${locale === "tr" ? "" : "&lang=en"}`,
            "query-input": "required name=search_term_string",
          },
        }}
      />
      {topLevel.length > 0 && (
        <section aria-labelledby="help-collections" className="mb-12">
          <h2
            id="help-collections"
            className="font-display mb-4 text-xl font-semibold tracking-tight"
          >
            {tc("collections")}
          </h2>
          <CollectionGrid org={slug} collections={topLevel} locale={locale} />
        </section>
      )}
      {forumCategories.length > 0 && (
        <section
          aria-labelledby="help-forum"
          className="border-border bg-card mb-12 flex flex-col items-start gap-3 rounded-xl border p-5 sm:flex-row sm:items-center sm:justify-between"
        >
          <div>
            <h2 id="help-forum" className="font-display text-lg font-semibold">
              {translate(locale, "forum.public.title")}
            </h2>
            <p className="text-muted-foreground text-sm">
              {translate(locale, "forum.public.home_teaser")}
            </p>
          </div>
          <Link
            href={routes.help.forum(slug, locale)}
            className="bg-primary text-primary-foreground inline-flex h-9 shrink-0 items-center gap-2 rounded-md px-4 text-sm font-medium outline-none hover:opacity-90 focus-visible:ring-2"
          >
            <MessagesSquare aria-hidden className="size-4" />
            {translate(locale, "forum.public.home_cta")}
          </Link>
        </section>
      )}
      {home.recent.length > 0 ? (
        <section aria-labelledby="help-recent">
          <h2
            id="help-recent"
            className="font-display mb-4 text-xl font-semibold tracking-tight"
          >
            {tc("recent")}
          </h2>
          <ArticleList org={slug} articles={home.recent} locale={locale} />
        </section>
      ) : (
        <p className="text-muted-foreground">{tc("empty_locale")}</p>
      )}
    </HelpShell>
  );
}
