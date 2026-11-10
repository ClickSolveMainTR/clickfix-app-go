"use client";

import { Check, Plus, Trash2, X } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { routes } from "@/config/routes";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import {
  FieldIssues,
  Group,
  LocalizedRow,
  NumberRow,
  Row,
  SelectRow,
  SwitchRow,
  TextRow,
  UrlRulesRow,
  useEditor,
} from "@/features/widget/components/fields";
import { LIMITS, newId, type Trigger } from "@/features/widget/lib/settings";
import { useLocale } from "@/providers/locale-provider";
import { eligible, visible } from "@widget/shared/targeting";

const ATTACHMENT_TYPES = [
  "image",
  "pdf",
  "document",
  "spreadsheet",
  "video",
  "audio",
  "archive",
  "text",
] as const;
const FEATURES = [
  "emoji",
  "typing",
  "read_receipts",
  "sound",
  "notifications",
  "markdown",
  "history",
] as const;

export function ConversationSection() {
  const { t } = useLocale();
  const ed = useEditor();
  const s = ed.settings;
  const types = s.attachments.types as string[];
  return (
    <div className="space-y-4">
      <Group title={t("widget.g.features")}>
        <div className="grid gap-3 sm:grid-cols-2">
          {FEATURES.map((f) => (
            <SwitchRow
              key={f}
              path={`features.${f}`}
              label={t(`widget.feature.${f}`)}
              hint={t(`widget.feature.${f}_hint`)}
            />
          ))}
        </div>
      </Group>
      <Group title={t("widget.g.attachments")}>
        <SwitchRow
          path="file_upload"
          label={t("widget.fields.file_upload")}
          hint={t("widget.fields.file_upload_hint")}
        />
        {s.file_upload ? (
          <>
            <NumberRow
              path="attachments.max_mb"
              label={t("widget.f.max_mb")}
              min={1}
              max={10}
              unit="MB"
            />
            <Row
              label={t("widget.f.attachment_types")}
              path="attachments.types"
            >
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                {ATTACHMENT_TYPES.map((type) => (
                  <label key={type} className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={types.includes(type)}
                      disabled={ed.disabled}
                      onCheckedChange={(v) =>
                        ed.set(
                          "attachments.types",
                          v
                            ? ATTACHMENT_TYPES.filter(
                                (x) => x === type || types.includes(x),
                              )
                            : types.filter((x) => x !== type),
                        )
                      }
                    />
                    {t(`widget.attachment.${type}`)}
                  </label>
                ))}
              </div>
            </Row>
          </>
        ) : null}
      </Group>
      <Group title={t("widget.g.csat")}>
        <SwitchRow
          path="csat"
          label={t("widget.fields.csat")}
          hint={t("widget.fields.csat_hint")}
        />
        {s.csat ? (
          <>
            <SelectRow
              path="csat_options.scale"
              label={t("widget.f.csat_scale")}
              options={(["stars", "emoji", "thumbs"] as const).map((v) => [
                v,
                t(`widget.csat_scale.${v}`),
              ])}
            />
            <SwitchRow
              path="csat_options.comment"
              label={t("widget.f.csat_comment")}
            />
            <LocalizedRow
              path="csat_options.question"
              label={t("widget.f.csat_question")}
              maxLength={160}
            />
          </>
        ) : null}
      </Group>
      <Group
        title={t("widget.g.offline")}
        description={t("widget.g.offline_hint")}
      >
        <p className="text-muted-foreground text-sm">
          {t("widget.g.hours_source")}{" "}
          <Link
            href={`${routes.cms.settings.sla}?tab=hours`}
            className="text-foreground underline underline-offset-4"
          >
            {t("widget.g.hours_source_link")}
          </Link>
        </p>
        <SwitchRow
          path="away_mode"
          label={t("widget.f.away_mode")}
          hint={t("widget.f.away_mode_hint")}
        />
        <SelectRow
          path="offline.mode"
          label={t("widget.f.offline_mode")}
          options={(["form", "message", "hide"] as const).map((v) => [
            v,
            t(`widget.offline_mode.${v}`),
          ])}
        />
        <SwitchRow
          path="offline.collect_email"
          label={t("widget.f.collect_email")}
        />
      </Group>
      <Group title={t("widget.g.rate_limit")}>
        <NumberRow
          path="rate_limit.messages_per_minute"
          label={t("widget.f.rate_limit")}
          hint={t("widget.f.rate_limit_hint")}
          min={5}
          max={120}
        />
      </Group>
    </div>
  );
}

