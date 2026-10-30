"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { PlayCircle, ScanEye, Video } from "lucide-react";
import { useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useInboxScope } from "../hooks/use-inbox";
import { replayService, type WidgetSession } from "../services/replay.service";
import { CobrowseDialog } from "./cobrowse-dialog";
import { ReplayPlayer } from "./replay-player";

const STATUS_VARIANT: Record<
  WidgetSession["status"],
  "success" | "secondary" | "outline" | "warning"
> = {
  requested: "warning",
  active: "success",
  ended: "secondary",
  revoked: "outline",
  declined: "outline",
};

/**
 * "Oturum kayıtları" (CF-126): the visitor's consented, masked session
 * recordings and co-browse sessions of a widget conversation, plus the
 * "Ortak tarama başlat" action. Listing needs tenant.replays.read, co-browse
 * tenant.cobrowse.use. Hidden for other channels.
 */
export function SessionReplayPanel({
  conversationUuid,
  channelType,
}: {
  conversationUuid: string;
  channelType: string;
}) {
  const { t, locale } = useLocale();
  const { org } = useInboxScope();
  const { active } = useActiveOrganization();
  const perms = new Set(active?.permissions ?? []);
  const canReplays = perms.has(Permission.TenantReplaysRead);
  const canCobrowse = perms.has(Permission.TenantCobrowseUse);
  const isWidget = channelType === "widget";
  const qc = useQueryClient();
  const queryKey = ["inbox", org, "sessions", conversationUuid];
  const list = useQuery({
    queryKey,
    queryFn: () => replayService.sessions(conversationUuid),
    enabled: Boolean(org) && isWidget && canReplays,
    staleTime: 30_000,
    retry: false,
  });
  const [playing, setPlaying] = useState<WidgetSession | null>(null);
  const [cobrowse, setCobrowse] = useState<{ session?: WidgetSession } | null>(
    null,
  );

  if (!isWidget || (!canReplays && !canCobrowse)) return null;
  const items = list.data ?? [];
  const recordings = items.filter((s) => s.kind === "replay");
  const open = items.find(
    (s) =>
      s.kind === "cobrowse" &&
      (s.status === "requested" || s.status === "active"),
  );

  return (
    <section>
      <div className="mb-2 flex items-center gap-2">
        <h3 className="text-muted-foreground text-[11px] font-semibold tracking-wide uppercase">
          {t("inbox.replay.title")}
        </h3>
        {canCobrowse && (
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="ml-auto h-7 px-2 text-xs"
            onClick={() => setCobrowse(open ? { session: open } : {})}
          >
            <ScanEye className="size-3.5" />
            {open ? t("inbox.cobrowse.rejoin") : t("inbox.cobrowse.start")}
          </Button>
        )}
      </div>
      {canReplays && list.isLoading && <Skeleton className="h-12 w-full" />}
      {canReplays && list.isError && (
        <p className="text-muted-foreground text-xs">
          {t("inbox.replay.unavailable")}
        </p>
      )}
      {canReplays && list.isSuccess && items.length === 0 && (
        <p className="text-muted-foreground text-xs">
          {t("inbox.replay.none")}
        </p>
      )}
      {items.length > 0 && (
        <ul className="flex flex-col gap-1.5">
          {items.map((s) => {
            const playable =
              s.kind === "replay" &&
              s.status !== "revoked" &&
              s.status !== "declined";
            const Icon = s.kind === "replay" ? Video : ScanEye;
            return (
              <li key={s.uuid}>
                <button
                  type="button"
                  disabled={!playable}
                  onClick={() => setPlaying(s)}
                  className="hover:bg-muted/60 focus-visible:ring-ring flex w-full items-center gap-2.5 rounded-md border px-2.5 py-2 text-left text-xs outline-none focus-visible:ring-2 disabled:cursor-default disabled:hover:bg-transparent"
                >
                  <span className="bg-muted flex size-8 shrink-0 items-center justify-center rounded">
                    <Icon className="text-muted-foreground size-4" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium">
                      {t(`inbox.replay.kind.${s.kind}`)}
                    </span>
                    <span className="text-muted-foreground block truncate">
                      {datetime(
                        s.accepted_at ?? s.timeout_at,
                        "dd.MM HH:mm",
                        locale,
                      )}
                      {s.kind === "replay" &&
                        ` · ${t("inbox.replay.expires", {
                          date: datetime(s.expires_at, "dd.MM", locale),
                        })}`}
                    </span>
                  </span>
                  <Badge variant={STATUS_VARIANT[s.status]}>
                    {t(`inbox.replay.status.${s.status}`)}
                  </Badge>
                  {playable && (
                    <PlayCircle className="text-muted-foreground size-4 shrink-0" />
                  )}
                </button>
              </li>
            );
          })}
        </ul>
      )}
      {canReplays && recordings.length > 0 && (
        <p className="text-muted-foreground mt-1.5 text-[11px]">
          {t("inbox.replay.masked_note")}
        </p>
      )}
      {playing && (
        <AppDialog
          open
          onOpenChange={(o) => !o && setPlaying(null)}
          title={t("inbox.replay.player_title")}
          description={t("inbox.replay.player_description", {
            version: playing.consent_version || "—",
            expires: datetime(playing.expires_at, "dd.MM.yyyy", locale),
          })}
          className="max-h-[94vh] overflow-y-auto sm:max-w-5xl"
        >
          <ReplayPlayer
            conversationUuid={conversationUuid}
            sessionUuid={playing.uuid}
          />
          <p className="text-muted-foreground text-xs">
            {t("inbox.replay.audit_note")}
          </p>
        </AppDialog>
      )}
      {cobrowse && (
        <CobrowseDialog
          conversationUuid={conversationUuid}
          session={cobrowse.session}
          onClose={() => {
            setCobrowse(null);
            void qc.invalidateQueries({ queryKey });
          }}
        />
      )}
    </section>
  );
}
