"use client";

import {
  Download,
  Headset,
  MonitorSmartphone,
  ShieldAlert,
} from "lucide-react";
import { useEffect, useState } from "react";

import {
  appOrder,
  detectOS,
  formatSize,
  isApiDownloadPath,
  recommended,
  sortDownloads,
  type DesktopApp,
  type DesktopDownload,
  type DesktopOS,
} from "@/features/desktop/lib/downloads";
import { landingHref } from "@/features/landing/lib/locale";

import { useLanding } from "./landing-context";
import { PageHero } from "./page-hero";
import { Reveal } from "./section";

const APP_ICONS = { support: Headset, agent: MonitorSmartphone } as const;

function platformLabel(
  t: ReturnType<typeof useLanding>["t"],
  item: DesktopDownload,
) {
  if (item.os === "darwin")
    return item.arch === "aarch64"
      ? t("landing.download.os_mac_arm")
      : t("landing.download.os_mac_intel");
  if (item.os === "windows") return t("landing.download.os_windows");
  return t("landing.download.os_linux");
}

/** Reads the visitor's OS and the `?app=` choice after hydration. */
function useVisitor(): { os: DesktopOS | null; app: string | null } {
  const [state, setState] = useState<{
    os: DesktopOS | null;
    app: string | null;
  }>({ os: null, app: null });
  useEffect(() => {
    const nav = navigator as Navigator & {
      userAgentData?: { platform?: string };
    };
    // eslint-disable-next-line react-hooks/set-state-in-effect -- browser-only values, read once after hydration
    setState({
      os: detectOS(nav.userAgent, nav.userAgentData?.platform ?? ""),
      app: new URLSearchParams(window.location.search).get("app"),
    });
  }, []);
  return state;
}

function DownloadRow({
  item,
  primary,
}: {
  item: DesktopDownload;
  primary: boolean;
}) {
  const { t, locale } = useLanding();
  if (!isApiDownloadPath(item.file.url)) return null;
  return (
    <li className="space-y-3 border-t border-white/8 py-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="space-y-1">
          <p className="text-sm font-medium text-white">
            {platformLabel(t, item)}
          </p>
          <p className="text-xs text-white/55">
            {t("landing.download.version", { version: item.version })} ·{" "}
            {formatSize(item.file.size, locale)} · {item.file.name}
          </p>
        </div>
        <a
          href={item.file.url}
          download={item.file.name}
          className={
            primary
              ? "inline-flex items-center gap-2 rounded-full bg-[#03FF13] px-5 py-2.5 text-sm font-semibold text-[#0A0D0B] transition-opacity hover:opacity-90"
              : "inline-flex items-center gap-2 rounded-full border border-white/15 px-4 py-2 text-sm text-white/85 transition-colors hover:border-white/30 hover:text-white"
          }
        >
          <Download className="size-4" aria-hidden />
          {t("landing.download.button")}
        </a>
      </div>
      {!item.os_signed ? (
        <p className="flex items-start gap-2 rounded-xl border border-amber-400/30 bg-amber-400/10 px-3 py-2 text-xs text-amber-200">
          <ShieldAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
          {t("landing.download.unsigned")}
        </p>
      ) : null}
      <p className="text-xs text-white/45">
        <span className="mr-1">SHA-256:</span>
        <code className="font-mono break-all text-white/70">
          {item.file.sha256}
        </code>
      </p>
    </li>
  );
}

function AppCard({
  app,
  items,
  os,
}: {
  app: DesktopApp;
  items: DesktopDownload[];
  os: DesktopOS | null;
}) {
  const { t } = useLanding();
  const Icon = APP_ICONS[app];
  const own = sortDownloads(items.filter((item) => item.app === app));
  const best = recommended(items, app, os);
  // The recommended build first, then the others.
  const rows = best ? [best, ...own.filter((item) => item !== best)] : own;
  return (
    <article
      id={app}
      className="flex h-full scroll-mt-24 flex-col gap-5 rounded-3xl border border-white/10 bg-[#0F1310] p-6 sm:p-8"
    >
      <span className="flex size-10 items-center justify-center rounded-xl bg-white/[0.06] ring-1 ring-white/10">
        <Icon className="size-5 text-[#7DFF86]" aria-hidden />
      </span>
      <div className="space-y-2">
        <h2 className="font-display text-2xl font-semibold tracking-tight">
          {t(`landing.download.${app}_title`)}
        </h2>
        <p className="text-[15px] leading-relaxed text-white/60">
          {t(`landing.download.${app}_body`)}
        </p>
      </div>
      {rows.length === 0 ? (
        <p className="border-t border-white/8 pt-4 text-sm text-white/55">
          {t("landing.download.none")}
        </p>
      ) : (
        <>
          {os && !best ? (
            <p className="text-sm text-white/55">
              {t("landing.download.no_build_for_os")}
            </p>
          ) : null}
          <ul>
            {rows.map((item) => (
              <DownloadRow
                key={item.target}
                item={item}
                primary={item === best}
              />
            ))}
          </ul>
        </>
      )}
    </article>
  );
}

/** `/indir`: both desktop apps per operating system with SHA-256 values. */
export function DownloadPage({ items }: { items: DesktopDownload[] }) {
  const { t, locale } = useLanding();
  const visitor = useVisitor();
  const order = appOrder(visitor.app);

  return (
    <>
      <PageHero
        compact
        crumbs={[
          { href: landingHref(locale), label: t("landing.nav.home_short") },
          { label: t("landing.nav.download") },
        ]}
        eyebrow={t("landing.download.eyebrow")}
        title={t("landing.download.title")}
        lead={t("landing.download.lead")}
      />
      <section className="py-16 sm:py-24">
        <div className="mx-auto grid max-w-7xl gap-4 px-4 sm:px-6 lg:grid-cols-2">
          {order.map((app, i) => (
            <Reveal key={app} delay={i * 0.06}>
              <AppCard app={app} items={items} os={visitor.os} />
            </Reveal>
          ))}
        </div>
        <div className="mx-auto mt-10 max-w-7xl space-y-3 px-4 text-sm text-white/60 sm:px-6">
          <h2 className="font-display text-lg font-semibold text-white">
            {t("landing.download.verify_title")}
          </h2>
          <p>{t("landing.download.verify_body")}</p>
          <ul className="space-y-1 text-xs">
            <li>
              macOS / Linux:{" "}
              <code className="font-mono text-white/80">
                shasum -a 256 &lt;{t("landing.download.file")}&gt;
              </code>
            </li>
            <li>
              Windows (PowerShell):{" "}
              <code className="font-mono text-white/80">
                Get-FileHash &lt;{t("landing.download.file")}&gt; -Algorithm
                SHA256
              </code>
            </li>
          </ul>
          <p>{t("landing.download.scam")}</p>
        </div>
      </section>
    </>
  );
}
