"use client";

import { useQuery } from "@tanstack/react-query";
import { ImagePlus, Trash2 } from "lucide-react";
import { useRef, useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { Permission } from "@/config/permissions";
import { storageConfig } from "@/config/storage";
import { useActiveOrganization } from "@/features/organizations";
import { slaService } from "@/features/sla/services/sla.service";
import { isValidColor } from "@/features/widget/lib/settings";
import { isApiError } from "@/lib/api";
import { resolveLogoSrc } from "@/lib/branding/logo-url";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  brandsService,
  type Brand,
  type BrandInput,
} from "../services/brands.service";

const NONE = "__none__";
const INHERIT = "__inherit__";
// Mirrors backend/internal/modules/brands/service.go validation.
const SUBDOMAIN_RE = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/;
const PATH_RE = /^[a-z0-9][a-z0-9-]{0,79}$/;
const EMAIL_RE = /^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/;
/** Widget settings a brand may preset; a widget's own settings still win. */
const WIDGET_TEXT_KEYS = [
  "greeting_title",
  "greeting_text",
  "offline_message",
] as const;

function toInput(b: Brand | null): BrandInput {
  return {
    name: b?.name ?? "",
    logo_attachment_id: b?.logo_attachment_id ?? null,
    primary_color: b?.primary_color ?? "#2563eb",
    secondary_color: b?.secondary_color ?? "#ffffff",
    support_email: b?.support_email ?? "",
    sender_name: b?.sender_name ?? "",
    reply_to: b?.reply_to ?? "",
    help_subdomain: b?.help_subdomain ?? "",
    help_path: b?.help_path ?? "",
    widget_defaults: b?.widget_defaults ?? {},
    signature: b?.signature ?? "",
    sla_policy_id: b?.sla_policy_id ?? null,
    ai_persona: b?.ai_persona ?? "",
  };
}

/**
 * Create or edit a brand (CF-124): identity and logo, e-mail sender and
 * signature, help center host/path, widget defaults, SLA override and AI
 * persona. Working hours are the SLA schedule (CF-198). Saves the whole
 * configuration (PUT replaces).
 */
