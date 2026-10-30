"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";

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
import { Textarea } from "@/components/ui/textarea";
import { ApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

import { useSaveObjectType } from "../../hooks/use-custom-field-mutations";
import { OBJECT_ICONS } from "../../lib/object-icons";
import { KEY_PATTERN, slugifyKey } from "../../lib/values";
import type { CustomObjectType } from "../../types";

type ObjectTypeDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  objectType?: CustomObjectType | null;
};

type Draft = {
  singular_label: string;
  plural_label: string;
  key: string;
  description: string;
  icon: string;
};

function schema(t: (key: string) => string, isEdit: boolean) {
  return z.object({
    singular_label: z
      .string()
      .trim()
      .min(1, t("custom_fields.validation.label_required"))
      .max(100, t("custom_fields.validation.label_length")),
    plural_label: z
      .string()
      .trim()
      .min(1, t("custom_fields.validation.label_required"))
      .max(100, t("custom_fields.validation.label_length")),
    key: isEdit
      ? z.string()
      : z
          .string()
          .trim()
          .regex(KEY_PATTERN, t("custom_fields.validation.key_format")),
    description: z
      .string()
      .max(500, t("custom_fields.objects.validation.description_length")),
    icon: z.string(),
  });
}

function toDraft(o?: CustomObjectType | null): Draft {
  return {
    singular_label: o?.singular_label ?? "",
    plural_label: o?.plural_label ?? "",
    key: o?.key ?? "",
    description: o?.description ?? "",
    icon: o?.icon || "box",
  };
}

/** Create / edit a custom object type. */
export function ObjectTypeDialog({
  open,
  onOpenChange,
  objectType,
}: ObjectTypeDialogProps) {
  const { t } = useLocale();
  const isEdit = Boolean(objectType);
  const save = useSaveObjectType();
  const form = useForm<Draft>({
    resolver: zodResolver(schema(t, isEdit) as never),
    defaultValues: toDraft(objectType),
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
    reset(toDraft(objectType));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, objectType?.uuid]);

  // On create the key follows the singular name until edited by hand.
  const followLabel = (value: string) => {
    if (isEdit || dirtyFields.key) return;
    setValue("key", slugifyKey(value));
  };

  const onSubmit = handleSubmit(async (draft) => {
    try {
      await save.mutateAsync({
        uuid: objectType?.uuid,
        body: {
          key: isEdit ? undefined : draft.key.trim(),
          singular_label: draft.singular_label.trim(),
          plural_label: draft.plural_label.trim(),
          description: draft.description.trim(),
          icon: draft.icon,
          // PATCH replaces the type: keep its archived state.
          archived: objectType?.archived ?? false,
        },
      });
      onOpenChange(false);
    } catch (error) {
      if (
        error instanceof ApiError &&
        error.code === "CUSTOM_FIELD_KEY_TAKEN"
      ) {
        setError("key", {
          type: "server",
          message: t("custom_fields.validation.key_taken"),
        });
        return;
      }
      if (error instanceof ApiError && error.isValidation) {
        let mapped = false;
        for (const d of error.details) {
          if (
            d.field === "key" ||
            d.field === "singular_label" ||
            d.field === "plural_label" ||
            d.field === "description"
          ) {
            setError(d.field, { type: "server", message: d.message });
            mapped = true;
          }
        }
        if (mapped) return;
      }
      appToast.error(
        error instanceof Error ? error.message : t("common.error_generic"),
      );
    }
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {isEdit
              ? t("custom_fields.objects.dialog.edit_title")
              : t("custom_fields.objects.dialog.create_title")}
          </DialogTitle>
          <DialogDescription>
            {t("custom_fields.objects.dialog.description")}
          </DialogDescription>
        </DialogHeader>
        <form
          id="custom-object-type-form"
          className="grid gap-4 sm:grid-cols-2"
          onSubmit={onSubmit}
          noValidate
        >
          <FormFieldShell
            name="cot-singular"
            label={t("custom_fields.objects.fields.singular_label")}
            error={errors.singular_label?.message}
          >
            <Input
              id="cot-singular"
              autoFocus
              placeholder={t("custom_fields.objects.placeholders.singular")}
              aria-invalid={Boolean(errors.singular_label)}
              {...register("singular_label", {
                onChange: (e) => followLabel(e.target.value),
              })}
            />
          </FormFieldShell>
          <FormFieldShell
            name="cot-plural"
            label={t("custom_fields.objects.fields.plural_label")}
            error={errors.plural_label?.message}
          >
            <Input
              id="cot-plural"
              placeholder={t("custom_fields.objects.placeholders.plural")}
              aria-invalid={Boolean(errors.plural_label)}
              {...register("plural_label")}
            />
          </FormFieldShell>
          <FormFieldShell
            name="cot-key"
            label={t("custom_fields.fields.key")}
            description={
              isEdit
                ? t("custom_fields.dialog.key_locked")
                : t("custom_fields.dialog.key_hint")
            }
            error={errors.key?.message}
            className="sm:col-span-2"
          >
            <Input
              id="cot-key"
              className="font-mono"
              disabled={isEdit}
              readOnly={isEdit}
              maxLength={63}
              aria-invalid={Boolean(errors.key)}
              {...register("key")}
            />
          </FormFieldShell>
          <FormFieldShell
            name="cot-description"
            label={t("custom_fields.fields.description")}
            error={errors.description?.message}
            className="sm:col-span-2"
          >
            <Textarea
              id="cot-description"
              rows={2}
              maxLength={500}
              {...register("description")}
            />
          </FormFieldShell>
          <FormFieldShell
            name="cot-icon"
            label={t("custom_fields.objects.fields.icon")}
            className="sm:col-span-2"
          >
            <Controller
              control={control}
              name="icon"
              render={({ field }) => (
                <div
                  className="flex flex-wrap gap-1.5"
                  role="radiogroup"
                  aria-label={t("custom_fields.objects.fields.icon")}
                >
                  {Object.entries(OBJECT_ICONS).map(([name, Icon]) => (
                    <Button
                      key={name}
                      type="button"
                      variant="outline"
                      size="icon-sm"
                      role="radio"
                      aria-checked={field.value === name}
                      aria-label={name}
                      className={cn(
                        field.value === name &&
                          "border-primary bg-primary/10 text-primary",
                      )}
                      onClick={() => field.onChange(name)}
                    >
                      <Icon className="size-4" />
                    </Button>
                  ))}
                </div>
              )}
            />
          </FormFieldShell>
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
            form="custom-object-type-form"
            disabled={isSubmitting}
          >
            {isEdit
              ? t("common.save")
              : t("custom_fields.objects.actions.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
