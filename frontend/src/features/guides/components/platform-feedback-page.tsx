"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MessageSquareHeart } from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useLocale } from "@/providers/locale-provider";

import {
  platformGuidesService,
  type ProductFeedback,
} from "../services/guides.service";

const KINDS = ["bug", "idea", "other"] as const;
const STATUSES = ["new", "reviewed", "closed"] as const;
const LIMIT = 25;
const ALL = "all";

/** Product feedback about ClickFix (platform.product_feedback.read). */
export function PlatformFeedbackPage() {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const [kind, setKind] = useState<string>(ALL);
  const [status, setStatus] = useState<string>("new");
  const [offset, setOffset] = useState(0);
  const params = {
    kind: kind === ALL ? undefined : kind,
    status: status === ALL ? undefined : status,
    limit: LIMIT,
    offset,
  };
  const list = useQuery({
    queryKey: ["platform-product-feedback", params],
    queryFn: () => platformGuidesService.feedback(params),
  });
  const setStatusMutation = useMutation({
    mutationFn: (v: { uuid: string; status: ProductFeedback["status"] }) =>
      platformGuidesService.setFeedbackStatus(v.uuid, v.status),
    onSettled: () =>
      qc.invalidateQueries({ queryKey: ["platform-product-feedback"] }),
  });
  const total = list.data?.total ?? 0;

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<MessageSquareHeart className="size-7" />}
        title={t("guides.platform_feedback.title")}
        description={t("guides.platform_feedback.description")}
        actions={
          <div className="flex gap-2">
            <Select
              value={kind}
              onValueChange={(v) => {
                setKind(v);
                setOffset(0);
              }}
            >
              <SelectTrigger className="w-36">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>{t("guides.all")}</SelectItem>
                {KINDS.map((k) => (
                  <SelectItem key={k} value={k}>
                    {t(`guides.feedback.kind.${k}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value={status}
              onValueChange={(v) => {
                setStatus(v);
                setOffset(0);
              }}
            >
              <SelectTrigger className="w-36">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>{t("guides.all")}</SelectItem>
                {STATUSES.map((s) => (
                  <SelectItem key={s} value={s}>
                    {t(`guides.platform_feedback.status.${s}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        }
      />
      <div className="space-y-3">
        {list.isLoading
          ? [0, 1, 2].map((i) => (
              <Skeleton key={i} className="h-28 w-full rounded-xl" />
            ))
          : null}
        {list.isError ? (
          <ErrorState
            title={t("common.error_server")}
            onRetry={() => void list.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : null}
        {(list.data?.items ?? []).map((f) => (
          <Card key={f.uuid}>
            <CardContent className="space-y-2 p-4">
              <div className="flex flex-wrap items-center gap-2 text-xs">
                <Badge variant={f.kind === "bug" ? "danger" : "secondary"}>
                  {t(`guides.feedback.kind.${f.kind}`)}
                </Badge>
                <span className="font-medium">{f.organization_name}</span>
                <span className="text-muted-foreground">{f.user_email}</span>
                <span className="text-muted-foreground">
                  {f.surface} · {f.route || "—"}
                  {f.app_version ? ` · ${f.app_version}` : ""}
                </span>
                <span className="text-muted-foreground ms-auto">
                  {new Date(f.created_at).toLocaleString(locale)}
                </span>
              </div>
              <p className="text-sm whitespace-pre-wrap">{f.message}</p>
              <div className="flex flex-wrap items-center gap-2">
                {f.attachment_uuids.length ? (
                  <span className="text-muted-foreground text-xs">
                    {t("guides.platform_feedback.attachments", {
                      count: f.attachment_uuids.length,
                    })}
                  </span>
                ) : null}
                <Select
                  value={f.status}
                  disabled={
                    setStatusMutation.isPending &&
                    setStatusMutation.variables?.uuid === f.uuid
                  }
                  onValueChange={(v) =>
                    setStatusMutation.mutate({
                      uuid: f.uuid,
                      status: v as ProductFeedback["status"],
                    })
                  }
                >
                  <SelectTrigger className="ms-auto h-8 w-36">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {STATUSES.map((s) => (
                      <SelectItem key={s} value={s}>
                        {t(`guides.platform_feedback.status.${s}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </CardContent>
          </Card>
        ))}
        {list.data && list.data.items.length === 0 ? (
          <EmptyState title={t("guides.platform_feedback.empty")} />
        ) : null}
      </div>
      {total > LIMIT ? (
        <div className="flex items-center justify-end gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={offset === 0}
            onClick={() => setOffset(Math.max(0, offset - LIMIT))}
          >
            {t("guides.prev")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={offset + LIMIT >= total}
            onClick={() => setOffset(offset + LIMIT)}
          >
            {t("guides.next")}
          </Button>
        </div>
      ) : null}
    </div>
  );
}
