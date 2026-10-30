"use client";

import { ExternalLink, TriangleAlert } from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { CopyField } from "@/features/channels/components/setup-fields";
import {
  apiDocsUrl,
  providerWebhookUrl,
  webhookModeOf,
} from "@/features/channels/lib/channel-setup";
import type { ChannelInstance } from "@/features/channels/types";
import { useLocale } from "@/providers/locale-provider";

/**
 * What to paste into the provider after connecting: the webhook URL (with
 * the token for unsigned callbacks) and, for the API channel, the signing
 * secret. Generated secrets exist only in this form's memory; the API keeps
 * them masked, so this is the one time they are shown.
 */
export function ConnectedStep({
  channel,
  values,
}: {
  channel: ChannelInstance;
  values: Record<string, string>;
}) {
  const { t } = useLocale();
  const type = channel.type;
  const provider = values.provider;
  const mode = webhookModeOf(type, provider);
  const token = mode === "token" ? values.webhook_token : undefined;
  const url = providerWebhookUrl(channel, token);
  const key = provider ? `${type}.${provider}` : type;
  const secret = type === "api" ? values.signing_secret : undefined;
  const showOnce = Boolean(token || secret);
  const docs = type === "api" ? apiDocsUrl(channel) : "";

  return (
    <div className="space-y-4">
      {showOnce ? (
        <Alert>
          <TriangleAlert className="size-4 text-amber-600" />
          <AlertTitle>{t("channels.setup.show_once_title")}</AlertTitle>
          <AlertDescription>{t("channels.setup.show_once")}</AlertDescription>
        </Alert>
      ) : null}
      {mode === "none" ? (
        <p className="text-muted-foreground text-sm">
          {t(`channels.setup.webhook_hint.${key}`)}
        </p>
      ) : (
        <CopyField
          id="connected-webhook"
          label={t(`channels.setup.webhook_label.${type}`)}
          value={url}
          hint={t(`channels.setup.webhook_hint.${key}`)}
        />
      )}
      {secret ? (
        <CopyField
          id="connected-secret"
          label={t("channels.fields.signing_secret")}
          value={secret}
          hint={t("channels.setup.api_signature")}
        />
      ) : null}
      {docs ? (
        <a
          href={docs}
          target="_blank"
          rel="noreferrer"
          className="text-primary inline-flex items-center gap-1 text-sm underline-offset-4 hover:underline"
        >
          <ExternalLink className="size-3.5" />
          {t("channels.setup.api_docs")}
        </a>
      ) : null}
    </div>
  );
}
