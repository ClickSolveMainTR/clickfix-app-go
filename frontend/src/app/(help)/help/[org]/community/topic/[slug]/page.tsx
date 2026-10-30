import { CheckCircle2 } from "lucide-react";
import type { Metadata } from "next";

import { routes } from "@/config/routes";
import {
  ForumComposer,
  ForumSubscribe,
  ForumVote,
} from "@/features/help-center/components/forum-client";
import {
  ForumPagination,
  forumLabels,
  forumText,
  readForumQuery,
  FORUM_PAGE_SIZE,
  type ForumSearchParams,
} from "@/features/help-center/components/forum-parts";
import { HelpShell } from "@/features/help-center/components/help-shell";
import {
  HelpBreadcrumbs,
  JsonLd,
} from "@/features/help-center/components/help-parts";
import {
  helpApi,
  resolveHelpLocale,
  siteOrigin,
} from "@/features/help-center/lib/api";
import { orNotFound } from "@/features/help-center/lib/load";
import { helpMetadata, localized } from "@/features/help-center/lib/seo";
import { translate } from "@/lib/i18n/messages";
import { datetime } from "@/lib/utils/format";

type Props = {
  params: Promise<{ org: string; slug: string }>;
  searchParams: ForumSearchParams;
};

async function load({ params, searchParams }: Props) {
  const [{ org, slug }, locale, query] = await Promise.all([
    params,
    resolveHelpLocale(searchParams),
    readForumQuery(searchParams),
  ]);
  const [home, topic] = await Promise.all([
    orNotFound(helpApi.home(org, locale)),
    orNotFound(helpApi.forumTopic(org, slug, FORUM_PAGE_SIZE, query.offset)),
  ]);
  return { home, topic, locale, page: query.page };
}

function excerpt(text: string, max = 160) {
  const flat = text.replace(/\s+/g, " ").trim();
  return flat.length > max ? `${flat.slice(0, max - 1)}…` : flat;
}

export async function generateMetadata(props: Props): Promise<Metadata> {
  const { home, topic, locale, page } = await load(props);
  return helpMetadata({
    org: home.organization,
    locale,
    path: routes.help.forumTopic(home.organization.slug, topic.slug),
    title: topic.title,
    description: excerpt(topic.body),
    type: "article",
    publishedTime: topic.created_at,
    modifiedTime: topic.updated_at,
    // Visitors write in one language; no hreflang alternates.
    alternates: false,
    index: page === 1,
  });
}

/** One forum topic (CF-78): question, accepted answer, replies, voting. */
export default async function ForumTopicPage(props: Props) {
  const { home, topic, locale, page } = await load(props);
  const org = home.organization;
  const path = routes.help.forumTopic(org.slug, topic.slug);
  const labels = forumLabels(locale, org.name);
  const accepted = topic.accepted_answer;
  const replies = topic.replies;
  const when = (iso: string) => datetime(iso, "d MMMM yyyy HH:mm", locale);
  const base = siteOrigin();

  return (
    <HelpShell org={org} locale={locale} path={path}>
      <JsonLd
        data={{
          "@context": "https://schema.org",
          "@type": "QAPage",
          mainEntity: {
            "@type": "Question",
            name: topic.title,
            text: topic.body,
            dateCreated: topic.created_at,
            answerCount: replies.total,
            url: `${base}${localized(path, locale)}`,
            ...(accepted
              ? {
                  acceptedAnswer: {
                    "@type": "Answer",
                    text: accepted.body,
                    dateCreated: accepted.created_at,
                  },
                }
              : {}),
            suggestedAnswer: replies.items
              .filter((r) => r.uuid !== accepted?.uuid)
              .map((r) => ({
                "@type": "Answer",
                text: r.body,
                dateCreated: r.created_at,
                upvoteCount: r.votes,
              })),
          },
        }}
      />
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
          { label: topic.title },
        ]}
      />
      <article aria-labelledby="forum-topic-title">
        <header className="mb-4">
          <h1
            id="forum-topic-title"
            className="font-display text-2xl font-semibold tracking-tight break-words sm:text-3xl"
          >
            {topic.title}
          </h1>
          <p className="text-muted-foreground mt-2 text-sm">
            <time dateTime={topic.created_at}>{when(topic.created_at)}</time>
          </p>
        </header>
        <div className="text-base leading-relaxed break-words whitespace-pre-wrap">
          {topic.body}
        </div>
        <div className="mt-4 flex flex-wrap items-center gap-2">
          <ForumVote org={org.slug} topicSlug={topic.slug} labels={labels} />
          <ForumSubscribe
            org={org.slug}
            topicSlug={topic.slug}
            labels={labels}
          />
        </div>
      </article>

      {accepted && (
        <section
          aria-labelledby="forum-accepted"
          className="mt-8 rounded-xl border border-emerald-500/40 bg-emerald-500/5 p-4 sm:p-5"
        >
          <h2
            id="forum-accepted"
            className="mb-2 flex items-center gap-2 text-sm font-semibold text-emerald-700 dark:text-emerald-300"
          >
            <CheckCircle2 aria-hidden className="size-4" />
            {forumText(locale, "accepted_answer")}
          </h2>
          <p className="break-words whitespace-pre-wrap">{accepted.body}</p>
          <p className="text-muted-foreground mt-2 text-xs">
            <time dateTime={accepted.created_at}>
              {when(accepted.created_at)}
            </time>
          </p>
        </section>
      )}

      <section aria-labelledby="forum-replies" className="mt-10">
        <h2
          id="forum-replies"
          className="font-display mb-4 text-xl font-semibold tracking-tight"
        >
          {forumText(locale, "replies", { count: replies.total })}
        </h2>
        {replies.items.length === 0 ? (
          <p className="text-muted-foreground">
            {forumText(locale, "no_replies")}
          </p>
        ) : (
          <ol className="space-y-3">
            {replies.items.map((r) => {
              const isAccepted = r.uuid === accepted?.uuid;
              return (
                <li
                  key={r.uuid}
                  id={`yanit-${r.uuid}`}
                  className={
                    isAccepted
                      ? "rounded-xl border border-emerald-500/40 p-4"
                      : "border-border bg-card rounded-xl border p-4"
                  }
                >
                  {isAccepted && (
                    <p className="mb-2 flex items-center gap-1.5 text-xs font-semibold text-emerald-700 dark:text-emerald-300">
                      <CheckCircle2 aria-hidden className="size-3.5" />
                      {forumText(locale, "accepted_answer")}
                    </p>
                  )}
                  <p className="break-words whitespace-pre-wrap">{r.body}</p>
                  <div className="text-muted-foreground mt-3 flex flex-wrap items-center gap-3 text-xs">
                    <time dateTime={r.created_at}>{when(r.created_at)}</time>
                    <ForumVote
                      org={org.slug}
                      topicSlug={topic.slug}
                      replyUuid={r.uuid}
                      votes={r.votes}
                      labels={labels}
                    />
                  </div>
                </li>
              );
            })}
          </ol>
        )}
        <ForumPagination
          path={path}
          page={page}
          total={replies.total}
          locale={locale}
        />
      </section>

      <section aria-label={labels.reply} className="mt-10">
        <ForumComposer org={org.slug} labels={labels} topicSlug={topic.slug} />
      </section>
    </HelpShell>
  );
}
