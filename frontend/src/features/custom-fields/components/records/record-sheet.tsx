"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useMemo } from "react";
import { FormProvider, useForm } from "react-hook-form";
import { z } from "zod";

import { FormFieldShell } from "@/components/forms/form-field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { ApiError } from "@/lib/api";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

import { useSaveRecord } from "../../hooks/use-custom-field-mutations";
import { buildCustomFieldsPayload, toInputValues } from "../../lib/values";
import { customFieldsSchema } from "../../schemas/custom-fields-schema";
import type {
  CustomFieldDefinition,
  CustomFieldValues,
  CustomObjectRecord,
  CustomObjectType,
} from "../../types";
import {
  CustomFieldsForm,
  applyCustomFieldApiErrors,
} from "../custom-fields-form";

type RecordSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  objectType: CustomObjectType;
  definitions: readonly CustomFieldDefinition[];
  record?: CustomObjectRecord | null;
};

type RecordDraft = {
  display_name: string;
  custom_fields: CustomFieldValues;
};

/** Create / edit sheet of one custom object record. */
export function RecordSheet({
  open,
  onOpenChange,
  objectType,
  definitions,
  record,
}: RecordSheetProps) {
  const { t } = useLocale();
  const save = useSaveRecord(objectType.uuid);
  const isEdit = Boolean(record);

  const schema = useMemo(
    () =>
      z.object({
        display_name: z
          .string()
          .trim()
          .min(1, t("custom_fields.records.validation.name_required"))
          .max(255, t("custom_fields.records.validation.name_length")),
        custom_fields: customFieldsSchema(definitions, t),
      }),
    [definitions, t],
  );

  const initial = (): RecordDraft => ({
    display_name: record?.display_name ?? "",
    custom_fields: toInputValues(definitions, record?.custom_fields, {
      applyDefaults: !record,
    }),
  });

  const form = useForm<RecordDraft>({
    resolver: zodResolver(schema as never),
    defaultValues: initial(),
  });

  useEffect(() => {
    if (open) form.reset(initial());
    // Reset when the sheet opens or switches record.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, record?.uuid, definitions]);

  const onSubmit = form.handleSubmit(async (draft) => {
    try {
      await save.mutateAsync({
        uuid: record?.uuid,
        body: {
          display_name: draft.display_name.trim(),
          custom_fields: buildCustomFieldsPayload(
            definitions,
            draft.custom_fields,
            { omitEmpty: !isEdit },
          ),
        },
      });
      onOpenChange(false);
    } catch (error) {
      const mapped = applyCustomFieldApiErrors(error, form.setError, t);
      if (
        error instanceof ApiError &&
        error.details.some((d) => d.field === "display_name")
      ) {
        form.setError("display_name", {
          type: "server",
          message: t("custom_fields.records.validation.name_required"),
        });
        return;
      }
      if (!mapped) {
        appToast.error(
          error instanceof Error ? error.message : t("common.error_generic"),
        );
      }
    }
  });

  const nameError = form.formState.errors.display_name?.message;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="flex w-full flex-col gap-0 p-0 sm:max-w-lg">
        <SheetHeader className="border-b p-6">
          <SheetTitle>
            {isEdit
              ? t("custom_fields.records.edit_title", {
                  name: objectType.singular_label,
                })
              : t("custom_fields.records.create_title", {
                  name: objectType.singular_label,
                })}
          </SheetTitle>
          <SheetDescription>
            {t("custom_fields.records.sheet_description")}
          </SheetDescription>
        </SheetHeader>
        <FormProvider {...form}>
          <form
            id="custom-object-record-form"
            className="flex-1 space-y-4 overflow-y-auto p-6"
            onSubmit={onSubmit}
            noValidate
          >
            <FormFieldShell
              name="record-display-name"
              label={t("custom_fields.records.display_name")}
              error={nameError}
            >
              <Input
                id="record-display-name"
                autoFocus
                maxLength={255}
                aria-invalid={Boolean(nameError)}
                {...form.register("display_name")}
              />
            </FormFieldShell>
            <CustomFieldsForm definitions={definitions} />
          </form>
        </FormProvider>
        <div className="flex justify-end gap-2 border-t p-4">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="submit"
            form="custom-object-record-form"
            disabled={form.formState.isSubmitting}
          >
            {isEdit ? t("common.save") : t("custom_fields.records.create")}
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}
