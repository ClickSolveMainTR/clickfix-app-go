"use client";

import { Copy, RefreshCw, Upload } from "lucide-react";

import { BrandLogo, isBrandSlug } from "@/components/brand-logos";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  fieldsOf,
  generateSecret,
  IYS_FIELDS,
  providersOf,
} from "@/features/channels/lib/channel-setup";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Read-only value with a copy button (webhook URLs, generated secrets). */
export function CopyField({
  id,
  label,
  value,
  hint,
}: {
  id: string;
  label: string;
  value: string;
  hint?: React.ReactNode;
}) {
  const { t } = useLocale();
  return (
    <div className="space-y-1">
      <Label htmlFor={id}>{label}</Label>
      <div className="flex gap-2">
        <Input id={id} readOnly value={value} className="font-mono text-xs" />
        <Button
          type="button"
          size="icon"
          variant="outline"
          aria-label={t("channels.copy")}
          disabled={!value}
          onClick={() =>
            void navigator.clipboard
              .writeText(value)
              .then(() => appToast.success(t("channels.copied")))
          }
        >
          <Copy className="size-4" />
        </Button>
      </div>
      {hint ? <p className="text-muted-foreground text-xs">{hint}</p> : null}
    </div>
  );
}

/** Largest key file read into the form (a service account key is ~2.5 KB). */
const MAX_FILE_BYTES = 64 * 1024;

/**
 * Credential form of SMS, voice, Teams, Google Chat and API channels. Creating shows
 * every field (generated secrets prefilled); editing keeps the provider and
 * leaves secrets empty, which the API reads as "unchanged".
 */
export function SetupFields({
  type,
  values,
  onChange,
  editing = false,
  hints = {},
  commercial,
  onCommercialChange,
}: {
  type: string;
  values: Record<string, string>;
  onChange: (values: Record<string, string>) => void;
  editing?: boolean;
  /** Stored non-secret values (credential_hints), shown as placeholders. */
  hints?: Record<string, string>;
  commercial?: boolean;
  onCommercialChange?: (v: boolean) => void;
}) {
  const { t } = useLocale();
  const providers = providersOf(type);
  const provider = values.provider ?? providers?.[0]?.value;
  const iys = new Set(IYS_FIELDS.map((f) => f.key));
  const fields = fieldsOf(type, provider);
  const set = (key: string, v: string) => onChange({ ...values, [key]: v });

  const pickProvider = (next: string) => {
    // Prefill generated secrets of the new provider; drop the old fields.
    const keep: Record<string, string> = { provider: next };
    for (const f of fieldsOf(type, next)) {
      if (iys.has(f.key) && values[f.key]) keep[f.key] = values[f.key];
      if (f.generated) keep[f.key] = generateSecret();
    }
    onChange(keep);
  };

  const readFile = (key: string, file: File | undefined) => {
    if (!file) return;
    if (file.size > MAX_FILE_BYTES) {
      appToast.error(t("channels.setup.file_too_large"));
      return;
    }
    void file.text().then((text) => set(key, text.trim()));
  };

  const input = (key: string) => {
    const f = fields.find((x) => x.key === key)!;
    if (f.file) {
      // Key files are pasted or loaded; write-only like every secret.
      return (
        <div key={f.key} className="space-y-1 sm:col-span-2">
          <Label htmlFor={`cred-${f.key}`}>
            {t(`channels.fields.${f.key}`)}
            {f.required && !editing ? (
              <span className="text-destructive"> *</span>
            ) : null}
          </Label>
          <Textarea
            id={`cred-${f.key}`}
            rows={4}
            className="font-mono text-xs"
            spellCheck={false}
            autoComplete="off"
            value={values[f.key] ?? ""}
            placeholder={
              editing && f.secret
                ? t("channels.setup.unchanged")
                : f.placeholder
            }
            onChange={(e) => set(f.key, e.target.value)}
          />
          <div className="flex items-center gap-2">
            <Button type="button" size="sm" variant="outline" asChild>
              <label className="cursor-pointer">
                <Upload className="size-4" />
                {t("channels.setup.upload_file")}
                <input
                  type="file"
                  accept={f.file}
                  className="sr-only"
                  onChange={(e) => {
                    readFile(f.key, e.target.files?.[0]);
                    e.target.value = "";
                  }}
                />
              </label>
            </Button>
            <p className="text-muted-foreground text-xs">
              {t("channels.setup.file_hint")}
            </p>
          </div>
        </div>
      );
    }
    return (
      <div key={f.key} className="space-y-1">
        <Label htmlFor={`cred-${f.key}`}>
          {t(`channels.fields.${f.key}`)}
          {f.required && !editing ? (
            <span className="text-destructive"> *</span>
          ) : null}
        </Label>
        <div className="flex gap-2">
          <Input
            id={`cred-${f.key}`}
            type={f.secret && !f.generated ? "password" : "text"}
            className={f.generated ? "font-mono text-xs" : undefined}
            value={values[f.key] ?? ""}
            placeholder={
              editing && f.secret
                ? t("channels.setup.unchanged")
                : editing && hints[f.key]
                  ? hints[f.key]
                  : f.placeholder
            }
            autoComplete="off"
            onChange={(e) => set(f.key, e.target.value)}
          />
          {f.generated ? (
            <Button
              type="button"
              size="icon"
              variant="outline"
              aria-label={t("channels.setup.generate")}
              title={t("channels.setup.generate")}
              onClick={() => set(f.key, generateSecret())}
            >
              <RefreshCw className="size-4" />
            </Button>
          ) : null}
        </div>
        {f.generated ? (
          <p className="text-muted-foreground text-xs">
            {t("channels.setup.generated_hint")}
          </p>
        ) : null}
      </div>
    );
  };

  const own = fields.filter((f) => !iys.has(f.key));
  const iysFields = fields.filter((f) => iys.has(f.key));

  return (
    <div className="space-y-4">
      {providers ? (
        <div className="space-y-1">
          <Label>{t("channels.fields.provider")}</Label>
          <Select
            value={provider}
            disabled={editing}
            onValueChange={pickProvider}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {providers.map((p) => (
                <SelectItem key={p.value} value={p.value}>
                  <span className="flex items-center gap-2">
                    {isBrandSlug(p.value) ? (
                      <BrandLogo slug={p.value} className="size-4" decorative />
                    ) : null}
                    {t(`channels.setup.providers.${p.value}`)}
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-muted-foreground text-xs">
            {t(`channels.setup.provider_hint.${type}.${provider}`)}
          </p>
        </div>
      ) : null}
      <div className="grid gap-3 sm:grid-cols-2">
        {own.map((f) => input(f.key))}
      </div>
      {type === "sms" ? (
        <div className="space-y-3 rounded-md border p-3">
          <div className="flex items-center justify-between gap-4">
            <div>
              <Label htmlFor="sms-commercial">
                {t("channels.setup.commercial")}
              </Label>
              <p className="text-muted-foreground text-xs">
                {t("channels.setup.commercial_hint")}
              </p>
            </div>
            <Switch
              id="sms-commercial"
              checked={commercial ?? false}
              onCheckedChange={(v) => onCommercialChange?.(v)}
            />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            {iysFields.map((f) => input(f.key))}
          </div>
        </div>
      ) : null}
    </div>
  );
}
