"use client";

import { useQuery } from "@tanstack/react-query";
import {
  AlertTriangle,
  Camera,
  ExternalLink,
  FileWarning,
  Globe,
  Monitor,
  Terminal,
  Video,
} from "lucide-react";
import { useState, type ReactNode } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn, datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useInboxScope } from "../hooks/use-inbox";
import {
  inboxService,
  type WidgetDiagnostic,
  type WidgetTechnicalContext,
} from "../services/inbox.service";

const KIND_ICON = {
  screenshot: Camera,
  recording: Video,
  technical_context: Terminal,
} as const;

/** Signed URLs live 15 minutes; refetch well before they expire. */
const SRC_STALE_MS = 10 * 60_000;

function useDiagnostics(uuid: string) {
  const { org, canRead } = useInboxScope();
  return useQuery({
    queryKey: ["inbox", org, "diagnostics", uuid],
    queryFn: () => inboxService.diagnostics(uuid),
    enabled: Boolean(org && uuid && canRead),
  });
}

function useAttachmentSrc(attachment: string | null | undefined) {
  const { org } = useInboxScope();
  return useQuery({
    queryKey: ["inbox", org, "attachment-src", attachment],
    queryFn: () => inboxService.attachmentSrc(attachment as string),
    enabled: Boolean(org && attachment),
    staleTime: SRC_STALE_MS,
    refetchInterval: SRC_STALE_MS,
  });
}

function issueCount(c: WidgetTechnicalContext | undefined) {
  return (c?.console_errors?.length ?? 0) + (c?.network_failures?.length ?? 0);
}

/**
 * Technical attachments a visitor sent from the web widget (CF-125):
 * masked screenshots, short screen recordings and the technical log
 * (console errors, failed requests, environment). Captures expire after 30
 * days; the API omits expired ones. Hidden when there are none.
 */
export function DiagnosticsPanel({
  conversationUuid,
}: {
  conversationUuid: string;
}) {
  const { t, locale } = useLocale();
  const list = useDiagnostics(conversationUuid);
  const [open, setOpen] = useState<WidgetDiagnostic | null>(null);
  const items = list.data ?? [];
  if (list.isLoading) {
    return <Skeleton className="h-16 w-full" />;
  }
  if (items.length === 0) return null;
  return (
    <section>
      <h3 className="text-muted-foreground mb-2 text-[11px] font-semibold tracking-wide uppercase">
        {t("inbox.diagnostics.title")}
      </h3>
      <ul className="flex flex-col gap-1.5">
        {items.map((d) => {
          const Icon = KIND_ICON[d.kind] ?? FileWarning;
          const issues = issueCount(d.context);
          return (
            <li key={d.uuid}>
              <button
                type="button"
                onClick={() => setOpen(d)}
                className="hover:bg-muted/60 focus-visible:ring-ring flex w-full items-center gap-2.5 rounded-md border px-2.5 py-2 text-left text-xs outline-none focus-visible:ring-2"
              >
                {d.kind === "screenshot" ? (
                  <Thumbnail attachment={d.attachment_id} />
                ) : (
                  <span className="bg-muted flex size-9 shrink-0 items-center justify-center rounded">
                    <Icon className="text-muted-foreground size-4" />
                  </span>
                )}
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium">
                    {t(`inbox.diagnostics.kind.${d.kind}`)}
                    {d.kind === "recording" && d.duration_ms > 0
                      ? ` · ${t("inbox.diagnostics.seconds", { count: Math.round(d.duration_ms / 1000) })}`
                      : ""}
                  </span>
                  <span className="text-muted-foreground block truncate">
                    {datetime(d.created_at, "dd.MM HH:mm", locale)}
                  </span>
                </span>
                {issues > 0 && (
                  <span
                    className="flex shrink-0 items-center gap-1 rounded bg-red-500/10 px-1.5 py-0.5 text-[10px] font-medium text-red-700 dark:text-red-300"
                    title={t("inbox.diagnostics.issues", { count: issues })}
                  >
                    <AlertTriangle className="size-3" />
                    {issues}
                  </span>
                )}
              </button>
            </li>
          );
        })}
      </ul>
      {open && <DiagnosticViewer item={open} onClose={() => setOpen(null)} />}
    </section>
  );
}

function Thumbnail({ attachment }: { attachment: string | null | undefined }) {
  const src = useAttachmentSrc(attachment);
  return (
    <span className="bg-muted flex size-9 shrink-0 items-center justify-center overflow-hidden rounded">
      {src.data ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img src={src.data} alt="" className="size-full object-cover" />
      ) : (
        <Camera className="text-muted-foreground size-4" />
      )}
    </span>
  );
}

