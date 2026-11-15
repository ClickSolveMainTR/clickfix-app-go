"use client";

import Link from "next/link";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { routes } from "@/config/routes";
import {
  useCategoryReview,
  useLabelRecord,
} from "@/features/categories/hooks/use-categories";
import { flatten } from "@/features/categories/lib";
import type {
  CategoryAssignment,
  CategoryNode,
  CategorySubjectType,
} from "@/features/categories/types";
import { useLocale } from "@/providers/locale-provider";

/** Static label keys (no template keys, see check-i18n). */
const SUBJECT_KEYS: Record<CategorySubjectType, string> = {
  conversation: "categories.subject.conversation",
  archive_record: "categories.subject.archive_record",
  knowledge_unit: "categories.subject.knowledge_unit",
  suggestion: "categories.subject.suggestion",
};

const SOURCE_KEYS: Record<CategoryAssignment["source"], string> = {
  embedding: "categories.source.embedding",
  ai: "categories.source.ai",
  human: "categories.source.human",
  model: "categories.source.model",
};

function ReviewRow({
  item,
  options,
}: {
  item: CategoryAssignment;
  options: CategoryNode[];
}) {
  const { t } = useLocale();
  const label = useLabelRecord();
  const [choice, setChoice] = useState(item.category?.uuid ?? "");
  const subject =
    item.subject_type === "conversation" ? (
      <Link
        className="font-mono text-xs underline-offset-4 hover:underline"
        href={`${routes.cms.inbox}?c=${item.subject_uuid}`}
      >
        {item.subject_uuid.slice(0, 8)}
      </Link>
    ) : (
      <span className="font-mono text-xs">{item.subject_uuid.slice(0, 8)}</span>
    );
  return (
    <li className="flex flex-wrap items-center gap-3 border-b py-3 last:border-0">
      <div className="min-w-48 flex-1 space-y-1">
        <div className="flex items-center gap-2">
          <Badge variant="secondary">
            {t(SUBJECT_KEYS[item.subject_type])}
          </Badge>
          {subject}
        </div>
        <p className="text-muted-foreground text-sm">
          {item.category
            ? t("categories.review.suggested", {
                name: item.category.name,
                confidence: Math.round(item.confidence * 100),
                source: t(SOURCE_KEYS[item.source]),
              })
            : t("categories.review.no_suggestion")}
        </p>
      </div>
      <Select value={choice} onValueChange={setChoice}>
        <SelectTrigger className="w-56">
          <SelectValue placeholder={t("categories.review.choose")} />
        </SelectTrigger>
        <SelectContent>
          {options.map((n) => (
            <SelectItem key={n.uuid} value={n.uuid}>
              {"— ".repeat(n.depth)}
              {n.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button
        size="sm"
        disabled={!choice || label.isPending}
        onClick={() =>
          label.mutate({
            subjectType: item.subject_type,
            subjectUuid: item.subject_uuid,
            categoryUuid: choice,
          })
        }
      >
        {choice && choice === item.category?.uuid
          ? t("categories.review.confirm")
          : t("categories.review.correct")}
      </Button>
    </li>
  );
}

/**
 * Records the classifier was unsure about. Every confirmation or correction
 * is stored as a label and improves the embedding stage at once.
 */
export function CategoryReviewQueue({ tree }: { tree: CategoryNode[] }) {
  const { t } = useLocale();
  const [offset, setOffset] = useState(0);
  const review = useCategoryReview(offset);
  const options = useMemo(
    () => flatten(tree).filter((n) => !n.archived),
    [tree],
  );
  const items = review.data?.items ?? [];
  const total = review.data?.total ?? 0;

  if (review.isError) {
    return (
      <ErrorState
        title={t("categories.review.load_failed")}
        onRetry={() => void review.refetch()}
      />
    );
  }
  if (!review.isLoading && items.length === 0) {
    return (
      <EmptyState
        title={t("categories.review.empty_title")}
        description={t("categories.review.empty_description")}
      />
    );
  }
  return (
    <div className="space-y-3">
      <ul className="rounded-lg border px-3">
        {items.map((item) => (
          <ReviewRow
            key={`${item.subject_type}:${item.subject_uuid}`}
            item={item}
            options={options}
          />
        ))}
      </ul>
      {total > 20 ? (
        <div className="flex items-center justify-end gap-2">
          <Button
            size="sm"
            variant="outline"
            disabled={offset === 0}
            onClick={() => setOffset(Math.max(0, offset - 20))}
          >
            {t("categories.review.previous")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={offset + 20 >= total}
            onClick={() => setOffset(offset + 20)}
          >
            {t("categories.review.next")}
          </Button>
        </div>
      ) : null}
    </div>
  );
}
