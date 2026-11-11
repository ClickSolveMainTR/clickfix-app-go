"use client";

import { useQuery } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, Plus, Trash2, TriangleAlert } from "lucide-react";
import Link from "next/link";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { routes } from "@/config/routes";
import {
  FieldIssues,
  Group,
  LocalizedRow,
  Row,
  SelectRow,
  SwitchRow,
  TextRow,
  useEditor,
} from "@/features/widget/components/fields";
import { LIMITS, newId, type HomeCard } from "@/features/widget/lib/settings";
import { TemplateVariableChips } from "@/features/custom-fields";
import { getActiveOrganizationSlug } from "@/features/organizations";
import { requestFormsService } from "@/features/request-forms/services/request-forms.service";
import { useLocale } from "@/providers/locale-provider";

const TEXT_FIELDS = [
  ["greeting_title", 80, false],
  ["greeting_text", 300, true],
  ["team_intro", 200, true],
  ["offline_message", 300, true],
  ["away_message", 300, true],
  ["placeholder", 80, false],
] as const;

export function TextsSection() {
  const { t } = useLocale();
  const ed = useEditor();
  return (
    <div className="space-y-4">
      <Group title={t("widget.g.language")}>
        <SelectRow
          path="locale"
          label={t("widget.f.locale")}
          hint={t("widget.f.locale_hint")}
          options={[
            ["auto", t("widget.locale.auto")],
            ["tr", "Türkçe"],
            ["en", "English"],
          ]}
        />
      </Group>
      <Group title={t("widget.g.texts")} description={t("widget.g.texts_hint")}>
        {TEXT_FIELDS.map(([key, max, multiline]) => (
          <div key={key} className="space-y-2">
            <p className="text-sm font-medium">{t(`widget.text.${key}`)}</p>
            {key === "greeting_title" || key === "greeting_text" ? (
              <div className="space-y-1">
                <p className="text-muted-foreground text-xs">
                  {t("widget.text.greeting_variables_hint")}
                </p>
                <TemplateVariableChips
                  audience="ai"
                  onPick={(token) => {
                    void navigator.clipboard?.writeText(token);
                  }}
                />
              </div>
            ) : null}
            <div className="grid gap-2 sm:grid-cols-2">
              {(["tr", "en"] as const).map((loc) => (
                <TextRow
                  key={loc}
                  path={`texts.${loc}.${key}`}
                  label={loc === "tr" ? "Türkçe" : "English"}
                  maxLength={max}
                  multiline={multiline}
                  placeholder={t(`widget.text_default.${loc}.${key}`)}
                />
              ))}
            </div>
          </div>
        ))}
      </Group>
      <Group
        title={t("widget.g.reply_time")}
        description={t("widget.g.reply_time_hint")}
      >
        <div className="grid gap-4 sm:grid-cols-2">
          <SelectRow
            path="reply_time.mode"
            label={t("widget.f.reply_mode")}
            options={(["auto", "manual", "hidden"] as const).map((v) => [
              v,
              t(`widget.reply_mode.${v}`),
            ])}
          />
          <SelectRow
            path="reply_time.value"
            label={t("widget.f.reply_value")}
            disabled={ed.settings.reply_time.mode !== "manual"}
            options={(["minutes", "hour", "hours", "day"] as const).map((v) => [
              v,
              t(`widget.reply_value.${v}`),
            ])}
          />
        </div>
      </Group>
    </div>
  );
}

const CARD_TYPES: HomeCard["type"][] = [
  "conversation",
  "help_search",
  "articles",
  "status",
  "link",
  "booking",
  "channels",
];

