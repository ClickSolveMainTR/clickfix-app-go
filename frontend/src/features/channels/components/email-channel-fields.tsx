"use client";

import { CheckCircle2, Loader2, PlugZap, XCircle } from "lucide-react";
import { useState } from "react";

import { BrandLogo } from "@/components/brand-logos";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { EmailDomainWizard } from "@/features/channels/components/email-domain-wizard";
import {
  EMAIL_SECRETS,
  emailCredentials,
  emailProviderOf,
  emailSettings,
  emailTestReady,
  IMAP_DEFAULT_SYNC_DAYS,
  type EmailValues,
} from "@/features/channels/lib/email-setup";
import { channelsService } from "@/features/channels/services/channels.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

function TextField({
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
    <div className="min-w-0 space-y-1">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        type={type}
        value={value}
        placeholder={placeholder}
        autoComplete={type === "password" ? "new-password" : "off"}
        onChange={(e) => onChange(e.target.value)}
      />
      {hint ? <p className="text-muted-foreground text-xs">{hint}</p> : null}
    </div>
  );
}

function TlsSelect({
  id,
  label,
  value,
  onChange,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (v: string) => void;
}) {
  const { t } = useLocale();
  return (
    <div className="min-w-0 space-y-1">
      <Label htmlFor={id}>{label}</Label>
      <Select
        value={value || "auto"}
        onValueChange={(v) => onChange(v === "auto" ? "" : v)}
      >
        <SelectTrigger id={id} className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="auto">{t("channels.email.tls_auto")}</SelectItem>
          <SelectItem value="implicit">
            {t("channels.email.tls_implicit")}
          </SelectItem>
          <SelectItem value="starttls">
            {t("channels.email.tls_starttls")}
          </SelectItem>
        </SelectContent>
      </Select>
    </div>
  );
}

type TestState =
  | { status: "idle" }
  | { status: "running" }
  | { status: "ok" }
  | { status: "error"; message: string };

/** The API's own (Turkish) reason: the field detail, else the message. */
function testError(err: unknown, fallback: string) {
  if (!isApiError(err)) return fallback;
  return err.details.find((d) => d.message)?.message || err.message || fallback;
}

/**
 * E-mail channel form shared by "add channel" and the channel settings
 * (CF-56): forwarding (routing address, replies through ClickFix or the
 * customer's SMTP) or the customer's own mailbox (IMAP in, SMTP out), and a
 * connection test against `POST /v1/tenant/channels/email/test`. Passwords
 * are write-only; when editing an empty password keeps the stored one.
 */
