"use client";

import { ArrowDown, ArrowUp, Plus, Trash2, X } from "lucide-react";
import { useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { browserTimezone } from "@/features/reports/lib/format";
import {
  useMemberDirectory,
  type DirectoryOption,
} from "@/features/tenant-members/hooks/use-member-directory";
import { useLocale } from "@/providers/locale-provider";

import { fromInstant, ROTATION_PRESETS, toInstant } from "../lib/rota";
import type { OncallRota, OncallRotaInput } from "../services/oncall.service";

type LayerDraft = {
  startDay: string;
  startTime: string;
  endDay: string;
  endTime: string;
  rotationHours: string;
  agents: string[];
};
type OverrideDraft = {
  startDay: string;
  startTime: string;
  endDay: string;
  endTime: string;
  agent: string;
};

const PICK = "__pick__";

function today() {
  return fromInstant(new Date().toISOString()).day;
}

/**
 * Rota editor: layers (start, optional end, rotation length, ordered
 * agents) and one-off overrides. Times are entered in the browser's zone
 * and stored as instants; the rota zone labels the schedule.
 */
export function RotaDialog({
  rota,
  pending,
  onOpenChange,
  onSubmit,
}: {
  rota: OncallRota | null;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (body: OncallRotaInput) => void;
}) {
  const { t } = useLocale();
  const directory = useMemberDirectory();
  const [name, setName] = useState(rota?.name ?? "");
  const [timezone, setTimezone] = useState(rota?.timezone ?? browserTimezone);
  const [layers, setLayers] = useState<LayerDraft[]>(
    rota?.layers.length
      ? rota.layers.map((l) => {
          const s = fromInstant(l.starts_at);
          const e = fromInstant(l.ends_at);
          return {
            startDay: s.day,
            startTime: s.time,
            endDay: e.day,
            endTime: e.time,
            rotationHours: String(l.rotation_minutes / 60),
            agents: l.agents,
          };
        })
      : [
          {
            startDay: today(),
            startTime: "09:00",
            endDay: "",
            endTime: "",
            rotationHours: "168",
            agents: [],
          },
        ],
  );
  const [overrides, setOverrides] = useState<OverrideDraft[]>(
    (rota?.overrides ?? []).map((o) => {
      const s = fromInstant(o.starts_at);
      const e = fromInstant(o.ends_at);
      return {
        startDay: s.day,
        startTime: s.time,
        endDay: e.day,
        endTime: e.time,
        agent: o.agent,
      };
    }),
  );

  const body: OncallRotaInput = {
    name: name.trim(),
    timezone: timezone.trim(),
    layers: layers.map((l) => {
      const ends = toInstant(l.endDay, l.endTime);
      return {
        starts_at: toInstant(l.startDay, l.startTime),
        ...(ends ? { ends_at: ends } : {}),
        rotation_minutes: Math.round(Number(l.rotationHours) * 60),
        agents: l.agents,
      };
    }),
    overrides: overrides.map((o) => ({
      starts_at: toInstant(o.startDay, o.startTime),
      ends_at: toInstant(o.endDay, o.endTime),
      agent: o.agent,
    })),
  };
  const valid =
    body.name.length > 0 &&
    body.name.length <= 120 &&
    body.timezone.length > 0 &&
    body.layers.length > 0 &&
    body.layers.every(
      (l) =>
        l.starts_at &&
        l.agents.length > 0 &&
        l.rotation_minutes >= 1 &&
        l.rotation_minutes <= 525600 &&
        (!l.ends_at || l.ends_at > l.starts_at),
    ) &&
    body.overrides.every(
      (o) => o.starts_at && o.ends_at && o.ends_at > o.starts_at && o.agent,
    );

  const patchLayer = (i: number, patch: Partial<LayerDraft>) =>
    setLayers((list) => list.map((l, j) => (j === i ? { ...l, ...patch } : l)));
  const patchOverride = (i: number, patch: Partial<OverrideDraft>) =>
    setOverrides((list) =>
      list.map((o, j) => (j === i ? { ...o, ...patch } : o)),
    );

  return (
    <AppDialog
      open
      onOpenChange={onOpenChange}
      title={t(rota ? "oncall.rotas.edit" : "oncall.rotas.new")}
      description={t("oncall.rotas.form_hint")}
      className="max-h-[90vh] overflow-y-auto sm:max-w-3xl"
    >
      <div className="grid gap-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="grid gap-1.5">
            <Label htmlFor="rota-name">{t("oncall.fields.name")}</Label>
            <Input
              id="rota-name"
              value={name}
              maxLength={120}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="rota-tz">{t("oncall.fields.timezone")}</Label>
            <Input
              id="rota-tz"
              value={timezone}
              onChange={(e) => setTimezone(e.target.value)}
            />
          </div>
        </div>

        <section className="grid gap-2">
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-sm font-medium">{t("oncall.rotas.layers")}</h3>
            <Button
              size="sm"
              variant="outline"
              disabled={layers.length >= 20}
              onClick={() =>
                setLayers((list) => [
                  ...list,
                  {
                    startDay: today(),
                    startTime: "09:00",
                    endDay: "",
                    endTime: "",
                    rotationHours: "24",
                    agents: [],
                  },
                ])
              }
            >
              <Plus className="size-4" />
              {t("oncall.rotas.add_layer")}
            </Button>
          </div>
          <p className="text-muted-foreground text-xs">
            {t("oncall.rotas.layers_hint")}
          </p>
          {layers.map((l, i) => (
            <Card key={i}>
              <CardContent className="grid gap-3 py-3">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-medium">
                    {t("oncall.rotas.layer_n", { n: i + 1 })}
                  </span>
                  {layers.length > 1 && (
                    <Button
                      size="icon"
                      variant="ghost"
                      aria-label={t("oncall.rotas.remove_layer")}
                      onClick={() =>
                        setLayers((list) => list.filter((_, j) => j !== i))
                      }
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  )}
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
                  <InstantFields
                    label={t("oncall.fields.starts_at")}
                    day={l.startDay}
                    time={l.startTime}
                    onChange={(day, time) =>
                      patchLayer(i, { startDay: day, startTime: time })
                    }
                  />
                  <InstantFields
                    label={t("oncall.fields.ends_at_optional")}
                    day={l.endDay}
                    time={l.endTime}
                    onChange={(day, time) =>
                      patchLayer(i, { endDay: day, endTime: time })
                    }
                  />
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor={`layer-${i}-rotation`} className="text-xs">
                    {t("oncall.fields.rotation_hours")}
                  </Label>
                  <div className="flex flex-wrap items-center gap-2">
                    <Input
                      id={`layer-${i}-rotation`}
                      type="number"
                      min={1}
                      step="0.5"
                      className="w-28"
                      value={l.rotationHours}
                      onChange={(e) =>
                        patchLayer(i, { rotationHours: e.target.value })
                      }
                    />
                    {ROTATION_PRESETS.map((p) => (
                      <Button
                        key={p.key}
                        size="sm"
                        variant="outline"
                        onClick={() =>
                          patchLayer(i, {
                            rotationHours: String(p.minutes / 60),
                          })
                        }
                      >
                        {t(`oncall.rotation.${p.key}`)}
                      </Button>
                    ))}
                  </div>
                </div>
                <AgentOrder
                  id={`layer-${i}-agents`}
                  value={l.agents}
                  options={directory.members}
                  nameOf={directory.memberName}
                  onChange={(agents) => patchLayer(i, { agents })}
                />
              </CardContent>
            </Card>
          ))}
        </section>

        <section className="grid gap-2">
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-sm font-medium">
              {t("oncall.rotas.overrides")}
            </h3>
            <Button
              size="sm"
              variant="outline"
              disabled={overrides.length >= 100}
              onClick={() =>
                setOverrides((list) => [
                  ...list,
                  {
                    startDay: today(),
                    startTime: "18:00",
                    endDay: today(),
                    endTime: "23:59",
                    agent: "",
                  },
                ])
              }
            >
              <Plus className="size-4" />
              {t("oncall.rotas.add_override")}
            </Button>
          </div>
          <p className="text-muted-foreground text-xs">
            {t("oncall.rotas.overrides_hint")}
          </p>
          {overrides.map((o, i) => (
            <div
              key={i}
              className="grid items-end gap-3 rounded-md border p-3 sm:grid-cols-[1fr_1fr_12rem_auto]"
            >
              <InstantFields
                label={t("oncall.fields.starts_at")}
                day={o.startDay}
                time={o.startTime}
                onChange={(day, time) =>
                  patchOverride(i, { startDay: day, startTime: time })
                }
              />
              <InstantFields
                label={t("oncall.fields.ends_at")}
                day={o.endDay}
                time={o.endTime}
                onChange={(day, time) =>
                  patchOverride(i, { endDay: day, endTime: time })
                }
              />
              <div className="grid gap-1.5">
                <Label className="text-xs">{t("oncall.fields.agent")}</Label>
                <Select
                  value={o.agent || PICK}
                  onValueChange={(v) =>
                    patchOverride(i, { agent: v === PICK ? "" : v })
                  }
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={PICK} disabled>
                      {t("oncall.fields.pick_agent")}
                    </SelectItem>
                    {directory.members.map((m) => (
                      <SelectItem key={m.value} value={m.value}>
                        {m.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <Button
                size="icon"
                variant="ghost"
                aria-label={t("oncall.rotas.remove_override")}
                onClick={() =>
                  setOverrides((list) => list.filter((_, j) => j !== i))
                }
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          ))}
        </section>
      </div>
      <FormActions>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          {t("common.cancel")}
        </Button>
        <Button disabled={pending || !valid} onClick={() => onSubmit(body)}>
          {t("common.save")}
        </Button>
      </FormActions>
    </AppDialog>
  );
}

/** Day (DatePicker) + local time. */
function InstantFields({
  label,
  day,
  time,
  onChange,
}: {
  label: string;
  day: string;
  time: string;
  onChange: (day: string, time: string) => void;
}) {
  return (
    <div className="grid gap-1.5">
      <Label className="text-xs">{label}</Label>
      <div className="flex gap-2">
        <DatePicker
          value={day}
          onChange={(v) => onChange(v, time || "00:00")}
          className="min-w-0 flex-1"
        />
        <Input
          type="time"
          aria-label={label}
          className="w-28"
          value={time}
          onChange={(e) => onChange(day, e.target.value)}
        />
      </div>
    </div>
  );
}

/** Ordered agent list; rotation goes top to bottom. */
function AgentOrder({
  id,
  value,
  options,
  nameOf,
  onChange,
}: {
  id: string;
  value: string[];
  options: DirectoryOption[];
  nameOf: (uuid: string) => string;
  onChange: (value: string[]) => void;
}) {
  const { t } = useLocale();
  const move = (i: number, by: number) => {
    const next = [...value];
    const [item] = next.splice(i, 1);
    next.splice(i + by, 0, item!);
    onChange(next);
  };
  const remaining = options.filter((o) => !value.includes(o.value));
  return (
    <div className="grid gap-1.5">
      <Label htmlFor={id} className="text-xs">
        {t("oncall.fields.agents_order")}
      </Label>
      {value.length > 0 && (
        <ol className="divide-y rounded-md border">
          {value.map((uuid, i) => (
            <li
              key={uuid}
              className="flex items-center gap-2 px-2 py-1 text-sm"
            >
              <span className="text-muted-foreground w-5 text-xs tabular-nums">
                {i + 1}.
              </span>
              <span className="min-w-0 flex-1 truncate">{nameOf(uuid)}</span>
              <Button
                size="icon"
                variant="ghost"
                className="size-7"
                disabled={i === 0}
                aria-label={t("oncall.fields.move_up")}
                onClick={() => move(i, -1)}
              >
                <ArrowUp className="size-3.5" />
              </Button>
              <Button
                size="icon"
                variant="ghost"
                className="size-7"
                disabled={i === value.length - 1}
                aria-label={t("oncall.fields.move_down")}
                onClick={() => move(i, 1)}
              >
                <ArrowDown className="size-3.5" />
              </Button>
              <Button
                size="icon"
                variant="ghost"
                className="size-7"
                aria-label={t("oncall.fields.remove_agent")}
                onClick={() => onChange(value.filter((x) => x !== uuid))}
              >
                <X className="size-3.5" />
              </Button>
            </li>
          ))}
        </ol>
      )}
      <Select
        value={PICK}
        onValueChange={(v) => v !== PICK && onChange([...value, v])}
        disabled={remaining.length === 0}
      >
        <SelectTrigger id={id} className="w-full sm:w-64">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={PICK} disabled>
            {t("oncall.fields.add_agent")}
          </SelectItem>
          {remaining.map((m) => (
            <SelectItem key={m.value} value={m.value}>
              {m.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
