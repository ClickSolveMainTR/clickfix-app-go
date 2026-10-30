"use client";

import "@livekit/components-styles";

import {
  LiveKitRoom,
  RoomAudioRenderer,
  VideoConference,
} from "@livekit/components-react";
import {
  ExternalE2EEKeyProvider,
  isE2EESupported,
  Room,
  type RoomOptions,
} from "livekit-client";
import { Circle, Lock, ShieldAlert } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";

import { useLocale } from "@/providers/locale-provider";

import type { VideoJoinGrant } from "../services/video.service";

type Props = {
  grant: VideoJoinGrant;
  /** Called when the participant leaves or is disconnected. */
  onLeave: () => void;
  /** Extra controls (lobby panel, recording) rendered above the call. */
  toolbar?: ReactNode;
};

/**
 * One LiveKit call. Encrypted rooms get an E2EE key from the API only after
 * admission; media and chat are then encrypted in the browser with
 * `ExternalE2EEKeyProvider` so the media server never sees them.
 */
export function VideoCall({ grant, onLeave, toolbar }: Props) {
  const { t } = useLocale();
  const [room, setRoom] = useState<Room | null>(null);
  const [error, setError] = useState<string | null>(null);
  const encrypted = Boolean(grant.e2ee_key);

  useEffect(() => {
    let cancelled = false;
    let worker: Worker | undefined;
    const options: RoomOptions = { adaptiveStream: true, dynacast: true };
    const setup = async () => {
      if (encrypted) {
        if (!isE2EESupported()) {
          setError("e2ee_unsupported");
          return;
        }
        const keyProvider = new ExternalE2EEKeyProvider();
        worker = new Worker(
          new URL("livekit-client/e2ee-worker", import.meta.url),
        );
        options.encryption = { keyProvider, worker };
        const r = new Room(options);
        await keyProvider.setKey(grant.e2ee_key as string);
        await r.setE2EEEnabled(true);
        if (!cancelled) setRoom(r);
        return;
      }
      if (!cancelled) setRoom(new Room(options));
    };
    setup().catch(() => !cancelled && setError("connect_failed"));
    return () => {
      cancelled = true;
      worker?.terminate();
    };
  }, [encrypted, grant.e2ee_key]);

  if (error) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center">
        <ShieldAlert className="text-destructive size-10" />
        <p className="max-w-md text-sm">{t(`video.call.error.${error}`)}</p>
      </div>
    );
  }
  if (!room || !grant.token || !grant.server_url) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center text-sm">
        {t("video.call.connecting")}
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-wrap items-center gap-3 border-b px-4 py-2 text-xs">
        <span className="font-medium">{grant.room.name}</span>
        {encrypted ? (
          <span className="inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400">
            <Lock className="size-3.5" />
            {t("video.call.e2ee")}
          </span>
        ) : null}
        {grant.room.recording ? (
          <span className="inline-flex items-center gap-1 text-red-600 dark:text-red-400">
            <Circle className="size-3 fill-current" />
            {t("video.call.recordable")}
          </span>
        ) : null}
        <div className="ml-auto flex items-center gap-2">{toolbar}</div>
      </div>
      <LiveKitRoom
        room={room}
        serverUrl={grant.server_url}
        token={grant.token}
        connect
        audio
        video={grant.room.allow_camera}
        onDisconnected={onLeave}
        onError={() => setError("connect_failed")}
        data-lk-theme="default"
        className="min-h-0 flex-1"
      >
        <VideoConference />
        <RoomAudioRenderer />
      </LiveKitRoom>
    </div>
  );
}