export function ChannelsSection() {
  const { t } = useLocale();
  return (
    <div className="space-y-4">
      <Group
        title={t("widget.g.channels")}
        description={t("widget.g.channels_hint")}
      >
        <SwitchRow
          path="channels.chat"
          label={t("widget.channel.chat")}
          hint={t("widget.channel.chat_hint")}
        />
        <div className="grid gap-4 sm:grid-cols-2">
          <TextRow
            path="channels.whatsapp"
            label="WhatsApp"
            brand="whatsapp"
            placeholder="+90 555 000 00 00"
            type="tel"
          />
          <TextRow
            path="channels.phone"
            label={t("widget.channel.phone")}
            placeholder="+90 212 000 00 00"
            type="tel"
          />
          <TextRow
            path="channels.email"
            label={t("widget.channel.email")}
            placeholder="destek@example.com"
            type="email"
          />
          <TextRow
            path="channels.instagram"
            label="Instagram"
            brand="instagram"
            placeholder="@acme"
          />
          <TextRow
            path="channels.telegram"
            label="Telegram"
            brand="telegram"
            placeholder="@acme"
          />
          <TextRow
            path="channels.messenger"
            label="Messenger"
            brand="messenger"
            placeholder="acme"
          />
        </div>
        <SwitchRow
          path="channels.callback"
          label={t("widget.channel.callback")}
          hint={t("widget.channel.callback_hint")}
        />
        <SwitchRow
          path="channels.video"
          label={t("widget.channel.video")}
          hint={t("widget.channel.video_hint")}
        />
      </Group>
    </div>
  );
}

export function AISection() {
  const { t } = useLocale();
  const ed = useEditor();
  const keywords = ed.settings.ai.handoff_keywords;
  return (
    <div className="space-y-4">
      <Group title={t("widget.g.ai")} description={t("widget.g.ai_hint")}>
        <SwitchRow
          path="ai.enabled"
          label={t("widget.f.ai_enabled")}
          hint={t("widget.f.ai_enabled_hint")}
        />
        <SwitchRow
          path="ai.human_button"
          label={t("widget.f.ai_human")}
          hint={t("widget.f.ai_human_hint")}
          disabled={!ed.settings.ai.enabled}
        />
        <Row
          label={t("widget.f.handoff_keywords")}
          hint={t("widget.f.handoff_keywords_hint")}
          path="ai.handoff_keywords"
        >
          <Input
            value={keywords.join(", ")}
            disabled={ed.disabled}
            placeholder="insan, temsilci, şikayet"
            onChange={(e) =>
              ed.set(
                "ai.handoff_keywords",
                e.target.value
                  .split(",")
                  .map((k) => k.trimStart())
                  .slice(0, 30),
              )
            }
          />
        </Row>
      </Group>
    </div>
  );
}

function emptyTrigger(): Trigger {
  return {
    id: newId("t"),
    name: "",
    enabled: true,
    message: { tr: "", en: "" },
    delay_s: 0,
    frequency: "once",
    open: false,
    conditions: {
      url: [],
      time_on_page_s: 10,
      scroll_pct: 0,
      exit_intent: false,
      visitor: "any",
      device: "any",
      utm_source: "",
      utm_medium: "",
      utm_campaign: "",
      identified: "any",
      languages: [],
      countries: [],
    },
  };
}

