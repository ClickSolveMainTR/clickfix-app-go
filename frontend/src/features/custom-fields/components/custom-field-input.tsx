"use client";

import { Check, ChevronsUpDown, X } from "lucide-react";
import { useMemo, useState } from "react";

import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { optionSwatchClass } from "../lib/colors";
import { DEFAULT_CURRENCY, type MoneyInputValue } from "../lib/values";
import type { CustomFieldDefinition, CustomFieldOption } from "../types";
import { OptionBadge } from "./option-badge";
import { useCustomFieldUserSource } from "./user-source";

export type CustomFieldInputProps = {
  /** Definition to render (only type, options and config are read). */
  definition: Pick<
    CustomFieldDefinition,
    "key" | "type" | "options" | "config" | "label"
  >;
  /**
   * Input value (see `toInputValue`): strings for text-like, number and
   * date fields, `{amount, currency}` for money, boolean or `null`,
   * `string[]` for multiselect.
   */
  value: unknown;
  onChange: (value: unknown) => void;
  onBlur?: () => void;
  id?: string;
  disabled?: boolean;
  invalid?: boolean;
  className?: string;
};

const COMMON_CURRENCIES = ["TRY", "USD", "EUR", "GBP"];
const UNSET = "__unset__";

/** The control matching one custom field definition (no label/error chrome). */
export function CustomFieldInput({
  definition,
  value,
  onChange,
  onBlur,
  id,
  disabled,
  invalid,
  className,
}: CustomFieldInputProps) {
  const { t } = useLocale();
  const config = definition.config ?? {};
  const placeholder = config.placeholder || undefined;
  const inputId = id ?? `cf-${definition.key}`;
  const str = typeof value === "string" ? value : "";

  switch (definition.type) {
    case "textarea":
      return (
        <Textarea
          id={inputId}
          value={str}
          onChange={(e) => onChange(e.target.value)}
          onBlur={onBlur}
          placeholder={placeholder}
          maxLength={config.max_length || undefined}
          disabled={disabled}
          aria-invalid={invalid}
          className={className}
          rows={3}
        />
      );
    case "number":
      return (
        <Input
          id={inputId}
          type="text"
          inputMode="decimal"
          value={str}
          onChange={(e) => onChange(e.target.value)}
          onBlur={onBlur}
          placeholder={placeholder}
          disabled={disabled}
          aria-invalid={invalid}
          className={className}
        />
      );
    case "money":
      return (
        <MoneyInput
          id={inputId}
          value={value as MoneyInputValue | undefined}
          defaultCurrency={config.currency || DEFAULT_CURRENCY}
          onChange={onChange}
          onBlur={onBlur}
          placeholder={placeholder}
          disabled={disabled}
          invalid={invalid}
          className={className}
        />
      );
    case "date":
      return (
        <DatePicker
          id={inputId}
          value={str}
          onChange={onChange}
          placeholder={placeholder}
          disabled={disabled}
          aria-invalid={invalid}
          className={cn(invalid && "border-destructive", className)}
          captionLayout="dropdown"
        />
      );
    case "boolean":
      return (
        <Select
          value={value === true ? "true" : value === false ? "false" : UNSET}
          onValueChange={(next) =>
            onChange(next === "true" ? true : next === "false" ? false : null)
          }
          disabled={disabled}
        >
          <SelectTrigger
            id={inputId}
            aria-invalid={invalid}
            className={cn(invalid && "border-destructive", className)}
            onBlur={onBlur}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={UNSET}>
              <span className="text-muted-foreground">
                {t("custom_fields.value.unset")}
              </span>
            </SelectItem>
            <SelectItem value="true">{t("custom_fields.value.yes")}</SelectItem>
            <SelectItem value="false">{t("custom_fields.value.no")}</SelectItem>
          </SelectContent>
        </Select>
      );
    case "select":
      return (
        <SelectOptionInput
          id={inputId}
          options={definition.options ?? []}
          value={str}
          onChange={onChange}
          placeholder={placeholder}
          disabled={disabled}
          invalid={invalid}
          className={className}
        />
      );
    case "multiselect":
      return (
        <MultiSelectInput
          id={inputId}
          options={definition.options ?? []}
          value={Array.isArray(value) ? value.map(String) : []}
          onChange={onChange}
          maxSelected={config.max_selected || undefined}
          placeholder={placeholder}
          disabled={disabled}
          invalid={invalid}
          className={className}
        />
      );
    case "user":
      return (
        <UserInput
          id={inputId}
          value={str}
          onChange={onChange}
          placeholder={placeholder}
          disabled={disabled}
          invalid={invalid}
          className={className}
        />
      );
    case "url":
    case "email":
    case "phone":
    case "text":
    default:
      return (
        <Input
          id={inputId}
          type={
            definition.type === "url"
              ? "url"
              : definition.type === "email"
                ? "email"
                : definition.type === "phone"
                  ? "tel"
                  : "text"
          }
          inputMode={
            definition.type === "phone"
              ? "tel"
              : definition.type === "email"
                ? "email"
                : definition.type === "url"
                  ? "url"
                  : undefined
          }
          value={str}
          onChange={(e) => onChange(e.target.value)}
          onBlur={onBlur}
          placeholder={
            placeholder ?? (definition.type === "url" ? "https://" : undefined)
          }
          maxLength={
            definition.type === "text"
              ? config.max_length || undefined
              : undefined
          }
          disabled={disabled}
          aria-invalid={invalid}
          className={className}
        />
      );
  }
}

