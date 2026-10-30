"use client";

import { ArrowDown, ArrowUp, Plus, Trash2 } from "lucide-react";
import { useState } from "react";

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
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  useCustomFieldDefinitions,
  type CustomFieldDefinition,
} from "@/features/custom-fields";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import type {
  RequestForm,
  RequestFormField,
  RequestFormInput,
} from "../services/request-forms.service";

export const SYSTEM_KEYS = [
  "name",
  "email",
  "phone",
  "subject",
  "message",
  "priority",
] as const;
export const CHANNELS = ["widget", "web", "email", "agent"] as const;
const OPERATORS = ["equals", "not_equals", "is_set", "is_not_set"] as const;
const NONE = "__none__";

type Draft = RequestFormField & { id: string };

let seq = 0;
const nextId = () => `f${++seq}`;

/** Form key of a field (the server derives custom keys the same way). */
function keyOf(f: RequestFormField, defs: CustomFieldDefinition[]): string {
  if (f.source === "system") return f.key ?? "";
  const d = defs.find((x) => x.uuid === f.custom_field_uuid);
  return d ? `${f.source}.${d.key}` : (f.key ?? "");
}

/** Create or edit a request form: channels and an ordered field list. */
export function FormDialog({
  open,
  onOpenChange,
  form,
  pending,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  form: RequestForm | null;
  pending: boolean;
  onSubmit: (body: RequestFormInput) => void;
}) {
  const { t } = useLocale();
  const convDefs = useCustomFieldDefinitions({ entityType: "conversation" });
  const contactDefs = useCustomFieldDefinitions({ entityType: "contact" });
  const defs = [...(convDefs.data ?? []), ...(contactDefs.data ?? [])].filter(
    (d) => !d.archived,
  );
  const [name, setName] = useState(form?.name ?? "");
  const [description, setDescription] = useState(form?.description ?? "");
  const [active, setActive] = useState(form?.is_active ?? true);
  const [channels, setChannels] = useState<string[]>(
    form?.channels ?? ["widget"],
  );
  const [fields, setFields] = useState<Draft[]>(() =>
    (
      form?.fields ?? [
        { key: "name", source: "system", required: true },
        { key: "email", source: "system", required: true },
        { key: "message", source: "system", required: true },
      ]
    ).map((f) => ({ ...f, id: nextId() })),
  );
  const [adding, setAdding] = useState("");

  const used = new Set(fields.map((f) => keyOf(f, defs)));
  const addable = [
    ...SYSTEM_KEYS.filter((k) => !used.has(k)).map((k) => ({
      value: `system:${k}`,
      label: t(`request_forms.system.${k}`),
    })),
    ...defs
      .filter((d) => !used.has(`${d.entity_type}.${d.key}`))
      .map((d) => ({
        value: `${d.entity_type}:${d.uuid}`,
        label: `${d.label} (${t(`request_forms.source.${d.entity_type}`)})`,
      })),
  ];

  const add = (value: string) => {
    const [source, ref] = value.split(":");
    const f: Draft =
      source === "system"
        ? { id: nextId(), key: ref, source: "system" }
        : {
            id: nextId(),
            source: source as Draft["source"],
            custom_field_uuid: ref,
          };
    setFields((l) => [...l, f]);
    setAdding("");
  };
  const patch = (id: string, p: Partial<Draft>) =>
    setFields((l) => l.map((f) => (f.id === id ? { ...f, ...p } : f)));
  const move = (i: number, d: -1 | 1) =>
    setFields((l) => {
      const n = [...l];
      const j = i + d;
      if (j < 0 || j >= n.length) return l;
      [n[i], n[j]] = [n[j], n[i]];
      return n;
    });

  const labelOf = (f: RequestFormField) => {
    if (f.source === "system") return t(`request_forms.system.${f.key}`);
    return defs.find((d) => d.uuid === f.custom_field_uuid)?.label ?? "?";
  };
  const defOf = (key: string) => {
    const f = fields.find((x) => keyOf(x, defs) === key);
    if (!f || f.source === "system") return null;
    return defs.find((d) => d.uuid === f.custom_field_uuid) ?? null;
  };

  const submit = () =>
    onSubmit({
      name: name.trim(),
      description: description.trim(),
      is_active: active,
      channels: channels as RequestFormInput["channels"],
      fields: fields.map(({ id: _id, ...f }) => ({
        ...f,
        key: keyOf(f, defs),
      })),
    });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {form ? t("request_forms.edit") : t("request_forms.new")}
          </DialogTitle>
          <DialogDescription>
            {t("request_forms.dialog_hint")}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="rf-name">{t("request_forms.fields.name")}</Label>
              <Input
                id="rf-name"
                value={name}
                maxLength={120}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="flex items-end gap-2 pb-2">
              <Switch
                id="rf-active"
                checked={active}
                onCheckedChange={setActive}
              />
              <Label htmlFor="rf-active">
                {t("request_forms.fields.active")}
              </Label>
            </div>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="rf-desc">
              {t("request_forms.fields.description")}
            </Label>
            <Textarea
              id="rf-desc"
              rows={2}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="text-muted-foreground mr-1 text-xs">
              {t("request_forms.fields.channels")}
            </span>
            {CHANNELS.map((c) => (
              <button
                key={c}
                type="button"
                className={cn(
                  "rounded-md border px-2 py-1 text-xs",
                  channels.includes(c)
                    ? "border-primary bg-primary/10"
                    : "text-muted-foreground hover:bg-muted",
                )}
                onClick={() =>
                  setChannels((l) =>
                    l.includes(c) ? l.filter((x) => x !== c) : [...l, c],
                  )
                }
              >
                {t(`request_forms.channel.${c}`)}
              </button>
            ))}
          </div>

          <div className="space-y-2">
            <div className="text-sm font-medium">
              {t("request_forms.fields.title")}
            </div>
            {fields.map((f, i) => {
              const key = keyOf(f, defs);
              const earlier = fields
                .slice(0, i)
                .map((x) => ({ key: keyOf(x, defs), label: labelOf(x) }));
              const condDef = f.condition ? defOf(f.condition.field) : null;
              return (
                <div key={f.id} className="space-y-2 rounded-md border p-3">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">{labelOf(f)}</span>
                    <span className="text-muted-foreground text-xs">
                      {t(`request_forms.source.${f.source}`)}
                    </span>
                    <div className="ml-auto flex gap-1">
                      <Button
                        size="icon"
                        variant="ghost"
                        aria-label={t("request_forms.move_up")}
                        disabled={i === 0}
                        onClick={() => move(i, -1)}
                      >
                        <ArrowUp className="size-4" />
                      </Button>
                      <Button
                        size="icon"
                        variant="ghost"
                        aria-label={t("request_forms.move_down")}
                        disabled={i === fields.length - 1}
                        onClick={() => move(i, 1)}
                      >
                        <ArrowDown className="size-4" />
                      </Button>
                      <Button
                        size="icon"
                        variant="ghost"
                        aria-label={t("request_forms.remove_field")}
                        onClick={() =>
                          setFields((l) => l.filter((x) => x.id !== f.id))
                        }
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    </div>
                  </div>
                  <div className="grid gap-2 sm:grid-cols-2">
                    <Input
                      value={f.label ?? ""}
                      maxLength={120}
                      placeholder={t("request_forms.fields.label")}
                      onChange={(e) => patch(f.id, { label: e.target.value })}
                    />
                    <Input
                      value={f.help ?? ""}
                      maxLength={300}
                      placeholder={t("request_forms.fields.help")}
                      onChange={(e) => patch(f.id, { help: e.target.value })}
                    />
                  </div>
                  <div className="flex flex-wrap items-center gap-4 text-sm">
                    <label className="flex items-center gap-2">
                      <Switch
                        checked={Boolean(f.required)}
                        onCheckedChange={(v) => patch(f.id, { required: v })}
                      />
                      {t("request_forms.fields.required")}
                    </label>
                    {f.source === "conversation" && (
                      <label className="flex items-center gap-2">
                        <Switch
                          checked={Boolean(f.required_to_resolve)}
                          onCheckedChange={(v) =>
                            patch(f.id, { required_to_resolve: v })
                          }
                        />
                        {t("request_forms.fields.required_to_resolve")}
                      </label>
                    )}
                  </div>
                  {earlier.length > 0 && (
                    <div className="flex flex-wrap items-center gap-2 text-xs">
                      <span className="text-muted-foreground">
                        {t("request_forms.condition.show_when")}
                      </span>
                      <Select
                        value={f.condition?.field ?? NONE}
                        onValueChange={(v) =>
                          patch(f.id, {
                            condition:
                              v === NONE
                                ? null
                                : { field: v, operator: "is_set" },
                          })
                        }
                      >
                        <SelectTrigger className="h-8 w-48">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value={NONE}>
                            {t("request_forms.condition.always")}
                          </SelectItem>
                          {earlier.map((e) => (
                            <SelectItem key={e.key} value={e.key}>
                              {e.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      {f.condition && (
                        <Select
                          value={f.condition.operator}
                          onValueChange={(v) =>
                            patch(f.id, {
                              condition: {
                                ...f.condition!,
                                operator: v as (typeof OPERATORS)[number],
                              },
                            })
                          }
                        >
                          <SelectTrigger className="h-8 w-36">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {OPERATORS.map((o) => (
                              <SelectItem key={o} value={o}>
                                {t(`request_forms.condition.${o}`)}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      )}
                      {f.condition &&
                        (f.condition.operator === "equals" ||
                          f.condition.operator === "not_equals") &&
                        (condDef?.options?.length ? (
                          <Select
                            value={String(f.condition.value ?? "")}
                            onValueChange={(v) =>
                              patch(f.id, {
                                condition: { ...f.condition!, value: v },
                              })
                            }
                          >
                            <SelectTrigger className="h-8 w-40">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {condDef.options
                                .filter((o) => !o.archived)
                                .map((o) => (
                                  <SelectItem key={o.value} value={o.value}>
                                    {o.label}
                                  </SelectItem>
                                ))}
                            </SelectContent>
                          </Select>
                        ) : (
                          <Input
                            className="h-8 w-40"
                            value={String(f.condition.value ?? "")}
                            onChange={(e) =>
                              patch(f.id, {
                                condition: {
                                  ...f.condition!,
                                  value: e.target.value,
                                },
                              })
                            }
                          />
                        ))}
                    </div>
                  )}
                  <span className="sr-only">{key}</span>
                </div>
              );
            })}
            {addable.length > 0 && (
              <div className="flex items-center gap-2">
                <Select value={adding} onValueChange={setAdding}>
                  <SelectTrigger className="w-72">
                    <SelectValue placeholder={t("request_forms.add_field")} />
                  </SelectTrigger>
                  <SelectContent>
                    {addable.map((a) => (
                      <SelectItem key={a.value} value={a.value}>
                        {a.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={!adding}
                  onClick={() => add(adding)}
                >
                  <Plus className="size-4" />
                  {t("request_forms.add")}
                </Button>
              </div>
            )}
            <p className="text-muted-foreground text-xs">
              {t("request_forms.custom_hint")}
            </p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={pending || !name.trim() || fields.length === 0}
            onClick={submit}
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
