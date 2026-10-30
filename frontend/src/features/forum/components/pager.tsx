"use client";

import { ChevronLeft, ChevronRight } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

/** Previous / next pager for offset lists. Hidden on a single page. */
export function Pager({
  total,
  limit,
  offset,
  onChange,
}: {
  total: number;
  limit: number;
  offset: number;
  onChange: (offset: number) => void;
}) {
  const { t } = useLocale();
  if (total <= limit) return null;
  const page = Math.floor(offset / limit) + 1;
  const pages = Math.max(1, Math.ceil(total / limit));
  return (
    <div className="flex items-center justify-end gap-2">
      <span className="text-muted-foreground text-xs tabular-nums">
        {t("forum.pager", { page, pages })}
      </span>
      <Button
        size="icon"
        variant="outline"
        className="size-8"
        aria-label={t("forum.pager_prev")}
        disabled={offset === 0}
        onClick={() => onChange(Math.max(0, offset - limit))}
      >
        <ChevronLeft className="size-4" />
      </Button>
      <Button
        size="icon"
        variant="outline"
        className="size-8"
        aria-label={t("forum.pager_next")}
        disabled={offset + limit >= total}
        onClick={() => onChange(offset + limit)}
      >
        <ChevronRight className="size-4" />
      </Button>
    </div>
  );
}