export function HomeSection() {
  const { t } = useLocale();
  const ed = useEditor();
  const cards = ed.settings.home.cards;
  const setCards = (next: HomeCard[]) => ed.set("home.cards", next);
  const move = (i: number, d: number) => {
    const next = [...cards];
    const [c] = next.splice(i, 1);
    next.splice(i + d, 0, c!);
    setCards(next);
  };
  const missing = CARD_TYPES.filter((type) =>
    type === "link" || type === "status"
      ? true
      : !cards.some((c) => c.type === type),
  );
  const replies = ed.settings.quick_replies;
  return (
    <div className="space-y-4">
      <Group
        title={t("widget.g.home_cards")}
        description={t("widget.g.home_cards_hint")}
      >
        <ol className="space-y-2">
          {cards.map((c, i) => (
            <li
              key={c.id}
              className="space-y-3 rounded-lg border p-3"
              data-testid={`card-${c.id}`}
            >
              <div className="flex items-center gap-2">
                <Switch
                  checked={c.enabled}
                  aria-label={t(`widget.card.${c.type}`)}
                  disabled={ed.disabled}
                  onCheckedChange={(v) => ed.set(`home.cards.${i}.enabled`, v)}
                />
                <span className="flex-1 text-sm font-medium">
                  {t(`widget.card.${c.type}`)}
                </span>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={t("widget.editor.move_up")}
                  disabled={ed.disabled || i === 0}
                  onClick={() => move(i, -1)}
                >
                  <ArrowUp className="size-4" />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={t("widget.editor.move_down")}
                  disabled={ed.disabled || i === cards.length - 1}
                  onClick={() => move(i, 1)}
                >
                  <ArrowDown className="size-4" />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={t("widget.editor.remove")}
                  disabled={ed.disabled}
                  onClick={() => setCards(cards.filter((_, n) => n !== i))}
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
              {c.enabled ? (
                <div className="grid gap-3 sm:grid-cols-2">
                  <LocalizedRow
                    path={`home.cards.${i}.title`}
                    label={t("widget.f.card_title")}
                    hint={
                      c.type === "link"
                        ? undefined
                        : t("widget.f.card_title_hint")
                    }
                    maxLength={80}
                  />
                  <LocalizedRow
                    path={`home.cards.${i}.description`}
                    label={t("widget.f.card_description")}
                    maxLength={200}
                  />
                  {c.type === "link" || c.type === "status" ? (
                    <TextRow
                      path={`home.cards.${i}.url`}
                      label={t("widget.f.card_url")}
                      placeholder="https://"
                      type="url"
                    />
                  ) : null}
                </div>
              ) : null}
              <FieldIssues path={`home.cards.${i}`} />
            </li>
          ))}
        </ol>
        <div className="flex flex-wrap gap-2">
          {missing.map((type) => (
            <Button
              key={type}
              type="button"
              variant="outline"
              size="sm"
              disabled={ed.disabled || cards.length >= LIMITS.homeCards}
              onClick={() =>
                setCards([
                  ...cards,
                  {
                    id:
                      type === "link" || type === "status" ? newId(type) : type,
                    type,
                    enabled: true,
                    title: {},
                    description: {},
                    url: "",
                  },
                ])
              }
            >
              <Plus className="size-4" />
              {t(`widget.card.${type}`)}
            </Button>
          ))}
        </div>
      </Group>
      <Group
        title={t("widget.g.quick_replies")}
        description={t("widget.g.quick_replies_hint")}
      >
        {replies.map((_, i) => (
          <div key={i} className="flex items-start gap-2">
            <div className="flex-1">
              <LocalizedRow
                path={`quick_replies.${i}`}
                label={`${t("widget.f.quick_reply")} ${i + 1}`}
                maxLength={120}
              />
            </div>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="mt-6"
              aria-label={t("widget.editor.remove")}
              disabled={ed.disabled}
              onClick={() =>
                ed.set(
                  "quick_replies",
                  replies.filter((_, n) => n !== i),
                )
              }
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        ))}
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={ed.disabled || replies.length >= LIMITS.quickReplies}
          onClick={() =>
            ed.set("quick_replies", [...replies, { tr: "", en: "" }])
          }
        >
          <Plus className="size-4" />
          {t("widget.editor.add_quick_reply")}
        </Button>
      </Group>
    </div>
  );
}

/**
 * The pre-chat form is a request form (CF-198): the organization's forms
 * live under Settings > Request forms, the widget only picks one.
 */
