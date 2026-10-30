"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { Video } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  availabilityKey,
  availabilityLabel,
  useVideoScope,
  videoErrorKey,
  videoKeys,
} from "../lib/use-video-scope";
import { videoService } from "../services/video.service";

/**
 * Inbox header action: open a video room linked to the conversation. When
 * the organization requires a PIN, it is asked for first.
 */
export function StartVideoButton({
  conversation,
  name,
}: {
  conversation: string;
  name: string;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const { slug, canUse } = useVideoScope();
  const [asking, setAsking] = useState(false);
  const [pin, setPin] = useState("");
  const availability = useQuery({
    queryKey: videoKeys.availability(slug),
    queryFn: () => videoService.availability(slug),
    enabled: Boolean(slug && canUse),
  });
  const pinRequired = Boolean(availability.data?.policy.require_pin);
  const create = useMutation({
    mutationFn: () =>
      videoService.create(slug, {
        name: name || t("video.rooms.conversation_room"),
        conversation_uuid: conversation,
        pin: pin || undefined,
      }),
    onSuccess: (room) => router.push(`/video/${room.uuid}`),
    onError: (err) => appToast.error(t(`video.errors.${videoErrorKey(err)}`)),
  });
  if (!canUse) return null;
  // Not configured / turned off / no plan / no minutes: say why instead of
  // letting the click fail (VIDEO_NOT_CONFIGURED and friends).
  const a = availability.data;
  const blockedKey = a ? availabilityKey(a) : "";
  const blocked = Boolean(blockedKey) && blockedKey !== "low_balance";
  const label = blocked
    ? t(availabilityLabel[blockedKey] ?? "video.errors.VIDEO_DISABLED", {
        balance: a?.balance ?? 0,
      })
    : t("video.rooms.start_from_conversation");
  return (
    <>
      <span title={label} className="inline-flex">
        <Button
          variant="outline"
          size="sm"
          className="h-8 gap-1.5"
          disabled={create.isPending || blocked}
          onClick={() => (pinRequired ? setAsking(true) : create.mutate())}
          aria-label={label}
        >
          <Video className="size-3.5" />
        </Button>
      </span>
      <Dialog
        open={asking}
        onOpenChange={(o) => {
          setAsking(o);
          if (!o) setPin("");
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t("video.rooms.start_from_conversation")}
            </DialogTitle>
          </DialogHeader>
          <form
            id="conversation-call-pin"
            className="space-y-1.5"
            onSubmit={(e) => {
              e.preventDefault();
              if (pin.length >= 4) create.mutate();
            }}
          >
            <Label htmlFor="conversation-call-pin-input">
              {t("video.rooms.pin_required")}
            </Label>
            <Input
              id="conversation-call-pin-input"
              autoFocus
              inputMode="numeric"
              pattern="[0-9]{4,12}"
              value={pin}
              onChange={(e) =>
                setPin(e.target.value.replace(/\D/g, "").slice(0, 12))
              }
            />
            <p className="text-muted-foreground text-xs">
              {t("video.links.pin_hint")}
            </p>
          </form>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAsking(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              form="conversation-call-pin"
              disabled={pin.length < 4 || create.isPending}
            >
              {t("video.rooms.create")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
