import type { Metadata } from "next";

import { routes } from "@/config/routes";
import { HelpShell } from "@/features/help-center/components/help-shell";
import {
  ArticleList,
  CollectionGrid,
  HelpBreadcrumbs,
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
import { translate } from "@/lib/i18n/messages";

type Props = {
  params: Promise<{ org: string; collection: string }>;
  searchParams: HelpSearchParams;
};

async function load({ params, searchParams }: Props) {
  const [{ org, collection }, locale] = await Promise.all([
    params,
    resolveHelpLocale(searchParams),
  ]);
  const page = await orNotFound(helpApi.collection(org, collection, locale));
  return { page, locale };
}

export async function generateMetadata(props: Props): Promise<Metadata> {
  const { page, locale } = await load(props);
  return helpMetadata({
    org: page.organization,
    locale,
    path: routes.help.collection(page.organization.slug, page.collection.slug),
    title: page.collection.name,
    description: translate(locale, "help_center.collection_description", {
      name: page.collection.name,
      count: page.total,
    }),
  });
}

/** Collection page: its categories and articles (CF-74). */
export default async function HelpCollectionPage(props: Props) {
  const { page, locale } = await load(props);
  const org = page.organization;
  const tc = (key: string, params?: Record<string, string | number>) =>
    translate(locale, `help_center.${key}`, params);
  const base = siteOrigin();
  const crumbs = [
    { label: tc("title"), href: routes.help.home(org.slug, locale) },
    ...(page.parent_collection
      ? [
          {
            label: page.parent_collection.name,
            href: routes.help.collection(
              org.slug,
              page.parent_collection.slug,
              locale,
            ),
          },
        ]
      : []),
    { label: page.collection.name },
  ];
  const abs = (path: string) => `${base}${path}`;

  return (
    <HelpShell
      org={org}
      locale={locale}
      path={routes.help.collection(org.slug, page.collection.slug)}
    >
      <JsonLd
        data={[
          {
            "@context": "https://schema.org",
            "@type": "BreadcrumbList",
            itemListElement: crumbs.map((c, i) => ({
              "@type": "ListItem",
              position: i + 1,
              name: c.label,
              ...(c.href
                ? { item: abs(c.href) }
                : {
                    item: abs(
                      localized(
                        routes.help.collection(org.slug, page.collection.slug),
                        locale,
                      ),
                    ),
                  }),
            })),
          },
          ...(page.articles.length > 0
            ? [
                {
                  "@context": "https://schema.org",
                  "@type": "FAQPage",
                  inLanguage: locale,
                  mainEntity: page.articles.map((a) => ({
                    "@type": "Question",
                    name: a.title,
                    acceptedAnswer: {
                      "@type": "Answer",
                      text: a.excerpt,
                      url: abs(routes.help.article(org.slug, a.slug, a.locale)),
                    },
                  })),
                },
              ]
            : []),
        ]}
      />
      <HelpBreadcrumbs items={crumbs} locale={locale} />
      <h1 className="font-display mb-2 text-3xl font-semibold tracking-tight">
        {page.collection.name}
      </h1>
      <p className="text-muted-foreground mb-8">
        {tc("article_count", { count: page.total })}
      </p>
      {page.children.length > 0 && (
        <section aria-labelledby="help-categories" className="mb-10">
          <h2 id="help-categories" className="sr-only">
            {tc("categories")}
          </h2>
          <CollectionGrid
            org={org.slug}
            collections={page.children}
            locale={locale}
          />
        </section>
      )}
      {page.articles.length > 0 && (
        <section aria-labelledby="help-articles">
          <h2
            id="help-articles"
            className="font-display mb-4 text-xl font-semibold tracking-tight"
          >
            {tc("articles")}
          </h2>
          <ArticleList
            org={org.slug}
            articles={page.articles}
            locale={locale}
          />
        </section>
      )}
    </HelpShell>
  );
}
