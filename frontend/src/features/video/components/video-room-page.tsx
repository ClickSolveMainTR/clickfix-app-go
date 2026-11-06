"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  Check,
  Circle,
  Copy,
  Download,
  Mail,
  PhoneOff,
  Square,
  Trash2,
  UserMinus,
  Users,
  Video,
  X,
} from "lucide-react";
import { useMemo, useState } from "react";

import {
  EntityDeleteDialog,
  EntityRowActions,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  useVideoScope,
  videoErrorKey,
  videoKeys,
} from "../lib/use-video-scope";
import {
  videoService,
  type VideoJoinGrant,
  type VideoParticipant,
  type VideoRecording,
  type VideoRoom,
} from "../services/video.service";
import { VideoCall } from "./video-call";

/**
 * One room for organization members: share links, invite guests, run the
 * lobby, record, and join the call as host or agent.
 */
export function VideoRoomPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug, canUse } = useVideoScope();
  const qc = useQueryClient();
  const [grant, setGrant] = useState<VideoJoinGrant | null>(null);
  const room = useQuery({
    queryKey: videoKeys.room(slug, uuid),
    queryFn: () => videoService.room(slug, uuid),
    enabled: Boolean(slug && canUse),
    refetchInterval: 10_000,
  });
  const fail = (err: unknown) =>
    appToast.error(t(`video.errors.${videoErrorKey(err)}`));
  const refresh = () => qc.invalidateQueries({ queryKey: ["video", slug] });
  const join = useMutation({
    mutationFn: () => videoService.join(slug, uuid),
    onSuccess: setGrant,
    onError: fail,
  });
  const end = useMutation({
    mutationFn: () => videoService.end(slug, uuid),
    onSuccess: () => {
      setGrant(null);
      void refresh();
    },
    onError: fail,
  });

  if (!canUse) {
    return (
      <p className="text-muted-foreground text-sm">{t("video.forbidden")}</p>
    );
  }
  const r = room.data;
  if (!r) return null;

  if (grant?.token) {
    return (
      <div className="bg-background fixed inset-0 z-50 flex flex-col">
        <VideoCall
          grant={grant}
          onLeave={() => setGrant(null)}
          toolbar={
            <>
              <LobbyPopover room={r} />
              {grant.role === "host" ? <RecordingButton room={r} /> : null}
              {grant.role === "host" ? (
                <Button
                  size="sm"
                  variant="destructive"
                  onClick={() => end.mutate()}
                  disabled={end.isPending}
                >
                  <PhoneOff className="size-3.5" />
                  {t("video.rooms.end")}
                </Button>
              ) : null}
            </>
          }
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Video className="size-7" />}
        title={r.name}
        description={t(`video.status.${r.status}`)}
        actions={
          r.status !== "ended" ? (
            <>
              <Button onClick={() => join.mutate()} disabled={join.isPending}>
                <Video className="size-4" />
                {t("video.rooms.join")}
              </Button>
              <Button
                variant="outline"
                onClick={() => end.mutate()}
                disabled={end.isPending}
              >
                <PhoneOff className="size-4" />
                {t("video.rooms.end")}
              </Button>
            </>
          ) : null
        }
      />
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              {t("video.rooms.details")}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <Row label={t("video.rooms.encryption")}>
              {r.e2ee ? t("video.call.e2ee") : t("video.rooms.recordable")}
            </Row>
            {r.pin ? <Row label={t("video.rooms.pin")}>{r.pin}</Row> : null}
            <Row label={t("video.rooms.lobby")}>
              {r.lobby ? t("video.yes") : t("video.no")}
            </Row>
            <Row label={t("video.rooms.max_participants")}>
              {r.max_participants}
            </Row>
            <Row label={t("video.rooms.max_duration")}>
              {r.max_duration_minutes}
            </Row>
            {r.scheduled_start_at ? (
              <Row label={t("video.rooms.starts_at")}>
                {datetime(r.scheduled_start_at)}
              </Row>
            ) : null}
            <Row label={t("video.rooms.links_expire")}>
              {datetime(r.links_expire_at)}
            </Row>
            <Row label={t("video.minutes.billed")}>
              {r.billed_minutes}
              {r.unbilled_minutes > 0
                ? ` (+${r.unbilled_minutes} ${t("video.minutes.unbilled")})`
                : ""}
            </Row>
          </CardContent>
        </Card>
        {r.status !== "ended" ? <ShareCard room={r} /> : null}
        {r.status !== "ended" ? <LobbyCard room={r} /> : null}
      </div>
    </div>
  );
}

function Row({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      <span className="text-right font-medium">{children}</span>
    </div>
  );
}

