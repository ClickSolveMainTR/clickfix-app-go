"use client";

import { Archive, ArchiveRestore, Plus, Trash2 } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { OPTION_COLORS, optionSwatchClass } from "../../lib/colors";
import {
  uniqueOptionValue,
  type OptionDraft,
} from "../../lib/definition-draft";

type OptionsEditorProps = {
  value: OptionDraft[];
  onChange: (value: OptionDraft[]) => void;
  /** Error per option index, plus `-1` for the list itself. */
  errors?: Record<number, string>;
  disabled?: boolean;
  maxOptions?: number;
};

/**
 * Select options: add, relabel, color, archive. Saved options cannot be
 * removed (stored values point at them), only archived; new ones can.
 */
export function OptionsEditor({
  value,
  onChange,
  errors,
  disabled,
  maxOptions,
}: OptionsEditorProps) {
  const { t } = useLocale();

  const update = (index: number, patch: Partial<OptionDraft>) => {
    onChange(value.map((o, i) => (i === index ? { ...o, ...patch } : o)));
  };

  const add = () => {
    onChange([
      ...value,
      { value: "", label: "", color: "", archived: false, saved: false },
    ]);
  };

  const atLimit = Boolean(maxOptions && value.length >= maxOptions);

  return (
    <div className="flex flex-col gap-2">
      {value.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("custom_fields.options.empty")}
        </p>
      ) : null}
      {value.map((option, index) => {
        const taken = value.filter((_, i) => i !== index).map((o) => o.value);
        const error = errors?.[index];
        return (
          <div key={index} className="flex flex-col gap-1">
            <div className="flex items-center gap-2">
              <ColorPicker
                value={option.color}
                onChange={(color) => update(index, { color })}
                disabled={disabled || option.archived}
              />
              <Input
                value={option.label}
                placeholder={t("custom_fields.options.label_placeholder")}
                aria-label={t("custom_fields.options.label")}
                aria-invalid={Boolean(error)}
                disabled={disabled || option.archived}
                className={cn(
                  "h-8 flex-1",
                  option.archived && "line-through opacity-60",
                )}
                onChange={(e) => {
                  const label = e.target.value;
                  update(
                    index,
                    option.saved
                      ? { label }
                      : { label, value: uniqueOptionValue(label, taken) },
                  );
                }}
              />
              <code
                className="text-muted-foreground hidden w-28 truncate text-xs sm:block"
                title={option.value}
              >
                {option.value || "—"}
              </code>
              {option.archived ? (
                <Badge variant="secondary" className="hidden sm:inline-flex">
                  {t("custom_fields.archived")}
                </Badge>
              ) : null}
              {option.saved ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  disabled={disabled}
                  aria-label={
                    option.archived
                      ? t("custom_fields.options.restore")
                      : t("custom_fields.options.archive")
                  }
                  title={
                    option.archived
                      ? t("custom_fields.options.restore")
                      : t("custom_fields.options.archive")
                  }
                  onClick={() => update(index, { archived: !option.archived })}
                >
                  {option.archived ? (
                    <ArchiveRestore className="size-4" />
                  ) : (
                    <Archive className="size-4" />
                  )}
                </Button>
              ) : (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  disabled={disabled}
                  aria-label={t("custom_fields.options.remove")}
                  title={t("custom_fields.options.remove")}
                  onClick={() => onChange(value.filter((_, i) => i !== index))}
                >
                  <Trash2 className="size-4" />
                </Button>
              )}
            </div>
            {error ? <p className="text-destructive text-xs">{error}</p> : null}
          </div>
        );
      })}
      {errors?.[-1] ? (
        <p className="text-destructive text-xs">{errors[-1]}</p>
      ) : null}
      <div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={add}
          disabled={disabled || atLimit}
        >
          <Plus className="size-4" />
          {t("custom_fields.options.add")}
        </Button>
      </div>
    </div>
  );
}

function ColorPicker({
  value,
  onChange,
  disabled,
}: {
  value: string;
  onChange: (color: string) => void;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="icon-sm"
          disabled={disabled}
          aria-label={t("custom_fields.options.color")}
          title={t("custom_fields.options.color")}
        >
          <span
            className={cn("size-3.5 rounded-full", optionSwatchClass(value))}
          />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-2" align="start">
        <div className="grid grid-cols-5 gap-1.5">
          <button
            type="button"
            className={cn(
              "border-border flex size-6 items-center justify-center rounded-full border text-xs",
              !value && "ring-ring ring-2",
            )}
            aria-label={t("custom_fields.options.no_color")}
            title={t("custom_fields.options.no_color")}
            onClick={() => onChange("")}
          >
            ∅
          </button>
          {OPTION_COLORS.map((color) => (
            <button
              key={color}
              type="button"
              className={cn(
                "size-6 rounded-full",
                optionSwatchClass(color),
                value === color && "ring-ring ring-2 ring-offset-1",
              )}
              aria-label={t(`custom_fields.colors.${color}`)}
              title={t(`custom_fields.colors.${color}`)}
              onClick={() => onChange(color)}
            />
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
}
