"use client";

import { useQuery } from "@tanstack/react-query";
import { MonitorUp } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

import { isOpen, remoteSupportAction } from "../lib/access";
import {
  remoteSupportKeys,
  useRemoteSupportScope,
} from "../lib/use-remote-support";
import { remoteSupportService } from "../services/remote-support.service";
import { RemoteSupportDialog } from "./remote-support-dialog";

/**
 * Conversation header action (tenant.remote_support.use). Hidden without
 * the permission; with an open session it reopens that session.
 */
export function RemoteSupportButton({
  conversation,
}: {
  conversation: string;
}) {
  const { t } = useLocale();
  const { slug, use, control } = useRemoteSupportScope();
  const [open, setOpen] = useState(false);
  const { visible } = remoteSupportAction({ use, control });
  const sessions = useQuery({
    queryKey: remoteSupportKeys.conversation(slug, conversation),
    queryFn: () => remoteSupportService.forConversation(slug, conversation),
    enabled: Boolean(slug && visible),
    staleTime: 15_000,
  });
  if (!visible) return null;
  const current = (sessions.data?.items ?? []).find((s) => isOpen(s.status));
  const label = current
    ? t("remote_support.action.resume")
    : t("remote_support.action.start");
  return (
    <>
      <span title={label} className="relative inline-flex">
        <Button
          variant="outline"
          size="sm"
          className="h-8 gap-1.5"
          onClick={() => setOpen(true)}
          aria-label={label}
          aria-haspopup="dialog"
        >
          <MonitorUp className="size-3.5" />
        </Button>
        {current ? (
          <span
            aria-hidden
            className="absolute -top-0.5 -right-0.5 size-2 rounded-full bg-emerald-500"
          />
        ) : null}
      </span>
      {open ? (
        <RemoteSupportDialog
          conversation={conversation}
          session={current ?? null}
          onClose={() => {
            setOpen(false);
            void sessions.refetch();
          }}
        />
      ) : null}
    </>
  );
}
