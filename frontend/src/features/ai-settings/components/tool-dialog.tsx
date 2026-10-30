"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Wand2 } from "lucide-react";
import { useState } from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
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
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { aiErrorMessage } from "../lib/errors";
import {
  aiSettingsService,
  BUILTIN_TOOLS,
  type AIToolInput,
  type AIToolView,
} from "../services/ai-settings.service";
import { SettingRow } from "./parts";

const NAME_PATTERN = /^[a-z][a-z0-9_]{2,49}$/;
const MAX_SCHEMA_BYTES = 16000;

const EXAMPLE_SCHEMA = {
  type: "object",
  properties: {
    order_number: {
      type: "string",
      description: "Sipariş numarası",
      maxLength: 40,
    },
  },
  required: ["order_number"],
  additionalProperties: false,
};

type SchemaCheck =
  | { ok: true; value: Record<string, unknown>; params: SchemaParam[] }
  | { ok: false; error: string };
type SchemaParam = {
  name: string;
  type: string;
  required: boolean;
  description: string;
};

/**
 * Client-side pre-check of the JSON schema (the server compiles it again with
 * `ai.ValidateSchema`): valid JSON, an object schema, `properties` an object.
 */
function checkSchema(raw: string, t: (key: string) => string): SchemaCheck {
  let value: unknown;
  try {
    value = JSON.parse(raw);
  } catch {
    return { ok: false, error: t("ai_settings.tools.schema_invalid_json") };
  }
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return { ok: false, error: t("ai_settings.tools.schema_not_object") };
  }
  const schema = value as Record<string, unknown>;
  if (schema.type !== "object") {
    return { ok: false, error: t("ai_settings.tools.schema_type") };
  }
  if (
    new TextEncoder().encode(JSON.stringify(schema)).length > MAX_SCHEMA_BYTES
  ) {
    return { ok: false, error: t("ai_settings.tools.schema_too_large") };
  }
  const props = schema.properties ?? {};
  if (typeof props !== "object" || Array.isArray(props) || props === null) {
    return { ok: false, error: t("ai_settings.tools.schema_properties") };
  }
  const required = Array.isArray(schema.required) ? schema.required : [];
  const params = Object.entries(props as Record<string, unknown>).map(
    ([name, def]) => {
      const d = (def ?? {}) as Record<string, unknown>;
      return {
        name,
        type: typeof d.type === "string" ? d.type : "-",
        required: required.includes(name),
        description: typeof d.description === "string" ? d.description : "",
      };
    },
  );
  return { ok: true, value: schema, params };
}

