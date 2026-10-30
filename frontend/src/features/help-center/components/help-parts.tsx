import { ChevronRight, FileText, FolderOpen } from "lucide-react";
import Link from "next/link";

import { routes } from "@/config/routes";
import { translate } from "@/lib/i18n/messages";
import { date } from "@/lib/utils/format";

import type {
  HelpArticleSummary,
  HelpCollection,
  HelpLocale,
} from "../lib/api";

/** Inline JSON-LD; `<` is escaped so article text cannot close the script. */
export function JsonLd({ data }: { data: unknown }) {
  return (
    <script
      type="application/ld+json"
      dangerouslySetInnerHTML={{
        __html: JSON.stringify(data).replace(/</g, "\\u003c"),
      }}
    />
  );
}

export type Crumb = { label: string; href?: string };

export function HelpBreadcrumbs({
  items,
  locale,
}: {
  items: Crumb[];
  locale: HelpLocale;
}) {
  return (
    <nav
      aria-label={translate(locale, "help_center.breadcrumb")}
      className="text-muted-foreground mb-6 text-sm"
    >
      <ol className="flex flex-wrap items-center gap-1">
        {items.map((item, i) => (
          <li key={`${item.label}-${i}`} className="flex items-center gap-1">
            {i > 0 && <ChevronRight aria-hidden className="size-3.5" />}
            {item.href ? (
              <Link
                href={item.href}
                className="hover:text-foreground rounded-sm underline-offset-4 outline-none hover:underline focus-visible:ring-2"
              >
                {item.label}
              </Link>
            ) : (
              <span aria-current="page" className="text-foreground">
                {item.label}
              </span>
            )}
          </li>
        ))}
      </ol>
    </nav>
  );
}

export function ArticleList({
  org,
  articles,
  locale,
}: {
  org: string;
  articles: HelpArticleSummary[];
  locale: HelpLocale;
}) {
  return (
    <ul className="divide-border border-border bg-card divide-y overflow-hidden rounded-xl border">
      {articles.map((a) => (
        <li key={a.uuid}>
          <Link
            href={routes.help.article(org, a.slug, a.locale)}
            className="hover:bg-muted/50 focus-visible:bg-muted/50 group flex gap-3 px-4 py-4 outline-none sm:px-5"
          >
            <FileText
              aria-hidden
              className="text-muted-foreground mt-0.5 size-4 shrink-0"
            />
            <span className="min-w-0 flex-1">
              <span className="block font-medium group-hover:underline group-hover:underline-offset-4">
                {a.title}
              </span>
              {a.excerpt && (
                <span className="text-muted-foreground mt-1 line-clamp-2 block text-sm">
                  {a.excerpt}
                </span>
              )}
              <span className="text-muted-foreground mt-1 block text-xs">
                {translate(locale, "help_center.updated", {
                  date: date(a.updated_at, "d MMMM yyyy", locale),
                })}
              </span>
            </span>
          </Link>
        </li>
      ))}
    </ul>
  );
}

export function CollectionGrid({
  org,
  collections,
  locale,
}: {
  org: string;
  collections: HelpCollection[];
  locale: HelpLocale;
}) {
  return (
    <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {collections.map((c) => (
        <li key={c.uuid}>
          <Link
            href={routes.help.collection(org, c.slug, locale)}
            className="border-border bg-card hover:border-foreground/30 focus-visible:ring-ring flex h-full flex-col gap-3 rounded-xl border p-5 transition-colors outline-none focus-visible:ring-2"
          >
            <span className="bg-signal-soft flex size-9 items-center justify-center rounded-lg">
              <FolderOpen aria-hidden className="size-4" />
            </span>
            <span className="font-display text-base font-semibold tracking-tight">
              {c.name}
            </span>
            <span className="text-muted-foreground mt-auto text-sm">
              {translate(locale, "help_center.article_count", {
                count: c.article_count,
              })}
            </span>
          </Link>
        </li>
      ))}
    </ul>
  );
}
