"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef, ColumnFiltersState } from "@tanstack/react-table";
import { ThumbsDown, ThumbsUp } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { EntityTable, useServerListState } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { routes } from "@/config/routes";
import { useActiveOrganization } from "@/features/organizations";
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

const ALL = "all";

type FilterKey = "locale" | "visibility" | "state" | "collection";

function filterValue(filters: ColumnFiltersState, key: FilterKey): string {
  const value = filters.find((filter) => filter.id === key)?.value;
  return typeof value === "string" && value ? value : ALL;
}

/** Same list apart from the page and the search text. */
function sameFilters(a: unknown, b: KnowledgeArticleFilters): boolean {
  if (!a || typeof a !== "object") return false;
  const strip = (filters: KnowledgeArticleFilters) =>
    JSON.stringify({ ...filters, q: undefined, limit: 0, offset: 0 });
  return strip(a as KnowledgeArticleFilters) === strip(b);
}

export function ArticlesTab() {
  const { t, locale } = useLocale();
  const router = useRouter();
  const { active: org } = useActiveOrganization();
  const listState = useServerListState({ initialPageSize: 20 });
  const { columnFilters, onColumnFiltersChange } = listState;

  // The four selects live in the table's column-filter state: changing one
  // goes back to the first page and the toolbar's reset clears them.
  const filters = {
    locale: filterValue(columnFilters, "locale"),
    visibility: filterValue(columnFilters, "visibility"),
    state: filterValue(columnFilters, "state"),
    collection: filterValue(columnFilters, "collection"),
  };
  const set = (key: FilterKey) => (value: string) =>
    onColumnFiltersChange((prev) => [
      ...prev.filter((filter) => filter.id !== key),
      ...(value === ALL ? [] : [{ id: key, value }]),
    ]);

  const { limit, offset, q } = listState.params;
  const params: KnowledgeArticleFilters = {
    limit,
    offset,
    ...(q ? { q } : {}),
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
    // Keep the rows while paging or typing; a filter change must not show
    // the previous filter's articles.
    placeholderData: (previous, previousQuery) =>
      sameFilters(previousQuery?.queryKey[4], params) ? previous : undefined,
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

  const columns = useMemo(
    () =>
      [
        createColumn<KnowledgeArticleListItem>({
          accessorKey: "title",
          labelKey: "knowledge.fields.title",
          gridPrimary: true,
          enableHiding: false,
          cell: ({ row }) => (
            <div className="flex flex-wrap items-center gap-2">
              <Link
                href={routes.cms.settings.knowledge.detail(row.original.uuid)}
                className="truncate font-medium underline-offset-2 hover:underline"
              >
                {row.original.title}
              </Link>
              <ArticleStateBadge
                version={row.original.version}
                publishedVersion={row.original.published_version}
              />
              {row.original.visibility === "internal" && (
                <Badge variant="outline">
                  {t("knowledge.visibility.internal")}
                </Badge>
              )}
            </div>
          ),
        }),
        createColumn<KnowledgeArticleListItem>({
          accessorKey: "locale",
          labelKey: "knowledge.fields.locale",
          cell: ({ row }) => (
            <Badge variant="secondary" className="uppercase">
              {row.original.locale}
            </Badge>
          ),
        }),
        createColumn<KnowledgeArticleListItem>({
          id: "collection",
          accessorFn: (row) => row.collection_uuid ?? "",
          labelKey: "knowledge.fields.collection",
          cell: ({ row }) => (
            <span className="text-muted-foreground">
              {(row.original.collection_uuid
                ? names.get(row.original.collection_uuid)
                : undefined) ?? t("knowledge.no_collection")}
            </span>
          ),
        }),
        createColumn<KnowledgeArticleListItem>({
          id: "department",
          accessorFn: (row) => row.department_name ?? "",
          labelKey: "knowledge.fields.department",
          cell: ({ row }) => (
            <span className="text-muted-foreground truncate text-xs">
              {row.original.department_name ??
                t("knowledge.department_organization_wide")}
            </span>
          ),
        }),
        createColumn<KnowledgeArticleListItem>({
          accessorKey: "version",
          labelKey: "knowledge.articles.columns.version",
          cell: ({ row }) => (
            <span className="text-muted-foreground tabular-nums">
              {t("knowledge.version_short", { version: row.original.version })}
            </span>
          ),
        }),
        createColumn<KnowledgeArticleListItem>({
          accessorKey: "updated_at",
          labelKey: "knowledge.articles.columns.updated",
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs">
              {relativeDatetime(row.original.updated_at, locale)}
            </span>
          ),
        }),
        createColumn<KnowledgeArticleListItem>({
          id: "feedback",
          accessorFn: (row) => row.helpful_count + row.not_helpful_count,
          labelKey: "knowledge.articles.columns.feedback",
          cell: ({ row }) => <Feedback article={row.original} />,
        }),
      ] as ColumnDef<KnowledgeArticleListItem, unknown>[],
    [locale, names, t],
  );

  const filtered =
    Boolean(q) || Object.values(filters).some((value) => value !== ALL);

  return (
    <EntityTable
      columns={columns}
      data={articles.data?.items ?? []}
      getRowId={(row) => row.uuid}
      onRowClick={(row) =>
        router.push(routes.cms.settings.knowledge.detail(row.uuid))
      }
      isLoading={articles.isLoading}
      isError={articles.isError}
      errorTitle={t("knowledge.error.title")}
      errorDescription={t("knowledge.error.description")}
      onRetry={() => void articles.refetch()}
      emptyTitle={
        filtered
          ? t("knowledge.articles.no_match")
          : t("knowledge.articles.empty_title")
      }
      emptyDescription={
        filtered
          ? t("table.reset_filters")
          : t("knowledge.articles.empty_description")
      }
      searchPlaceholder={t("knowledge.articles.search")}
      total={articles.data?.total ?? 0}
      state={listState.tableState}
      features={{
        persistKey: "tenant-knowledge-articles-v1",
        // The endpoint has a fixed order (last updated first) and takes the
        // four filters below, not per-column filters.
        sorting: false,
        columnFilters: false,
        rowSelection: false,
      }}
      toolbarExtra={
        <>
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
        </>
      }
    />
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
      <SelectTrigger aria-label={label} className="h-9 w-full sm:w-44">
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

function Feedback({ article: a }: { article: KnowledgeArticleListItem }) {
  const { t } = useLocale();
  const votes = a.helpful_count + a.not_helpful_count;
  return (
    <div
      className="text-muted-foreground flex items-center gap-3 text-xs tabular-nums"
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
  );
}
