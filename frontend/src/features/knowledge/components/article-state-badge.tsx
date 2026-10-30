"use client";

import { Badge } from "@/components/ui/badge";
import { useLocale } from "@/providers/locale-provider";

export type ArticleState = "draft" | "published" | "changed";

export function articleState(
  version: number,
  publishedVersion: number | null | undefined,
): ArticleState {
  if (publishedVersion == null) return "draft";
  return publishedVersion === version ? "published" : "changed";
}

/** Draft / published / published with a newer draft. */
export function ArticleStateBadge({
  version,
  publishedVersion,
}: {
  version: number;
  publishedVersion: number | null | undefined;
}) {
  const { t } = useLocale();
  const state = articleState(version, publishedVersion);
  if (state === "draft") {
    return <Badge variant="secondary">{t("knowledge.state.draft")}</Badge>;
  }
  if (state === "changed") {
    return <Badge variant="warning">{t("knowledge.state.changed")}</Badge>;
  }
  return (
    <Badge className="bg-signal-soft text-foreground border-transparent">
      <span aria-hidden className="bg-signal size-1.5 rounded-full" />
      {t("knowledge.state.published")}
    </Badge>
  );
}
