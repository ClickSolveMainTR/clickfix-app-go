"use client";

import { ArrowLeft } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { BrandLogo } from "@/components/brand-logos";
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
import { ChannelIcon } from "@/features/channels/components/channel-icon";
import { ConnectedStep } from "@/features/channels/components/connected-step";
import { EmailChannelFields } from "@/features/channels/components/email-channel-fields";
import { SetupFields } from "@/features/channels/components/setup-fields";
import {
  useChannelTypes,
  useCreateChannel,
  useInvalidateChannels,
  useWhatsAppSignupConfig,
} from "@/features/channels/hooks/use-channels";
import {
  credentialsOf,
  initialValues,
  isComplete,
  SETUP_TYPES,
  webhookModeOf,
} from "@/features/channels/lib/channel-setup";
import {
  emailComplete,
  emailCredentials,
  emailSettings,
} from "@/features/channels/lib/email-setup";
import { runEmbeddedSignup } from "@/features/channels/lib/facebook-sdk";
import { channelsService } from "@/features/channels/services/channels.service";
import type {
  ChannelInstance,
  ChannelType,
  OAuthChannelType,
} from "@/features/channels/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Types connected through a provider login rather than a credential form. */
const OAUTH_TYPES = new Set(["instagram", "messenger", "slack"]);
/** Widget instances are created and designed on their own page. */
const WIDGET_PAGE = "/widget";
/** Display order of the type picker. */
const ORDER = [
  "widget",
  "email",
  "whatsapp",
  "instagram",
  "messenger",
  "telegram",
  "slack",
  "teams",
  "google_chat",
  "sms",
  "voice",
  "api",
  "test",
];

type Values = Record<string, string>;

function Field({
  id,
  label,
  value,
  onChange,
  type = "text",
  placeholder,
  hint,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (v: string) => void;
  type?: string;
  placeholder?: string;
  hint?: string;
}) {
  return (
    <div className="space-y-1">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        type={type}
        value={value}
        placeholder={placeholder}
        autoComplete="off"
        onChange={(e) => onChange(e.target.value)}
      />
      {hint ? <p className="text-muted-foreground text-xs">{hint}</p> : null}
    </div>
  );
}

/**
 * "Add channel": pick a type, then its wizard. Credential forms post to
 * `/v1/tenant/channels`; WhatsApp runs Embedded Signup; Instagram and
 * Messenger leave for the Meta login and Slack for its install screen,
 * coming back to `/channels/connect/{type}`. SMS, voice, Teams, Google
 * Chat and API channels end on a step with the webhook URL to paste into the provider.
 */
