import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { routes } from "@/config/routes";
import { ForumComposer } from "@/features/help-center/components/forum-client";
import {
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
  params: Promise<{ org: string; category: string }>;
  searchParams: ForumSearchParams;
};

async function load({ params, searchParams }: Props) {
  const [{ org, category: slug }, locale, query] = await Promise.all([
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
        category: slug,
        q: query.q || undefined,
      }),
    ),
  ]);
  const category = categories.find((c) => c.slug === slug);
  if (!category) notFound();
  return { home, categories, category, topics, locale, ...query };
}

export async function generateMetadata(props: Props): Promise<Metadata> {
  const { home, category, locale, q, page } = await load(props);
  return helpMetadata({
    org: home.organization,
    locale,
    path: routes.help.forumCategory(home.organization.slug, category.slug),
    title: `${category.name} · ${forumText(locale, "title")}`,
    description: forumText(locale, "category_description", {
      category: category.name,
      name: home.organization.name,
    }),
    index: !q && page === 1,
  });
}

/** Topics of one forum category (CF-78). */
export default async function ForumCategoryPage(props: Props) {
  const { home, categories, category, topics, locale, q, page } =
    await load(props);
  const org = home.organization;
  const path = routes.help.forumCategory(org.slug, category.slug);

  return (
    <HelpShell org={org} locale={locale} path={path}>
      <HelpBreadcrumbs
        locale={locale}
        items={[
          {
            label: translate(locale, "help_center.title"),
            href: routes.help.home(org.slug, locale),
          },
          {
            label: forumText(locale, "title"),
            href: routes.help.forum(org.slug, locale),
          },
          { label: category.name },
        ]}
      />
      <h1 className="font-display mb-6 text-2xl font-semibold tracking-tight sm:text-3xl">
        {category.name}
      </h1>
      <div className="mb-8">
        <ForumComposer
          org={org.slug}
          labels={forumLabels(locale, org.name)}
          categories={[{ slug: category.slug, name: category.name }]}
          defaultCategory={category.slug}
        />
      </div>
      <div className="mb-4">
        <ForumSearch action={path} q={q} locale={locale} />
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
    </HelpShell>
  );
}