function ShareCard({ room }: { room: VideoRoom }) {
  const { t, locale } = useLocale();
  const { slug } = useVideoScope();
  const [email, setEmail] = useState("");
  const [guestName, setGuestName] = useState("");
  const copy = async (url: string) => {
    await navigator.clipboard.writeText(url);
    appToast.success(t("video.links.copied"));
  };
  const agentLink = useMutation({
    mutationFn: () => videoService.link(slug, room.uuid, "agent"),
    onSuccess: (l) => copy(l.url),
    onError: (err) => appToast.error(t(`video.errors.${videoErrorKey(err)}`)),
  });
  const invite = useMutation({
    mutationFn: () =>
      videoService.invite(slug, room.uuid, {
        email: email.trim(),
        name: guestName.trim() || undefined,
        locale: locale === "en" ? "en" : "tr",
      }),
    onSuccess: () => {
      setEmail("");
      setGuestName("");
      appToast.success(t("video.links.invited"));
    },
    onError: (err) => appToast.error(t(`video.errors.${videoErrorKey(err)}`)),
  });
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("video.links.title")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap gap-2">
          {room.guest_link ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => copy(room.guest_link)}
            >
              <Copy className="size-3.5" />
              {t("video.links.copy_guest")}
            </Button>
          ) : null}
          <Button
            disabled={agentLink.isPending}
            variant="outline"
            size="sm"
            onClick={() => agentLink.mutate()}
          >
            <Copy className="size-3.5" />
            {t("video.links.copy_agent")}
          </Button>
        </div>
        {room.guest_join ? (
          <form
            className="flex flex-col gap-2 sm:flex-row"
            onSubmit={(e) => {
              e.preventDefault();
              invite.mutate();
            }}
          >
            <Input
              type="email"
              required
              placeholder={t("video.links.email")}
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
            <Input
              placeholder={t("video.links.guest_name")}
              value={guestName}
              onChange={(e) => setGuestName(e.target.value)}
            />
            <Button type="submit" size="sm" disabled={invite.isPending}>
              <Mail className="size-3.5" />
              {t("video.links.invite")}
            </Button>
          </form>
        ) : null}
        <p className="text-muted-foreground text-xs">
          {t("video.links.pin_hint")}
        </p>
      </CardContent>
    </Card>
  );
}

function useParticipants(room: VideoRoom) {
  const { slug } = useVideoScope();
  return useQuery({
    queryKey: videoKeys.participants(slug, room.uuid),
    queryFn: () => videoService.participants(slug, room.uuid),
    refetchInterval: 3_000,
  });
}

