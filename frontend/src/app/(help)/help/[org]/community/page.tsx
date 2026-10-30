import type { Metadata } from "next";

import { routes } from "@/config/routes";
import { ForumComposer } from "@/features/help-center/components/forum-client";
import {
  ForumCategoryGrid,
  ForumPagination,
  ForumSearch,
  ForumTopicList,
  forumLabels,
  forumText,
  readForumQuery,
  FORUM_PAGE_SIZE,
  type ForumSearchParams,
} from "@/features/help-center/components/forum-parts";
import { HelpShell } from "@/features/help-center/components/help-shell";
import { HelpBreadcrumbs } from "@/features/help-center/components/help-parts";
import { helpApi, resolveHelpLocale } from "@/features/help-center/lib/api";
import { orNotFound } from "@/features/help-center/lib/load";
import { helpMetadata } from "@/features/help-center/lib/seo";
import { translate } from "@/lib/i18n/messages";

type Props = {
  params: Promise<{ org: string }>;
  searchParams: ForumSearchParams;
};

async function load({ params, searchParams }: Props) {
  const [{ org }, locale, query] = await Promise.all([
    params,
    resolveHelpLocale(searchParams),
    readForumQuery(searchParams),
  ]);
  const [home, categories, topics] = await Promise.all([
    orNotFound(helpApi.home(org, locale)),
    orNotFound(helpApi.forumCategories(org)),
    orNotFound(
      helpApi.forumTopics(org, {
        limit: FORUM_PAGE_SIZE,
        offset: query.offset,
        q: query.q || undefined,
      }),
    ),
  ]);
  return { home, categories, topics, locale, ...query };
}

export async function generateMetadata(props: Props): Promise<Metadata> {
  const { home, locale, q, page } = await load(props);
  return helpMetadata({
    org: home.organization,
    locale,
    path: routes.help.forum(home.organization.slug),
    title: forumText(locale, "title"),
    description: forumText(locale, "meta_description", {
      name: home.organization.name,
    }),
    // Search results and deep pages are not indexed; topics are.
    index: !q && page === 1,
  });
}

/** Public community forum (CF-78): categories, latest topics, ask. */
export default async function ForumHomePage(props: Props) {
  const { home, categories, topics, locale, q, page } = await load(props);
  const org = home.organization;
  const path = routes.help.forum(org.slug);

  return (
    <HelpShell org={org} locale={locale} path={path}>
      <HelpBreadcrumbs
        locale={locale}
        items={[
          {
            label: translate(locale, "help_center.title"),
            href: routes.help.home(org.slug, locale),
          },
          { label: forumText(locale, "title") },
        ]}
      />
      <div className="mb-8 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
            {forumText(locale, "title")}
          </h1>
          <p className="text-muted-foreground mt-1">
            {forumText(locale, "intro", { name: org.name })}
          </p>
        </div>
      </div>
      {categories.length === 0 ? (
        <p className="text-muted-foreground">{forumText(locale, "empty")}</p>
      ) : (
        <>
          <section aria-labelledby="forum-categories" className="mb-10">
            <h2
              id="forum-categories"
              className="font-display mb-4 text-xl font-semibold tracking-tight"
            >
              {forumText(locale, "categories")}
            </h2>
            <ForumCategoryGrid
              org={org.slug}
              categories={categories}
              locale={locale}
            />
          </section>
          <section aria-labelledby="forum-ask" className="mb-10">
            <h2 id="forum-ask" className="sr-only">
              {forumText(locale, "ask")}
            </h2>
            <ForumComposer
              org={org.slug}
              labels={forumLabels(locale, org.name)}
              categories={categories.map((c) => ({
                slug: c.slug,
                name: c.name,
              }))}
            />
          </section>
          <section aria-labelledby="forum-topics">
            <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <h2
                id="forum-topics"
                className="font-display text-xl font-semibold tracking-tight"
              >
                {q
                  ? forumText(locale, "search_title", { q })
                  : forumText(locale, "latest")}
              </h2>
              <div className="sm:w-80">
                <ForumSearch action={path} q={q} locale={locale} />
              </div>
            </div>
            {topics.items.length === 0 ? (
              <p className="text-muted-foreground" role="status">
                {q
                  ? forumText(locale, "search_empty")
                  : forumText(locale, "no_topics")}
              </p>
            ) : (
              <ForumTopicList
                org={org.slug}
                topics={topics.items}
                categories={categories}
                locale={locale}
              />
            )}
            <ForumPagination
              path={path}
              page={page}
              total={topics.total}
              q={q}
              locale={locale}
            />
          </section>
        </>
      )}
    </HelpShell>
  );
}