export function BrandDialog({
  open,
  onOpenChange,
  brand,
  canManage,
  pending,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  brand: Brand | null;
  canManage: boolean;
  pending: boolean;
  onSubmit: (body: BrandInput) => void;
}) {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const canReadSla = (org?.permissions ?? []).includes(
    Permission.TenantSlaRead,
  );
  const policies = useQuery({
    queryKey: ["tenant", org?.slug, "sla", "policies"],
    queryFn: slaService.policies,
    enabled: open && canReadSla,
  });
  const [form, setForm] = useState<BrandInput>(() => toInput(brand));
  const [logoPreview, setLogoPreview] = useState<string | undefined>(() =>
    resolveLogoSrc(brand?.logo_url),
  );
  const [uploading, setUploading] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const disabled = !canManage || pending;

  const patch = (p: Partial<BrandInput>) => setForm((f) => ({ ...f, ...p }));
  const defaults = (form.widget_defaults ?? {}) as Record<string, unknown>;
  const setDefault = (key: string, value: string | null) => {
    const next = { ...defaults };
    if (value === null || value === "") delete next[key];
    else next[key] = value;
    patch({ widget_defaults: next });
  };
  const uploadLogo = async (file: File | undefined) => {
    if (!file) return;
    if (
      !(storageConfig.acceptedImageTypes as readonly string[]).includes(
        file.type,
      ) ||
      !["image/png", "image/jpeg", "image/webp"].includes(file.type)
    ) {
      appToast.error(t("branding.logo.invalid_type"));
      return;
    }
    if (file.size > storageConfig.logoMaxFileSizeBytes) {
      appToast.error(t("branding.logo.too_large"));
      return;
    }
    setUploading(true);
    try {
      const res = await brandsService.uploadLogo(file);
      patch({ logo_attachment_id: res.uuid });
      setLogoPreview(URL.createObjectURL(file));
    } catch (error) {
      appToast.error(
        isApiError(error) ? error.message : t("brands.toast.logo_failed"),
      );
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  };

  const errors = {
    name: !form.name.trim(),
    primary: !isValidColor(form.primary_color ?? ""),
    secondary: !isValidColor(form.secondary_color ?? ""),
    support_email: !!form.support_email && !EMAIL_RE.test(form.support_email),
    reply_to: !!form.reply_to && !EMAIL_RE.test(form.reply_to),
    subdomain:
      !!form.help_subdomain &&
      !form.help_subdomain
        .split(".")
        .every((label) => SUBDOMAIN_RE.test(label)),
    path: !!form.help_path && !PATH_RE.test(form.help_path),
  };
  const invalid = Object.values(errors).some(Boolean);

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={brand ? t("brands.dialog.edit") : t("brands.dialog.create")}
      description={t("brands.dialog.description")}
      className="max-h-[90vh] overflow-y-auto sm:max-w-2xl"
    >
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (!invalid && !uploading)
            onSubmit({ ...form, name: form.name.trim() });
        }}
      >
        <Tabs defaultValue="identity">
          <TabsList className="flex h-auto w-full flex-wrap justify-start">
            <TabsTrigger value="identity">
              {t("brands.tabs.identity")}
            </TabsTrigger>
            <TabsTrigger value="email">{t("brands.tabs.email")}</TabsTrigger>
            <TabsTrigger value="help">{t("brands.tabs.help")}</TabsTrigger>
            <TabsTrigger value="widget">{t("brands.tabs.widget")}</TabsTrigger>
            <TabsTrigger value="sla">{t("brands.tabs.sla_ai")}</TabsTrigger>
          </TabsList>

          <TabsContent value="identity" className="mt-4 space-y-4">
            <div className="space-y-2">
              <Label htmlFor="brand-name">{t("brands.fields.name")}</Label>
              <Input
                id="brand-name"
                value={form.name}
                maxLength={120}
                disabled={disabled}
                aria-invalid={errors.name}
                onChange={(e) => patch({ name: e.target.value })}
              />
            </div>
            <div className="space-y-2">
              <Label>{t("brands.fields.logo")}</Label>
              <div className="flex flex-wrap items-center gap-3">
                <div className="bg-muted flex size-16 items-center justify-center overflow-hidden rounded-lg border">
                  {logoPreview && form.logo_attachment_id ? (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img
                      src={logoPreview}
                      alt=""
                      className="size-full object-contain"
                    />
                  ) : (
                    <ImagePlus className="text-muted-foreground size-6" />
                  )}
                </div>
                <input
                  ref={fileRef}
                  type="file"
                  accept="image/png,image/jpeg,image/webp"
                  className="hidden"
                  onChange={(e) => void uploadLogo(e.target.files?.[0])}
                />
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={disabled || uploading}
                  onClick={() => fileRef.current?.click()}
                >
                  <ImagePlus className="size-4" />
                  {uploading
                    ? t("brands.fields.logo_uploading")
                    : t("brands.fields.logo_upload")}
                </Button>
                {form.logo_attachment_id && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={disabled || uploading}
                    onClick={() => {
                      patch({ logo_attachment_id: null });
                      setLogoPreview(undefined);
                    }}
                  >
                    <Trash2 className="size-4" />
                    {t("brands.fields.logo_remove")}
                  </Button>
                )}
              </div>
              <p className="text-muted-foreground text-xs">
                {t("brands.fields.logo_hint")}
              </p>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <ColorField
                id="brand-primary"
                label={t("brands.fields.primary_color")}
                value={form.primary_color ?? ""}
                invalid={errors.primary}
                disabled={disabled}
                onChange={(v) => patch({ primary_color: v })}
              />
              <ColorField
                id="brand-secondary"
                label={t("brands.fields.secondary_color")}
                value={form.secondary_color ?? ""}
                invalid={errors.secondary}
                disabled={disabled}
                onChange={(v) => patch({ secondary_color: v })}
              />
            </div>
          </TabsContent>

          <TabsContent value="email" className="mt-4 space-y-4">
            <p className="text-muted-foreground text-xs">
              {t("brands.email.hint")}
            </p>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="brand-sender">
                  {t("brands.fields.sender_name")}
                </Label>
                <Input
                  id="brand-sender"
                  value={form.sender_name ?? ""}
                  maxLength={120}
                  disabled={disabled}
                  onChange={(e) =>
                    patch({
                      sender_name: e.target.value.replace(/[\r\n]/g, ""),
                    })
                  }
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="brand-support-email">
                  {t("brands.fields.support_email")}
                </Label>
                <Input
                  id="brand-support-email"
                  type="email"
                  value={form.support_email ?? ""}
                  maxLength={254}
                  disabled={disabled}
                  aria-invalid={errors.support_email}
                  placeholder="destek@marka.com"
                  onChange={(e) =>
                    patch({ support_email: e.target.value.trim() })
                  }
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="brand-reply-to">
                {t("brands.fields.reply_to")}
              </Label>
              <Input
                id="brand-reply-to"
                type="email"
                value={form.reply_to ?? ""}
                maxLength={254}
                disabled={disabled}
                aria-invalid={errors.reply_to}
                onChange={(e) => patch({ reply_to: e.target.value.trim() })}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="brand-signature">
                {t("brands.fields.signature")}
              </Label>
              <Textarea
                id="brand-signature"
                rows={5}
                value={form.signature ?? ""}
                maxLength={10000}
                disabled={disabled}
                onChange={(e) => patch({ signature: e.target.value })}
              />
              <p className="text-muted-foreground text-xs">
                {t("brands.fields.signature_hint")}
              </p>
            </div>
          </TabsContent>

          <TabsContent value="help" className="mt-4 space-y-4">
            <div className="space-y-2">
              <Label htmlFor="brand-subdomain">
                {t("brands.fields.help_subdomain")}
              </Label>
              <Input
                id="brand-subdomain"
                value={form.help_subdomain ?? ""}
                maxLength={253}
                disabled={disabled}
                aria-invalid={errors.subdomain}
                placeholder="yardim.marka.com"
                onChange={(e) =>
                  patch({ help_subdomain: e.target.value.trim().toLowerCase() })
                }
              />
              <p className="text-muted-foreground text-xs">
                {t("brands.fields.help_subdomain_hint")}
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="brand-path">{t("brands.fields.help_path")}</Label>
              <Input
                id="brand-path"
                value={form.help_path ?? ""}
                maxLength={80}
                disabled={disabled}
                aria-invalid={errors.path}
                placeholder="marka-adi"
                onChange={(e) =>
                  patch({ help_path: e.target.value.trim().toLowerCase() })
                }
              />
              <p className="text-muted-foreground text-xs">
                {t("brands.fields.help_path_hint")}
              </p>
            </div>
            <p className="text-muted-foreground text-xs">
              {t("brands.help.articles_hint")}
            </p>
          </TabsContent>

          <TabsContent value="widget" className="mt-4 space-y-4">
            <p className="text-muted-foreground text-xs">
              {t("brands.widget.hint")}
            </p>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="brand-w-position">
                  {t("widget.fields.position")}
                </Label>
                <Select
                  value={String(defaults.position ?? INHERIT)}
                  disabled={disabled}
                  onValueChange={(v) =>
                    setDefault("position", v === INHERIT ? null : v)
                  }
                >
                  <SelectTrigger id="brand-w-position" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={INHERIT}>
                      {t("brands.widget.inherit")}
                    </SelectItem>
                    <SelectItem value="right">
                      {t("widget.position.right")}
                    </SelectItem>
                    <SelectItem value="left">
                      {t("widget.position.left")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="brand-w-locale">
                  {t("widget.fields.locale")}
                </Label>
                <Select
                  value={String(defaults.locale ?? INHERIT)}
                  disabled={disabled}
                  onValueChange={(v) =>
                    setDefault("locale", v === INHERIT ? null : v)
                  }
                >
                  <SelectTrigger id="brand-w-locale" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={INHERIT}>
                      {t("brands.widget.inherit")}
                    </SelectItem>
                    <SelectItem value="auto">
                      {t("widget.locale.auto")}
                    </SelectItem>
                    <SelectItem value="tr">{t("brands.widget.tr")}</SelectItem>
                    <SelectItem value="en">{t("brands.widget.en")}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
            {WIDGET_TEXT_KEYS.map((key) => (
              <div key={key} className="space-y-2">
                <Label htmlFor={`brand-w-${key}`}>
                  {t(`widget.fields.${key}`)}
                </Label>
                <Input
                  id={`brand-w-${key}`}
                  value={
                    typeof defaults[key] === "string"
                      ? String(defaults[key])
                      : ""
                  }
                  maxLength={key === "greeting_title" ? 80 : 500}
                  disabled={disabled}
                  placeholder={t("brands.widget.inherit")}
                  onChange={(e) => setDefault(key, e.target.value)}
                />
              </div>
            ))}

            <p className="text-muted-foreground text-sm">
              {t("brands.widget.hours_source")}
            </p>
          </TabsContent>

          <TabsContent value="sla" className="mt-4 space-y-4">
            <div className="space-y-2">
              <Label htmlFor="brand-sla">{t("brands.fields.sla_policy")}</Label>
              {canReadSla ? (
                <Select
                  value={form.sla_policy_id ?? NONE}
                  disabled={disabled}
                  onValueChange={(v) =>
                    patch({ sla_policy_id: v === NONE ? null : v })
                  }
                >
                  <SelectTrigger id="brand-sla" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NONE}>
                      {t("brands.fields.sla_default")}
                    </SelectItem>
                    {(policies.data ?? []).map((p) => (
                      <SelectItem key={p.uuid} value={p.uuid}>
                        {p.name}
                      </SelectItem>
                    ))}
                    {form.sla_policy_id &&
                      policies.data &&
                      !policies.data.some(
                        (p) => p.uuid === form.sla_policy_id,
                      ) && (
                        <SelectItem value={form.sla_policy_id}>
                          {t("brands.fields.sla_unknown")}
                        </SelectItem>
                      )}
                  </SelectContent>
                </Select>
              ) : (
                <p className="text-muted-foreground text-sm">
                  {t("brands.fields.sla_forbidden")}
                </p>
              )}
              <p className="text-muted-foreground text-xs">
                {t("brands.fields.sla_hint")}
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="brand-persona">
                {t("brands.fields.ai_persona")}
              </Label>
              <Textarea
                id="brand-persona"
                rows={4}
                value={form.ai_persona ?? ""}
                maxLength={10000}
                disabled={disabled}
                onChange={(e) => patch({ ai_persona: e.target.value })}
              />
              <p className="text-muted-foreground text-xs">
                {t("brands.fields.ai_persona_hint")}
              </p>
            </div>
          </TabsContent>
        </Tabs>

        {invalid && (
          <p className="text-destructive text-xs" role="alert">
            {t("brands.dialog.invalid")}
          </p>
        )}
        <FormActions sticky={false}>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={pending}
          >
            {t("common.cancel")}
          </Button>
          {canManage && (
            <Button type="submit" disabled={invalid || pending || uploading}>
              {pending ? t("brands.dialog.saving") : t("common.save")}
            </Button>
          )}
        </FormActions>
      </form>
    </AppDialog>
  );
}

function ColorField({
  id,
  label,
  value,
  invalid,
  disabled,
  onChange,
}: {
  id: string;
  label: string;
  value: string;
  invalid: boolean;
  disabled: boolean;
  onChange: (v: string) => void;
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      <div className="flex items-center gap-2">
        <input
          type="color"
          aria-label={label}
          value={isValidColor(value) ? value : "#000000"}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
          className="border-input size-9 shrink-0 cursor-pointer rounded-md border bg-transparent p-0.5"
        />
        <Input
          id={id}
          value={value}
          maxLength={7}
          disabled={disabled}
          aria-invalid={invalid}
          onChange={(e) => onChange(e.target.value.trim())}
        />
      </div>
    </div>
  );
}
