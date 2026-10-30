"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { SlackChannelPicker } from "@/features/channels/components/slack-channel-picker";
import {
  channelsKeys,
  useUpdateChannel,
} from "@/features/channels/hooks/use-channels";
import { channelsService } from "@/features/channels/services/channels.service";
import type {
  ChannelInstance,
  MessengerPages,
  OAuthChannelType,
} from "@/features/channels/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";
import { useQueryClient } from "@tanstack/react-query";

/**
 * Return leg of the Meta login and the Slack install
 * (`/channels/connect/{type}?code=&state=`): Instagram connects the account
 * right away; Messenger lists the pages the user manages to pick one; Slack
 * installs the workspace and asks which channels to route (nothing is
 * ingested until one is picked). The code is used once, so a reload after
 * an error starts over from the channels page.
 */
export function OAuthCallback({ type }: { type: OAuthChannelType }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const queryClient = useQueryClient();
  const params = useSearchParams();
  const started = useRef(false);
  const [failed, setFailed] = useState(false);
  const [pages, setPages] = useState<MessengerPages | null>(null);
  const [saving, setSaving] = useState(false);
  const [slack, setSlack] = useState<ChannelInstance | null>(null);
  const [channelIds, setChannelIds] = useState<string[]>([]);
  const update = useUpdateChannel();

  const done = () => {
    void queryClient.invalidateQueries({ queryKey: channelsKeys.all(slug) });
    appToast.success(t("channels.toast.connected"));
    router.replace("/channels");
  };

  const code = params.get("code") ?? "";
  const state = params.get("state") ?? "";
  // Meta and Slack send ?error=… when the user cancels the dialog.
  const invalid = !code || !state || params.get("error") !== null;

  useEffect(() => {
    if (started.current || invalid) return;
    started.current = true;
    const run =
      type === "instagram"
        ? channelsService.instagramComplete(slug, code, state).then(done)
        : type === "slack"
          ? channelsService.slackComplete(slug, code, state).then((inst) => {
              void queryClient.invalidateQueries({
                queryKey: channelsKeys.all(slug),
              });
              setSlack(inst);
            })
          : channelsService.messengerComplete(slug, code, state).then(setPages);
    run.catch(() => setFailed(true));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const select = (pageId: string) => {
    if (!pages) return;
    setSaving(true);
    channelsService
      .messengerSelectPage(slug, pages.selection, pageId)
      .then(done)
      .catch(() => setSaving(false));
  };

  return (
    <EntityPage
      title={t(`channels.types.${type}`)}
      description={t("channels.oauth.callback")}
      permission={permissions.tenant.channels.manage}
    >
      {failed || invalid ? (
        <ErrorState
          title={t("channels.error")}
          description={t("channels.oauth.failed")}
          action={
            <Button onClick={() => router.replace("/channels")}>
              {t("channels.oauth.back")}
            </Button>
          }
        />
      ) : slack ? (
        <div className="max-w-lg space-y-4">
          <SlackChannelPicker
            uuid={slack.uuid}
            value={channelIds}
            onChange={setChannelIds}
          />
          <div className="flex gap-2">
            <Button
              disabled={update.isPending}
              onClick={() =>
                update.mutate(
                  {
                    uuid: slack.uuid,
                    body: {
                      settings: { ...slack.settings, channel_ids: channelIds },
                    },
                  },
                  { onSuccess: () => router.replace("/channels") },
                )
              }
            >
              {t("channels.save")}
            </Button>
            <Button variant="ghost" onClick={() => router.replace("/channels")}>
              {t("channels.slack.later")}
            </Button>
          </div>
        </div>
      ) : pages ? (
        pages.pages.length === 0 ? (
          <ErrorState
            title={t("channels.error")}
            description={t("channels.oauth.no_pages")}
          />
        ) : (
          <div className="max-w-md space-y-2">
            <p className="text-sm">{t("channels.oauth.pick_page")}</p>
            {pages.pages.map((p) => (
              <Button
                key={p.id}
                variant="outline"
                className="w-full justify-start"
                disabled={saving}
                onClick={() => select(p.id)}
              >
                {p.name}
              </Button>
            ))}
          </div>
        )
      ) : (
        <Skeleton className="h-24 max-w-md" />
      )}
    </EntityPage>
  );
}