export function EmailChannelFields({
  values,
  onChange,
  editing = false,
  hints = {},
}: {
  values: EmailValues;
  onChange: (next: EmailValues) => void;
  editing?: boolean;
  hints?: Record<string, string>;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [test, setTest] = useState<TestState>({ status: "idle" });
  const provider = emailProviderOf(values);
  const mailbox = provider === "imap";

  const set = (key: string) => (v: string) => {
    setTest({ status: "idle" });
    onChange({ ...values, [key]: v });
  };
  const label = (key: string) => t(`channels.fields.${key}`);
  const text = (
    key: string,
    extra: { placeholder?: string; hint?: string } = {},
  ) => {
    const secret = EMAIL_SECRETS.has(key);
    return (
      <TextField
        id={`email-${key}`}
        type={secret ? "password" : "text"}
        label={label(key)}
        value={values[key] ?? ""}
        onChange={set(key)}
        placeholder={
          secret && editing && hints[key] ? hints[key] : extra.placeholder
        }
        hint={
          secret && editing && hints[key]
            ? t("channels.email.secret_keep")
            : extra.hint
        }
      />
    );
  };

  const runTest = async () => {
    setTest({ status: "running" });
    try {
      await channelsService.emailTest(slug, {
        credentials: emailCredentials(values),
        settings: emailSettings(values),
      });
      setTest({ status: "ok" });
    } catch (err) {
      setTest({
        status: "error",
        message: testError(err, t("channels.email.test_failed")),
      });
    }
  };
  const ready = emailTestReady(values, editing ? hints : {});

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>{t("channels.email.mode")}</Label>
        <RadioGroup
          value={mailbox ? "mailbox" : "forwarding"}
          onValueChange={(v) =>
            set("provider")(v === "mailbox" ? "imap" : "resend")
          }
          className="grid gap-2 sm:grid-cols-2"
        >
          {(["forwarding", "mailbox"] as const).map((mode) => (
            <Label
              key={mode}
              htmlFor={`email-mode-${mode}`}
              className="has-[[data-state=checked]]:border-primary has-[[data-state=checked]]:bg-primary/5 flex cursor-pointer items-start gap-3 rounded-md border p-3 font-normal"
            >
              <RadioGroupItem
                id={`email-mode-${mode}`}
                value={mode}
                className="mt-0.5"
              />
              <span className="space-y-0.5">
                <span className="block text-sm font-medium">
                  {t(`channels.email.mode_${mode}`)}
                </span>
                <span className="text-muted-foreground block text-xs">
                  {t(`channels.email.mode_${mode}_hint`)}
                </span>
              </span>
            </Label>
          ))}
        </RadioGroup>
      </div>

      {mailbox ? null : (
        <>
          {editing ? null : (
            <TextField
              id="inbound_alias"
              label={t("channels.email.alias")}
              value={values.inbound_alias ?? ""}
              onChange={set("inbound_alias")}
              placeholder="acme-destek"
              hint={t("channels.email.alias_hint")}
            />
          )}
          <div className="space-y-1">
            <Label htmlFor="email-provider">{label("provider")}</Label>
            <Select value={provider} onValueChange={set("provider")}>
              <SelectTrigger id="email-provider" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="resend">
                  <span className="flex items-center gap-2">
                    <BrandLogo slug="resend" className="size-4" decorative />
                    {t("channels.email.provider_resend")}
                  </span>
                </SelectItem>
                <SelectItem value="smtp">
                  {t("channels.email.provider_smtp")}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
        </>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        {text("from_address", {
          placeholder: "destek@acme.com",
          hint: mailbox
            ? t("channels.email.from_hint_mailbox")
            : t("channels.email.from_hint"),
        })}
        {text("from_name")}
      </div>

      {mailbox ? (
        <fieldset className="space-y-3 rounded-md border p-3">
          <legend className="px-1 text-sm font-medium">
            {t("channels.email.imap_section")}
          </legend>
          <div className="grid gap-3 sm:grid-cols-[1fr_7rem]">
            {text("imap_host", { placeholder: "imap.acme.com" })}
            {text("imap_port", { placeholder: "993" })}
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            {text("imap_username", { placeholder: "destek@acme.com" })}
            {text("imap_password", {
              hint: t("channels.email.app_password_hint"),
            })}
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <TlsSelect
              id="email-imap_tls"
              label={label("imap_tls")}
              value={values.imap_tls ?? ""}
              onChange={set("imap_tls")}
            />
            {text("imap_folder", { placeholder: "INBOX" })}
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <TextField
              id="email-imap_sync_days"
              type="number"
              label={t("channels.email.sync_days")}
              value={values.imap_sync_days ?? ""}
              onChange={set("imap_sync_days")}
              placeholder={String(IMAP_DEFAULT_SYNC_DAYS)}
              hint={t("channels.email.sync_days_hint")}
            />
            <TextField
              id="email-imap_move_folder"
              label={t("channels.email.move_folder")}
              value={values.imap_move_folder ?? ""}
              onChange={set("imap_move_folder")}
              hint={t("channels.email.move_folder_hint")}
            />
          </div>
          <div className="flex items-center justify-between gap-4">
            <Label htmlFor="email-imap_mark_read" className="font-normal">
              {t("channels.email.mark_read")}
            </Label>
            <Switch
              id="email-imap_mark_read"
              checked={values.imap_mark_read === "true"}
              onCheckedChange={(v) => set("imap_mark_read")(String(v))}
            />
          </div>
        </fieldset>
      ) : null}

      {provider === "resend" ? (
        !editing && values.from_address?.includes("@") ? (
          <EmailDomainWizard
            key={values.from_address.split("@")[1]}
            initialDomain={values.from_address.split("@")[1] ?? ""}
          />
        ) : null
      ) : (
        <fieldset className="space-y-3 rounded-md border p-3">
          <legend className="px-1 text-sm font-medium">
            {t("channels.email.smtp_section")}
          </legend>
          <div className="grid gap-3 sm:grid-cols-[1fr_7rem]">
            {text("smtp_host", { placeholder: "smtp.acme.com" })}
            {text("smtp_port", { placeholder: "587" })}
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            {text("smtp_username")}
            {text("smtp_password")}
          </div>
          <TlsSelect
            id="email-smtp_tls"
            label={label("smtp_tls")}
            value={values.smtp_tls ?? ""}
            onChange={set("smtp_tls")}
          />
        </fieldset>
      )}

      {provider === "resend" ? null : (
        <div className="space-y-2">
          <Button
            type="button"
            variant="outline"
            disabled={!ready || test.status === "running"}
            onClick={() => void runTest()}
          >
            {test.status === "running" ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <PlugZap className="size-4" />
            )}
            {t("channels.email.test")}
          </Button>
          {!ready && editing ? (
            <p className="text-muted-foreground text-xs">
              {t("channels.email.test_needs_password")}
            </p>
          ) : null}
          {test.status === "ok" ? (
            <p
              role="status"
              className="flex items-center gap-1.5 text-sm text-emerald-600 dark:text-emerald-400"
            >
              <CheckCircle2 className="size-4 shrink-0" />
              {t("channels.email.test_ok")}
            </p>
          ) : null}
          {test.status === "error" ? (
            <p
              role="alert"
              className="text-destructive flex items-start gap-1.5 text-sm"
            >
              <XCircle className="mt-0.5 size-4 shrink-0" />
              <span className="break-words">{test.message}</span>
            </p>
          ) : null}
        </div>
      )}
    </div>
  );
}
