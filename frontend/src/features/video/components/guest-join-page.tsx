"use client";

import { useQuery } from "@tanstack/react-query";
import { Circle, Clock, Lock, Video } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useLocale } from "@/providers/locale-provider";

import {
  PublicVideoError,
  publicVideoService,
  type VideoJoinGrant,
} from "../services/video.service";
import { VideoCall } from "./video-call";

const POLL_MS = 3000;

function errorKey(err: unknown) {
  return err instanceof PublicVideoError ? err.code : "UNKNOWN";
}

/**
 * Guest flow: room info → name (+ PIN) → lobby (polls the ticket until the
 * host admits) → call. The ticket lives in memory only; a reload rejoins.
 */
export function GuestJoinPage({ code }: { code: string }) {
  const { t } = useLocale();
  const router = useRouter();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [pin, setPin] = useState("");
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  const [grant, setGrant] = useState<VideoJoinGrant | null>(null);
  const [left, setLeft] = useState(false);

  const info = useQuery({
    queryKey: ["video-public-room", code],
    queryFn: () => publicVideoService.room(code),
    retry: false,
  });

  // Lobby: poll until admitted, denied or the room ends.
  const ticket = grant?.status === "waiting" ? grant.ticket : undefined;
  useEffect(() => {
    if (!ticket) return;
    const id = window.setInterval(async () => {
      try {
        const next = await publicVideoService.ticket(ticket);
        if (next.status !== "waiting") setGrant({ ...next, ticket });
      } catch (err) {
        setFailure(errorKey(err));
        setGrant(null);
      }
    }, POLL_MS);
    return () => window.clearInterval(id);
  }, [ticket]);

  if (info.isLoading) {
    return <Shell>{t("video.guest.loading")}</Shell>;
  }
  if (info.error || !info.data) {
    return (
      <Shell>
        <p className="text-sm">{t(`video.errors.${errorKey(info.error)}`)}</p>
      </Shell>
    );
  }
  const room = info.data;

  if (room.link_role === "agent") {
    return (
      <Shell title={room.name} org={room.organization_name}>
        <p className="text-sm">{t("video.guest.agent_link")}</p>
        <Button onClick={() => router.push(`/video/${room.uuid}`)}>
          {t("video.guest.agent_open")}
        </Button>
      </Shell>
    );
  }
  if (left) {
    return (
      <Shell title={room.name} org={room.organization_name}>
        <p className="text-sm">{t("video.guest.left")}</p>
        <Button variant="outline" onClick={() => window.location.reload()}>
          {t("video.guest.rejoin")}
        </Button>
      </Shell>
    );
  }
  if (grant?.status === "admitted" && grant.token) {
    return (
      <div className="bg-background fixed inset-0 flex flex-col">
        <VideoCall grant={grant} onLeave={() => setLeft(true)} />
      </div>
    );
  }
  if (grant && grant.status !== "waiting") {
    return (
      <Shell title={room.name} org={room.organization_name}>
        <p className="text-sm">{t(`video.guest.status.${grant.status}`)}</p>
      </Shell>
    );
  }
  if (grant?.status === "waiting") {
    return (
      <Shell title={room.name} org={room.organization_name}>
        <Clock className="text-muted-foreground size-8 animate-pulse" />
        <p className="text-sm">{t("video.guest.waiting")}</p>
      </Shell>
    );
  }

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setFailure(null);
    try {
      setGrant(
        await publicVideoService.join(code, {
          name: name.trim(),
          email: email.trim() || undefined,
          pin: room.pin_required ? pin : undefined,
        }),
      );
    } catch (err) {
      setFailure(errorKey(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Shell title={room.name} org={room.organization_name}>
      <ul className="text-muted-foreground space-y-1 text-xs">
        {room.e2ee ? (
          <li className="flex items-center gap-1.5">
            <Lock className="size-3.5" /> {t("video.guest.e2ee")}
          </li>
        ) : null}
        {room.recording ? (
          <li className="flex items-center gap-1.5 text-red-600 dark:text-red-400">
            <Circle className="size-3 fill-current" />{" "}
            {t("video.guest.recording_notice")}
          </li>
        ) : null}
        {room.lobby ? <li>{t("video.guest.lobby_notice")}</li> : null}
      </ul>
      <form onSubmit={submit} className="w-full space-y-3 text-left">
        <div className="space-y-1.5">
          <Label htmlFor="video-name">{t("video.guest.name")}</Label>
          <Input
            id="video-name"
            required
            maxLength={120}
            autoComplete="name"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="video-email">{t("video.guest.email")}</Label>
          <Input
            id="video-email"
            type="email"
            autoComplete="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </div>
        {room.pin_required ? (
          <div className="space-y-1.5">
            <Label htmlFor="video-pin">{t("video.guest.pin")}</Label>
            <Input
              id="video-pin"
              required
              inputMode="numeric"
              pattern="[0-9]{4,12}"
              autoComplete="one-time-code"
              value={pin}
              onChange={(e) => setPin(e.target.value)}
            />
          </div>
        ) : null}
        {failure ? (
          <p className="text-destructive text-sm">
            {t(`video.errors.${failure}`)}
          </p>
        ) : null}
        <Button
          type="submit"
          className="w-full"
          disabled={busy || !name.trim()}
        >
          {t("video.guest.join")}
        </Button>
        <p className="text-muted-foreground text-xs">
          {t("video.guest.permissions_hint")}
        </p>
      </form>
    </Shell>
  );
}

function Shell({
  title,
  org,
  children,
}: {
  title?: string;
  org?: string;
  children: React.ReactNode;
}) {
  return (
    <main className="bg-muted/30 flex min-h-dvh items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardContent className="flex flex-col items-center gap-4 p-6 text-center">
          <Video className="text-primary size-8" />
          {org ? (
            <p className="text-muted-foreground text-xs uppercase">{org}</p>
          ) : null}
          {title ? <h1 className="text-xl font-semibold">{title}</h1> : null}
          {children}
        </CardContent>
      </Card>
    </main>
  );
}