export function PreChatFormPicker({ disabled }: { disabled: boolean }) {
  const { t } = useLocale();
  const ed = useEditor();
  const slug = getActiveOrganizationSlug() ?? "";
  const forms = useQuery({
    queryKey: ["tenant", slug, "request-forms"],
    queryFn: () => requestFormsService.list(),
    enabled: Boolean(slug),
  });
  const widgetForms = (forms.data ?? []).filter(
    (f) => f.is_active && f.channels.includes("widget"),
  );
  const value = ed.settings.prechat.form_uuid || AUTO_FORM;
  const chosen = widgetForms.find((f) => f.uuid === value);
  return (
    <Row
      label={t("widget.f.prechat_form")}
      hint={t("widget.f.prechat_form_hint")}
      path="prechat.form_uuid"
    >
      <Select
        value={value}
        disabled={disabled || forms.isLoading}
        onValueChange={(v) =>
          ed.set("prechat.form_uuid", v === AUTO_FORM ? "" : v)
        }
      >
        <SelectTrigger className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={AUTO_FORM}>
            {t("widget.f.prechat_form_auto")}
          </SelectItem>
          {widgetForms.map((f) => (
            <SelectItem key={f.uuid} value={f.uuid}>
              {f.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {!forms.isLoading && widgetForms.length === 0 ? (
        <Alert>
          <TriangleAlert />
          <AlertDescription>{t("widget.f.prechat_form_none")}</AlertDescription>
        </Alert>
      ) : null}
      <p className="text-muted-foreground text-xs">
        {chosen
          ? t("widget.f.prechat_form_fields", {
              fields: chosen.fields.map((f) => f.label || f.key).join(", "),
            })
          : null}{" "}
        <Link
          href={routes.cms.settings.requestForms}
          className="text-foreground underline underline-offset-4"
        >
          {t("widget.f.prechat_form_manage")}
        </Link>
      </p>
    </Row>
  );
}

const AUTO_FORM = "_auto";

export function PreChatSection() {
  const { t } = useLocale();
  const ed = useEditor();
  const pc = ed.settings.prechat;
  return (
    <div className="space-y-4">
      <Group
        title={t("widget.g.prechat")}
        description={t("widget.g.prechat_hint")}
      >
        <SwitchRow
          path="prechat.enabled"
          label={t("widget.fields.prechat")}
          hint={t("widget.fields.prechat_hint")}
        />
        <PreChatFormPicker disabled={ed.disabled || !pc.enabled} />
      </Group>
      <Group
        title={t("widget.g.consent")}
        description={t("widget.g.consent_hint")}
      >
        <SwitchRow
          path="prechat.consent.enabled"
          label={t("widget.f.consent_enabled")}
        />
        {pc.consent.enabled ? (
          <>
            <SwitchRow
              path="prechat.consent.required"
              label={t("widget.f.consent_required")}
            />
            <LocalizedRow
              path="prechat.consent.text"
              label={t("widget.f.consent_text")}
              maxLength={500}
              multiline
            />
            <TextRow
              path="prechat.consent.url"
              label={t("widget.f.privacy_url")}
              placeholder="https://"
              type="url"
            />
          </>
        ) : null}
      </Group>
      <AttributeMapping />
    </div>
  );
}

function AttributeMapping() {
  const { t } = useLocale();
  const ed = useEditor();
  const list = ed.settings.attribute_mapping;
  return (
    <Group
      title={t("widget.g.attributes")}
      description={t("widget.g.attributes_hint")}
    >
      <Alert>
        <TriangleAlert />
        <AlertDescription>
          {t("widget.g.attributes_publish_notice")}
        </AlertDescription>
      </Alert>
      {list.map((m, i) => (
        <div
          key={i}
          className="grid items-start gap-2 sm:grid-cols-[1fr_1fr_auto_auto]"
        >
          <Row
            label={t("widget.f.attribute_key")}
            path={`attribute_mapping.${i}.key`}
          >
            <Input
              value={m.key}
              maxLength={40}
              className="font-mono"
              placeholder="plan"
              disabled={ed.disabled}
              onChange={(e) =>
                ed.set(
                  `attribute_mapping.${i}.key`,
                  e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, "_"),
                )
              }
            />
          </Row>
          <Row
            label={t("widget.f.attribute_target")}
            hint={t("widget.f.attribute_target_hint")}
            path={`attribute_mapping.${i}.target`}
          >
            <Input
              value={m.target}
              maxLength={50}
              className="font-mono"
              placeholder="custom:plan"
              disabled={ed.disabled}
              onChange={(e) =>
                ed.set(`attribute_mapping.${i}.target`, e.target.value.trim())
              }
            />
          </Row>
          <label className="mt-7 flex items-center gap-2 text-xs">
            <Switch
              checked={m.anonymous}
              disabled={ed.disabled}
              onCheckedChange={(v) =>
                ed.set(`attribute_mapping.${i}.anonymous`, v)
              }
            />
            {t("widget.f.attribute_anonymous")}
          </label>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="mt-6"
            aria-label={t("widget.editor.remove")}
            disabled={ed.disabled}
            onClick={() =>
              ed.set(
                "attribute_mapping",
                list.filter((_, n) => n !== i),
              )
            }
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      ))}
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={ed.disabled || list.length >= LIMITS.mappings}
          onClick={() =>
            ed.set("attribute_mapping", [
              ...list,
              { key: "", target: "custom:", anonymous: false },
            ])
          }
        >
          <Plus className="size-4" />
          {t("widget.editor.add_mapping")}
        </Button>
        {(["name", "email", "phone"] as const).map((target) => (
          <Badge key={target} variant="secondary" className="font-mono">
            {target}
          </Badge>
        ))}
        <Badge variant="secondary" className="font-mono">
          custom:&lt;key&gt;
        </Badge>
      </div>
    </Group>
  );
}
