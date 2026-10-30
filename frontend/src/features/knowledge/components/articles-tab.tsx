"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import {
  ChevronLeft,
  ChevronRight,
  FileText,
  Search,
  ThumbsDown,
  ThumbsUp,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { routes } from "@/config/routes";
import { useActiveOrganization } from "@/features/organizations";
import { useDebounce } from "@/hooks/use-debounce";
import { relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

import { collectionOptions, collectionTree } from "../lib/collections";
import {
  knowledgeKeys,
  knowledgeService,
  type KnowledgeArticleFilters,
  type KnowledgeArticleListItem,
} from "../services/knowledge.service";
import { ArticleStateBadge } from "./article-state-badge";

const PAGE = 20;
const ALL = "all";

type Filters = {
  locale: string;
  visibility: string;
  state: string;
  collection: string;
};

const EMPTY: Filters = {
  locale: ALL,
  visibility: ALL,
  state: ALL,
  collection: ALL,
};

export function ArticlesTab({ canWrite }: { canWrite: boolean }) {
  const { t, locale } = useLocale();
  const { active: org } = useActiveOrganization();
  const [q, setQ] = useState("");
  const query = useDebounce(q.trim(), 300);
  const [filters, setFilters] = useState<Filters>(EMPTY);
  const [offset, setOffset] = useState(0);

  const params: KnowledgeArticleFilters = {
    limit: PAGE,
    offset,
    ...(query ? { q: query } : {}),
    ...(filters.locale !== ALL
      ? { locale: filters.locale as "tr" | "en" }
      : {}),
    ...(filters.visibility !== ALL
      ? { visibility: filters.visibility as "public" | "internal" }
      : {}),
    ...(filters.state !== ALL
      ? { state: filters.state as "draft" | "published" | "changed" }
      : {}),
    ...(filters.collection !== ALL
      ? { collection_uuid: filters.collection }
      : {}),
  };
  const articles = useQuery({
    queryKey: knowledgeKeys.articles(org?.slug, params),
    queryFn: () => knowledgeService.articles(params),
    enabled: Boolean(org),
    placeholderData: keepPreviousData,
  });
  const collections = useQuery({
    queryKey: knowledgeKeys.collections(org?.slug),
    queryFn: knowledgeService.collections,
    enabled: Boolean(org),
  });
  const options = useMemo(
    () => collectionOptions(collectionTree(collections.data ?? [], locale)),
    [collections.data, locale],
  );
  const names = useMemo(
    () => new Map((collections.data ?? []).map((c) => [c.uuid, c.name])),
    [collections.data],
  );

  const set = (key: keyof Filters) => (value: string) => {
    setFilters((f) => ({ ...f, [key]: value }));
    setOffset(0);
  };
  const filtered =
    query !== "" || Object.values(filters).some((v) => v !== ALL);
  const total = articles.data?.total ?? 0;

  return (
    <div className="space-y-4">
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-[minmax(0,2fr)_repeat(4,minmax(0,1fr))]">
        <div className="relative sm:col-span-2 lg:col-span-1">
          <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
          <Input
            value={q}
            onChange={(e) => {
              setQ(e.target.value);
              setOffset(0);
            }}
            placeholder={t("knowledge.articles.search")}
            aria-label={t("knowledge.articles.search")}
            className="pl-9"
          />
        </div>
        <FilterSelect
          label={t("knowledge.fields.locale")}
          value={filters.locale}
          onChange={set("locale")}
          items={[
            [ALL, t("knowledge.filters.all_locales")],
            ["tr", t("knowledge.locale.tr")],
            ["en", t("knowledge.locale.en")],
          ]}
        />
        <FilterSelect
          label={t("knowledge.fields.visibility")}
          value={filters.visibility}
          onChange={set("visibility")}
          items={[
            [ALL, t("knowledge.filters.all_visibility")],
            ["public", t("knowledge.visibility.public")],
            ["internal", t("knowledge.visibility.internal")],
          ]}
        />
        <FilterSelect
          label={t("knowledge.fields.state")}
          value={filters.state}
          onChange={set("state")}
          items={[
            [ALL, t("knowledge.filters.all_states")],
            ["draft", t("knowledge.state.draft")],
            ["published", t("knowledge.state.published")],
            ["changed", t("knowledge.state.changed")],
          ]}
        />
        <FilterSelect
          label={t("knowledge.fields.collection")}
          value={filters.collection}
          onChange={set("collection")}
          items={[
            [ALL, t("knowledge.filters.all_collections")],
            ...options.map(
              ({ collection, depth }) =>
                [
                  collection.uuid,
                  depth > 0 ? `— ${collection.name}` : collection.name,
                ] as [string, string],
            ),
          ]}
        />
      </div>

      {articles.isLoading && <Loading label={t("common.loading")} />}
      {articles.isError && (
        <ErrorState
          title={t("knowledge.error.title")}
          description={t("knowledge.error.description")}
          onRetry={() => void articles.refetch()}
        />
      )}
      {articles.data && articles.data.items.length === 0 && (
        <EmptyState
          icon={<FileText className="text-signal size-5" />}
          title={
            filtered
              ? t("knowledge.articles.no_match")
              : t("knowledge.articles.empty_title")
          }
          description={
            filtered ? undefined : t("knowledge.articles.empty_description")
          }
          action={
            filtered ? (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setQ("");
                  setFilters(EMPTY);
                  setOffset(0);
                }}
              >
                {t("knowledge.filters.clear")}
              </Button>
            ) : canWrite ? (
              <Button size="sm" asChild>
                <Link href={routes.cms.settings.knowledge.create}>
                  {t("knowledge.articles.new")}
                </Link>
              </Button>
            ) : undefined
          }
        />
      )}

      {articles.data && articles.data.items.length > 0 && (
        <Card className="py-0">
          <CardContent className="divide-border divide-y p-0">
            {articles.data.items.map((a) => (
              <ArticleRow
                key={a.uuid}
                article={a}
                collection={
                  a.collection_uuid ? names.get(a.collection_uuid) : undefined
                }
              />
            ))}
          </CardContent>
        </Card>
      )}

      {total > PAGE && (
        <div className="flex items-center justify-between gap-2 text-sm">
          <span className="text-muted-foreground">
            {t("knowledge.pagination", {
              from: offset + 1,
              to: Math.min(offset + PAGE, total),
              total,
            })}
          </span>
          <div className="flex gap-1">
            <Button
              variant="outline"
              size="icon"
              aria-label={t("knowledge.previous")}
              disabled={offset === 0}
              onClick={() => setOffset(Math.max(0, offset - PAGE))}
            >
              <ChevronLeft className="size-4" />
            </Button>
            <Button
              variant="outline"
              size="icon"
              aria-label={t("knowledge.next")}
              disabled={offset + PAGE >= total}
              onClick={() => setOffset(offset + PAGE)}
            >
              <ChevronRight className="size-4" />
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

function FilterSelect({
  label,
  value,
  onChange,
  items,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  items: [string, string][];
}) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger aria-label={label} className="w-full">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {items.map(([v, l]) => (
          <SelectItem key={v} value={v}>
            {l}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function ArticleRow({
  article: a,
  collection,
}: {
  article: KnowledgeArticleListItem;
  collection?: string;
}) {
  const { t, locale } = useLocale();
  const votes = a.helpful_count + a.not_helpful_count;
  return (
    <Link
      href={routes.cms.settings.knowledge.detail(a.uuid)}
      className="hover:bg-muted/50 focus-visible:bg-muted/50 flex flex-col gap-2 px-4 py-3 outline-none sm:flex-row sm:items-center sm:gap-4"
    >
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="truncate font-medium">{a.title}</span>
          <ArticleStateBadge
            version={a.version}
            publishedVersion={a.published_version}
          />
          {a.visibility === "internal" && (
            <Badge variant="outline">
              {t("knowledge.visibility.internal")}
            </Badge>
          )}
          <Badge variant="secondary" className="uppercase">
            {a.locale}
          </Badge>
        </div>
        <p className="text-muted-foreground truncate text-xs">
          {[
            collection ?? t("knowledge.no_collection"),
            t("knowledge.version_short", { version: a.version }),
            t("knowledge.updated", {
              when: relativeDatetime(a.updated_at, locale),
            }),
          ].join(" · ")}
        </p>
      </div>
      <div
        className="text-muted-foreground flex shrink-0 items-center gap-3 text-xs tabular-nums"
        title={t("knowledge.feedback.title")}
      >
        {votes === 0 ? (
          <span>{t("knowledge.feedback.none")}</span>
        ) : (
          <>
            <span className="flex items-center gap-1">
              <ThumbsUp className="size-3.5" aria-hidden />
              <span className="sr-only">{t("knowledge.feedback.helpful")}</span>
              {a.helpful_count}
            </span>
            <span className="flex items-center gap-1">
              <ThumbsDown className="size-3.5" aria-hidden />
              <span className="sr-only">
                {t("knowledge.feedback.not_helpful")}
              </span>
              {a.not_helpful_count}
            </span>
          </>
        )}
      </div>
    </Link>
  );
}
