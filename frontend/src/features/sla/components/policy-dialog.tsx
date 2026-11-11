"use client";

import { useQuery } from "@tanstack/react-query";
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
  CatalogMatchEditor,
  type CustomFieldMatch,
} from "@/features/custom-fields";
import { getActiveOrganizationSlug } from "@/features/organizations";
import { apiClient, unwrap } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { CHANNEL_TYPES, PRIORITIES } from "../lib/format";
import type {
  SlaBusinessHours,
  SlaPolicy,
  SlaPolicyInput,
  SlaTarget,
} from "../services/sla.service";

const NO_HOURS = "__auto__";
const METRICS = [
  "first_response_minutes",
  "next_response_minutes",
  "resolution_minutes",
] as const;

/** Defaults ported from the Laravel TicketSlaCalculator (minutes). */
const DEFAULT_TARGETS: SlaTarget[] = [
  {
    priority: "urgent",
    first_response_minutes: 15,
    next_response_minutes: 30,
    resolution_minutes: 240,
  },
  {
    priority: "high",
    first_response_minutes: 30,
    next_response_minutes: 60,
    resolution_minutes: 480,
  },
  {
    priority: "normal",
    first_response_minutes: 60,
    next_response_minutes: 120,
    resolution_minutes: 1440,
  },
  {
    priority: "low",
    first_response_minutes: 120,
    next_response_minutes: 240,
    resolution_minutes: 2880,
  },
];

function toggle<T>(list: T[], v: T): T[] {
  return list.includes(v) ? list.filter((x) => x !== v) : [...list, v];
}

type Tag = { uuid: string; name: string; color: string };