function ParticipantList({
  room,
  items,
}: {
  room: VideoRoom;
  items: VideoParticipant[];
}) {
  const { t } = useLocale();
  const { slug } = useVideoScope();
  const qc = useQueryClient();
  const act = useMutation({
    mutationFn: (v: { p: string; action: "admit" | "deny" | "remove" }) =>
      videoService.participant(slug, room.uuid, v.p, v.action),
    onSuccess: () =>
      qc.invalidateQueries({
        queryKey: videoKeys.participants(slug, room.uuid),
      }),
    onError: (err) => appToast.error(t(`video.errors.${videoErrorKey(err)}`)),
  });
  if (items.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">{t("video.lobby.empty")}</p>
    );
  }
  return (
    <ul className="divide-y">
      {items.map((p) => (
        <li key={p.uuid} className="flex items-center gap-2 py-2 text-sm">
          <div className="min-w-0 flex-1">
            <span className="font-medium">{p.display_name}</span>
            {p.email ? (
              <span className="text-muted-foreground ml-2 text-xs">
                {p.email}
              </span>
            ) : null}
          </div>
          <Badge variant="secondary">{t(`video.roles.${p.role}`)}</Badge>
          <Badge variant={p.status === "waiting" ? "default" : "outline"}>
            {t(`video.participant_status.${p.status}`)}
          </Badge>
          {p.status === "waiting" ? (
            <>
              <Button
                disabled={act.isPending}
                size="icon"
                variant="ghost"
                aria-label={t("video.lobby.admit")}
                onClick={() => act.mutate({ p: p.uuid, action: "admit" })}
              >
                <Check className="size-4" />
              </Button>
              <Button
                disabled={act.isPending}
                size="icon"
                variant="ghost"
                aria-label={t("video.lobby.deny")}
                onClick={() => act.mutate({ p: p.uuid, action: "deny" })}
              >
                <X className="size-4" />
              </Button>
            </>
          ) : null}
          {p.status === "admitted" && p.role !== "host" ? (
            <Button
              disabled={act.isPending}
              size="icon"
              variant="ghost"
              aria-label={t("video.lobby.remove")}
              onClick={() => act.mutate({ p: p.uuid, action: "remove" })}
            >
              <UserMinus className="size-4" />
            </Button>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

function LobbyCard({ room }: { room: VideoRoom }) {
  const { t } = useLocale();
  const list = useParticipants(room);
  return (
    <Card className="lg:col-span-2">
      <CardHeader>
        <CardTitle className="text-base">{t("video.lobby.title")}</CardTitle>
      </CardHeader>
      <CardContent>
        <ParticipantList room={room} items={list.data?.items ?? []} />
      </CardContent>
    </Card>
  );
}

function LobbyPopover({ room }: { room: VideoRoom }) {
  const { t } = useLocale();
  const list = useParticipants(room);
  const items = list.data?.items ?? [];
  const waiting = items.filter((p) => p.status === "waiting").length;
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button size="sm" variant={waiting > 0 ? "default" : "outline"}>
          <Users className="size-3.5" />
          {waiting > 0
            ? t("video.lobby.waiting_count", { count: waiting })
            : t("video.lobby.title")}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-96" align="end">
        <ParticipantList room={room} items={items} />
      </PopoverContent>
    </Popover>
  );
}

function RecordingButton({ room }: { room: VideoRoom }) {
  const { t } = useLocale();
  const { slug } = useVideoScope();
  const [active, setActive] = useState(false);
  const toggle = useMutation({
    mutationFn: async () => {
      if (active) await videoService.stopRecording(slug, room.uuid);
      else await videoService.startRecording(slug, room.uuid);
    },
    onSuccess: () => setActive((v) => !v),
    onError: (err) => appToast.error(t(`video.errors.${videoErrorKey(err)}`)),
  });
  if (!room.recording) return null;
  return (
    <Button
      size="sm"
      variant={active ? "destructive" : "outline"}
      onClick={() => toggle.mutate()}
      disabled={toggle.isPending}
    >
      {active ? (
        <Square className="size-3.5" />
      ) : (
        <Circle className="size-3.5 fill-current text-red-600" />
      )}
      {active ? t("video.recording.stop") : t("video.recording.start")}
    </Button>
  );
}

/** Recordings of the organization (tenant.video.manage). */
export function RecordingsList({ room }: { room?: string }) {
  const { t } = useLocale();
  const { slug, canManage } = useVideoScope();
  const qc = useQueryClient();
  const listState = useServerListState({ initialPageSize: 50 });
  const { limit, offset } = listState.params;
  const [deleteTarget, setDeleteTarget] = useState<VideoRecording | null>(null);
  const list = useQuery({
    queryKey: [...videoKeys.recordings(slug, room), limit, offset],
    queryFn: () => videoService.recordings(slug, { room, limit, offset }),
    enabled: Boolean(slug && canManage),
    placeholderData: keepPreviousData,
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => videoService.deleteRecording(slug, uuid),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: videoKeys.recordings(slug, room) }),
  });

  const columns = useMemo(
    () =>
      [
        createColumn<VideoRecording>({
          accessorKey: "room_name",
          labelKey: "video.rooms.name",
          gridPrimary: true,
          enableHiding: false,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.room_name}</span>
          ),
        }),
        createColumn<VideoRecording>({
          accessorKey: "started_at",
          labelKey: "video.recording.columns.started",
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs">
              {datetime(row.original.started_at)}
            </span>
          ),
        }),
        createColumn<VideoRecording>({
          accessorKey: "status",
          labelKey: "video.recording.columns.status",
          cell: ({ row }) => (
            <Badge variant="secondary">
              {t(`video.recording.status.${row.original.status}`)}
            </Badge>
          ),
        }),
        createColumn<VideoRecording>({
          accessorKey: "storage",
          labelKey: "video.recording.columns.storage",
          cell: ({ row }) => (
            <Badge variant="outline">
              {t(`video.recording.storage.${row.original.storage}`)}
            </Badge>
          ),
        }),
        createColumn<VideoRecording>({
          accessorKey: "expires_at",
          labelKey: "video.recording.columns.expires",
          cell: ({ row }) =>
            row.original.expires_at ? (
              <span className="text-muted-foreground text-xs">
                {datetime(row.original.expires_at)}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<VideoRecording>({
          id: "actions",
          labelKey: "common.actions",
          enableHiding: false,
          meta: { headerClassName: "text-right" },
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                ...(row.original.status === "complete"
                  ? [
                      {
                        id: "download",
                        label: t("video.recording.download"),
                        icon: Download,
                        onSelect: () =>
                          void videoService.downloadRecording(
                            slug,
                            row.original.uuid,
                          ),
                      },
                    ]
                  : []),
                {
                  id: "delete",
                  label: t("video.recording.delete"),
                  icon: Trash2,
                  variant: "destructive" as const,
                  onSelect: () => setDeleteTarget(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<VideoRecording, unknown>[],
    [slug, t],
  );

  if (!canManage) return null;
  return (
    <>
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyDescription={t("video.recording.empty")}
        total={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          // The endpoint takes `room`, `limit` and `offset` only.
          toolbar: false,
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
          viewMode: false,
        }}
      />
      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        softDelete={false}
        entityLabel={
          deleteTarget
            ? `${deleteTarget.room_name} (${datetime(deleteTarget.started_at)})`
            : ""
        }
        isPending={remove.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          remove.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </>
  );
}
