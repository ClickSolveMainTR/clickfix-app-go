"use client";

import "rrweb/dist/style.css";

import { useQuery } from "@tanstack/react-query";
import { Pause, Play, RotateCcw } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { Replayer } from "rrweb";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

import { useInboxScope } from "../hooks/use-inbox";
import {
  createReplayer,
  formatClock,
  useFittedViewport,
} from "../lib/replayer";
import { replayService } from "../services/replay.service";

const SPEEDS = [1, 2, 4] as const;

/**
 * Plays one masked session recording (CF-126). The manifest request is
 * audited as a playback; chunks are fetched once and kept only in memory.
 */
export function ReplayPlayer({
  conversationUuid,
  sessionUuid,
}: {
  conversationUuid: string;
  sessionUuid: string;
}) {
  const { t } = useLocale();
  const { org } = useInboxScope();
  const data = useQuery({
    queryKey: ["inbox", org, "replay", conversationUuid, sessionUuid],
    queryFn: () => replayService.events(conversationUuid, sessionUuid),
    enabled: Boolean(org),
    // Signed URLs expire in two minutes and every fetch is audited.
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
  });
  const root = useRef<HTMLDivElement | null>(null);
  const player = useRef<Replayer | null>(null);
  const { frame, size, scale, bind } = useFittedViewport(480);
  const [playing, setPlaying] = useState(false);
  const [current, setCurrent] = useState(0);
  const [total, setTotal] = useState(0);
  const [speed, setSpeed] = useState<(typeof SPEEDS)[number]>(1);
  const [broken, setBroken] = useState(false);

  const events = data.data?.events;
  useEffect(() => {
    const el = root.current;
    if (!el || !events || events.length < 2) return;
    let cancelled = false;
    let made: Replayer | null = null;
    void createReplayer(el, events, false)
      .then((r) => {
        if (cancelled) {
          r.destroy();
          return;
        }
        made = r;
        player.current = r;
        bind(r);
        r.on("finish", () => setPlaying(false));
        setTotal(r.getMetaData().totalTime);
        r.pause(0);
      })
      .catch(() => setBroken(true));
    return () => {
      cancelled = true;
      bind(null);
      player.current = null;
      made?.destroy();
      el.replaceChildren();
    };
  }, [events, bind]);

  useEffect(() => {
    if (!playing) return;
    let raf = 0;
    const tick = () => {
      setCurrent(player.current?.getCurrentTime() ?? 0);
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [playing]);

  const toggle = () => {
    const r = player.current;
    if (!r) return;
    if (playing) {
      r.pause();
      setPlaying(false);
    } else {
      r.play(current >= total ? 0 : current);
      setPlaying(true);
    }
  };
  const seek = (ms: number) => {
    const r = player.current;
    if (!r) return;
    setCurrent(ms);
    if (playing) r.play(ms);
    else r.pause(ms);
  };
  const changeSpeed = (s: (typeof SPEEDS)[number]) => {
    setSpeed(s);
    player.current?.setConfig({ speed: s });
  };

  if (data.isError || broken) {
    return (
      <p className="text-destructive text-sm" role="alert">
        {isApiError(data.error)
          ? data.error.message
          : t("inbox.replay.load_failed")}
      </p>
    );
  }
  const empty = data.isSuccess && (events?.length ?? 0) < 2;
  return (
    <div className="space-y-3">
      {data.isLoading && <Skeleton className="aspect-video w-full" />}
      {empty && (
        <p className="text-muted-foreground py-8 text-center text-sm">
          {t("inbox.replay.empty")}
        </p>
      )}
      <div
        ref={frame}
        className="bg-muted/40 relative w-full overflow-hidden rounded-lg border"
        style={{
          height: size ? size.height * scale : undefined,
          display: data.isSuccess && !empty ? undefined : "none",
        }}
      >
        <div ref={root} className="absolute top-0 left-0" />
      </div>
      {data.isSuccess && !empty && (
        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            size="icon"
            variant="outline"
            className="size-8"
            aria-label={
              playing ? t("inbox.replay.pause") : t("inbox.replay.play")
            }
            onClick={toggle}
          >
            {playing ? (
              <Pause className="size-4" />
            ) : (
              <Play className="size-4" />
            )}
          </Button>
          <Button
            type="button"
            size="icon"
            variant="ghost"
            className="size-8"
            aria-label={t("inbox.replay.restart")}
            onClick={() => seek(0)}
          >
            <RotateCcw className="size-4" />
          </Button>
          <input
            type="range"
            min={0}
            max={Math.max(total, 1)}
            step={100}
            value={Math.min(current, total)}
            aria-label={t("inbox.replay.seek")}
            className="accent-primary min-w-32 flex-1"
            onChange={(e) => seek(Number(e.target.value))}
          />
          <span className="text-muted-foreground text-xs tabular-nums">
            {formatClock(current)} / {formatClock(total)}
          </span>
          <div
            className="flex gap-1"
            role="group"
            aria-label={t("inbox.replay.speed")}
          >
            {SPEEDS.map((s) => (
              <Button
                key={s}
                type="button"
                size="sm"
                variant={speed === s ? "secondary" : "ghost"}
                className="h-8 px-2 text-xs"
                aria-pressed={speed === s}
                onClick={() => changeSpeed(s)}
              >
                {s}×
              </Button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
