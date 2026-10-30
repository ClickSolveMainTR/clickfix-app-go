"use client";

import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { BrandSelect } from "@/features/brands/components/brand-select";
import { EmailChannelFields } from "@/features/channels/components/email-channel-fields";
import {
  GoogleChatSpacePicker,
  googleChatSpaceIds,
} from "@/features/channels/components/google-chat-space-picker";
import {
  CopyField,
  SetupFields,
} from "@/features/channels/components/setup-fields";
import {
  SlackChannelPicker,
  slackChannelIds,
} from "@/features/channels/components/slack-channel-picker";
import { useUpdateChannel } from "@/features/channels/hooks/use-channels";
import {
  credentialsOf,
  providerWebhookUrl,
  SETUP_TYPES,
  webhookModeOf,
} from "@/features/channels/lib/channel-setup";
import {
  emailComplete,
  emailCredentials,
  emailSettings,
  emailValuesOf,
} from "@/features/channels/lib/email-setup";
import type {
  ChannelInstance,
  ChannelOperationalSettings,
} from "@/features/channels/types";
import { useLocale } from "@/providers/locale-provider";

const DAYS = [1, 2, 3, 4, 5, 6, 0];
const DEFAULT_HOURS = {
  timezone: "Europe/Istanbul",
  start: "09:00",
  end: "18:00",
  days: [1, 2, 3, 4, 5],
};

/**
 * Per-inbox settings (CF-61): name, AI agent, automatic greeting and
 * business hours. They live in the instance settings next to the channel's
 * own keys, which are kept untouched. The webhook URL is shown
 * for channels configured on the provider side. SMS, voice, Teams, Google
 * Chat and API channels can change credentials (secrets are write-only:
 * empty keeps the stored value); Slack and Google Chat edit their routed
 * channel / room allowlist.
 */