/** Lightbox / player / log viewer of one capture. */
function DiagnosticViewer({
  item,
  onClose,
}: {
  item: WidgetDiagnostic;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const media = item.kind === "screenshot" || item.kind === "recording";
  const src = useAttachmentSrc(media ? item.attachment_id : null);
  return (
    <AppDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t(`inbox.diagnostics.kind.${item.kind}`)}
      description={t("inbox.diagnostics.captured", {
        at: datetime(item.created_at, "dd.MM.yyyy HH:mm", locale),
        expires: datetime(item.expires_at, "dd.MM.yyyy", locale),
      })}
      className="max-h-[92vh] overflow-y-auto sm:max-w-4xl"
    >
      <div className="space-y-4">
        {media &&
          (src.isError ? (
            <p className="text-destructive text-sm">
              {t("inbox.diagnostics.media_failed")}
            </p>
          ) : !src.data ? (
            <Skeleton className="aspect-video w-full" />
          ) : item.kind === "screenshot" ? (
            <div className="bg-muted/40 flex justify-center overflow-auto rounded-lg border">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={src.data}
                alt={t("inbox.diagnostics.kind.screenshot")}
                className="max-h-[65vh] w-auto object-contain"
              />
            </div>
          ) : (
            <video
              src={src.data}
              controls
              playsInline
              preload="metadata"
              className="max-h-[65vh] w-full rounded-lg border bg-black"
            >
              {t("inbox.diagnostics.video_unsupported")}
            </video>
          ))}
        {media && src.data && (
          <Button variant="outline" size="sm" asChild>
            <a href={src.data} target="_blank" rel="noopener noreferrer">
              <ExternalLink className="size-4" />
              {t("inbox.diagnostics.open_original")}
            </a>
          </Button>
        )}
        <TechnicalLog context={item.context} />
        <p className="text-muted-foreground text-xs">
          {t("inbox.diagnostics.masked_note")}
        </p>
      </div>
    </AppDialog>
  );
}

/** Structured technical log: environment, console errors, failed requests. */
function TechnicalLog({ context }: { context: WidgetTechnicalContext }) {
  const { t } = useLocale();
  const consoleErrors = context.console_errors ?? [];
  const network = context.network_failures ?? [];
  const metadata = Object.entries(context.metadata ?? {});
  const env = [
    { key: "browser", icon: Monitor, value: context.browser },
    { key: "os", icon: Monitor, value: context.os },
    { key: "url", icon: Globe, value: context.url },
  ].filter((e) => e.value);
  if (
    env.length === 0 &&
    consoleErrors.length === 0 &&
    network.length === 0 &&
    metadata.length === 0
  ) {
    return null;
  }
  return (
    <div className="grid gap-4">
      {env.length > 0 && (
        <LogSection title={t("inbox.diagnostics.environment")}>
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-xs">
            {env.map((e) => (
              <div key={e.key} className="contents">
                <dt className="text-muted-foreground flex items-center gap-1.5">
                  <e.icon className="size-3.5" />
                  {t(`inbox.diagnostics.env.${e.key}`)}
                </dt>
                <dd className="font-mono break-all">{e.value}</dd>
              </div>
            ))}
          </dl>
        </LogSection>
      )}
      <LogSection
        title={t("inbox.diagnostics.console", { count: consoleErrors.length })}
        tone={consoleErrors.length > 0 ? "error" : undefined}
      >
        {consoleErrors.length === 0 ? (
          <p className="text-muted-foreground text-xs">
            {t("inbox.diagnostics.none")}
          </p>
        ) : (
          <ol className="bg-muted/50 max-h-64 overflow-auto rounded-md border font-mono text-[11px]">
            {consoleErrors.map((line, i) => (
              <li
                key={i}
                className="border-b px-2.5 py-1.5 break-all whitespace-pre-wrap text-red-700 last:border-b-0 dark:text-red-300"
              >
                {line}
              </li>
            ))}
          </ol>
        )}
      </LogSection>
      <LogSection
        title={t("inbox.diagnostics.network", { count: network.length })}
        tone={network.length > 0 ? "error" : undefined}
      >
        {network.length === 0 ? (
          <p className="text-muted-foreground text-xs">
            {t("inbox.diagnostics.none")}
          </p>
        ) : (
          <div className="max-h-64 overflow-auto rounded-md border">
            <table className="w-full text-left text-[11px]">
              <thead className="bg-muted/60 text-muted-foreground sticky top-0">
                <tr>
                  <th className="px-2.5 py-1.5 font-medium">
                    {t("inbox.diagnostics.col.method")}
                  </th>
                  <th className="px-2.5 py-1.5 font-medium">
                    {t("inbox.diagnostics.col.status")}
                  </th>
                  <th className="px-2.5 py-1.5 font-medium">
                    {t("inbox.diagnostics.col.url")}
                  </th>
                  <th className="px-2.5 py-1.5 font-medium">
                    {t("inbox.diagnostics.col.error")}
                  </th>
                </tr>
              </thead>
              <tbody className="font-mono">
                {network.map((n, i) => (
                  <tr key={i} className="border-t align-top">
                    <td className="px-2.5 py-1.5">{n.method || "—"}</td>
                    <td
                      className={cn(
                        "px-2.5 py-1.5 tabular-nums",
                        (n.status ?? 0) >= 400 || !n.status
                          ? "text-red-700 dark:text-red-300"
                          : "",
                      )}
                    >
                      {n.status || "—"}
                    </td>
                    <td className="px-2.5 py-1.5 break-all">{n.url || "—"}</td>
                    <td className="px-2.5 py-1.5 break-all">
                      {n.error || "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </LogSection>
      {metadata.length > 0 && (
        <LogSection title={t("inbox.diagnostics.metadata")}>
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
            {metadata.map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="text-muted-foreground font-mono">{k}</dt>
                <dd className="font-mono break-all">
                  {typeof v === "string" ? v : JSON.stringify(v)}
                </dd>
              </div>
            ))}
          </dl>
        </LogSection>
      )}
    </div>
  );
}

function LogSection({
  title,
  tone,
  children,
}: {
  title: string;
  tone?: "error";
  children: ReactNode;
}) {
  return (
    <section className="space-y-2">
      <h4
        className={cn(
          "text-xs font-semibold",
          tone === "error" && "text-red-700 dark:text-red-300",
        )}
      >
        {title}
      </h4>
      {children}
    </section>
  );
}
