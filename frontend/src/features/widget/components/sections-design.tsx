"use client";

import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Label } from "@/components/ui/label";
import {
  ColorRow,
  Group,
  LocalizedRow,
  NumberRow,
  SelectRow,
  SwitchRow,
  TextRow,
  useEditor,
} from "@/features/widget/components/fields";
import { useLocale } from "@/providers/locale-provider";

const SWATCHES = [
  "#4f46e5",
  "#2563eb",
  "#0891b2",
  "#059669",
  "#16a34a",
  "#ca8a04",
  "#ea580c",
  "#dc2626",
  "#db2777",
  "#7c3aed",
  "#0f172a",
  "#475569",
];

export function AppearanceSection() {
  const { t } = useLocale();
  const ed = useEditor();
  const s = ed.settings;
  return (
    <div className="space-y-4">
      <Group
        title={t("widget.g.colors")}
        description={t("widget.g.colors_hint")}
      >
        <div className="space-y-2">
          <Label>{t("widget.f.swatches")}</Label>
          <div
            className="flex flex-wrap gap-2"
            role="group"
            aria-label={t("widget.f.swatches")}
          >
            {SWATCHES.map((c) => (
              <button
                key={c}
                type="button"
                aria-label={c}
                aria-pressed={s.color === c}
                disabled={ed.disabled}
                onClick={() => ed.set("color", c)}
                className="size-7 rounded-full border-2 outline-offset-2 aria-pressed:outline aria-pressed:outline-2"
                style={{ background: c, borderColor: "white" }}
              />
            ))}
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <ColorRow
            path="color"
            label={t("widget.f.color")}
            fallback="#4f46e5"
          />
          <ColorRow
            path="appearance.header_color"
            label={t("widget.f.header_color")}
            allowEmpty
            fallback={s.color}
          />
          <ColorRow
            path="appearance.bubble_color"
            label={t("widget.f.bubble_color")}
            allowEmpty
            fallback={s.color}
          />
          <ColorRow
            path="appearance.launcher_color"
            label={t("widget.f.launcher_color")}
            allowEmpty
            fallback={s.color}
          />
        </div>
      </Group>
      <Group title={t("widget.g.theme")}>
        <div className="space-y-1.5">
          <Label>{t("widget.f.theme")}</Label>
          <ToggleGroup
            type="single"
            variant="outline"
            value={s.appearance.theme}
            disabled={ed.disabled}
            onValueChange={(v) => v && ed.set("appearance.theme", v)}
          >
            {(["light", "dark", "auto"] as const).map((v) => (
              <ToggleGroupItem key={v} value={v}>
                {t(`widget.theme.${v}`)}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
          <p className="text-muted-foreground text-xs">
            {t("widget.f.theme_hint")}
          </p>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <SelectRow
            path="appearance.font"
            label={t("widget.f.font")}
            options={(
              ["system", "inherit", "rounded", "serif", "mono"] as const
            ).map((f) => [f, t(`widget.font.${f}`)])}
          />
          <SelectRow
            path="appearance.pattern"
            label={t("widget.f.pattern")}
            options={(
              ["none", "dots", "grid", "waves", "diagonal"] as const
            ).map((f) => [f, t(`widget.pattern.${f}`)])}
          />
        </div>
        <NumberRow
          path="appearance.radius"
          label={t("widget.f.radius")}
          min={0}
          max={28}
          unit="px"
        />
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
        <SwitchRow
          path="appearance.show_branding"
          label={t("widget.f.show_branding")}
        />
      </Group>
      <Group title={t("widget.g.mobile")}>
        <SwitchRow
          path="mobile.fullscreen"
          label={t("widget.f.fullscreen")}
          hint={t("widget.f.fullscreen_hint")}
        />
      </Group>
      <Group
        title={t("widget.g.advanced")}
        description={t("widget.g.advanced_hint")}
      >
        <NumberRow
          path="appearance.z_index"
          label={t("widget.f.z_index")}
          min={1}
          max={2147484000}
          step={1000}
        />
        <TextRow
          path="appearance.custom_css"
          label={t("widget.f.custom_css")}
          hint={t("widget.f.custom_css_hint")}
          multiline
          mono
          maxLength={8000}
          placeholder={
            ".launcher { box-shadow: none; }\n.header h2 { letter-spacing: .02em; }"
          }
        />
      </Group>
    </div>
  );
}

export function LauncherSection() {
  const { t } = useLocale();
  const ed = useEditor();
  const l = ed.settings.appearance.launcher;
  return (
    <div className="space-y-4">
      <Group title={t("widget.g.launcher")}>
        <div className="grid gap-4 sm:grid-cols-2">
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
              ["chat", "help", "headset", "sparkles", "wave", "custom"] as const
            ).map((v) => [v, t(`widget.icon.${v}`)])}
          />
          {l.icon === "custom" ? (
            <TextRow
              path="appearance.launcher.image_url"
              label={t("widget.f.launcher_image")}
              placeholder="https://"
              type="url"
            />
          ) : null}
        </div>
        <LocalizedRow
          path="appearance.launcher.label"
          label={t("widget.f.launcher_label")}
          hint={t("widget.f.launcher_label_hint")}
          maxLength={40}
        />
        <NumberRow
          path="appearance.launcher.size"
          label={t("widget.f.launcher_size")}
          min={40}
          max={80}
          unit="px"
        />
        <div className="grid gap-4 sm:grid-cols-2">
          <NumberRow
            path="appearance.launcher.offset_x"
            label={t("widget.f.offset_x")}
            min={0}
            max={200}
            unit="px"
          />
          <NumberRow
            path="appearance.launcher.offset_y"
            label={t("widget.f.offset_y")}
            min={0}
            max={200}
            unit="px"
          />
        </div>
        <p className="text-muted-foreground text-xs">
          {t("widget.f.custom_launcher_hint")}
        </p>
      </Group>
    </div>
  );
}
