"use client";

import { PhoneIncoming, PhoneOutgoing } from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import {
  useChannels,
  useVoiceCalls,
  VOICE_CALLS_PAGE_SIZE,
} from "@/features/channels/hooks/use-channels";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const FAILED = new Set(["busy", "failed", "no-answer", "canceled"]);

function duration(seconds: number) {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

/**
 * Call log of one voice channel (CF-122): direction, numbers, status,
 * duration and whether a recording reached the conversation. Recordings
 * themselves open from the conversation through the attachment API.
 */
export function VoiceCallsPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const [page, setPage] = useState(0);
  const channels = useChannels();
  const calls = useVoiceCalls(uuid, page);
  const channel = channels.data?.find((c) => c.uuid === uuid);
  const items = calls.data?.items ?? [];
  const total = calls.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / VOICE_CALLS_PAGE_SIZE));

  return (
    <EntityPage
      title={t("channels.calls.title")}
      description={channel?.name ?? t("channels.types.voice")}
      breadcrumbs={[
        { label: t("channels.title"), href: "/channels" },
        { label: t("channels.calls.title") },
      ]}
      permission={permissions.tenant.channels.read}
    >
      {channel?.health.detail.startsWith("Bakiye:") && (
        <p className="text-muted-foreground text-sm">{channel.health.detail}</p>
      )}
      {calls.isError ? (
        <ErrorState
          title={t("channels.error")}
          onRetry={() => void calls.refetch()}
        />
      ) : calls.isLoading ? (
        <Skeleton className="h-64" />
      ) : items.length === 0 && page === 0 ? (
        <EmptyState
          title={t("channels.calls.empty")}
          description={t("channels.calls.empty_hint")}
        />
      ) : (
        <div className="space-y-3">
          <Card className="py-0">
            <CardContent className="p-0">
              <div className="overflow-x-auto">
                <table className="w-full min-w-[40rem] text-sm">
                  <thead className="text-muted-foreground border-b text-xs">
                    <tr>
                      <th className="px-3 py-2 text-start font-medium">
                        {t("channels.calls.when")}
                      </th>
                      <th className="px-3 py-2 text-start font-medium">
                        {t("channels.calls.from")}
                      </th>
                      <th className="px-3 py-2 text-start font-medium">
                        {t("channels.calls.to")}
                      </th>
                      <th className="px-3 py-2 text-start font-medium">
                        {t("channels.calls.status")}
                      </th>
                      <th className="px-3 py-2 text-end font-medium">
                        {t("channels.calls.duration")}
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y">
                    {items.map((c) => (
                      <tr key={c.uuid}>
                        <td className="px-3 py-2 whitespace-nowrap">
                          <span className="inline-flex items-center gap-2">
                            {c.direction === "inbound" ? (
                              <PhoneIncoming
                                className="text-muted-foreground size-3.5"
                                aria-label={t("channels.calls.inbound")}
                              />
                            ) : (
                              <PhoneOutgoing
                                className="text-muted-foreground size-3.5"
                                aria-label={t("channels.calls.outbound")}
                              />
                            )}
                            {datetime(c.created_at, undefined, locale)}
                          </span>
                        </td>
                        <td className="px-3 py-2 font-mono text-xs">
                          {c.from_number || "—"}
                        </td>
                        <td className="px-3 py-2 font-mono text-xs">
                          {c.to_number || "—"}
                          {c.internal_number && (
                            <span className="text-muted-foreground block">
                              Dahili {c.internal_number}
                            </span>
                          )}
                        </td>
                        <td className="px-3 py-2">
                          <div className="flex flex-wrap gap-1">
                            <Badge
                              variant={
                                FAILED.has(c.status) ? "danger" : "secondary"
                              }
                            >
                              {t(`channels.calls.statuses.${c.status}`, {
                                defaultValue: c.status,
                              })}
                            </Badge>
                            {c.recording_message_uuid ? (
                              <Badge variant="outline">
                                {t("channels.calls.recorded")}
                              </Badge>
                            ) : null}
                          </div>
                        </td>
                        <td className="px-3 py-2 text-end tabular-nums">
                          {duration(c.duration_seconds)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>
          <div className="text-muted-foreground flex items-center justify-between text-xs">
            <span>{t("channels.calls.total", { count: total })}</span>
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                variant="outline"
                disabled={page === 0}
                onClick={() => setPage((p) => p - 1)}
              >
                {t("channels.calls.prev")}
              </Button>
              <span className="tabular-nums">
                {page + 1} / {pages}
              </span>
              <Button
                size="sm"
                variant="outline"
                disabled={page + 1 >= pages}
                onClick={() => setPage((p) => p + 1)}
              >
                {t("channels.calls.next")}
              </Button>
            </div>
          </div>
        </div>
      )}
    </EntityPage>
  );
}