export function TriggersSection() {
  const { t } = useLocale();
  const ed = useEditor();
  const list = ed.settings.triggers;
  return (
    <div className="space-y-4">
      <Group
        title={t("widget.g.triggers")}
        description={t("widget.g.triggers_hint")}
      >
        {list.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("widget.f.no_triggers")}
          </p>
        ) : null}
        {list.map((tr, i) => {
          const p = `triggers.${i}`;
          return (
            <div
              key={tr.id}
              className="space-y-4 rounded-lg border p-3"
              data-testid={`trigger-${i}`}
            >
              <div className="flex items-center gap-2">
                <Switch
                  checked={tr.enabled}
                  aria-label={t("widget.f.trigger_enabled")}
                  disabled={ed.disabled}
                  onCheckedChange={(v) => ed.set(`${p}.enabled`, v)}
                />
                <Input
                  value={tr.name}
                  maxLength={80}
                  placeholder={t("widget.f.trigger_name")}
                  aria-label={t("widget.f.trigger_name")}
                  disabled={ed.disabled}
                  onChange={(e) => ed.set(`${p}.name`, e.target.value)}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={t("widget.editor.remove")}
                  disabled={ed.disabled}
                  onClick={() =>
                    ed.set(
                      "triggers",
                      list.filter((_, n) => n !== i),
                    )
                  }
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
              <LocalizedRow
                path={`${p}.message`}
                label={t("widget.f.trigger_message")}
                maxLength={500}
                multiline
              />
              <div className="grid gap-4 sm:grid-cols-2">
                <NumberRow
                  path={`${p}.conditions.time_on_page_s`}
                  label={t("widget.f.time_on_page")}
                  min={0}
                  max={600}
                  unit="sn"
                />
                <NumberRow
                  path={`${p}.conditions.scroll_pct`}
                  label={t("widget.f.scroll_pct")}
                  min={0}
                  max={100}
                  unit="%"
                />
                <NumberRow
                  path={`${p}.delay_s`}
                  label={t("widget.f.delay")}
                  min={0}
                  max={120}
                  unit="sn"
                />
                <SelectRow
                  path={`${p}.frequency`}
                  label={t("widget.f.frequency")}
                  options={(["once", "session", "always"] as const).map((v) => [
                    v,
                    t(`widget.frequency.${v}`),
                  ])}
                />
                <SelectRow
                  path={`${p}.conditions.visitor`}
                  label={t("widget.f.visitor")}
                  options={(["any", "new", "returning"] as const).map((v) => [
                    v,
                    t(`widget.visitor.${v}`),
                  ])}
                />
                <SelectRow
                  path={`${p}.conditions.device`}
                  label={t("widget.f.device")}
                  options={(["any", "desktop", "mobile"] as const).map((v) => [
                    v,
                    t(`widget.device.${v}`),
                  ])}
                />
                <SelectRow
                  path={`${p}.conditions.identified`}
                  label={t("widget.f.identified")}
                  options={(["any", "yes", "no"] as const).map((v) => [
                    v,
                    t(`widget.identified.${v}`),
                  ])}
                />
              </div>
              <SwitchRow
                path={`${p}.conditions.exit_intent`}
                label={t("widget.f.exit_intent")}
                hint={t("widget.f.exit_intent_hint")}
              />
              <SwitchRow
                path={`${p}.open`}
                label={t("widget.f.trigger_open")}
                hint={t("widget.f.trigger_open_hint")}
              />
              <UrlRulesRow
                path={`${p}.conditions.url`}
                label={t("widget.f.trigger_urls")}
                hint={t("widget.f.url_rules_hint")}
              />
              <div className="grid gap-4 sm:grid-cols-3">
                <TextRow
                  path={`${p}.conditions.utm_source`}
                  label="utm_source"
                  maxLength={100}
                />
                <TextRow
                  path={`${p}.conditions.utm_medium`}
                  label="utm_medium"
                  maxLength={100}
                />
                <TextRow
                  path={`${p}.conditions.utm_campaign`}
                  label="utm_campaign"
                  maxLength={100}
                />
              </div>
              <FieldIssues path={p} />
            </div>
          );
        })}
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={ed.disabled || list.length >= LIMITS.triggers}
          onClick={() => ed.set("triggers", [...list, emptyTrigger()])}
        >
          <Plus className="size-4" />
          {t("widget.editor.add_trigger")}
        </Button>
      </Group>
    </div>
  );
}

