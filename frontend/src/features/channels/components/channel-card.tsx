"use client";

import {
  Activity,
  FileText,
  MoreHorizontal,
  Pause,
  PhoneCall,
  Play,
  PlugZap,
  Settings2,
  Trash2,
} from "lucide-react";
import Link from "next/link";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ChannelIcon } from "@/features/channels/components/channel-icon";
import type { ChannelInstance } from "@/features/channels/types";
import { relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const healthTone = {
  ok: "success",
  warning: "warning",
  error: "danger",
  paused: "secondary",
  unknown: "outline",
} as const;

export type ChannelCardActions = {
  onCheck: (c: ChannelInstance) => void;
  onReconnect: (c: ChannelInstance) => void;
  onToggle: (c: ChannelInstance) => void;
  onSettings: (c: ChannelInstance) => void;
  onTemplates: (c: ChannelInstance) => void;
  onDelete: (c: ChannelInstance) => void;
};

/**
 * One connected inbox: type, account, health (connection, token expiry,
 * webhook last-seen, 24h error rate) and its actions.
 */
export function ChannelCard({
  channel,
  canManage,
  busy,
  actions,
}: {
  channel: ChannelInstance;
  canManage: boolean;
  busy: boolean;
  actions: ChannelCardActions;
}) {
  const { t, locale } = useLocale();
  const h = channel.health;
  const when = (v?: string | null) =>
    v ? relativeDatetime(v, locale) : t("channels.health.never");
  const account =
    channel.credential_hints.username ??
    channel.credential_hints.from_number ??
    channel.credential_hints.msgheader ??
    (channel.settings.display_phone_number as string | undefined) ??
    channel.external_id;
  const total = h.sent_24h + h.failed_24h;

  return (
    <Card className="gap-4">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <ChannelIcon type={channel.type} className="size-4" />
          <span className="truncate">{channel.name}</span>
        </CardTitle>
        <CardDescription className="truncate">
          {t(`channels.types.${channel.type}`)}
          {account ? ` · ${account}` : ""}
        </CardDescription>
        {canManage ? (
          <CardAction>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t("channels.actions.menu")}
                >
                  <MoreHorizontal className="size-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => actions.onSettings(channel)}>
                  <Settings2 className="size-4" />
                  {t("channels.actions.settings")}
                </DropdownMenuItem>
                {channel.type === "whatsapp" ? (
                  <DropdownMenuItem
                    onSelect={() => actions.onTemplates(channel)}
                  >
                    <FileText className="size-4" />
                    {t("channels.actions.templates")}
                  </DropdownMenuItem>
                ) : null}
                <DropdownMenuItem onSelect={() => actions.onToggle(channel)}>
                  {channel.status === "paused" ? (
                    <Play className="size-4" />
                  ) : (
                    <Pause className="size-4" />
                  )}
                  {channel.status === "paused"
                    ? t("channels.actions.resume")
                    : t("channels.actions.pause")}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  variant="destructive"
                  onSelect={() => actions.onDelete(channel)}
                >
                  <Trash2 className="size-4" />
                  {t("channels.actions.disconnect")}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </CardAction>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={healthTone[h.state]}>
            {t(`channels.health.state.${h.state}`)}
          </Badge>
          {h.reconnect ? (
            <Badge variant="warning">{t("channels.health.reconnect")}</Badge>
          ) : null}
        </div>
        {h.detail ? (
          <p className="text-muted-foreground line-clamp-2 text-xs break-words">
            {h.detail.startsWith("Bakiye:")
              ? h.detail
              : h.detail.startsWith("identity_rejected:")
                ? // CF-195: the widget's last identify calls were rejected.
                  t("channels.health.detail.identity_rejected", {
                    reason: t(
                      `channels.health.identity_code.${h.detail.slice("identity_rejected:".length)}`,
                    ),
                  })
                : t(`channels.health.detail.${h.detail}`, {
                    defaultValue: h.detail,
                  })}
          </p>
        ) : null}
        <dl className="text-muted-foreground grid grid-cols-2 gap-x-3 gap-y-1 text-xs">
          <dt>{t("channels.health.last_inbound")}</dt>
          <dd className="text-foreground text-right">
            {when(channel.last_inbound_at)}
          </dd>
          <dt>{t("channels.health.last_webhook")}</dt>
          <dd className="text-foreground text-right">
            {when(h.last_webhook_at)}
          </dd>
          <dt>{t("channels.health.sent_24h")}</dt>
          <dd className="text-foreground text-right tabular-nums">
            {total === 0
              ? "0"
              : t("channels.health.sent_failed", {
                  sent: h.sent_24h,
                  failed: h.failed_24h,
                  rate: Math.round(h.error_rate * 100),
                })}
          </dd>
          {h.token_expires_at ? (
            <>
              <dt>{t("channels.health.token_expires")}</dt>
              <dd className="text-foreground text-right">
                {relativeDatetime(h.token_expires_at, locale)}
              </dd>
            </>
          ) : null}
          <dt>{t("channels.health.checked")}</dt>
          <dd className="text-foreground text-right">{when(h.checked_at)}</dd>
        </dl>
        {channel.type === "voice" ? (
          <Link
            href={`/channels/${channel.uuid}/calls`}
            className="text-primary inline-flex items-center gap-1 text-xs underline-offset-4 hover:underline"
          >
            <PhoneCall className="size-3.5" />
            {t("channels.actions.calls")}
          </Link>
        ) : null}
      </CardContent>
      {canManage ? (
        <CardFooter className="gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={busy}
            onClick={() => actions.onCheck(channel)}
          >
            <Activity className="size-4" />
            {t("channels.actions.check")}
          </Button>
          {h.reconnect ? (
            <Button
              size="sm"
              disabled={busy}
              onClick={() => actions.onReconnect(channel)}
            >
              <PlugZap className="size-4" />
              {t("channels.actions.reconnect")}
            </Button>
          ) : null}
        </CardFooter>
      ) : null}
    </Card>
  );
}
