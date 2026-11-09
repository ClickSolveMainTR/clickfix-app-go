"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import get from "lodash/get";
import { useEffect, useMemo, type ReactNode } from "react";
import { Controller, useForm, useWatch } from "react-hook-form";

import { FormFieldShell } from "@/components/forms/form-field";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { ApiError } from "@/lib/api";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  useCreateCustomField,
  useUpdateCustomField,
} from "../../hooks/use-custom-field-mutations";
import {
  configFromDraft,
  configKeysFor,
  createRequestFromDraft,
  draftFromDefinition,
  emptyDraft,
  hasOptions,
  optionsFromDraft,
  updateRequestFromDraft,
  type ConfigDraft,
  type DefinitionDraft,
} from "../../lib/definition-draft";
import { slugifyKey, toInputValue } from "../../lib/values";
import { fieldDefinitionSchema } from "../../schemas/field-definition";
import type { CustomFieldScope } from "../../services/custom-fields.service";
import {
  CUSTOM_FIELD_TYPES,
  VISIBILITY_KEYS,
  WIDGET_ACCESS_LEVELS,
  type CustomFieldDefinition,
  type CustomFieldType,
} from "../../types";
import { CustomFieldInput } from "../custom-field-input";
import { ConditionEditor } from "./condition-editor";
import { OptionsEditor } from "./options-editor";

type FieldDefinitionDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  scope: CustomFieldScope;
  /** Field to edit; omitted → create. */
  definition?: CustomFieldDefinition | null;
  /** Every field of the entity (for condition targets and key checks). */
  siblings: readonly CustomFieldDefinition[];
  maxOptions?: number;
  /** Create only: prefilled values, e.g. from an undefined-key warning. */
  initial?: Partial<DefinitionDraft>;
};

/** Fields whose condition chain leads back to `key` (would form a cycle). */
function dependsOn(
  key: string,
  start: CustomFieldDefinition,
  byKey: Map<string, CustomFieldDefinition>,
): boolean {
  const seen = new Set<string>();
  let cur: CustomFieldDefinition | undefined = start;
  while (cur?.condition && !seen.has(cur.key)) {
    if (cur.condition.field === key) return true;
    seen.add(cur.key);
    cur = byKey.get(cur.condition.field);
  }
  return false;
}

