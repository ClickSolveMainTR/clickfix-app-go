"use client";

import { ArrowLeft, ArrowRight, Check, Rocket } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  ColorRow,
  Group,
  Row,
  SelectRow,
  SwitchRow,
  TextRow,
  useEditor,
} from "@/features/widget/components/fields";
import { PreChatFormPicker } from "@/features/widget/components/sections-content";
import { InstallSection } from "@/features/widget/components/sections-security";
import {
  BASIC_STEPS,
  basicStepIssues,
  parseDomains,
  type BasicStepId,
  type Issue,
} from "@/features/widget/lib/settings";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/**
 * First-time set-up of a widget in three steps (CF-198): look, identity,
 * channel. Each step edits the same settings as the advanced editor, so
 * switching between the two never loses anything. The last step offers the
 * install snippet and the publish button.
 */
export function BasicSetup({
  widgetId,
  hasSecret,
  issues,
  canPublish,
  onPublish,
  onOpenAdvanced,
}: {
  widgetId: string;
  hasSecret: boolean;
  issues: Issue[];
  canPublish: boolean;
  onPublish: () => void;
  onOpenAdvanced: () => void;
}) {
  const { t } = useLocale();
  const ed = useEditor();
  const [step, setStep] = useState<BasicStepId>("appearance");
  const index = BASIC_STEPS.findIndex((s) => s.id === step);
  const last = index === BASIC_STEPS.length - 1;
  const [domains, setDomains] = useState(() =>
    ed.settings.allowed_domains.join("\n"),
  );

  return (
    <div className="space-y-4" data-testid="basic-setup">
      <ol className="flex flex-wrap gap-2" aria-label={t("widget.basic.steps")}>
        {BASIC_STEPS.map((s, i) => {
          const count = basicStepIssues(issues, s.id);
          const done = i < index && count === 0;
          return (
            <li key={s.id}>
              <button
                type="button"
                aria-current={s.id === step ? "step" : undefined}
                onClick={() => setStep(s.id)}
                className={cn(
                  "hover:bg-accent flex items-center gap-2 rounded-full border px-3 py-1.5 text-sm",
                  s.id === step && "bg-accent font-medium",
                )}
              >
                <span
                  className={cn(
                    "flex size-5 items-center justify-center rounded-full text-[11px]",
                    done
                      ? "bg-signal text-signal-foreground"
                      : "bg-muted text-muted-foreground",
                  )}
                  aria-hidden
                >
                  {done ? <Check className="size-3" /> : i + 1}
                </span>
                {t(`widget.basic.step.${s.id}`)}
                {count ? (
                  <span className="bg-destructive text-destructive-foreground rounded-full px-1.5 text-[10px] font-semibold">
                    {count}
                  </span>
                ) : null}
              </button>
            </li>
          );
        })}
      </ol>
      <div>
        <h2 className="text-lg font-semibold">
          {t(`widget.basic.step.${step}`)}
        </h2>
        <p className="text-muted-foreground text-sm">
          {t(`widget.basic.hint.${step}`)}
        </p>
      </div>

      {step === "appearance" ? (
        <Group title={t("widget.basic.step.appearance")}>
          <div className="grid gap-4 sm:grid-cols-2">
            <ColorRow
              path="color"
              label={t("widget.f.color")}
              fallback="#4f46e5"
            />
            <SelectRow
              path="appearance.theme"
              label={t("widget.f.theme")}
              options={(["light", "dark", "auto"] as const).map((v) => [
                v,
                t(`widget.theme.${v}`),
              ])}
            />
            <SelectRow
              path="position"
              label={t("widget.f.position")}
              options={[
                ["right", t("widget.position.right")],
                ["left", t("widget.position.left")],
              ]}
            />
            <SelectRow
              path="appearance.launcher.shape"
              label={t("widget.f.launcher_shape")}
              options={(["circle", "rounded", "pill"] as const).map((v) => [
                v,
                t(`widget.shape.${v}`),
              ])}
            />
            <SelectRow
              path="appearance.launcher.icon"
              label={t("widget.f.launcher_icon")}
              options={(
                ["chat", "help", "headset", "sparkles", "wave"] as const
              ).map((v) => [v, t(`widget.icon.${v}`)])}
            />
          </div>
        </Group>
      ) : null}

      {step === "identity" ? (
        <Group title={t("widget.basic.step.identity")}>
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
          {(["greeting_title", "greeting_text"] as const).map((key) => (
            <div key={key} className="space-y-2">
              <p className="text-sm font-medium">{t(`widget.text.${key}`)}</p>
              <div className="grid gap-2 sm:grid-cols-2">
                {(["tr", "en"] as const).map((loc) => (
                  <TextRow
                    key={loc}
                    path={`texts.${loc}.${key}`}
                    label={loc === "tr" ? "Türkçe" : "English"}
                    maxLength={key === "greeting_title" ? 80 : 300}
                    multiline={key === "greeting_text"}
                  />
                ))}
              </div>
            </div>
          ))}
          <TextRow
            path="appearance.logo_url"
            label={t("widget.f.logo_url")}
            hint={t("widget.f.logo_url_hint")}
            placeholder="https://"
            type="url"
          />
          <SwitchRow
            path="show_team"
            label={t("widget.f.show_team")}
            hint={t("widget.f.show_team_hint")}
          />
        </Group>
      ) : null}

      {step === "channel" ? (
        <>
          <Group title={t("widget.basic.step.channel")}>
            <SwitchRow
              path="ai.enabled"
              label={t("widget.f.ai_enabled")}
              hint={t("widget.f.ai_enabled_hint")}
            />
            <SwitchRow
              path="prechat.enabled"
              label={t("widget.fields.prechat")}
              hint={t("widget.fields.prechat_hint")}
            />
            <PreChatFormPicker
              disabled={ed.disabled || !ed.settings.prechat.enabled}
            />
            <Row
              label={t("widget.fields.domains")}
              hint={t("widget.fields.domains_hint")}
              path="allowed_domains"
              htmlFor="widget-basic-domains"
            >
              <Textarea
                id="widget-basic-domains"
                value={domains}
                rows={3}
                className="font-mono text-xs"
                placeholder={"example.com\n*.example.com"}
                disabled={ed.disabled}
                onChange={(e) => {
                  setDomains(e.target.value);
                  ed.set("allowed_domains", parseDomains(e.target.value));
                }}
              />
            </Row>
          </Group>
          <InstallSection
            widgetId={widgetId}
            hasSecret={hasSecret}
            onOpenSecurity={onOpenAdvanced}
          />
        </>
      ) : null}

      <div className="flex flex-wrap items-center gap-2">
        {index > 0 ? (
          <Button
            type="button"
            variant="outline"
            onClick={() => setStep(BASIC_STEPS[index - 1].id)}
          >
            <ArrowLeft className="size-4" />
            {t("widget.basic.back")}
          </Button>
        ) : null}
        {!last ? (
          <Button
            type="button"
            onClick={() => setStep(BASIC_STEPS[index + 1].id)}
          >
            {t("widget.basic.next")}
            <ArrowRight className="size-4" />
          </Button>
        ) : canPublish ? (
          <Button type="button" onClick={onPublish}>
            <Rocket className="size-4" />
            {t("widget.editor.publish")}
          </Button>
        ) : null}
        <Button
          type="button"
          variant="ghost"
          className="ms-auto"
          onClick={onOpenAdvanced}
        >
          {t("widget.basic.open_advanced")}
        </Button>
      </div>
      <Label className="text-muted-foreground text-xs font-normal">
        {t("widget.basic.footnote")}
      </Label>
    </div>
  );
}