export function AddChannelDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated?: (c: ChannelInstance) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const types = useChannelTypes();
  const create = useCreateChannel();
  const [selected, setSelected] = useState<ChannelType | null>(null);
  const [name, setName] = useState("");
  const [values, setValues] = useState<Values>({});
  const [busy, setBusy] = useState(false);
  const [commercial, setCommercial] = useState(false);
  const [connected, setConnected] = useState<ChannelInstance | null>(null);
  const signup = useWhatsAppSignupConfig(selected?.type === "whatsapp");
  const invalidate = useInvalidateChannels();

  const reset = () => {
    setSelected(null);
    setName("");
    setValues({});
    setCommercial(false);
    setConnected(null);
  };
  const select = (type: ChannelType) => {
    setSelected(type);
    setValues(initialValues(type.type));
  };
  const setup = selected ? SETUP_TYPES.has(selected.type) : false;
  const close = (next: boolean) => {
    if (!next) reset();
    onOpenChange(next);
  };
  const set = (key: string) => (v: string) =>
    setValues((prev) => ({ ...prev, [key]: v }));
  const label = (key: string) => t(`channels.fields.${key}`);

  const available = [...(types.data ?? [])].sort(
    (a, b) => ORDER.indexOf(a.type) - ORDER.indexOf(b.type),
  );
  const metaOff = !available.some((x) => x.type === "whatsapp");

  const submit = () => {
    if (!selected) return;
    const email = selected.type === "email";
    const credentials = email
      ? emailCredentials(values)
      : setup
        ? credentialsOf(selected.type, values)
        : values;
    const settings: Record<string, unknown> = email
      ? emailSettings(values)
      : {};
    if (selected.type === "sms") settings.commercial = commercial;
    create.mutate(
      { type: selected.type, name: name.trim(), credentials, settings },
      {
        onSuccess: (inst) => {
          onCreated?.(inst);
          if (setup && webhookModeOf(inst.type, values.provider) !== "none") {
            setConnected(inst);
          } else {
            close(false);
          }
        },
      },
    );
  };

  const startOAuth = async (type: OAuthChannelType) => {
    setBusy(true);
    try {
      const { url } = await channelsService.oauthStart(slug, type);
      window.location.assign(url);
    } catch {
      setBusy(false);
    }
  };

  const startSignup = async () => {
    if (!signup.data) return;
    setBusy(true);
    try {
      const res = await runEmbeddedSignup(signup.data);
      const { items } = await channelsService.whatsappSignup(slug, {
        code: res.code,
        waba_id: res.wabaId,
        phone_number_id: res.phoneNumberId || undefined,
        name: name.trim() || undefined,
      });
      // Signup goes through the service directly, so refresh the list here.
      invalidate();
      appToast.success(t("channels.toast.connected"));
      items.forEach((i) => onCreated?.(i));
      close(false);
    } catch (err) {
      if (err instanceof Error && err.message === "cancelled") return;
      appToast.error(t("channels.whatsapp.signup_failed"));
    } finally {
      setBusy(false);
    }
  };

  const formFields = (): React.ReactNode => {
    if (!selected) return null;
    switch (selected.type) {
      case "email":
        return <EmailChannelFields values={values} onChange={setValues} />;
      case "whatsapp":
        return (
          <div className="space-y-3">
            <p className="text-muted-foreground text-sm">
              {t("channels.whatsapp.signup_hint")}
            </p>
            <Button
              type="button"
              disabled={busy || !signup.data?.config_id}
              onClick={() => void startSignup()}
            >
              <BrandLogo slug="facebook" className="size-4" decorative />
              {t("channels.whatsapp.signup")}
            </Button>
          </div>
        );
      case "instagram":
      case "messenger":
      case "slack":
        return (
          <div className="space-y-3">
            <p className="text-muted-foreground text-sm">
              {t(`channels.oauth.${selected.type}_hint`)}
            </p>
            <Button
              type="button"
              disabled={busy}
              onClick={() => void startOAuth(selected.type as OAuthChannelType)}
            >
              {selected.type === "slack" ? (
                <ChannelIcon type="slack" className="size-4" />
              ) : (
                <BrandLogo
                  slug={
                    selected.type === "instagram" ? "instagram" : "facebook"
                  }
                  className="size-4"
                  decorative
                />
              )}
              {selected.type === "slack"
                ? t("channels.oauth.connect_slack")
                : t("channels.oauth.connect")}
            </Button>
          </div>
        );
      case "sms":
      case "voice":
      case "teams":
      case "google_chat":
      case "api":
        return (
          <SetupFields
            type={selected.type}
            values={values}
            onChange={setValues}
            commercial={commercial}
            onCommercialChange={setCommercial}
          />
        );
      case "telegram":
        return (
          <Field
            id="bot_token"
            type="password"
            label={label("bot_token")}
            value={values.bot_token ?? ""}
            onChange={set("bot_token")}
            placeholder="123456789:AA…"
            hint={t("channels.telegram.hint")}
          />
        );
      default:
        return selected.fields.map((f) => (
          <Field
            key={f.key}
            id={f.key}
            type={f.secret ? "password" : "text"}
            label={t(f.label_key, { defaultValue: f.key })}
            value={values[f.key] ?? ""}
            onChange={set(f.key)}
          />
        ));
    }
  };

  const formless = selected
    ? OAUTH_TYPES.has(selected.type) || selected.type === "whatsapp"
    : true;

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {selected
              ? t(`channels.types.${selected.type}`)
              : t("channels.add.title")}
          </DialogTitle>
          <DialogDescription>
            {connected
              ? t("channels.setup.connected")
              : selected
                ? t(`channels.add.describe.${selected.type}`, {
                    defaultValue: "",
                  })
                : t("channels.add.description")}
          </DialogDescription>
        </DialogHeader>

        {connected ? (
          <>
            <ConnectedStep channel={connected} values={values} />
            <DialogFooter>
              <Button type="button" onClick={() => close(false)}>
                {t("channels.setup.done")}
              </Button>
            </DialogFooter>
          </>
        ) : !selected ? (
          <div className="grid gap-2 sm:grid-cols-2">
            {available.map((type) => (
              <button
                key={type.type}
                type="button"
                className="hover:bg-muted flex items-center gap-3 rounded-md border p-3 text-left"
                onClick={() =>
                  // Widgets have their own designer page (CF-55).
                  type.type === "widget"
                    ? router.push(WIDGET_PAGE)
                    : select(type)
                }
              >
                <ChannelIcon type={type.type} className="size-5" />
                <span className="font-medium">
                  {t(`channels.types.${type.type}`)}
                </span>
              </button>
            ))}
            {metaOff ? (
              <p className="text-muted-foreground text-xs sm:col-span-2">
                {t("channels.add.meta_pending")}
              </p>
            ) : null}
          </div>
        ) : (
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              submit();
            }}
          >
            {!OAUTH_TYPES.has(selected.type) ? (
              <Field
                id="channel-name"
                label={t("channels.name")}
                value={name}
                onChange={setName}
                placeholder={t(`channels.types.${selected.type}`)}
              />
            ) : null}
            {formFields()}
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={reset}>
                <ArrowLeft className="size-4" />
                {t("channels.add.back")}
              </Button>
              {!formless ? (
                <Button
                  type="submit"
                  disabled={
                    create.isPending ||
                    name.trim() === "" ||
                    (setup && !isComplete(selected.type, values, commercial)) ||
                    (selected.type === "email" && !emailComplete(values))
                  }
                >
                  {t("channels.add.connect")}
                </Button>
              ) : null}
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
