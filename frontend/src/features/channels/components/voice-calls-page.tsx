"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { PhoneIncoming, PhoneOutgoing } from "lucide-react";
import { useMemo } from "react";

import {
  EntityPage,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import {
  useChannels,
  useVoiceCalls,
} from "@/features/channels/hooks/use-channels";
import type { VoiceCall } from "@/features/channels/types";
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
  const listState = useServerListState({ initialPageSize: 25 });
  const { limit, offset } = listState.params;
  const channels = useChannels();
  const calls = useVoiceCalls(uuid, limit, offset);
  const channel = channels.data?.find((c) => c.uuid === uuid);

  const columns = useMemo(
    () =>
      [
        createColumn<VoiceCall>({
          accessorKey: "created_at",
          labelKey: "channels.calls.when",
          enableHiding: false,
          cell: ({ row }) => (
            <span className="inline-flex items-center gap-2 whitespace-nowrap">
              {row.original.direction === "inbound" ? (
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
              {datetime(row.original.created_at, undefined, locale)}
            </span>
          ),
        }),
        createColumn<VoiceCall>({
          accessorKey: "from_number",
          labelKey: "channels.calls.from",
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs">
              {row.original.from_number || "—"}
            </span>
          ),
        }),
        createColumn<VoiceCall>({
          accessorKey: "to_number",
          labelKey: "channels.calls.to",
          cell: ({ row }) => (
            <span className="font-mono text-xs">
              {row.original.to_number || "—"}
              {row.original.internal_number && (
                <span className="text-muted-foreground block">
                  {t("channels.calls.extension", {
                    number: row.original.internal_number,
                  })}
                </span>
              )}
            </span>
          ),
        }),
        createColumn<VoiceCall>({
          accessorKey: "status",
          labelKey: "channels.calls.status",
          cell: ({ row }) => (
            <div className="flex flex-wrap gap-1">
              <Badge
                variant={
                  FAILED.has(row.original.status) ? "danger" : "secondary"
                }
              >
                {t(`channels.calls.statuses.${row.original.status}`, {
                  defaultValue: row.original.status,
                })}
              </Badge>
              {row.original.recording_message_uuid ? (
                <Badge variant="outline">{t("channels.calls.recorded")}</Badge>
              ) : null}
            </div>
          ),
        }),
        createColumn<VoiceCall>({
          accessorKey: "duration_seconds",
          labelKey: "channels.calls.duration",
          meta: {
            headerClassName: "text-end",
            cellClassName: "text-end tabular-nums",
          },
          cell: ({ row }) => duration(row.original.duration_seconds),
        }),
      ] as ColumnDef<VoiceCall, unknown>[],
    [locale, t],
  );

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
      <EntityTable
        columns={columns}
        data={calls.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={calls.isLoading}
        isError={calls.isError}
        errorTitle={t("channels.error")}
        onRetry={() => void calls.refetch()}
        emptyTitle={t("channels.calls.empty")}
        emptyDescription={t("channels.calls.empty_hint")}
        total={calls.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: "tenant-voice-calls-v1",
          // The endpoint takes `limit` and `offset` only.
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
        }}
      />
    </EntityPage>
  );
}
