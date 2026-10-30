import type { Metadata } from "next";

import { routes } from "@/config/routes";
import { HelpShell } from "@/features/help-center/components/help-shell";
import {
  ArticleList,
  HelpBreadcrumbs,
} from "@/features/help-center/components/help-parts";
import {
  helpApi,
  resolveHelpLocale,
  type HelpSearchParams,
} from "@/features/help-center/lib/api";
import { orNotFound } from "@/features/help-center/lib/load";
import { helpMetadata } from "@/features/help-center/lib/seo";
import { translate } from "@/lib/i18n/messages";

type Props = {
  params: Promise<{ org: string }>;
  searchParams: HelpSearchParams;
};

async function load({ params, searchParams }: Props) {
  const [{ org }, locale, sp] = await Promise.all([
    params,
    resolveHelpLocale(searchParams),
    searchParams,
  ]);
  const raw = Array.isArray(sp.q) ? sp.q[0] : sp.q;
  const q = (raw ?? "").trim().slice(0, 200);
  // Without a query the page still needs the branding of the help center.
  const result = q
    ? await orNotFound(helpApi.search(org, q, locale))
    : {
        ...(await orNotFound(helpApi.home(org, locale))),
        query: "",
        items: [],
      };
  return { result, locale, q };
}

export async function generateMetadata(props: Props): Promise<Metadata> {
  const { result, locale, q } = await load(props);
  const title = q
    ? translate(locale, "help_center.search_title", { q })
    : translate(locale, "help_center.search_label");
  return helpMetadata({
    org: result.organization,
    locale,
    path: `/help/${encodeURIComponent(result.organization.slug)}/search`,
    title,
    description: title,
    alternates: false,
    // Search result pages are not indexed.
    index: false,
  });
}

/** Search results (CF-74): published public articles of the organization. */
export default async function HelpSearchPage(props: Props) {
  const { result, locale, q } = await load(props);
  const org = result.organization;
  const tc = (key: string, params?: Record<string, string | number>) =>
    translate(locale, `help_center.${key}`, params);

  return (
    <HelpShell
      org={org}
      locale={locale}
      path={`/help/${encodeURIComponent(org.slug)}/search${q ? `?q=${encodeURIComponent(q)}` : ""}`}
      query={q}
    >
      <HelpBreadcrumbs
        locale={locale}
        items={[
          { label: tc("title"), href: routes.help.home(org.slug, locale) },
          { label: tc("search_label") },
        ]}
      />
      <h1 className="font-display mb-6 text-2xl font-semibold tracking-tight">
        {q ? tc("search_title", { q }) : tc("search_label")}
      </h1>
      {q && result.items.length > 0 && (
        <>
          <p className="text-muted-foreground mb-4 text-sm" role="status">
            {tc("search_count", { count: result.items.length })}
          </p>
          <ArticleList org={org.slug} articles={result.items} locale={locale} />
        </>
      )}
      {q && result.items.length === 0 && (
        <div
          className="border-border rounded-xl border border-dashed px-6 py-12 text-center"
          role="status"
        >
          <p className="font-medium">{tc("search_empty")}</p>
          <p className="text-muted-foreground mt-1 text-sm">
            {tc("search_empty_hint")}
          </p>
        </div>
      )}
    </HelpShell>
  );
}