export function ChannelSettingsDialog({
  channel,
  onOpenChange,
}: {
  channel: ChannelInstance;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const update = useUpdateChannel();
  const initial = channel.settings as ChannelOperationalSettings &
    Record<string, unknown>;
  const [ws, setWs] = useState(String(initial.ws_url ?? ""));
  const [domain, setDomain] = useState(String(initial.domain ?? ""));
  const [ice, setIce] = useState(JSON.stringify(initial.ice_servers ?? []));
  const [iceError, setIceError] = useState(false);
  const [name, setName] = useState(channel.name);
  const [brandId, setBrandId] = useState(channel.brand_id ?? null);
  // Sent only when changed: an omitted brand_id keeps the current brand.
  const brandPatch =
    brandId !== (channel.brand_id ?? null) ? { brand_id: brandId } : {};
  const [s, setS] = useState<ChannelOperationalSettings>({
    ai_enabled: initial.ai_enabled ?? false,
    greeting_enabled: initial.greeting_enabled ?? false,
    greeting: initial.greeting ?? "",
    business_hours_enabled: initial.business_hours_enabled ?? false,
    business_hours: initial.business_hours ?? DEFAULT_HOURS,
  });
  const hours = s.business_hours ?? DEFAULT_HOURS;
  const setHours = (patch: Partial<typeof hours>) =>
    setS((prev) => ({ ...prev, business_hours: { ...hours, ...patch } }));

  const setup = SETUP_TYPES.has(channel.type);
  const provider = channel.credential_hints.provider;
  const [creds, setCreds] = useState<Record<string, string>>(
    provider ? { provider } : {},
  );
  const [commercial, setCommercial] = useState(initial.commercial === true);
  const [channelIds, setChannelIds] = useState(slackChannelIds(initial));
  const [spaceIds, setSpaceIds] = useState(googleChatSpaceIds(initial));
  const isEmail = channel.type === "email";
  const [emailInitial] = useState(() =>
    isEmail ? emailValuesOf(channel) : {},
  );
  const [email, setEmail] = useState(emailInitial);
  const mode = webhookModeOf(channel.type, provider);
  // A new token is known here, so the full callback URL can be shown.
  const webhook = providerWebhookUrl(
    channel,
    mode === "token" ? creds.webhook_token : undefined,
  );

  const save = () => {
    const settings: Record<string, unknown> = { ...initial, ...s };
    if (channel.type === "voice" && provider === "netgsm") {
      try {
        const servers: unknown = JSON.parse(ice);
        if (!Array.isArray(servers)) throw new Error();
        Object.assign(settings, { ws_url: ws, domain, ice_servers: servers });
        setIceError(false);
      } catch {
        setIceError(true);
        return;
      }
    }
    if (channel.type === "sms") settings.commercial = commercial;
    if (channel.type === "slack") settings.channel_ids = channelIds;
    if (channel.type === "google_chat") settings.space_ids = spaceIds;
    if (isEmail) {
      Object.assign(settings, emailSettings(email));
      const changed = Object.keys({ ...emailInitial, ...email }).some(
        (k) => (email[k] ?? "") !== (emailInitial[k] ?? ""),
      );
      update.mutate(
        {
          uuid: channel.uuid,
          body: {
            name: name.trim(),
            ...brandPatch,
            settings,
            // Unchanged credentials are not resent (no needless re-login).
            ...(changed ? { credentials: emailCredentials(email) } : {}),
          },
        },
        { onSuccess: () => onOpenChange(false) },
      );
      return;
    }
    const credentials = setup ? credentialsOf(channel.type, creds) : {};
    // Only the provider itself is left: nothing changed.
    delete credentials.provider;
    if (provider && Object.keys(credentials).length > 0) {
      credentials.provider = provider;
    }
    update.mutate(
      {
        uuid: channel.uuid,
        body: {
          name: name.trim(),
          ...brandPatch,
          settings,
          ...(Object.keys(credentials).length > 0 ? { credentials } : {}),
        },
      },
      { onSuccess: () => onOpenChange(false) },
    );
  };

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("channels.settings.title")}</DialogTitle>
          <DialogDescription>
            {t(`channels.types.${channel.type}`)}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-5">
          <div className="space-y-1">
            <Label htmlFor="channel-name">{t("channels.name")}</Label>
            <Input
              id="channel-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>

          <BrandSelect
            id="channel-brand"
            value={brandId}
            onChange={setBrandId}
            hint={t("brands.select.channel_hint")}
          />

          {channel.type === "slack" ? (
            <SlackChannelPicker
              uuid={channel.uuid}
              value={channelIds}
              onChange={setChannelIds}
            />
          ) : null}

          {channel.type === "google_chat" ? (
            <GoogleChatSpacePicker
              uuid={channel.uuid}
              value={spaceIds}
              onChange={setSpaceIds}
            />
          ) : null}

          {isEmail ? (
            <EmailChannelFields
              values={email}
              onChange={setEmail}
              editing
              hints={channel.credential_hints}
            />
          ) : null}

          {setup ? (
            <div className="space-y-2">
              <Label>{t("channels.setup.credentials")}</Label>
              <SetupFields
                type={channel.type}
                values={creds}
                onChange={setCreds}
                editing
                hints={channel.credential_hints}
                commercial={commercial}
                onCommercialChange={setCommercial}
              />
            </div>
          ) : null}

          <div className="flex items-center justify-between gap-4">
            <div>
              <Label htmlFor="ai-enabled">{t("channels.settings.ai")}</Label>
              <p className="text-muted-foreground text-xs">
                {t("channels.settings.ai_hint")}
              </p>
            </div>
            <Switch
              id="ai-enabled"
              checked={s.ai_enabled}
              onCheckedChange={(v) => setS({ ...s, ai_enabled: v })}
            />
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between gap-4">
              <Label htmlFor="greeting-enabled">
                {t("channels.settings.greeting")}
              </Label>
              <Switch
                id="greeting-enabled"
                checked={s.greeting_enabled}
                onCheckedChange={(v) => setS({ ...s, greeting_enabled: v })}
              />
            </div>
            {s.greeting_enabled ? (
              <Textarea
                value={s.greeting}
                maxLength={1000}
                placeholder={t("channels.settings.greeting_placeholder")}
                onChange={(e) => setS({ ...s, greeting: e.target.value })}
              />
            ) : null}
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between gap-4">
              <Label htmlFor="hours-enabled">
                {t("channels.settings.hours")}
              </Label>
              <Switch
                id="hours-enabled"
                checked={s.business_hours_enabled}
                onCheckedChange={(v) =>
                  setS({ ...s, business_hours_enabled: v })
                }
              />
            </div>
            {s.business_hours_enabled ? (
              <div className="space-y-2 rounded-md border p-3">
                <div className="flex flex-wrap gap-1">
                  {DAYS.map((d) => {
                    const on = hours.days.includes(d);
                    return (
                      <Button
                        key={d}
                        type="button"
                        size="sm"
                        variant={on ? "default" : "outline"}
                        onClick={() =>
                          setHours({
                            days: on
                              ? hours.days.filter((x) => x !== d)
                              : [...hours.days, d],
                          })
                        }
                      >
                        {t(`channels.settings.day.${d}`)}
                      </Button>
                    );
                  })}
                </div>
                <div className="grid grid-cols-3 gap-2">
                  <Input
                    type="time"
                    aria-label={t("channels.settings.hours_start")}
                    value={hours.start}
                    onChange={(e) => setHours({ start: e.target.value })}
                  />
                  <Input
                    type="time"
                    aria-label={t("channels.settings.hours_end")}
                    value={hours.end}
                    onChange={(e) => setHours({ end: e.target.value })}
                  />
                  <Input
                    aria-label={t("channels.settings.timezone")}
                    value={hours.timezone}
                    onChange={(e) => setHours({ timezone: e.target.value })}
                  />
                </div>
              </div>
            ) : null}
          </div>

          {mode !== "none" || !setup ? (
            <CopyField
              id="channel-webhook"
              label={
                setup
                  ? t(`channels.setup.webhook_label.${channel.type}`)
                  : t("channels.settings.webhook")
              }
              value={webhook}
              hint={
                mode === "token" && !creds.webhook_token
                  ? t("channels.setup.token_hidden")
                  : setup
                    ? t(
                        `channels.setup.webhook_hint.${provider ? `${channel.type}.${provider}` : channel.type}`,
                      )
                    : t("channels.settings.webhook_hint")
              }
            />
          ) : null}
        </div>
        {channel.type === "voice" && provider === "netgsm" && (
          <section className="space-y-3">
            <h3>Netsantral Webphone</h3>
            <Label htmlFor="netgsm-ws">WSS URL</Label>
            <Input
              id="netgsm-ws"
              value={ws}
              onChange={(e) => setWs(e.target.value)}
              placeholder="wss://"
            />
            <Label htmlFor="netgsm-domain">SIP domain</Label>
            <Input
              id="netgsm-domain"
              value={domain}
              onChange={(e) => setDomain(e.target.value)}
            />
            <Label htmlFor="netgsm-ice">ICE sunucuları (JSON)</Label>
            <Textarea
              id="netgsm-ice"
              value={ice}
              onChange={(e) => setIce(e.target.value)}
            />
            {iceError && <p role="alert">Geçerli bir JSON dizisi girin.</p>}
          </section>
        )}
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t("channels.cancel")}
          </Button>
          <Button
            disabled={
              update.isPending ||
              name.trim() === "" ||
              (isEmail && !emailComplete(email, channel.credential_hints))
            }
            onClick={save}
          >
            {t("channels.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
