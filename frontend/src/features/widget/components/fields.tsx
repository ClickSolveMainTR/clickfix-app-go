"use client";

import { Plus, Trash2 } from "lucide-react";
import { createContext, useContext, useId, type ReactNode } from "react";

import { BrandLogo, type BrandSlug } from "@/components/brand-logos";
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
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  getPath,
  issuesAt,
  isValidColor,
  LOCALES,
  type Issue,
  type Localized,
  type URLRule,
  type WidgetSettings,
} from "@/features/widget/lib/settings";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type Editor = {
  settings: WidgetSettings;
  set: (path: string, value: unknown) => void;
  issues: Issue[];
  disabled: boolean;
};

const EditorContext = createContext<Editor | null>(null);

export const EditorProvider = EditorContext.Provider;

export function useEditor(): Editor {
  const ctx = useContext(EditorContext);
  if (!ctx) throw new Error("useEditor outside EditorProvider");
  return ctx;
}

/** Inline messages of a path (errors first). */
export function FieldIssues({ path }: { path: string }) {
  const { t } = useLocale();
  const { issues } = useEditor();
  const list = issuesAt(issues, path);
  if (!list.length) return null;
  return (
    <ul className="space-y-0.5" data-testid={`issues-${path}`}>
      {list.slice(0, 3).map((i, n) => (
        <li
          key={`${i.path}-${n}`}
          role={i.level === "warning" ? "status" : "alert"}
          className={cn(
            "text-xs",
            i.level === "warning" ? "text-amber-600" : "text-destructive",
          )}
        >
          {issueText(t, i)}
        </li>
      ))}
    </ul>
  );
}

/** Localized message of an issue (API code, else its message). */
export function issueText(
  t: (key: string, vars?: Record<string, string | number>) => string,
  i: Issue,
): string {
  const known = [
    "invalid",
    "too_long",
    "out_of_range",
    "duplicate",
    "limit",
    "required",
    "unsafe",
    "unknown_key",
    "contrast",
    "contrast_bg",
    "identity",
  ];
  if (i.code === "unsafe" && i.message)
    return `${t("widget.issue.unsafe")} ${i.message}`;
  return known.includes(i.code) ? t(`widget.issue.${i.code}`) : i.message;
}

export function Row({
  label,
  brand,
  hint,
  path,
  htmlFor,
  children,
  className,
}: {
  label: string;
  /** External company the field is about; its logo sits before the label. */
  brand?: BrandSlug;
  hint?: string;
  path?: string;
  htmlFor?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("space-y-1.5", className)}>
      <Label
        htmlFor={htmlFor}
        className={brand ? "flex items-center gap-1.5" : undefined}
      >
        {brand ? (
          <BrandLogo slug={brand} className="size-3.5" decorative />
        ) : null}
        {label}
      </Label>
      {children}
      {hint ? <p className="text-muted-foreground text-xs">{hint}</p> : null}
      {path ? <FieldIssues path={path} /> : null}
    </div>
  );
}

export function SwitchRow({
  path,
  label,
  hint,
  disabled,
}: {
  path: string;
  label: string;
  hint?: string;
  disabled?: boolean;
}) {
  const id = useId();
  const ed = useEditor();
  return (
    <div className="flex items-start justify-between gap-3 rounded-lg border p-3">
      <div className="space-y-0.5">
        <Label htmlFor={id}>{label}</Label>
        {hint ? <p className="text-muted-foreground text-xs">{hint}</p> : null}
        <FieldIssues path={path} />
      </div>
      <Switch
        id={id}
        checked={getPath(ed.settings, path) === true}
        disabled={ed.disabled || disabled}
        onCheckedChange={(v) => ed.set(path, v)}
      />
    </div>
  );
}

