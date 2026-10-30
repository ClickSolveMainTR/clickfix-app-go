"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  Bot,
  FileText,
  PencilLine,
  PowerOff,
  RotateCcw,
  Save,
} from "lucide-react";
import { useMemo, useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
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
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { channelsService } from "@/features/channels/services/channels.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { aiErrorMessage, isConflict } from "../lib/errors";
import { minuteToTime, timeToMinute } from "../lib/format";
import {
  aiSettingsService,
  type AIAgentConfig,
  type AIAgentMode,
} from "../services/ai-settings.service";
import { QueryState, SettingRow, TagInput } from "./parts";
import { TestChatPanel } from "./test-chat-panel";
import type { AISettingsTabProps } from "./tab-registry";

const MODES: { value: AIAgentMode; icon: typeof Bot }[] = [
  { value: "off", icon: PowerOff },
  { value: "summary", icon: FileText },
  { value: "draft", icon: PencilLine },
  { value: "autonomous", icon: Bot },
];

/** Monday-first display order; values follow Go's time.Weekday (0 = Sunday). */
const WEEKDAYS = [1, 2, 3, 4, 5, 6, 0];

export const agentKey = (slug: string) =>
  ["tenant", slug, "ai", "agent"] as const;

function timezones(current: string) {
  const list =
    typeof Intl.supportedValuesOf === "function"
      ? Intl.supportedValuesOf("timeZone")
      : ["Europe/Istanbul", "UTC"];
  return list.includes(current) ? list : [current, ...list];
}

/**
 * "Ajan" tab (CF-64): mode, triage, persona, channel/category scope, working
 * hours, confidence threshold and handoff keywords. The config endpoint has no
 * server-side revision check, so the form re-reads the revision right before
 * saving and asks before overwriting someone else's change.
 */
export function AgentTab({ slug, canWrite, canRun }: AISettingsTabProps) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: agentKey(slug),
    queryFn: () => aiSettingsService.agent(slug),
  });
  const channelTypes = useQuery({
    queryKey: ["tenant", slug, "channels", "types"],
    queryFn: () => channelsService.types(slug).then((r) => r.items),
  });
  const channels = useQuery({
    queryKey: ["tenant", slug, "channels", "list"],
    queryFn: () => channelsService.list(slug).then((r) => r.items),
  });

  const [form, setForm] = useState<AIAgentConfig | null>(null);
  const [baseRevision, setBaseRevision] = useState(0);
  const [stale, setStale] = useState<number | null>(null);
  const [saving, setSaving] = useState(false);

  // First load seeds the editable copy (state adjusted during render).
  if (query.data && form === null) {
    setForm(query.data.config);
    setBaseRevision(query.data.revision);
  }

  const dirty = useMemo(
    () =>
      form !== null &&
      query.data !== undefined &&
      JSON.stringify(form) !== JSON.stringify(query.data.config),
    [form, query.data],
  );

  if (!form) {
    return (
      <QueryState
        loading={query.isLoading}
        error={query.isError}
        onRetry={() => void query.refetch()}
      />
    );
  }

  const set = <K extends keyof AIAgentConfig>(
    key: K,
    value: AIAgentConfig[K],
  ) => setForm((prev) => (prev ? { ...prev, [key]: value } : prev));
  const allDay = form.start_minute === 0 && form.end_minute === 1440;
  const hoursInvalid = form.start_minute >= form.end_minute;
  const readOnly = !canWrite;

  const connectedTypes = new Set((channels.data ?? []).map((c) => c.type));
  const typeOptions = Array.from(
    new Set([
      ...(channelTypes.data ?? []).map((c) => c.type),
      ...form.channels,
    ]),
  ).filter((type) => type !== "test" || connectedTypes.has(type));

  const channelLabel = (type: string) => {
    const key = `channels.types.${type}`;
    const label = t(key);
    return label === key ? type : label;
  };

  const reload = async () => {
    const fresh = await qc.fetchQuery({
      queryKey: agentKey(slug),
      queryFn: () => aiSettingsService.agent(slug),
    });
    setForm(fresh.config);
    setBaseRevision(fresh.revision);
    setStale(null);
  };

  const save = async (force = false) => {
    if (!form || hoursInvalid) return;
    setSaving(true);
    try {
      if (!force) {
        const current = await aiSettingsService.agent(slug);
        if (current.revision !== baseRevision) {
          setStale(current.revision);
          return;
        }
      }
      const saved = await aiSettingsService.saveAgent(slug, form);
      qc.setQueryData(agentKey(slug), saved);
      setForm(saved.config);
      setBaseRevision(saved.revision);
      setStale(null);
      appToast.success(t("ai_settings.agent.saved"));
    } catch (error) {
      if (isConflict(error)) setStale(baseRevision + 1);
      else appToast.error(aiErrorMessage(error, t));
    } finally {
      setSaving(false);
    }
  };

  const activeWithoutChannels =
    (form.mode !== "off" || form.triage_enabled) && form.channels.length === 0;

  return (
    <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_24rem]">
      <div className="min-w-0 space-y-6">
        {stale !== null && (
          <Alert variant="destructive">
            <AlertTriangle className="size-4" />
            <AlertTitle>{t("ai_settings.conflict.title")}</AlertTitle>
            <AlertDescription className="space-y-3">
              <p>{t("ai_settings.agent.conflict_description")}</p>
              <div className="flex flex-wrap gap-2">
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => void reload()}
                >
                  <RotateCcw className="size-4" />
                  {t("ai_settings.conflict.reload")}
                </Button>
                <Button
                  size="sm"
                  variant="destructive"
                  onClick={() => void save(true)}
                >
                  {t("ai_settings.conflict.overwrite")}
                </Button>
              </div>
            </AlertDescription>
          </Alert>
        )}

        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              {t("ai_settings.agent.mode_title")}
            </CardTitle>
            <p className="text-muted-foreground text-sm">
              {t("ai_settings.agent.mode_hint")}
            </p>
          </CardHeader>
          <CardContent>
            <RadioGroup
              value={form.mode}
              onValueChange={(value) => set("mode", value as AIAgentMode)}
              disabled={readOnly}
              aria-label={t("ai_settings.agent.mode_title")}
              className="grid gap-3 sm:grid-cols-2"
            >
              {MODES.map(({ value, icon: Icon }) => {
                const id = `ai-mode-${value}`;
                const selected = form.mode === value;
                return (
                  <Label
                    key={value}
                    htmlFor={id}
                    className={cn(
                      "hover:bg-muted/50 flex cursor-pointer items-start gap-3 rounded-lg border p-4 font-normal transition-colors",
                      selected && "border-foreground/60 bg-muted/40",
                      readOnly && "cursor-default",
                    )}
                  >
                    <RadioGroupItem id={id} value={value} className="mt-0.5" />
                    <span className="min-w-0 space-y-1">
                      <span className="flex items-center gap-2 text-sm font-medium">
                        <Icon className="size-4" />
                        {t(`ai_settings.agent.mode.${value}`)}
                      </span>
                      <span className="text-muted-foreground block text-xs leading-relaxed">
                        {t(`ai_settings.agent.mode.${value}_hint`)}
                      </span>
                    </span>
                  </Label>
                );
              })}
            </RadioGroup>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              {t("ai_settings.agent.behaviour_title")}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="flex items-start justify-between gap-4">
              <div className="space-y-1">
                <Label htmlFor="ai-triage">
                  {t("ai_settings.agent.triage")}
                </Label>
                <p className="text-muted-foreground text-xs">
                  {t("ai_settings.agent.triage_hint")}
                </p>
              </div>
              <Switch
                id="ai-triage"
                checked={form.triage_enabled}
                disabled={readOnly}
                onCheckedChange={(checked) => set("triage_enabled", checked)}
              />
            </div>

            <SettingRow
              label={t("ai_settings.agent.persona")}
              hint={t("ai_settings.agent.persona_hint")}
              htmlFor="ai-persona"
            >
              <Textarea
                id="ai-persona"
                rows={5}
                maxLength={3000}
                value={form.persona}
                disabled={readOnly}
                placeholder={t("ai_settings.agent.persona_placeholder")}
                onChange={(event) => set("persona", event.target.value)}
              />
              <p className="text-muted-foreground text-end text-xs tabular-nums">
                {form.persona.length}/3000
              </p>
            </SettingRow>

            <SettingRow
              label={t("ai_settings.agent.confidence")}
              hint={t("ai_settings.agent.confidence_hint")}
            >
              <div className="flex items-center gap-4">
                <Slider
                  min={0.5}
                  max={1}
                  step={0.01}
                  value={[form.confidence_threshold]}
                  disabled={readOnly}
                  aria-label={t("ai_settings.agent.confidence")}
                  onValueChange={([value]) =>
                    set("confidence_threshold", Math.round(value * 100) / 100)
                  }
                  className="flex-1"
                />
                <span className="w-12 text-end text-sm font-medium tabular-nums">
                  %{Math.round(form.confidence_threshold * 100)}
                </span>
              </div>
            </SettingRow>

            <SettingRow
              label={t("ai_settings.agent.handoff")}
              hint={t("ai_settings.agent.handoff_hint")}
              htmlFor="ai-handoff"
            >
              <TagInput
                id="ai-handoff"
                value={form.handoff_keywords}
                onChange={(next) => set("handoff_keywords", next)}
                placeholder={t("ai_settings.agent.handoff_placeholder")}
                max={30}
                disabled={readOnly}
              />
            </SettingRow>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              {t("ai_settings.agent.scope_title")}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-6">
            <SettingRow
              label={t("ai_settings.agent.channels")}
              hint={t("ai_settings.agent.channels_hint")}
            >
              {activeWithoutChannels && (
                <Alert>
                  <AlertTriangle className="size-4" />
                  <AlertDescription>
                    {t("ai_settings.agent.channels_empty_warning")}
                  </AlertDescription>
                </Alert>
              )}
              {channelTypes.isError ? (
                <p className="text-destructive text-sm">
                  {t("ai_settings.agent.channels_error")}
                </p>
              ) : (
                <ToggleGroup
                  type="multiple"
                  variant="outline"
                  spacing={2}
                  className="flex flex-wrap justify-start"
                  value={form.channels}
                  disabled={readOnly}
                  aria-label={t("ai_settings.agent.channels")}
                  onValueChange={(next) => set("channels", next)}
                >
                  {typeOptions.map((type) => (
                    <ToggleGroupItem key={type} value={type} className="gap-2">
                      {channelLabel(type)}
                      {connectedTypes.has(type) && (
                        <>
                          <span className="signal-dot" aria-hidden />
                          <span className="sr-only">
                            {t("ai_settings.agent.channel_connected")}
                          </span>
                        </>
                      )}
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
              )}
            </SettingRow>

            <SettingRow
              label={t("ai_settings.agent.categories")}
              hint={t("ai_settings.agent.categories_hint")}
              htmlFor="ai-categories"
            >
              <TagInput
                id="ai-categories"
                value={form.categories}
                onChange={(next) => set("categories", next)}
                placeholder={t("ai_settings.agent.categories_placeholder")}
                max={50}
                disabled={readOnly}
              />
            </SettingRow>

            <div className="grid gap-4 sm:grid-cols-2">
              <SettingRow
                label={t("ai_settings.agent.locale")}
                hint={t("ai_settings.agent.locale_hint")}
                htmlFor="ai-locale"
              >
                <Select
                  value={form.locale}
                  disabled={readOnly}
                  onValueChange={(value) =>
                    set("locale", value as AIAgentConfig["locale"])
                  }
                >
                  <SelectTrigger id="ai-locale" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="tr">
                      {t("ai_settings.locale.tr")}
                    </SelectItem>
                    <SelectItem value="en">
                      {t("ai_settings.locale.en")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </SettingRow>
              <SettingRow
                label={t("ai_settings.agent.timezone")}
                hint={t("ai_settings.agent.timezone_hint")}
                htmlFor="ai-timezone"
              >
                <Input
                  id="ai-timezone"
                  list="ai-timezones"
                  value={form.timezone}
                  disabled={readOnly}
                  onChange={(event) => set("timezone", event.target.value)}
                />
                <datalist id="ai-timezones">
                  {timezones(form.timezone).map((zone) => (
                    <option key={zone} value={zone} />
                  ))}
                </datalist>
              </SettingRow>
            </div>

            <SettingRow
              label={t("ai_settings.agent.hours")}
              hint={t("ai_settings.agent.hours_hint")}
            >
              <div className="flex items-center gap-3">
                <Switch
                  id="ai-all-day"
                  checked={allDay}
                  disabled={readOnly}
                  onCheckedChange={(checked) =>
                    setForm((prev) =>
                      prev
                        ? {
                            ...prev,
                            start_minute: checked ? 0 : 540,
                            end_minute: checked ? 1440 : 1080,
                          }
                        : prev,
                    )
                  }
                />
                <Label htmlFor="ai-all-day" className="font-normal">
                  {t("ai_settings.agent.all_day")}
                </Label>
              </div>
              {!allDay && (
                <div className="flex flex-wrap items-end gap-3">
                  <div className="space-y-1">
                    <Label htmlFor="ai-start" className="text-xs">
                      {t("ai_settings.agent.start")}
                    </Label>
                    <Input
                      id="ai-start"
                      type="time"
                      className="w-32"
                      value={minuteToTime(form.start_minute)}
                      disabled={readOnly}
                      onChange={(event) => {
                        const minute = timeToMinute(event.target.value);
                        if (minute !== null) set("start_minute", minute);
                      }}
                    />
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="ai-end" className="text-xs">
                      {t("ai_settings.agent.end")}
                    </Label>
                    <Input
                      id="ai-end"
                      type="time"
                      className="w-32"
                      value={
                        form.end_minute >= 1440
                          ? "00:00"
                          : minuteToTime(form.end_minute)
                      }
                      disabled={readOnly}
                      onChange={(event) => {
                        const minute = timeToMinute(event.target.value);
                        // 00:00 as the end means "until midnight".
                        if (minute !== null)
                          set("end_minute", minute === 0 ? 1440 : minute);
                      }}
                    />
                  </div>
                </div>
              )}
              {hoursInvalid && (
                <p className="text-destructive text-sm" role="alert">
                  {t("ai_settings.agent.hours_invalid")}
                </p>
              )}
              <ToggleGroup
                type="multiple"
                variant="outline"
                spacing={2}
                className="flex flex-wrap justify-start"
                value={form.weekdays.map(String)}
                disabled={readOnly}
                aria-label={t("ai_settings.agent.weekdays")}
                onValueChange={(next) =>
                  set(
                    "weekdays",
                    next.map(Number).sort((a, b) => a - b),
                  )
                }
              >
                {WEEKDAYS.map((day) => (
                  <ToggleGroupItem
                    key={day}
                    value={String(day)}
                    aria-label={t(`ai_settings.weekday.${day}`)}
                    className="min-w-12"
                  >
                    {t(`ai_settings.weekday_short.${day}`)}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
              {form.weekdays.length === 0 && (
                <p className="text-muted-foreground text-xs">
                  {t("ai_settings.agent.weekdays_empty")}
                </p>
              )}
            </SettingRow>
          </CardContent>
        </Card>

        {canWrite && (
          <div className="bg-background/95 sticky bottom-0 z-10 -mx-1 flex flex-wrap items-center justify-end gap-2 border-t px-1 py-3 backdrop-blur">
            {dirty && (
              <span className="text-muted-foreground me-auto text-xs">
                {t("ai_settings.unsaved")}
              </span>
            )}
            <Button
              variant="outline"
              disabled={!dirty || saving}
              onClick={() => {
                if (query.data) setForm(query.data.config);
              }}
            >
              {t("ai_settings.discard")}
            </Button>
            <Button
              disabled={!dirty || saving || hoursInvalid}
              onClick={() => void save()}
            >
              <Save className="size-4" />
              {saving ? t("ai_settings.saving") : t("ai_settings.save")}
            </Button>
          </div>
        )}
      </div>
      <div className="min-w-0">
        <TestChatPanel slug={slug} canRun={canRun} locale={form.locale} />
      </div>
    </div>
  );
}
