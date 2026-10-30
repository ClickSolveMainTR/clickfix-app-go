"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
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
import { Switch } from "@/components/ui/switch";
import { useLocale } from "@/providers/locale-provider";

import { useOncallError } from "../lib/use-oncall-error";
import {
  oncallService,
  type EscalationPolicy,
  type StatusComponent,
  type UptimeMonitor,
  type UptimeMonitorInput,
} from "../services/oncall.service";

const NO_POLICY = "__none__";
const PICK = "__pick__";
export const INTERVALS = [60, 300, 600, 1800, 3600];

function isPublicUrl(raw: string) {
  try {
    const u = new URL(raw);
    return u.protocol === "https:" || u.protocol === "http:";
  } catch {
    return false;
  }
}

/**
 * Uptime monitor: public URL, interval (60 s – 1 day), expected status,
 * optional body keyword, the status-page component it drives and the
 * escalation policy paged when it goes down.
 */
export function MonitorDialog({
  slug,
  monitor,
  components,
  policies,
  pending,
  canManageComponents,
  onOpenChange,
  onSubmit,
}: {
  slug: string;
  monitor: UptimeMonitor | null;
  components: StatusComponent[];
  policies: EscalationPolicy[];
  pending: boolean;
  canManageComponents: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (body: UptimeMonitorInput) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useOncallError();
  const [name, setName] = useState(monitor?.name ?? "");
  const [url, setUrl] = useState(monitor?.url ?? "https://");
  const [interval, setIntervalValue] = useState(
    String(monitor?.interval_seconds ?? 300),
  );
  const [expected, setExpected] = useState(
    String(monitor?.expected_status ?? 200),
  );
  const [keyword, setKeyword] = useState(monitor?.keyword ?? "");
  const [component, setComponent] = useState(
    monitor?.component_uuid ?? components[0]?.uuid ?? "",
  );
  const [policy, setPolicy] = useState(monitor?.policy_uuid ?? "");
  const [enabled, setEnabled] = useState(monitor?.enabled ?? true);
  const [newComponent, setNewComponent] = useState("");

  const createComponent = useMutation({
    mutationFn: () =>
      oncallService.createComponent(slug, { name: newComponent.trim() }),
    onSuccess: async (created) => {
      setNewComponent("");
      if (created.uuid) setComponent(created.uuid);
      await qc.invalidateQueries({
        queryKey: ["tenant", slug, "status", "components"],
      });
    },
    onError,
  });

  const seconds = Number(interval);
  const status = Number(expected);
  const valid =
    name.trim().length > 0 &&
    name.trim().length <= 120 &&
    isPublicUrl(url.trim()) &&
    Number.isInteger(seconds) &&
    seconds >= 60 &&
    seconds <= 86400 &&
    Number.isInteger(status) &&
    status >= 100 &&
    status <= 599 &&
    keyword.length <= 1000 &&
    Boolean(component);

  const submit = () =>
    onSubmit({
      name: name.trim(),
      url: url.trim(),
      interval_seconds: seconds,
      expected_status: status,
      keyword,
      component_uuid: component,
      ...(policy ? { policy_uuid: policy } : {}),
      enabled,
    });

  return (
    <AppDialog
      open
      onOpenChange={onOpenChange}
      title={t(monitor ? "oncall.monitors.edit" : "oncall.monitors.new")}
      description={t("oncall.monitors.form_hint")}
      className="max-h-[90vh] overflow-y-auto sm:max-w-xl"
    >
      <div className="grid gap-4">
        <div className="grid gap-1.5">
          <Label htmlFor="monitor-name">{t("oncall.fields.name")}</Label>
          <Input
            id="monitor-name"
            value={name}
            maxLength={120}
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="monitor-url">{t("oncall.monitors.url")}</Label>
          <Input
            id="monitor-url"
            type="url"
            inputMode="url"
            value={url}
            placeholder="https://status.example.com/health"
            onChange={(e) => setUrl(e.target.value)}
          />
          <p className="text-muted-foreground text-xs">
            {t("oncall.monitors.url_hint")}
          </p>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="grid gap-1.5">
            <Label htmlFor="monitor-interval">
              {t("oncall.monitors.interval")}
            </Label>
            <Select value={interval} onValueChange={setIntervalValue}>
              <SelectTrigger id="monitor-interval" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {[
                  ...new Set([
                    ...INTERVALS,
                    monitor?.interval_seconds ?? INTERVALS[1]!,
                  ]),
                ]
                  .sort((a, b) => a - b)
                  .map((s) => (
                    <SelectItem key={s} value={String(s)}>
                      {t("oncall.monitors.every", {
                        interval: intervalLabel(s, t),
                      })}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="monitor-status">
              {t("oncall.monitors.expected_status")}
            </Label>
            <Input
              id="monitor-status"
              type="number"
              min={100}
              max={599}
              value={expected}
              onChange={(e) => setExpected(e.target.value)}
            />
          </div>
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="monitor-keyword">
            {t("oncall.monitors.keyword")}
          </Label>
          <Input
            id="monitor-keyword"
            value={keyword}
            maxLength={1000}
            placeholder={t("oncall.monitors.keyword_placeholder")}
            onChange={(e) => setKeyword(e.target.value)}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="monitor-component">
            {t("oncall.monitors.component")}
          </Label>
          <Select
            value={component || PICK}
            onValueChange={(v) => setComponent(v === PICK ? "" : v)}
          >
            <SelectTrigger id="monitor-component" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={PICK} disabled>
                {t("oncall.monitors.pick_component")}
              </SelectItem>
              {components.map((c) => (
                <SelectItem key={c.uuid} value={c.uuid ?? ""}>
                  {c.group_name ? `${c.group_name} / ${c.name}` : c.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-muted-foreground text-xs">
            {t("oncall.monitors.component_hint")}
          </p>
          {canManageComponents && (
            <div className="flex gap-2">
              <Input
                value={newComponent}
                maxLength={120}
                aria-label={t("oncall.monitors.new_component")}
                placeholder={t("oncall.monitors.new_component")}
                onChange={(e) => setNewComponent(e.target.value)}
              />
              <Button
                variant="outline"
                disabled={!newComponent.trim() || createComponent.isPending}
                onClick={() => createComponent.mutate()}
              >
                <Plus className="size-4" />
                {t("oncall.monitors.add_component")}
              </Button>
            </div>
          )}
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="monitor-policy">{t("oncall.monitors.policy")}</Label>
          <Select
            value={policy || NO_POLICY}
            onValueChange={(v) => setPolicy(v === NO_POLICY ? "" : v)}
          >
            <SelectTrigger id="monitor-policy" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NO_POLICY}>
                {t("oncall.monitors.no_policy")}
              </SelectItem>
              {policies.map((p) => (
                <SelectItem key={p.uuid} value={p.uuid}>
                  {p.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-center gap-2">
          <Switch
            id="monitor-enabled"
            checked={enabled}
            onCheckedChange={setEnabled}
          />
          <Label htmlFor="monitor-enabled">
            {t("oncall.monitors.enabled")}
          </Label>
        </div>
      </div>
      <FormActions>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          {t("common.cancel")}
        </Button>
        <Button disabled={pending || !valid} onClick={submit}>
          {t("common.save")}
        </Button>
      </FormActions>
    </AppDialog>
  );
}

export function intervalLabel(
  seconds: number,
  t: (key: string, params?: Record<string, string | number>) => string,
) {
  if (seconds % 3600 === 0)
    return t("oncall.monitors.hours", { count: seconds / 3600 });
  if (seconds % 60 === 0)
    return t("oncall.monitors.minutes", { count: seconds / 60 });
  return t("oncall.monitors.seconds", { count: seconds });
}