function MoneyInput({
  id,
  value,
  defaultCurrency,
  onChange,
  onBlur,
  placeholder,
  disabled,
  invalid,
  className,
}: {
  id: string;
  value: MoneyInputValue | undefined;
  defaultCurrency: string;
  onChange: (value: MoneyInputValue) => void;
  onBlur?: () => void;
  placeholder?: string;
  disabled?: boolean;
  invalid?: boolean;
  className?: string;
}) {
  const { t } = useLocale();
  const amount = value?.amount ?? "";
  const currency = value?.currency || defaultCurrency;
  const currencies = useMemo(
    () => [...new Set([defaultCurrency, currency, ...COMMON_CURRENCIES])],
    [currency, defaultCurrency],
  );
  return (
    <div className={cn("flex gap-2", className)}>
      <Input
        id={id}
        type="text"
        inputMode="decimal"
        value={amount}
        onChange={(e) => onChange({ amount: e.target.value, currency })}
        onBlur={onBlur}
        placeholder={placeholder ?? "0,00"}
        disabled={disabled}
        aria-invalid={invalid}
        className="min-w-0 flex-1"
      />
      <Select
        value={currency}
        onValueChange={(next) => onChange({ amount, currency: next })}
        disabled={disabled}
      >
        <SelectTrigger
          className="w-24 shrink-0"
          aria-label={t("custom_fields.value.currency")}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {currencies.map((code) => (
            <SelectItem key={code} value={code}>
              {code}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

/** Options that can be picked: active ones plus archived ones already chosen. */
function pickableOptions(
  options: readonly CustomFieldOption[],
  chosen: readonly string[],
) {
  return options.filter((o) => !o.archived || chosen.includes(o.value));
}

function SelectOptionInput({
  id,
  options,
  value,
  onChange,
  placeholder,
  disabled,
  invalid,
  className,
}: {
  id: string;
  options: readonly CustomFieldOption[];
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  invalid?: boolean;
  className?: string;
}) {
  const { t } = useLocale();
  const items = useMemo<ComboboxOption[]>(
    () =>
      pickableOptions(options, value ? [value] : []).map((o) => ({
        value: o.value,
        label: o.label,
        description: o.archived ? t("custom_fields.value.archived") : undefined,
      })),
    [options, t, value],
  );
  return (
    <AsyncCombobox
      id={id}
      value={value}
      onValueChange={onChange}
      options={items}
      placeholder={placeholder ?? t("custom_fields.value.pick_option")}
      clearable
      disabled={disabled}
      aria-invalid={invalid}
      className={className}
    />
  );
}

function MultiSelectInput({
  id,
  options,
  value,
  onChange,
  maxSelected,
  placeholder,
  disabled,
  invalid,
  className,
}: {
  id: string;
  options: readonly CustomFieldOption[];
  value: string[];
  onChange: (value: string[]) => void;
  maxSelected?: number;
  placeholder?: string;
  disabled?: boolean;
  invalid?: boolean;
  className?: string;
}) {
  const { t } = useLocale();
  const [open, setOpen] = useState(false);
  const items = pickableOptions(options, value);
  const byValue = new Map(options.map((o) => [o.value, o]));
  const atLimit = Boolean(maxSelected && value.length >= maxSelected);

  const toggle = (optionValue: string) => {
    if (value.includes(optionValue)) {
      onChange(value.filter((v) => v !== optionValue));
      return;
    }
    if (atLimit) return;
    // Keep option order so equal sets look the same everywhere.
    const next = new Set([...value, optionValue]);
    onChange(options.map((o) => o.value).filter((v) => next.has(v)));
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-invalid={invalid}
          disabled={disabled}
          className={cn(
            "h-auto min-h-9 w-full justify-between px-3 py-1.5 font-normal",
            invalid && "border-destructive",
            className,
          )}
        >
          <span className="flex min-w-0 flex-wrap gap-1">
            {value.length ? (
              value.map((v) => {
                const option = byValue.get(v) ?? { label: v, value: v };
                return (
                  <OptionBadge
                    key={v}
                    option={option}
                    archivedLabel={t("custom_fields.value.archived")}
                  />
                );
              })
            ) : (
              <span className="text-muted-foreground">
                {placeholder ?? t("custom_fields.value.pick_options")}
              </span>
            )}
          </span>
          <span className="ms-2 flex shrink-0 items-center gap-1">
            {value.length && !disabled ? (
              <span
                role="button"
                tabIndex={-1}
                className="hover:bg-accent rounded-sm p-0.5 opacity-60 hover:opacity-100"
                aria-label={t("form.clear")}
                onClick={(event) => {
                  event.preventDefault();
                  event.stopPropagation();
                  onChange([]);
                }}
                onPointerDown={(event) => {
                  event.preventDefault();
                  event.stopPropagation();
                }}
              >
                <X className="size-3.5" />
              </span>
            ) : null}
            <ChevronsUpDown className="size-4 opacity-50" />
          </span>
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-[var(--radix-popover-trigger-width)] p-0"
        align="start"
      >
        <Command>
          <CommandInput placeholder={t("form.combobox_search")} />
          <CommandList>
            <CommandEmpty>{t("form.combobox_empty")}</CommandEmpty>
            <CommandGroup>
              {items.map((option) => {
                const selected = value.includes(option.value);
                return (
                  <CommandItem
                    key={option.value}
                    value={`${option.label} ${option.value}`}
                    disabled={!selected && (atLimit || option.archived)}
                    onSelect={() => toggle(option.value)}
                  >
                    <span
                      className={cn(
                        "size-2 shrink-0 rounded-full",
                        optionSwatchClass(option.color),
                      )}
                    />
                    <span
                      className={cn(
                        "flex-1 truncate",
                        option.archived && "text-muted-foreground line-through",
                      )}
                    >
                      {option.label}
                    </span>
                    {selected ? <Check className="size-4" /> : null}
                  </CommandItem>
                );
              })}
            </CommandGroup>
          </CommandList>
        </Command>
        {maxSelected ? (
          <p className="text-muted-foreground border-t px-3 py-2 text-xs">
            {t("custom_fields.value.max_selected", { count: maxSelected })}
          </p>
        ) : null}
      </PopoverContent>
    </Popover>
  );
}

function UserInput({
  id,
  value,
  onChange,
  placeholder,
  disabled,
  invalid,
  className,
}: {
  id: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  invalid?: boolean;
  className?: string;
}) {
  const { t } = useLocale();
  const source = useCustomFieldUserSource();
  const initialOptions = useMemo<ComboboxOption[]>(
    () =>
      value ? [{ value, label: source.resolveLabel(value) ?? value }] : [],
    [source, value],
  );
  return (
    <AsyncCombobox
      id={id}
      value={value}
      onValueChange={onChange}
      loadOptions={source.loadOptions}
      initialOptions={initialOptions}
      placeholder={placeholder ?? t("custom_fields.user.placeholder")}
      searchPlaceholder={t("custom_fields.user.search")}
      emptyText={t("custom_fields.user.empty")}
      clearable
      disabled={disabled}
      aria-invalid={invalid}
      className={className}
    />
  );
}
