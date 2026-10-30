import {
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  MessagesSquare,
  ThumbsUp,
} from "lucide-react";
import Link from "next/link";

import { routes } from "@/config/routes";
import { translate } from "@/lib/i18n/messages";
import { date } from "@/lib/utils/format";

import type { ForumCategory, ForumTopicSummary, HelpLocale } from "../lib/api";
import type { ForumLabels } from "./forum-client";

export const FORUM_PAGE_SIZE = 20;

export type ForumSearchParams = Promise<{
  lang?: string | string[];
  q?: string | string[];
  sayfa?: string | string[];
}>;

/** `?sayfa=` (1-based) and `?q=` of a forum list page. */
export async function readForumQuery(searchParams: ForumSearchParams) {
  const sp = await searchParams;
  const first = (v?: string | string[]) => (Array.isArray(v) ? v[0] : v);
  const page = Math.max(1, Math.min(500, Number(first(sp.sayfa)) || 1));
  const q = (first(sp.q) ?? "").trim().slice(0, 200);
  return { page, q, offset: (page - 1) * FORUM_PAGE_SIZE };
}

export function forumText(
  locale: HelpLocale,
  key: string,
  params?: Record<string, string | number>,
) {
  return translate(locale, `forum.public.${key}`, params);
}

/** Visitor strings for the client components (help center language). */
export function forumLabels(locale: HelpLocale, orgName: string): ForumLabels {
  const k = (key: string) => forumText(locale, key, { name: orgName });
  return {
    signin_title: k("signin_title"),
    signin_text: k("signin_text"),
    ask: k("ask"),
    reply: k("reply"),
    title: k("field_title"),
    body: k("field_body"),
    category: k("field_category"),
    pick_category: k("pick_category"),
    submit: k("submit"),
    sending: k("sending"),
    pending: k("pending"),
    failed: k("failed"),
    denied: k("denied"),
    rate_limited: k("rate_limited"),
    vote: k("vote"),
    voted: k("voted"),
    subscribe: k("subscribe"),
    unsubscribe: k("unsubscribe"),
    cancel: k("cancel"),
  };
}

export function ForumCategoryGrid({
  org,
  categories,
  locale,
}: {
  org: string;
  categories: ForumCategory[];
  locale: HelpLocale;
}) {
  return (
    <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {categories.map((c) => (
        <li key={c.uuid}>
          <Link
            href={routes.help.forumCategory(org, c.slug, locale)}
            className="border-border bg-card hover:border-foreground/30 focus-visible:ring-ring flex h-full items-center gap-3 rounded-xl border p-4 transition-colors outline-none focus-visible:ring-2"
          >
            <MessagesSquare
              aria-hidden
              className="text-muted-foreground size-5 shrink-0"
            />
            <span className="font-medium break-words">{c.name}</span>
          </Link>
        </li>
      ))}
    </ul>
  );
}

export function ForumTopicList({
  org,
  topics,
  categories,
  locale,
}: {
  org: string;
  topics: ForumTopicSummary[];
  categories: ForumCategory[];
  locale: HelpLocale;
}) {
  const name = (slug: string) =>
    categories.find((c) => c.slug === slug)?.name ?? slug;
  return (
    <ul className="divide-border border-border bg-card divide-y overflow-hidden rounded-xl border">
      {topics.map((topic) => (
        <li key={topic.uuid}>
          <Link
            href={routes.help.forumTopic(org, topic.slug, locale)}
            className="hover:bg-muted/50 focus-visible:bg-muted/50 flex gap-3 px-4 py-4 outline-none sm:px-5"
          >
            <span className="min-w-0 flex-1">
              <span className="block font-medium break-words">
                {topic.title}
              </span>
              <span className="text-muted-foreground mt-1 line-clamp-2 block text-sm break-words">
                {topic.body}
              </span>
              <span className="text-muted-foreground mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                <span>{name(topic.category_slug)}</span>
                <time dateTime={topic.created_at}>
                  {date(topic.created_at, "dd.MM.yyyy", locale)}
                </time>
                <span className="flex items-center gap-1">
                  <ThumbsUp aria-hidden className="size-3" />
                  {forumText(locale, "votes", { count: topic.votes })}
                </span>
                {topic.accepted_reply_uuid && (
                  <span className="flex items-center gap-1 font-medium text-emerald-700 dark:text-emerald-300">
                    <CheckCircle2 aria-hidden className="size-3.5" />
                    {forumText(locale, "answered")}
                  </span>
                )}
              </span>
            </span>
          </Link>
        </li>
      ))}
    </ul>
  );
}

/** Previous / next links for `?sayfa=`; keeps `?q=` and the language. */
export function ForumPagination({
  path,
  page,
  total,
  q,
  locale,
}: {
  path: string;
  page: number;
  total: number;
  q?: string;
  locale: HelpLocale;
}) {
  const pages = Math.ceil(total / FORUM_PAGE_SIZE);
  if (pages <= 1) return null;
  const href = (p: number) => {
    const params = new URLSearchParams();
    if (q) params.set("q", q);
    if (p > 1) params.set("sayfa", String(p));
    if (locale !== "tr") params.set("lang", locale);
    const qs = params.toString();
    return qs ? `${path}?${qs}` : path;
  };
  const cls =
    "border-border inline-flex h-9 items-center gap-1 rounded-md border px-3 text-sm outline-none hover:bg-muted focus-visible:ring-2";
  return (
    <nav
      aria-label={forumText(locale, "pagination")}
      className="mt-6 flex items-center justify-between gap-3"
    >
      {page > 1 ? (
        <Link href={href(page - 1)} rel="prev" className={cls}>
          <ChevronLeft aria-hidden className="size-4" />
          {forumText(locale, "prev")}
        </Link>
      ) : (
        <span />
      )}
      <span className="text-muted-foreground text-sm tabular-nums">
        {forumText(locale, "page_of", { page, pages })}
      </span>
      {page < pages ? (
        <Link href={href(page + 1)} rel="next" className={cls}>
          {forumText(locale, "next")}
          <ChevronRight aria-hidden className="size-4" />
        </Link>
      ) : (
        <span />
      )}
    </nav>
  );
}

/** Topic title search inside the forum (not the article search of the shell). */
export function ForumSearch({
  action,
  q,
  locale,
}: {
  action: string;
  q: string;
  locale: HelpLocale;
}) {
  return (
    <form action={action} method="get" role="search" className="flex gap-2">
      <label htmlFor="forum-search" className="sr-only">
        {forumText(locale, "search_label")}
      </label>
      <input
        id="forum-search"
        name="q"
        type="search"
        defaultValue={q}
        maxLength={200}
        placeholder={forumText(locale, "search_placeholder")}
        className="border-input bg-background h-10 min-w-0 flex-1 rounded-md border px-3 text-sm"
      />
      {locale !== "tr" && <input type="hidden" name="lang" value={locale} />}
      <button
        type="submit"
        className="bg-primary text-primary-foreground h-10 shrink-0 rounded-md px-4 text-sm font-medium outline-none hover:opacity-90 focus-visible:ring-2"
      >
        {forumText(locale, "search_submit")}
      </button>
    </form>
  );
}