export function ToolDialog({
  slug,
  tool,
  onClose,
  onSaved,
}: {
  slug: string;
  /** `null` creates a new tool. */
  tool: AIToolView | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const isNew = tool === null;
  const [name, setName] = useState(tool?.name ?? "");
  const [description, setDescription] = useState(tool?.description ?? "");
  const [url, setUrl] = useState(tool?.url ?? "https://");
  const [schemaText, setSchemaText] = useState(() =>
    JSON.stringify(tool?.schema ?? EXAMPLE_SCHEMA, null, 2),
  );
  const [requiresApproval, setRequiresApproval] = useState(
    tool?.requires_approval ?? true,
  );
  const [enabled, setEnabled] = useState(tool?.enabled ?? true);
  const [dailyLimit, setDailyLimit] = useState(tool?.daily_limit ?? 100);
  const [keepAuth, setKeepAuth] = useState(Boolean(tool?.has_auth));
  const [bearer, setBearer] = useState("");
  const [hmac, setHmac] = useState("");

  const schema = checkSchema(schemaText, t);
  const nameError = !NAME_PATTERN.test(name)
    ? t("ai_settings.tools.name_invalid")
    : BUILTIN_TOOLS.some((b) => b.name === name)
      ? t("ai_settings.tools.name_builtin")
      : null;
  let urlError: string | null = null;
  try {
    const parsed = new URL(url);
    if (parsed.protocol !== "https:" || parsed.search || parsed.hash) {
      urlError = t("ai_settings.tools.url_invalid");
    }
  } catch {
    urlError = t("ai_settings.tools.url_invalid");
  }
  const limitError = dailyLimit < 1 || dailyLimit > 1000;
  const invalid =
    Boolean(nameError) ||
    Boolean(urlError) ||
    !schema.ok ||
    limitError ||
    description.length > 1000;

  const save = useMutation({
    mutationFn: () => {
      if (!schema.ok) throw new Error("invalid schema");
      const body: AIToolInput = {
        name,
        description: description.trim(),
        schema: schema.value,
        url: url.trim(),
        requires_approval: requiresApproval,
        enabled,
        daily_limit: dailyLimit,
        keep_auth: keepAuth,
        ...(keepAuth ? {} : { bearer: bearer.trim(), hmac: hmac.trim() }),
      };
      return aiSettingsService.saveTool(slug, body);
    },
    onSuccess: async () => {
      setBearer("");
      setHmac("");
      appToast.success(t("ai_settings.tools.saved"));
      await qc.invalidateQueries({ queryKey: ["tenant", slug, "ai", "tools"] });
      onSaved();
    },
    onError: (error) => appToast.error(aiErrorMessage(error, t)),
  });

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {isNew ? t("ai_settings.tools.new") : t("ai_settings.tools.edit")}
          </DialogTitle>
          <DialogDescription>
            {t("ai_settings.tools.dialog_hint")}
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-5"
          onSubmit={(event) => {
            event.preventDefault();
            if (!invalid) save.mutate();
          }}
        >
          <div className="grid gap-4 sm:grid-cols-2">
            <SettingRow
              label={t("ai_settings.tools.name")}
              hint={
                isNew
                  ? t("ai_settings.tools.name_hint")
                  : t("ai_settings.tools.name_locked")
              }
              htmlFor="tool-name"
            >
              <Input
                id="tool-name"
                value={name}
                maxLength={50}
                disabled={!isNew}
                placeholder="order_lookup"
                className="font-mono"
                aria-invalid={Boolean(nameError) && name !== ""}
                onChange={(e) => setName(e.target.value.toLowerCase())}
              />
              {nameError && name !== "" && (
                <p className="text-destructive text-xs">{nameError}</p>
              )}
            </SettingRow>
            <SettingRow
              label={t("ai_settings.tools.daily_limit")}
              hint={t("ai_settings.tools.daily_limit_hint")}
              htmlFor="tool-limit"
            >
              <Input
                id="tool-limit"
                type="number"
                min={1}
                max={1000}
                value={dailyLimit}
                aria-invalid={limitError}
                onChange={(e) => setDailyLimit(Number(e.target.value))}
              />
            </SettingRow>
          </div>
          <SettingRow
            label={t("ai_settings.tools.description_label")}
            hint={t("ai_settings.tools.description_hint")}
            htmlFor="tool-description"
          >
            <Textarea
              id="tool-description"
              rows={2}
              maxLength={1000}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </SettingRow>
          <SettingRow
            label={t("ai_settings.tools.url")}
            hint={t("ai_settings.tools.url_hint")}
            htmlFor="tool-url"
          >
            <Input
              id="tool-url"
              type="url"
              value={url}
              maxLength={2000}
              aria-invalid={Boolean(urlError)}
              onChange={(e) => setUrl(e.target.value)}
            />
            {urlError && url !== "https://" && (
              <p className="text-destructive text-xs">{urlError}</p>
            )}
          </SettingRow>

          <SettingRow
            label={t("ai_settings.tools.schema")}
            hint={t("ai_settings.tools.schema_hint")}
            htmlFor="tool-schema"
          >
            <Textarea
              id="tool-schema"
              rows={10}
              spellCheck={false}
              value={schemaText}
              aria-invalid={!schema.ok}
              className="font-mono text-xs"
              onChange={(e) => setSchemaText(e.target.value)}
            />
            <div className="flex flex-wrap items-center justify-between gap-2">
              {schema.ok ? (
                <p className="text-signal-text text-xs">
                  {t("ai_settings.tools.schema_ok", {
                    count: schema.params.length,
                  })}
                </p>
              ) : (
                <p className="text-destructive text-xs" role="alert">
                  {schema.error}
                </p>
              )}
              <div className="flex gap-2">
                {schema.ok && (
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    onClick={() =>
                      setSchemaText(JSON.stringify(schema.value, null, 2))
                    }
                  >
                    {t("ai_settings.tools.schema_format")}
                  </Button>
                )}
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() =>
                    setSchemaText(JSON.stringify(EXAMPLE_SCHEMA, null, 2))
                  }
                >
                  <Wand2 className="size-4" />
                  {t("ai_settings.tools.schema_example")}
                </Button>
              </div>
            </div>
            {schema.ok && schema.params.length > 0 && (
              <ul className="divide-y rounded-md border text-xs">
                {schema.params.map((p) => (
                  <li key={p.name} className="flex flex-wrap gap-2 px-2 py-1.5">
                    <code className="font-medium">{p.name}</code>
                    <Badge variant="outline" className="text-[10px]">
                      {p.type}
                    </Badge>
                    {p.required && (
                      <Badge variant="secondary" className="text-[10px]">
                        {t("ai_settings.tools.param_required")}
                      </Badge>
                    )}
                    {p.description && (
                      <span className="text-muted-foreground">
                        {p.description}
                      </span>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </SettingRow>

          <fieldset className="space-y-3 rounded-md border p-3">
            <legend className="flex items-center gap-1.5 px-1 text-sm font-medium">
              <KeyRound className="size-4" />
              {t("ai_settings.tools.auth")}
            </legend>
            <p className="text-muted-foreground text-xs">
              {t("ai_settings.tools.auth_hint")}
            </p>
            {tool?.has_auth && (
              <div className="flex items-center gap-3">
                <Switch
                  id="tool-keep-auth"
                  checked={keepAuth}
                  onCheckedChange={setKeepAuth}
                />
                <Label htmlFor="tool-keep-auth" className="font-normal">
                  {t("ai_settings.tools.keep_auth")}
                </Label>
              </div>
            )}
            {!keepAuth && (
              <div className="grid gap-3 sm:grid-cols-2">
                <SettingRow
                  label={t("ai_settings.tools.bearer")}
                  htmlFor="tool-bearer"
                >
                  <Input
                    id="tool-bearer"
                    type="password"
                    autoComplete="off"
                    maxLength={4000}
                    value={bearer}
                    onChange={(e) => setBearer(e.target.value)}
                  />
                </SettingRow>
                <SettingRow
                  label={t("ai_settings.tools.hmac")}
                  htmlFor="tool-hmac"
                >
                  <Input
                    id="tool-hmac"
                    type="password"
                    autoComplete="off"
                    maxLength={4000}
                    value={hmac}
                    onChange={(e) => setHmac(e.target.value)}
                  />
                </SettingRow>
              </div>
            )}
            {!keepAuth && tool?.has_auth && !bearer && !hmac && (
              <Alert>
                <AlertDescription className="text-xs">
                  {t("ai_settings.tools.auth_will_clear")}
                </AlertDescription>
              </Alert>
            )}
            <p className="text-muted-foreground text-xs">
              {t("ai_settings.tools.signature_hint")}
            </p>
          </fieldset>

          <div className="grid gap-3 sm:grid-cols-2">
            <div className="flex items-start justify-between gap-3 rounded-md border p-3">
              <div className="space-y-1">
                <Label htmlFor="tool-approval">
                  {t("ai_settings.tools.requires_approval")}
                </Label>
                <p className="text-muted-foreground text-xs">
                  {t("ai_settings.tools.requires_approval_hint")}
                </p>
              </div>
              <Switch
                id="tool-approval"
                checked={requiresApproval}
                onCheckedChange={setRequiresApproval}
              />
            </div>
            <div className="flex items-start justify-between gap-3 rounded-md border p-3">
              <div className="space-y-1">
                <Label htmlFor="tool-enabled">
                  {t("ai_settings.tools.enabled")}
                </Label>
                <p className="text-muted-foreground text-xs">
                  {t("ai_settings.tools.enabled_hint")}
                </p>
              </div>
              <Switch
                id="tool-enabled"
                checked={enabled}
                onCheckedChange={setEnabled}
              />
            </div>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={invalid || save.isPending}>
              {save.isPending ? t("ai_settings.saving") : t("ai_settings.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
