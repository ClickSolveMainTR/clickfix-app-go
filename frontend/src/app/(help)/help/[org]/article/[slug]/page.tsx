import type { Metadata } from "next";

import { MarkdownContent } from "@/components/markdown/markdown-content";
import { routes } from "@/config/routes";
import { ArticleFeedback } from "@/features/help-center/components/article-feedback";
import { HelpShell } from "@/features/help-center/components/help-shell";
import {
  ArticleList,
  HelpBreadcrumbs,
  JsonLd,
} from "@/features/help-center/components/help-parts";
import {
  helpApi,
  helpLogoSrc,
  resolveHelpLocale,
  siteOrigin,
  type HelpSearchParams,
} from "@/features/help-center/lib/api";
import { orNotFound } from "@/features/help-center/lib/load";
import { helpMetadata, localized } from "@/features/help-center/lib/seo";
import { translate } from "@/lib/i18n/messages";
import { date } from "@/lib/utils/format";

type Props = {
  params: Promise<{ org: string; slug: string }>;
  searchParams: HelpSearchParams;
};

async function load({ params, searchParams }: Props) {
  const [{ org, slug }, locale] = await Promise.all([
    params,
    resolveHelpLocale(searchParams),
  ]);
  const page = await orNotFound(helpApi.article(org, slug, locale));
  return { page, locale };
}

export async function generateMetadata(props: Props): Promise<Metadata> {
  const { page, locale } = await load(props);
  const a = page.article;
  return helpMetadata({
    org: page.organization,
    locale,
    path: routes.help.article(page.organization.slug, a.slug),
    title: a.title,
    description: a.excerpt || a.title,
    type: "article",
    publishedTime: a.created_at,
    modifiedTime: a.updated_at,
    // Translations are separate articles with their own slugs.
    alternates: false,
  });
}

/** Published article (CF-74): markdown rendered without raw HTML. */
export default async function HelpArticlePage(props: Props) {
  const { page, locale } = await load(props);
  const org = page.organization;
  const a = page.article;
  const tc = (key: string, params?: Record<string, string | number>) =>
    translate(locale, `help_center.${key}`, params);
  const base = siteOrigin();
  const url = `${base}${localized(routes.help.article(org.slug, a.slug), locale)}`;
  const crumbs = [
    { label: tc("title"), href: routes.help.home(org.slug, locale) },
    ...(a.parent_collection
      ? [
          {
            label: a.parent_collection.name,
            href: routes.help.collection(
              org.slug,
              a.parent_collection.slug,
              locale,
            ),
          },
        ]
      : []),
    ...(a.collection
      ? [
          {
            label: a.collection.name,
            href: routes.help.collection(org.slug, a.collection.slug, locale),
          },
        ]
      : []),
    { label: a.title },
  ];
  const logo = helpLogoSrc(org);

  return (
    <HelpShell
      org={org}
      locale={locale}
      path={routes.help.article(org.slug, a.slug)}
    >
      <JsonLd
        data={[
          {
            "@context": "https://schema.org",
            "@type": "Article",
            headline: a.title,
            description: a.excerpt,
            inLanguage: a.locale,
            datePublished: a.created_at,
            dateModified: a.updated_at,
            mainEntityOfPage: url,
            url,
            ...(a.tags.length ? { keywords: a.tags.join(", ") } : {}),
            author: { "@type": "Organization", name: org.name },
            publisher: {
              "@type": "Organization",
              name: org.name,
              ...(logo
                ? { logo: { "@type": "ImageObject", url: `${base}${logo}` } }
                : {}),
            },
          },
          {
            "@context": "https://schema.org",
            "@type": "BreadcrumbList",
            itemListElement: crumbs.map((c, i) => ({
              "@type": "ListItem",
              position: i + 1,
              name: c.label,
              item: c.href ? `${base}${c.href}` : url,
            })),
          },
        ]}
      />
      <div className="mx-auto max-w-3xl">
        <HelpBreadcrumbs items={crumbs} locale={locale} />
        <article>
          <header className="mb-8">
            <h1 className="font-display text-3xl font-semibold tracking-tight text-balance sm:text-4xl">
              {a.title}
            </h1>
            <p className="text-muted-foreground mt-3 text-sm">
              <time dateTime={a.updated_at}>
                {tc("updated", {
                  date: date(a.updated_at, "d MMMM yyyy", locale),
                })}
              </time>
            </p>
          </header>
          <MarkdownContent markdown={a.body} />
          {a.tags.length > 0 && (
            <ul className="mt-8 flex flex-wrap gap-2" aria-label={tc("tags")}>
              {a.tags.map((tag) => (
                <li
                  key={tag}
                  className="bg-muted text-muted-foreground rounded-full px-3 py-1 text-xs"
                >
                  {tag}
                </li>
              ))}
            </ul>
          )}
        </article>
        <ArticleFeedback
          org={org.slug}
          slug={a.slug}
          locale={locale}
          labels={{
            question: tc("feedback.question"),
            yes: tc("feedback.yes"),
            no: tc("feedback.no"),
            thanks: tc("feedback.thanks"),
            failed: tc("feedback.failed"),
          }}
        />
        {a.related.length > 0 && (
          <section aria-labelledby="help-related" className="mt-12">
            <h2
              id="help-related"
              className="font-display mb-4 text-xl font-semibold tracking-tight"
            >
              {tc("related")}
            </h2>
            <ArticleList org={org.slug} articles={a.related} locale={locale} />
          </section>
        )}
      </div>
    </HelpShell>
  );
}