export function TargetingSection() {
  const { t } = useLocale();
  const ed = useEditor();
  const tg = ed.settings.targeting;
  const [testURL, setTestURL] = useState("");
  const [testMobile, setTestMobile] = useState(false);
  const ctx = {
    url: testURL,
    device: testMobile ? "mobile" : "desktop",
  };
  const result = testURL ? visible(tg, ctx) : null;
  const firing = testURL
    ? ed.settings.triggers.filter((tr) => eligible(tr, ctx))
    : [];
  return (
    <div className="space-y-4">
      <Group title={t("widget.g.pages")} description={t("widget.g.pages_hint")}>
        <UrlRulesRow
          path="targeting.show_on"
          label={t("widget.f.show_on")}
          hint={t("widget.f.show_on_hint")}
        />
        <UrlRulesRow
          path="targeting.hide_on"
          label={t("widget.f.hide_on")}
          hint={t("widget.f.hide_on_hint")}
        />
      </Group>
      <Group title={t("widget.g.audience")}>
        <SelectRow
          path="targeting.devices"
          label={t("widget.f.devices")}
          options={(["all", "desktop", "mobile"] as const).map((v) => [
            v,
            t(`widget.devices.${v}`),
          ])}
        />
        <SelectRow
          path="targeting.countries.mode"
          label={t("widget.f.countries_mode")}
          hint={t("widget.f.countries_hint")}
          options={(["all", "allow", "block"] as const).map((v) => [
            v,
            t(`widget.countries.${v}`),
          ])}
        />
        {tg.countries.mode !== "all" ? (
          <Row label={t("widget.f.countries")} path="targeting.countries.list">
            <Input
              value={tg.countries.list.join(", ")}
              placeholder="TR, DE, NL"
              disabled={ed.disabled}
              onChange={(e) =>
                ed.set(
                  "targeting.countries.list",
                  e.target.value
                    .toUpperCase()
                    .split(/[\s,]+/)
                    .filter(Boolean),
                )
              }
            />
          </Row>
        ) : null}
      </Group>
      <Group
        title={t("widget.g.url_tester")}
        description={t("widget.g.url_tester_hint")}
      >
        <div className="flex flex-wrap items-center gap-3">
          <Input
            className="min-w-0 flex-1"
            value={testURL}
            placeholder="https://www.example.com/pricing?utm_source=google"
            aria-label={t("widget.g.url_tester")}
            onChange={(e) => setTestURL(e.target.value)}
          />
          <label className="flex items-center gap-2 text-sm">
            <Switch checked={testMobile} onCheckedChange={setTestMobile} />
            {t("widget.device.mobile")}
          </label>
        </div>
        {result ? (
          <div className="space-y-2 text-sm" aria-live="polite">
            <div className="flex items-center gap-2">
              {result.visible ? (
                <Check className="size-4 text-green-600" />
              ) : (
                <X className="text-destructive size-4" />
              )}
              {result.visible
                ? t("widget.tester.visible")
                : t(`widget.tester.hidden_${result.reason}`)}
            </div>
            <div className="flex flex-wrap items-center gap-1">
              <span className="text-muted-foreground">
                {t("widget.tester.triggers")}
              </span>
              {firing.length ? (
                firing.map((tr) => (
                  <Badge key={tr.id} variant="secondary">
                    {tr.name || tr.id}
                  </Badge>
                ))
              ) : (
                <span>—</span>
              )}
            </div>
          </div>
        ) : null}
      </Group>
    </div>
  );
}
