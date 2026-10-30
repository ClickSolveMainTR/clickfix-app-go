"use client";

import { Monitor, Smartphone } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import type { WidgetSettings } from "@/features/widget/lib/settings";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export const PREVIEW_VIEWS = [
  "home",
  "chat",
  "messages",
  "help",
  "prechat",
  "csat",
  "offline",
  "proactive",
  "consent",
  "callback",
  "closed",
] as const;
export type PreviewView = (typeof PREVIEW_VIEWS)[number];

type Props = {
  settings: WidgetSettings;
  name: string;
  /** A version opened from the history shows instead of the draft. */
  label?: string;
};

/**
 * Live preview: the real widget runtime (public/widget.js in preview mode)
 * inside a same-origin iframe, fed with the draft settings by postMessage.
 * Desktop or phone frame, light/dark, language, direction and the state
 * to show (home, conversation, help, offline, …).
 */
export function WidgetPreview({ settings, name, label }: Props) {
  const { t, locale } = useLocale();
  const frame = useRef<HTMLIFrameElement>(null);
  const [ready, setReady] = useState(false);
  const [device, setDevice] = useState<"desktop" | "mobile">("desktop");
  const [theme, setTheme] = useState<"settings" | "light" | "dark">("settings");
  const [lang, setLang] = useState<"tr" | "en">(locale === "en" ? "en" : "tr");
  const [dir, setDir] = useState<"ltr" | "rtl">("ltr");
  const [view, setView] = useState<PreviewView>("home");

  const post = useCallback(() => {
    const win = frame.current?.contentWindow;
    if (!win) return;
    win.postMessage(
      {
        type: "clickfix:preview",
        state: {
          settings,
          name,
          view,
          locale: lang,
          dir,
          theme: theme === "settings" ? settings.appearance.theme : theme,
        },
      },
      window.location.origin,
    );
  }, [settings, name, view, lang, dir, theme]);

  useEffect(() => {
    const onMessage = (e: MessageEvent) => {
      if (
        e.origin !== window.location.origin ||
        e.source !== frame.current?.contentWindow
      )
        return;
      if ((e.data as { type?: string })?.type === "clickfix:preview-ready")
        setReady(true);
    };
    window.addEventListener("message", onMessage);
    return () => window.removeEventListener("message", onMessage);
  }, []);

  useEffect(() => {
    if (!ready) return;
    const id = window.setTimeout(post, 120);
    return () => window.clearTimeout(id);
  }, [ready, post]);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          value={device}
          onValueChange={(v) => v && setDevice(v as "desktop" | "mobile")}
          aria-label={t("widget.preview.device")}
        >
          <ToggleGroupItem
            value="desktop"
            aria-label={t("widget.preview.desktop")}
          >
            <Monitor className="size-4" />
          </ToggleGroupItem>
          <ToggleGroupItem
            value="mobile"
            aria-label={t("widget.preview.mobile")}
          >
            <Smartphone className="size-4" />
          </ToggleGroupItem>
        </ToggleGroup>
        <Select value={view} onValueChange={(v) => setView(v as PreviewView)}>
          <SelectTrigger
            size="sm"
            className="w-40"
            aria-label={t("widget.preview.state")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {PREVIEW_VIEWS.map((v) => (
              <SelectItem key={v} value={v}>
                {t(`widget.preview.view.${v}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={theme}
          onValueChange={(v) => setTheme(v as "settings" | "light" | "dark")}
        >
          <SelectTrigger
            size="sm"
            className="w-32"
            aria-label={t("widget.f.theme")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="settings">
              {t("widget.preview.theme_settings")}
            </SelectItem>
            <SelectItem value="light">{t("widget.theme.light")}</SelectItem>
            <SelectItem value="dark">{t("widget.theme.dark")}</SelectItem>
          </SelectContent>
        </Select>
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          value={lang}
          onValueChange={(v) => v && setLang(v as "tr" | "en")}
          aria-label={t("widget.f.locale")}
        >
          <ToggleGroupItem value="tr">TR</ToggleGroupItem>
          <ToggleGroupItem value="en">EN</ToggleGroupItem>
        </ToggleGroup>
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          value={dir}
          onValueChange={(v) => v && setDir(v as "ltr" | "rtl")}
          aria-label={t("widget.preview.direction")}
        >
          <ToggleGroupItem value="ltr">LTR</ToggleGroupItem>
          <ToggleGroupItem value="rtl">RTL</ToggleGroupItem>
        </ToggleGroup>
      </div>
      {label ? (
        <p className="text-xs font-medium text-amber-600">{label}</p>
      ) : null}
      <div
        className={cn(
          "bg-muted/40 flex justify-center rounded-xl border",
          device === "mobile" ? "py-4" : "",
        )}
      >
        <div
          className={cn(
            "overflow-hidden bg-white transition-all",
            device === "mobile"
              ? "h-[700px] w-[375px] rounded-[2rem] border-8 border-neutral-900 shadow-xl"
              : "h-[720px] w-full rounded-xl",
          )}
        >
          <iframe
            ref={frame}
            src="/widget-preview.html"
            title={t("widget.preview.label")}
            className="size-full"
            data-testid="widget-preview-frame"
            onLoad={() => {
              // The bridge posts "ready" itself; posting again on load covers a cached frame.
              if (ready) post();
            }}
          />
        </div>
      </div>
    </div>
  );
}
