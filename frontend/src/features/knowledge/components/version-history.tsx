"use client";

import { useQuery } from "@tanstack/react-query";
import { Eye, History, RotateCcw } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { MarkdownContent } from "@/components/markdown/markdown-content";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";
import { useActiveOrganization } from "@/features/organizations";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

import {
  knowledgeKeys,
  knowledgeService,
  type KnowledgeArticleVersion,
} from "../services/knowledge.service";

/**
 * Immutable versions of an article. "Restore" copies a version's title,
 * body and visibility into the editor; saving makes it the newest draft
 * (history is never rewritten).
 */
export function VersionHistory({
  uuid,
  currentVersion,
  publishedVersion,
  canWrite,
  onRestore,
}: {
  uuid: string;
  currentVersion: number;
  publishedVersion: number | null;
  canWrite: boolean;
  onRestore: (version: KnowledgeArticleVersion) => void;
}) {
  const { t, locale } = useLocale();
  const { active: org } = useActiveOrganization();
  const versions = useQuery({
    queryKey: [...knowledgeKeys.versions(org?.slug, uuid), currentVersion],
    queryFn: () => knowledgeService.versions(uuid),
    enabled: Boolean(org),
  });
  const [viewing, setViewing] = useState<KnowledgeArticleVersion | null>(null);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <History className="size-4" />
          {t("knowledge.versions.title")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {versions.isLoading && <Loading label={t("common.loading")} />}
        {versions.isError && (
          <ErrorState
            title={t("knowledge.versions.error")}
            onRetry={() => void versions.refetch()}
          />
        )}
        <ol className="space-y-1">
          {(versions.data ?? []).map((v) => (
            <li
              key={v.version}
              className="hover:bg-muted/50 flex items-center gap-2 rounded-md px-2 py-1.5"
            >
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-1.5 text-sm">
                  <span className="font-medium tabular-nums">
                    {t("knowledge.version_short", { version: v.version ?? 0 })}
                  </span>
                  {v.version === publishedVersion && (
                    <Badge className="bg-signal-soft text-foreground border-transparent">
                      {t("knowledge.versions.live")}
                    </Badge>
                  )}
                  {v.version === currentVersion && (
                    <Badge variant="outline">
                      {t("knowledge.versions.current")}
                    </Badge>
                  )}
                </div>
                <p className="text-muted-foreground truncate text-xs">
                  {v.created_at
                    ? datetime(v.created_at, undefined, locale)
                    : ""}
                  {v.visibility === "internal"
                    ? ` · ${t("knowledge.visibility.internal")}`
                    : ""}
                </p>
              </div>
              <Button
                size="icon"
                variant="ghost"
                aria-label={t("knowledge.versions.view", {
                  version: v.version ?? 0,
                })}
                onClick={() => setViewing(v)}
              >
                <Eye className="size-4" />
              </Button>
              {canWrite && v.version !== currentVersion && (
                <Button
                  size="icon"
                  variant="ghost"
                  aria-label={t("knowledge.versions.restore", {
                    version: v.version ?? 0,
                  })}
                  onClick={() => onRestore(v)}
                >
                  <RotateCcw className="size-4" />
                </Button>
              )}
            </li>
          ))}
        </ol>
      </CardContent>
      <Dialog
        open={viewing !== null}
        onOpenChange={(o) => !o && setViewing(null)}
      >
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{viewing?.title}</DialogTitle>
            <DialogDescription>
              {t("knowledge.version_short", { version: viewing?.version ?? 0 })}
              {viewing?.created_at
                ? ` · ${datetime(viewing.created_at, undefined, locale)}`
                : ""}
            </DialogDescription>
          </DialogHeader>
          <ScrollArea className="max-h-[60vh] pr-3">
            {viewing?.body ? (
              <MarkdownContent markdown={viewing.body} />
            ) : (
              <p className="text-muted-foreground text-sm">
                {t("knowledge.versions.empty_body")}
              </p>
            )}
          </ScrollArea>
          {canWrite && viewing && viewing.version !== currentVersion && (
            <DialogFooter>
              <Button
                onClick={() => {
                  onRestore(viewing);
                  setViewing(null);
                }}
              >
                <RotateCcw className="size-4" />
                {t("knowledge.versions.restore_action")}
              </Button>
            </DialogFooter>
          )}
        </DialogContent>
      </Dialog>
    </Card>
  );
}
