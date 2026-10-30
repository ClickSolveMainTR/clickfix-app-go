"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, Lock, Plus, Users, Video } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  useVideoScope,
  videoErrorKey,
  videoKeys,
  availabilityKey,
} from "../lib/use-video-scope";
import {
  videoService,
  type VideoAvailability,
  type VideoRoom,
} from "../services/video.service";

export function AvailabilityNotice({ a }: { a: VideoAvailability }) {
  const { t } = useLocale();
  const key = availabilityKey(a);
  if (!key) return null;
  return (
    <Card className="border-amber-500/40 bg-amber-500/5">
      <CardContent className="p-4 text-sm">
        {t(`video.availability.${key}`, { balance: a.balance })}
      </CardContent>
    </Card>
  );
}

/** Rooms of the active organization: create, share, join (CF-139). */
export function VideoRoomsPage() {
  const { t } = useLocale();
  const { slug, canUse } = useVideoScope();
  const [status, setStatus] = useState("open");
  const [creating, setCreating] = useState(false);
  const availability = useQuery({
    queryKey: videoKeys.availability(slug),
    queryFn: () => videoService.availability(slug),
    enabled: Boolean(slug && canUse),
  });
  const rooms = useQuery({
    queryKey: videoKeys.rooms(slug, status),
    queryFn: () =>
      videoService.rooms(slug, {
        status: status === "all" ? undefined : status,
        limit: 50,
        offset: 0,
      }),
    enabled: Boolean(slug && canUse),
    refetchInterval: 10_000,
  });

  if (!canUse) {
    return (
      <p className="text-muted-foreground text-sm">{t("video.forbidden")}</p>
    );
  }
  const a = availability.data;
  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Video className="size-7" />}
        title={t("video.rooms.title")}
        description={t("video.rooms.description")}
        actions={
          <Button onClick={() => setCreating(true)} disabled={!a?.ready}>
            <Plus className="size-4" />
            {t("video.rooms.create")}
          </Button>
        }
      />
      {a ? <AvailabilityNotice a={a} /> : null}
      {a ? (
        <p className="text-muted-foreground text-sm">
          {a.unlimited
            ? t("video.minutes.unlimited")
            : t("video.minutes.balance", { balance: a.balance })}
        </p>
      ) : null}
      <Tabs value={status} onValueChange={setStatus}>
        <TabsList>
          <TabsTrigger value="open">{t("video.status.open")}</TabsTrigger>
          <TabsTrigger value="live">{t("video.status.live")}</TabsTrigger>
          <TabsTrigger value="ended">{t("video.status.ended")}</TabsTrigger>
          <TabsTrigger value="all">{t("video.status.all")}</TabsTrigger>
        </TabsList>
      </Tabs>
      <div className="grid gap-3">
        {(rooms.data?.items ?? []).map((room) => (
          <RoomRow key={room.uuid} room={room} />
        ))}
        {rooms.data && rooms.data.items.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("video.rooms.empty")}
          </p>
        ) : null}
      </div>
      {creating && a ? (
        <CreateRoomDialog availability={a} onClose={() => setCreating(false)} />
      ) : null}
    </div>
  );
}