/** Create / edit dialog of one custom field definition. */
export function FieldDefinitionDialog({
  open,
  onOpenChange,
  scope,
  definition,
  siblings,
  maxOptions,
  initial,
}: FieldDefinitionDialogProps) {
  const { t } = useLocale();
  const isEdit = Boolean(definition);
  const create = useCreateCustomField();
  const update = useUpdateCustomField();

  const others = useMemo(
    () => siblings.filter((s) => s.uuid !== definition?.uuid),
    [siblings, definition?.uuid],
  );

  const form = useForm<DefinitionDraft>({
    resolver: zodResolver(fieldDefinitionSchema(t) as never),
    defaultValues: definition
      ? draftFromDefinition(definition, others)
      : { ...emptyDraft(initial?.type), ...initial },
  });
  const {
    control,
    register,
    reset,
    setValue,
    setError,
    handleSubmit,
    formState: { errors, isSubmitting, dirtyFields },
  } = form;

  useEffect(() => {
    if (!open) return;
    reset(
      definition
        ? draftFromDefinition(definition, others)
        : { ...emptyDraft(initial?.type), ...initial },
    );
    // Reset only when the dialog opens or switches field.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, definition?.uuid]);

  const type = useWatch({ control, name: "type" });
  const sensitive = useWatch({ control, name: "sensitive" });
  const options = useWatch({ control, name: "options" });
  const configDraft = useWatch({ control, name: "config" });

  // On create the key follows the label until it is edited by hand (dirty).
  const followLabel = (value: string) => {
    if (isEdit || dirtyFields.key) return;
    setValue("key", slugifyKey(value));
  };

  const candidates = useMemo(() => {
    const byKey = new Map(others.map((o) => [o.key, o]));
    const ownKey = definition?.key;
    return others.filter(
      (o) => !o.archived && !(ownKey && dependsOn(ownKey, o, byKey)),
    );
  }, [definition?.key, others]);

  // Live definition for the default value input.
  const previewDef = useMemo(
    () => ({
      key: "default",
      label: "",
      type,
      options: optionsFromDraft(type, options ?? []),
      config: configFromDraft(type, configDraft ?? ({} as ConfigDraft)),
    }),
    [configDraft, options, type],
  );

  const err = (path: string) =>
    get(errors, path)?.message as string | undefined;

  const applyServerErrors = (error: unknown) => {
    if (!(error instanceof ApiError)) return false;
    if (error.code === "CUSTOM_FIELD_KEY_TAKEN") {
      setError("key", {
        type: "server",
        message: t("custom_fields.validation.key_taken"),
      });
      return true;
    }
    if (!error.isValidation) return false;
    let mapped = false;
    for (const d of error.details) {
      if (!d.field) continue;
      const path = d.field.replace(/\[(\d+)\]/g, ".$1");
      const known = [
        "label",
        "key",
        "type",
        "description",
        "options",
        "default",
        "condition.field",
        "condition.operator",
        "condition.value",
      ];
      const target = path.startsWith("options.")
        ? path
        : path.startsWith("config.")
          ? path
          : known.includes(path)
            ? path
            : null;
      if (!target) continue;
      setError(target as never, {
        type: "server",
        message: d.message ?? t("custom_fields.api_errors.invalid"),
      });
      mapped = true;
    }
    return mapped;
  };

  const onSubmit = handleSubmit(async (draft) => {
    try {
      if (definition) {
        await update.mutateAsync({
          uuid: definition.uuid,
          body: updateRequestFromDraft(draft, others),
        });
      } else {
        await create.mutateAsync(createRequestFromDraft(draft, scope, others));
      }
      onOpenChange(false);
    } catch (error) {
      const mapped = applyServerErrors(error);
      if (!mapped) {
        appToast.error(
          error instanceof Error ? error.message : t("common.error_generic"),
        );
      }
    }
  });

  const configKeys = configKeysFor(type);
  const optionErrors = useMemo(() => {
    const out: Record<number, string> = {};
    const list = get(errors, "options") as
      ({ message?: string } & Record<number, { message?: string }>) | undefined;
    if (!list) return out;
    if (list.message) out[-1] = list.message;
    (options ?? []).forEach((_, i) => {
      const m = list[i]?.message;
      if (m) out[i] = m;
    });
    return out;
  }, [errors, options]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {isEdit
              ? t("custom_fields.dialog.edit_title")
              : t("custom_fields.dialog.create_title")}
          </DialogTitle>
          <DialogDescription>
            {t("custom_fields.dialog.description")}{" "}
            {t("custom_fields.dialog.live_notice")}
          </DialogDescription>
        </DialogHeader>

        <form
          id="custom-field-definition-form"
          className="flex flex-col gap-5"
          onSubmit={onSubmit}
          noValidate
        >
          <div className="grid gap-4 sm:grid-cols-2">
            <FormFieldShell
              name="cf-def-label"
              label={t("custom_fields.fields.label")}
              error={err("label")}
            >
              <Input
                id="cf-def-label"
                autoFocus
                maxLength={100}
                aria-invalid={Boolean(err("label"))}
                {...register("label", {
                  onChange: (e) => followLabel(e.target.value),
                })}
              />
            </FormFieldShell>
            <FormFieldShell
              name="cf-def-key"
              label={t("custom_fields.fields.key")}
              description={
                isEdit
                  ? t("custom_fields.dialog.key_locked")
                  : t("custom_fields.dialog.key_hint")
              }
              error={err("key")}
            >
              <Input
                id="cf-def-key"
                className="font-mono"
                readOnly={isEdit}
                disabled={isEdit}
                maxLength={63}
                aria-invalid={Boolean(err("key"))}
                {...register("key")}
              />
            </FormFieldShell>
            <FormFieldShell
              name="cf-def-type"
              label={t("custom_fields.fields.type")}
              description={
                isEdit ? t("custom_fields.dialog.type_locked") : undefined
              }
              error={err("type")}
            >
              <Controller
                control={control}
                name="type"
                render={({ field }) => (
                  <Select
                    value={field.value}
                    disabled={isEdit}
                    onValueChange={(next) => {
                      const nextType = next as CustomFieldType;
                      field.onChange(nextType);
                      setValue(
                        "default",
                        toInputValue({ type: nextType, config: {} }, undefined),
                      );
                    }}
                  >
                    <SelectTrigger id="cf-def-type">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {CUSTOM_FIELD_TYPES.map((ft) => (
                        <SelectItem key={ft} value={ft}>
                          {t(`custom_fields.types.${ft}`)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </FormFieldShell>
            <div className="flex items-end pb-2">
              <Controller
                control={control}
                name="required"
                render={({ field }) => (
                  <div className="flex items-center gap-2">
                    <Switch
                      id="cf-def-required"
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                    <Label htmlFor="cf-def-required" className="font-normal">
                      {t("custom_fields.fields.required")}
                    </Label>
                  </div>
                )}
              />
            </div>
            <FormFieldShell
              name="cf-def-description"
              label={t("custom_fields.fields.description")}
              className="sm:col-span-2"
            >
              <Textarea
                id="cf-def-description"
                rows={2}
                {...register("description")}
              />
            </FormFieldShell>
          </div>

          {hasOptions(type) ? (
            <Section title={t("custom_fields.dialog.section_options")}>
              <Controller
                control={control}
                name="options"
                render={({ field }) => (
                  <OptionsEditor
                    value={field.value}
                    onChange={field.onChange}
                    errors={optionErrors}
                    maxOptions={maxOptions}
                  />
                )}
              />
            </Section>
          ) : null}

          {configKeys.length ? (
            <Section title={t("custom_fields.dialog.section_config")}>
              <div className="grid gap-4 sm:grid-cols-2">
                {configKeys.map((key) => (
                  <FormFieldShell
                    key={key}
                    name={`cf-def-config-${key}`}
                    label={t(`custom_fields.config.${key}`)}
                    error={err(`config.${key}`)}
                  >
                    <Input
                      id={`cf-def-config-${key}`}
                      inputMode={
                        key === "placeholder" || key === "currency"
                          ? undefined
                          : "decimal"
                      }
                      maxLength={key === "currency" ? 3 : undefined}
                      className={key === "currency" ? "uppercase" : undefined}
                      placeholder={key === "currency" ? "TRY" : undefined}
                      aria-invalid={Boolean(err(`config.${key}`))}
                      {...register(`config.${key}`)}
                    />
                  </FormFieldShell>
                ))}
              </div>
            </Section>
          ) : null}

          {type !== "user" ? (
            <Section title={t("custom_fields.dialog.section_default")}>
              <FormFieldShell
                name="cf-default"
                description={t("custom_fields.dialog.default_hint")}
                error={err("default")}
              >
                <Controller
                  control={control}
                  name="default"
                  render={({ field }) => (
                    <CustomFieldInput
                      definition={previewDef}
                      value={field.value}
                      onChange={field.onChange}
                      id="cf-default"
                      invalid={Boolean(err("default"))}
                    />
                  )}
                />
              </FormFieldShell>
            </Section>
          ) : null}

          <Section
            title={t("custom_fields.dialog.section_visibility")}
            description={t("custom_fields.dialog.visibility_hint")}
          >
            <div className="grid gap-3 sm:grid-cols-2">
              {VISIBILITY_KEYS.map((key) => (
                <Controller
                  key={key}
                  control={control}
                  name={`visibility.${key}`}
                  render={({ field }) => {
                    const blocked =
                      sensitive && (key === "ai" || key === "reports");
                    return (
                      <div className="flex items-start gap-2">
                        <Switch
                          id={`cf-def-visibility-${key}`}
                          checked={field.value && !blocked}
                          disabled={blocked}
                          onCheckedChange={field.onChange}
                        />
                        <div className="space-y-0.5">
                          <Label
                            htmlFor={`cf-def-visibility-${key}`}
                            className="font-normal"
                          >
                            {t(`custom_fields.visibility.${key}`)}
                          </Label>
                          <p className="text-muted-foreground text-xs">
                            {blocked
                              ? t("custom_fields.visibility.blocked_sensitive")
                              : t(`custom_fields.visibility.${key}_hint`)}
                          </p>
                        </div>
                      </div>
                    );
                  }}
                />
              ))}
            </div>
            <Controller
              control={control}
              name="sensitive"
              render={({ field }) => (
                <div className="flex items-start gap-2">
                  <Switch
                    id="cf-def-sensitive"
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                  <div className="space-y-0.5">
                    <Label htmlFor="cf-def-sensitive" className="font-normal">
                      {t("custom_fields.fields.sensitive")}
                    </Label>
                    <p className="text-muted-foreground text-xs">
                      {t("custom_fields.dialog.sensitive_hint")}
                    </p>
                  </div>
                </div>
              )}
            />
            {scope.entityType === "contact" ? (
              <FormFieldShell
                name="cf-def-widget-access"
                label={t("custom_fields.fields.widget_access")}
                description={t("custom_fields.dialog.widget_access_hint", {
                  key: form.getValues("key") || "alan_anahtari",
                })}
              >
                <Controller
                  control={control}
                  name="widgetAccess"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger id="cf-def-widget-access">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {WIDGET_ACCESS_LEVELS.map((level) => (
                          <SelectItem key={level} value={level}>
                            {t(`custom_fields.widget_access.${level}`)}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                />
              </FormFieldShell>
            ) : null}
          </Section>

          <Section
            title={t("custom_fields.dialog.section_condition")}
            description={t("custom_fields.dialog.condition_hint")}
          >
            <Controller
              control={control}
              name="condition"
              render={({ field }) => (
                <ConditionEditor
                  value={field.value}
                  onChange={field.onChange}
                  candidates={candidates}
                  errors={{
                    field: err("condition.field"),
                    operator: err("condition.operator"),
                    value: err("condition.value"),
                  }}
                />
              )}
            />
          </Section>
        </form>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="submit"
            form="custom-field-definition-form"
            disabled={isSubmitting}
          >
            {isEdit ? t("common.save") : t("custom_fields.actions.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Section({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <section className="flex flex-col gap-3">
      <Separator />
      <div className="space-y-0.5">
        <h3 className="text-sm font-semibold">{title}</h3>
        {description ? (
          <p className="text-muted-foreground text-xs">{description}</p>
        ) : null}
      </div>
      {children}
    </section>
  );
}