/** Create or edit an SLA policy: conditions and per-priority targets. */
export function PolicyDialog({
  open,
  onOpenChange,
  policy,
  hours,
  pending,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  policy: SlaPolicy | null;
  hours: SlaBusinessHours[];
  pending: boolean;
  onSubmit: (body: SlaPolicyInput) => void;
}) {
  const { t } = useLocale();
  const [name, setName] = useState(policy?.name ?? "");
  const [description, setDescription] = useState(policy?.description ?? "");
  const [active, setActive] = useState(policy?.is_active ?? true);
  const [warning, setWarning] = useState(policy?.warning_percent ?? 80);
  const [hoursUuid, setHoursUuid] = useState(
    policy?.business_hours_uuid ?? NO_HOURS,
  );
  const [priorities, setPriorities] = useState<string[]>(
    policy?.conditions.priorities ?? [],
  );
  const [channels, setChannels] = useState<string[]>(
    policy?.conditions.channel_types ?? [],
  );
  const [tags, setTags] = useState<string[]>(
    policy?.conditions.tag_uuids ?? [],
  );
  const [tiers, setTiers] = useState(
    (policy?.conditions.company_tiers ?? []).join(", "),
  );
  const [plans, setPlans] = useState(
    (policy?.conditions.company_plans ?? []).join(", "),
  );
  const [fieldMatches, setFieldMatches] = useState<CustomFieldMatch[]>(
    policy?.conditions.custom_fields ?? [],
  );
  const [minMrr, setMinMrr] = useState(
    policy?.conditions.min_company_mrr_minor != null
      ? String(policy.conditions.min_company_mrr_minor / 100)
      : "",
  );
  const [targets, setTargets] = useState<SlaTarget[]>(
    PRIORITIES.map(
      (p) =>
        policy?.targets.find((x) => x.priority === p) ??
        (policy
          ? { priority: p }
          : DEFAULT_TARGETS.find((x) => x.priority === p)!),
    ),
  );

  const tagList = useQuery({
    queryKey: ["tenant", getActiveOrganizationSlug(), "tags"],
    queryFn: async () =>
      (
        await unwrap<{ items: Tag[] }>(
          await apiClient.GET("/v1/tenant/tags", {
            params: {
              header: { "X-Organization": getActiveOrganizationSlug() ?? "" },
            },
          }),
        )
      ).items,
    enabled: open,
  });

  const setTarget = (
    priority: string,
    key: (typeof METRICS)[number],
    raw: string,
  ) => {
    const n = raw === "" ? null : Math.max(1, Math.round(Number(raw)));
    setTargets((list) =>
      list.map((x) =>
        x.priority === priority
          ? { ...x, [key]: Number.isFinite(n) ? n : null }
          : x,
      ),
    );
  };

  const submit = () => {
    onSubmit({
      name: name.trim(),
      description: description.trim(),
      is_active: active,
      warning_percent: warning,
      business_hours_uuid: hoursUuid === NO_HOURS ? null : hoursUuid,
      conditions: {
        priorities: priorities as SlaTarget["priority"][],
        channel_types: channels,
        tag_uuids: tags,
        company_uuids: policy?.conditions.company_uuids ?? [],
        company_tiers: tiers
          .split(",")
          .map((x) => x.trim())
          .filter(Boolean),
        company_plans: plans
          .split(",")
          .map((x) => x.trim())
          .filter(Boolean),
        min_company_mrr_minor:
          minMrr.trim() === "" || !Number.isFinite(Number(minMrr))
            ? null
            : Math.max(0, Math.round(Number(minMrr) * 100)),
        custom_fields: fieldMatches.filter((m) => m.key && m.options.length),
      },
      targets: targets.filter(
        (x) =>
          x.first_response_minutes ||
          x.next_response_minutes ||
          x.resolution_minutes,
      ),
    });
  };

  const chip = (on: boolean) =>
    cn(
      "rounded-md border px-2 py-1 text-xs transition-colors",
      on
        ? "border-primary bg-primary/10 text-foreground"
        : "text-muted-foreground hover:bg-muted",
    );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {policy ? t("sla.policy.edit") : t("sla.policy.new")}
          </DialogTitle>
          <DialogDescription>{t("sla.policy.dialog_hint")}</DialogDescription>
        </DialogHeader>

        <div className="grid gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="sla-name">{t("sla.fields.name")}</Label>
            <Input
              id="sla-name"
              value={name}
              maxLength={120}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="sla-desc">{t("sla.fields.description")}</Label>
            <Textarea
              id="sla-desc"
              rows={2}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="flex items-center gap-2">
              <Switch
                id="sla-active"
                checked={active}
                onCheckedChange={setActive}
              />
              <Label htmlFor="sla-active">{t("sla.fields.active")}</Label>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="sla-warn">
                {t("sla.fields.warning_percent")}
              </Label>
              <Input
                id="sla-warn"
                type="number"
                min={1}
                max={99}
                value={warning}
                onChange={(e) => setWarning(Number(e.target.value) || 80)}
              />
            </div>
            <div className="grid gap-1.5">
              <Label>{t("sla.fields.business_hours")}</Label>
              <Select value={hoursUuid} onValueChange={setHoursUuid}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NO_HOURS}>
                    {t("sla.fields.business_hours_auto")}
                  </SelectItem>
                  {hours.map((h) => (
                    <SelectItem key={h.uuid} value={h.uuid}>
                      {h.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <fieldset className="grid gap-2">
            <legend className="mb-1 text-sm font-medium">
              {t("sla.policy.conditions")}
            </legend>
            <p className="text-muted-foreground text-xs">
              {t("sla.policy.conditions_hint")}
            </p>
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-muted-foreground w-20 text-xs">
                {t("sla.fields.priorities")}
              </span>
              {PRIORITIES.map((p) => (
                <button
                  key={p}
                  type="button"
                  className={chip(priorities.includes(p))}
                  onClick={() => setPriorities((l) => toggle(l, p))}
                >
                  {t(`inbox.priority.${p}`)}
                </button>
              ))}
            </div>
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-muted-foreground w-20 text-xs">
                {t("sla.fields.channels")}
              </span>
              {CHANNEL_TYPES.map((c) => (
                <button
                  key={c}
                  type="button"
                  className={chip(channels.includes(c))}
                  onClick={() => setChannels((l) => toggle(l, c))}
                >
                  {t(`sla.channel.${c}`)}
                </button>
              ))}
            </div>
            {(tagList.data?.length ?? 0) > 0 && (
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="text-muted-foreground w-20 text-xs">
                  {t("sla.fields.tags")}
                </span>
                {tagList.data!.map((tag) => (
                  <button
                    key={tag.uuid}
                    type="button"
                    className={chip(tags.includes(tag.uuid))}
                    onClick={() => setTags((l) => toggle(l, tag.uuid))}
                  >
                    {tag.name}
                  </button>
                ))}
              </div>
            )}
            <div className="flex flex-wrap items-center gap-1.5">
              <label
                htmlFor="sla-tiers"
                className="text-muted-foreground w-20 text-xs"
              >
                {t("sla.fields.company_tiers")}
              </label>
              <Input
                id="sla-tiers"
                className="max-w-sm flex-1"
                value={tiers}
                placeholder={t("sla.fields.company_tiers_placeholder")}
                onChange={(e) => setTiers(e.target.value)}
              />
            </div>
            <div className="flex flex-wrap items-center gap-1.5">
              <label
                htmlFor="sla-plans"
                className="text-muted-foreground w-20 text-xs"
              >
                {t("sla.fields.company_plans")}
              </label>
              <Input
                id="sla-plans"
                className="max-w-sm flex-1"
                value={plans}
                placeholder={t("sla.fields.company_plans_placeholder")}
                onChange={(e) => setPlans(e.target.value)}
              />
            </div>
            <div className="flex flex-wrap items-center gap-1.5">
              <label
                htmlFor="sla-min-mrr"
                className="text-muted-foreground w-20 text-xs"
              >
                {t("sla.fields.min_company_mrr")}
              </label>
              <Input
                id="sla-min-mrr"
                type="number"
                min={0}
                step="0.01"
                className="max-w-[10rem]"
                value={minMrr}
                onChange={(e) => setMinMrr(e.target.value)}
              />
              <span className="text-muted-foreground text-xs">
                {t("sla.fields.min_company_mrr_hint")}
              </span>
            </div>
            <div className="flex flex-col gap-1.5">
              <span className="text-muted-foreground text-xs">
                {t("sla.fields.custom_fields")}
              </span>
              <CatalogMatchEditor
                value={fieldMatches}
                onChange={setFieldMatches}
              />
            </div>
          </fieldset>

          <fieldset>
            <legend className="mb-1 text-sm font-medium">
              {t("sla.policy.targets")}
            </legend>
            <p className="text-muted-foreground mb-2 text-xs">
              {t("sla.policy.targets_hint")}
            </p>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-muted-foreground text-left text-xs">
                    <th className="py-1 pr-2 font-medium">
                      {t("sla.fields.priority")}
                    </th>
                    {METRICS.map((m) => (
                      <th key={m} className="py-1 pr-2 font-medium">
                        {t(`sla.metric.${m.replace("_minutes", "")}`)}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {targets.map((row) => (
                    <tr key={row.priority}>
                      <td className="py-1 pr-2">
                        {t(`inbox.priority.${row.priority}`)}
                      </td>
                      {METRICS.map((m) => (
                        <td key={m} className="py-1 pr-2">
                          <Input
                            type="number"
                            min={1}
                            inputMode="numeric"
                            aria-label={`${row.priority} ${m}`}
                            value={row[m] ?? ""}
                            placeholder="—"
                            onChange={(e) =>
                              setTarget(row.priority, m, e.target.value)
                            }
                          />
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </fieldset>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button disabled={pending || !name.trim()} onClick={submit}>
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