function RoomRow({ room }: { room: VideoRoom }) {
  const { t } = useLocale();
  return (
    <Card>
      <CardContent className="flex flex-wrap items-center gap-3 p-4">
        <div className="min-w-0 flex-1">
          <Link
            href={`/video/${room.uuid}`}
            className="font-medium hover:underline"
          >
            {room.name}
          </Link>
          <div className="text-muted-foreground flex flex-wrap items-center gap-2 text-xs">
            <Badge variant={room.status === "live" ? "default" : "secondary"}>
              {t(`video.status.${room.status}`)}
            </Badge>
            {room.e2ee ? (
              <span className="inline-flex items-center gap-1">
                <Lock className="size-3" />
                {t("video.call.e2ee")}
              </span>
            ) : null}
            {room.scheduled_start_at ? (
              <span>{datetime(room.scheduled_start_at)}</span>
            ) : null}
            {room.waiting > 0 ? (
              <span className="inline-flex items-center gap-1 text-amber-600">
                <Users className="size-3" />
                {t("video.lobby.waiting_count", { count: room.waiting })}
              </span>
            ) : null}
          </div>
        </div>
        {room.guest_link ? (
          <Button
            variant="outline"
            size="sm"
            onClick={async () => {
              await navigator.clipboard.writeText(room.guest_link);
              appToast.success(t("video.links.copied"));
            }}
          >
            <Copy className="size-3.5" />
            {t("video.links.copy_guest")}
          </Button>
        ) : null}
        {room.status !== "ended" ? (
          <Button asChild size="sm">
            <Link href={`/video/${room.uuid}`}>{t("video.rooms.open")}</Link>
          </Button>
        ) : null}
      </CardContent>
    </Card>
  );
}

function CreateRoomDialog({
  availability,
  onClose,
}: {
  availability: VideoAvailability;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useVideoScope();
  const qc = useQueryClient();
  const p = availability.policy;
  const [name, setName] = useState("");
  const [pin, setPin] = useState("");
  const [lobby, setLobby] = useState(true);
  const [record, setRecord] = useState(false);
  const [start, setStart] = useState("");
  const [minutes, setMinutes] = useState(
    String(Math.min(60, p.max_duration_minutes)),
  );
  const create = useMutation({
    mutationFn: () => {
      const startsAt = start ? new Date(start) : undefined;
      const duration = Number(minutes) || p.max_duration_minutes;
      return videoService.create(slug, {
        name: name.trim(),
        pin: pin || undefined,
        lobby: p.require_lobby ? true : lobby,
        record,
        max_duration_minutes: duration,
        scheduled_start_at: startsAt?.toISOString(),
        scheduled_end_at: startsAt
          ? new Date(startsAt.getTime() + duration * 60_000).toISOString()
          : undefined,
      });
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["video", slug] });
      onClose();
    },
    onError: (err) => appToast.error(t(`video.errors.${videoErrorKey(err)}`)),
  });
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("video.rooms.create")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="room-name">{t("video.rooms.name")}</Label>
            <Input
              id="room-name"
              maxLength={160}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="room-start">{t("video.rooms.starts_at")}</Label>
            <Input
              id="room-start"
              type="datetime-local"
              value={start}
              onChange={(e) => setStart(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="room-duration">
              {t("video.rooms.duration", { max: p.max_duration_minutes })}
            </Label>
            <Input
              id="room-duration"
              type="number"
              min={5}
              max={p.max_duration_minutes}
              value={minutes}
              onChange={(e) => setMinutes(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="room-pin">
              {p.require_pin
                ? t("video.rooms.pin_required")
                : t("video.rooms.pin")}
            </Label>
            <Input
              id="room-pin"
              inputMode="numeric"
              pattern="[0-9]{4,12}"
              value={pin}
              onChange={(e) =>
                setPin(e.target.value.replace(/\D/g, "").slice(0, 12))
              }
            />
          </div>
          <label className="flex items-center justify-between gap-3 text-sm">
            {t("video.rooms.lobby")}
            <Switch
              checked={p.require_lobby || lobby}
              disabled={p.require_lobby}
              onCheckedChange={setLobby}
            />
          </label>
          {p.allow_recording ? (
            <label className="flex items-start justify-between gap-3 text-sm">
              <span>
                {t("video.rooms.record")}
                <span className="text-muted-foreground block text-xs">
                  {t("video.rooms.record_hint")}
                </span>
              </span>
              <Switch checked={record} onCheckedChange={setRecord} />
            </label>
          ) : (
            <p className="text-muted-foreground text-xs">
              {t("video.rooms.e2ee_hint")}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={
              !name.trim() ||
              create.isPending ||
              (p.require_pin && pin.length < 4)
            }
            onClick={() => create.mutate()}
          >
            {t("video.rooms.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