export function SelectRow({
  path,
  label,
  hint,
  options,
  disabled,
}: {
  path: string;
  label: string;
  hint?: string;
  options: [string, string][];
  disabled?: boolean;
}) {
  const id = useId();
  const ed = useEditor();
  return (
    <Row label={label} hint={hint} path={path} htmlFor={id}>
      <Select
        value={String(getPath(ed.settings, path) ?? "")}
        disabled={ed.disabled || disabled}
        onValueChange={(v) => ed.set(path, v)}
      >
        <SelectTrigger id={id} className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map(([value, text]) => (
            <SelectItem key={value} value={value}>
              {text}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Row>
  );
}

export function TextRow({
  path,
  label,
  brand,
  hint,
  maxLength,
  placeholder,
  multiline,
  type = "text",
  mono,
  disabled,
}: {
  path: string;
  label: string;
  brand?: BrandSlug;
  hint?: string;
  maxLength?: number;
  placeholder?: string;
  multiline?: boolean;
  type?: string;
  mono?: boolean;
  disabled?: boolean;
}) {
  const id = useId();
  const ed = useEditor();
  const value = String(getPath(ed.settings, path) ?? "");
  const invalid = issuesAt(ed.issues, path).some((i) => i.level !== "warning");
  const props = {
    id,
    value,
    maxLength,
    placeholder,
    disabled: ed.disabled || disabled,
    "aria-invalid": invalid || undefined,
    className: mono ? "font-mono text-xs" : undefined,
  };
  return (
    <Row label={label} brand={brand} hint={hint} path={path} htmlFor={id}>
      {multiline ? (
        <Textarea
          {...props}
          rows={4}
          onChange={(e) => ed.set(path, e.target.value)}
        />
      ) : (
        <Input
          {...props}
          type={type}
          onChange={(e) => ed.set(path, e.target.value)}
        />
      )}
    </Row>
  );
}

export function NumberRow({
  path,
  label,
  hint,
  min,
  max,
  step = 1,
  unit,
}: {
  path: string;
  label: string;
  hint?: string;
  min: number;
  max: number;
  step?: number;
  unit?: string;
}) {
  const id = useId();
  const ed = useEditor();
  const value = Number(getPath(ed.settings, path) ?? min);
  return (
    <Row label={label} hint={hint} path={path} htmlFor={id}>
      <div className="flex items-center gap-3">
        <Slider
          aria-label={label}
          min={min}
          max={max}
          step={step}
          value={[value]}
          disabled={ed.disabled}
          onValueChange={(v) => ed.set(path, v[0] ?? min)}
          className="flex-1"
        />
        <Input
          id={id}
          type="number"
          inputMode="numeric"
          className="w-24"
          min={min}
          max={max}
          step={step}
          value={value}
          disabled={ed.disabled}
          onChange={(e) => {
            const n = Math.trunc(Number(e.target.value));
            if (Number.isFinite(n))
              ed.set(path, Math.min(max, Math.max(min, n)));
          }}
        />
        {unit ? (
          <span className="text-muted-foreground w-8 text-xs">{unit}</span>
        ) : null}
      </div>
    </Row>
  );
}

export function ColorRow({
  path,
  label,
  hint,
  allowEmpty,
  fallback,
}: {
  path: string;
  label: string;
  hint?: string;
  allowEmpty?: boolean;
  fallback: string;
}) {
  const { t } = useLocale();
  const id = useId();
  const ed = useEditor();
  const value = String(getPath(ed.settings, path) ?? "");
  return (
    <Row label={label} hint={hint} path={path} htmlFor={id}>
      <div className="flex gap-2">
        <input
          type="color"
          aria-label={label}
          className="h-9 w-12 cursor-pointer rounded-md border"
          value={isValidColor(value) ? value : fallback}
          disabled={ed.disabled}
          onChange={(e) => ed.set(path, e.target.value)}
        />
        <Input
          id={id}
          value={value}
          placeholder={allowEmpty ? t("widget.f.color_inherit") : "#4f46e5"}
          maxLength={7}
          className="font-mono"
          disabled={ed.disabled}
          aria-invalid={Boolean(value) && !isValidColor(value)}
          onChange={(e) => ed.set(path, e.target.value.trim())}
        />
        {allowEmpty && value ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={ed.disabled}
            onClick={() => ed.set(path, "")}
          >
            {t("widget.editor.reset")}
          </Button>
        ) : null}
      </div>
    </Row>
  );
}

/** One input per language (tr, en) for a Localized value. */
export function LocalizedRow({
  path,
  label,
  hint,
  maxLength,
  multiline,
  placeholder,
}: {
  path: string;
  label: string;
  hint?: string;
  maxLength: number;
  multiline?: boolean;
  placeholder?: Partial<Record<string, string>>;
}) {
  const ed = useEditor();
  const value = (getPath(ed.settings, path) ?? {}) as Localized;
  return (
    <Row label={label} hint={hint} path={path}>
      <div className="space-y-2">
        {LOCALES.map((loc) => {
          const props = {
            value: value[loc] ?? "",
            maxLength,
            placeholder: placeholder?.[loc],
            disabled: ed.disabled,
            "aria-label": `${label} (${loc.toUpperCase()})`,
          };
          const onChange = (v: string) => ed.set(path, { ...value, [loc]: v });
          return (
            <div key={loc} className="flex items-start gap-2">
              <span className="bg-muted text-muted-foreground mt-2 w-8 shrink-0 rounded px-1 text-center text-[10px] font-semibold uppercase">
                {loc}
              </span>
              {multiline ? (
                <Textarea
                  {...props}
                  rows={2}
                  onChange={(e) => onChange(e.target.value)}
                />
              ) : (
                <Input {...props} onChange={(e) => onChange(e.target.value)} />
              )}
            </div>
          );
        })}
      </div>
    </Row>
  );
}

export const URL_OPS = [
  "contains",
  "equals",
  "starts_with",
  "ends_with",
  "glob",
  "not_contains",
] as const;

/** Editable list of URL rules. */
export function UrlRulesRow({
  path,
  label,
  hint,
}: {
  path: string;
  label: string;
  hint?: string;
}) {
  const { t } = useLocale();
  const ed = useEditor();
  const rules = (getPath(ed.settings, path) ?? []) as URLRule[];
  const update = (next: URLRule[]) => ed.set(path, next);
  return (
    <Row label={label} hint={hint} path={path}>
      <div className="space-y-2">
        {rules.map((r, i) => (
          <div key={i} className="flex gap-2">
            <Select
              value={r.op}
              disabled={ed.disabled}
              onValueChange={(v) =>
                update(
                  rules.map((x, n) =>
                    n === i ? { ...x, op: v as URLRule["op"] } : x,
                  ),
                )
              }
            >
              <SelectTrigger
                className="w-40 shrink-0"
                aria-label={t("widget.f.url_op")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {URL_OPS.map((op) => (
                  <SelectItem key={op} value={op}>
                    {t(`widget.url_op.${op}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Input
              value={r.value}
              maxLength={300}
              placeholder="/pricing"
              aria-label={t("widget.f.url_value")}
              disabled={ed.disabled}
              onChange={(e) =>
                update(
                  rules.map((x, n) =>
                    n === i ? { ...x, value: e.target.value } : x,
                  ),
                )
              }
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={t("widget.editor.remove")}
              disabled={ed.disabled}
              onClick={() => update(rules.filter((_, n) => n !== i))}
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        ))}
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={ed.disabled || rules.length >= 20}
          onClick={() => update([...rules, { op: "contains", value: "" }])}
        >
          <Plus className="size-4" />
          {t("widget.editor.add_rule")}
        </Button>
      </div>
    </Row>
  );
}

/** A titled group of fields inside a section. */
export function Group({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <section className="space-y-4 rounded-xl border p-4">
      <div>
        <h3 className="text-sm font-semibold">{title}</h3>
        {description ? (
          <p className="text-muted-foreground text-xs">{description}</p>
        ) : null}
      </div>
      {children}
    </section>
  );
}
