"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CircleAlert,
  CloudCheck,
  History,
  Loader2,
  RotateCcw,
  Rocket,
  Search,
} from "lucide-react";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type MutableRefObject,
} from "react";

import { Badge } from "@/components/ui/badge";
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
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { BrandSelect } from "@/features/brands/components/brand-select";
import { EditorProvider, issueText } from "@/features/widget/components/fields";
import {
  AppearanceSection,
  LauncherSection,
} from "@/features/widget/components/sections-design";
import {
  HomeSection,
  PreChatSection,
  TextsSection,
} from "@/features/widget/components/sections-content";
import {
  AISection,
  ChannelsSection,
  ConversationSection,
  HoursSection,
  TargetingSection,
  TriggersSection,
} from "@/features/widget/components/sections-behaviour";
import {
  InstallSection,
  PrivacySection,
  ReplaySection,
  SecuritySection,
} from "@/features/widget/components/sections-security";
import { VersionHistory } from "@/features/widget/components/version-history";
import { WidgetPreview } from "@/features/widget/components/widget-preview";
import {
  fingerprint,
  publishable,
  quickChecks,
  readSettings,
  searchSections,
  sectionIssues,
  SECTIONS,
  setPath,
  type Issue,
  type SectionGroup,
  type SectionId,
  type WidgetSettings,
} from "@/features/widget/lib/settings";
import {
  widgetConfigService,
  type WidgetConfig,
  type WidgetInstance,
} from "@/features/widget/services/widget.service";
import { useUnsavedChangesGuard } from "@/hooks/use-unsaved-changes-guard";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";
import { useDialogs } from "@/providers/dialog-provider";

export type InstancePatch = {
  name?: string;
  status?: "active" | "paused";
  brandId?: string | null;
  identitySecret?: string;
};

type Props = {
  widget: WidgetInstance;
  canManage: boolean;
  canPublish: boolean;
  onInstanceSave: (patch: InstancePatch) => Promise<void>;
  /** The admin page flushes the draft before switching widgets. */
  flushRef?: MutableRefObject<(() => Promise<void>) | null>;
};

const GROUPS: SectionGroup[] = ["design", "content", "behaviour", "security"];
const AUTOSAVE_MS = 1200;

export function WidgetEditor(props: Props) {
  const { t } = useLocale();
  const config = useQuery({
    queryKey: ["widget-config", props.widget.uuid],
    queryFn: () => widgetConfigService.config(props.widget.uuid),
    // Only explicit invalidations (publish, discard, rollback) reload it,
    // never a window focus in the middle of an edit.
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });
  if (config.isLoading || !config.data)
    return (
      <div className="text-muted-foreground flex items-center gap-2 p-6 text-sm">
        <Loader2 className="size-4 animate-spin" />
        {config.isError ? t("widget.error.description") : t("common.loading")}
      </div>
    );
  return (
    <Editor
      key={`${props.widget.uuid}:${config.data.version}:${config.data.draft ? "draft" : "live"}`}
      {...props}
      config={config.data}
    />
  );
}

function Editor({
  widget,
  canManage,
  canPublish,
  onInstanceSave,
  flushRef,
  config,
}: Props & { config: WidgetConfig }) {
  const { confirm } = useDialogs();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const published = useMemo(
    () => readSettings(config.published),
    [config.published],
  );
  const [settings, setSettings] = useState<WidgetSettings>(() =>
    readSettings(config.draft?.settings ?? config.published),
  );
  const [serverIssues, setServerIssues] = useState<Issue[]>(
    () => (config.draft?.issues ?? []) as Issue[],
  );
  const [saved, setSaved] = useState(() =>
    fingerprint(
      config.draft?.settings ? readSettings(config.draft.settings) : published,
    ),
  );
  const [saveState, setSaveState] = useState<
    "idle" | "saving" | "saved" | "error"
  >(config.draft ? "saved" : "idle");
  const [savedAt, setSavedAt] = useState<string | null>(
    config.draft?.updated_at ?? null,
  );
  const [section, setSection] = useState<SectionId>("appearance");
  const [query, setQuery] = useState("");
  const [publishOpen, setPublishOpen] = useState(false);
  const [note, setNote] = useState("");
  const [historyOpen, setHistoryOpen] = useState(false);
  const [previewOf, setPreviewOf] = useState<{
    version: number;
    settings: WidgetSettings;
  } | null>(null);
  const [name, setName] = useState(widget.name);
  const disabled = !canManage;

  const current = fingerprint(settings);
  const dirty = current !== saved;
  const unpublished = current !== fingerprint(published);
  const issues = useMemo(() => {
    // Server issues of the last saved draft, unless the field changed since.
    const quick = quickChecks(settings);
    const server = dirty
      ? serverIssues.filter((i) => !quick.some((q) => q.path === i.path))
      : serverIssues;
    return [...quick, ...server];
  }, [settings, serverIssues, dirty]);
  const canPublishNow =
    canPublish && publishable(issues) && (unpublished || Boolean(config.draft));

  const set = useCallback(
    (path: string, value: unknown) =>
      setSettings((s) => setPath(s, path, value)),
    [],
  );

  const save = useMutation({
    mutationFn: (s: WidgetSettings) =>
      widgetConfigService.saveDraft(
        widget.uuid,
        s as unknown as Record<string, unknown>,
        config.version,
      ),
    onMutate: () => setSaveState("saving"),
    onSuccess: (draft, s) => {
      setServerIssues((draft.issues ?? []) as Issue[]);
      setSaved(fingerprint(s));
      setSavedAt(draft.updated_at);
      setSaveState("saved");
    },
    onError: () => setSaveState("error"),
  });

  const latest = useRef(settings);
  useEffect(() => {
    latest.current = settings;
  }, [settings]);
  const flush = useCallback(async () => {
    if (!canManage || fingerprint(latest.current) === saved) return;
    await save.mutateAsync(latest.current).catch(() => undefined);
  }, [canManage, saved, save]);

  // Autosave the draft shortly after the last change.
  useEffect(() => {
    if (!dirty || !canManage) return;
    const id = window.setTimeout(
      () => save.mutate(latest.current),
      AUTOSAVE_MS,
    );
    return () => window.clearTimeout(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current, dirty, canManage]);

  useEffect(() => {
    if (flushRef) flushRef.current = flush;
    return () => {
      if (flushRef) flushRef.current = null;
    };
  }, [flush, flushRef]);

  // Unsaved-change guard while an edit waits for the autosave.
  useUnsavedChangesGuard(dirty || saveState === "saving");

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ["widget-config", widget.uuid] });

  const publish = useMutation({
    mutationFn: () =>
      widgetConfigService.publish(widget.uuid, {
        settings: settings as unknown as Record<string, unknown>,
        base_version: config.version,
        note,
      }),
    onSuccess: async (v) => {
      setPublishOpen(false);
      setNote("");
      appToast.success(t("widget.toast.published", { version: v.version }));
      await queryClient.invalidateQueries({
        queryKey: ["widget-versions", widget.uuid],
      });
      await refresh();
    },
    onError: (err) => {
      if (isApiError(err) && err.code === "WIDGET_SETTINGS_INVALID") {
        setServerIssues(
          err.details.map((d) => ({
            path: d.field ?? "",
            code: d.code ?? "invalid",
            message: d.message ?? "",
          })),
        );
        setPublishOpen(false);
        appToast.error(t("widget.toast.invalid"));
      } else if (isApiError(err) && err.code === "WIDGET_VERSION_CONFLICT") {
        appToast.error(t("widget.toast.conflict"));
        void refresh();
      } else
        appToast.error(
          isApiError(err) ? err.message : t("widget.toast.failed"),
        );
    },
  });

  const discard = useMutation({
    mutationFn: () => widgetConfigService.discardDraft(widget.uuid),
    onSuccess: async () => {
      appToast.success(t("widget.toast.discarded"));
      await refresh();
    },
  });

  const titles = Object.fromEntries(
    SECTIONS.map((s) => [s.id, t(`widget.section.${s.id}`)]),
  ) as Record<SectionId, string>;
  const visibleSections = searchSections(query, titles);
  const errorCount = issues.filter(
    (i) => (i.level ?? "error") === "error",
  ).length;

  const body = (() => {
    switch (section) {
      case "appearance":
        return <AppearanceSection />;
      case "launcher":
        return <LauncherSection />;
      case "texts":
        return <TextsSection />;
      case "home":
        return <HomeSection />;
      case "prechat":
        return <PreChatSection />;
      case "conversation":
        return <ConversationSection />;
      case "channels":
        return <ChannelsSection />;
      case "ai":
        return <AISection />;
      case "triggers":
        return <TriggersSection />;
      case "targeting":
        return <TargetingSection />;
      case "hours":
        return <HoursSection />;
      case "privacy":
        return <PrivacySection />;
      case "security":
        return (
          <SecuritySection
            widgetId={widget.uuid}
            canManage={canManage}
            secretHint={widget.credential_hints?.identity_secret}
            onSaveSecret={(secret) =>
              onInstanceSave({ identitySecret: secret })
            }
          />
        );
      case "replay":
        return <ReplaySection />;
      case "install":
        return (
          <InstallSection
            widgetId={widget.uuid}
            hasSecret={Boolean(widget.credential_hints?.identity_secret)}
            onOpenSecurity={() => setSection("security")}
          />
        );
    }
  })();

  return (
    <EditorProvider value={{ settings, set, issues, disabled }}>
      <div className="space-y-4">
        {/* Top bar: identity of the widget and the publish flow. */}
        <div className="flex flex-wrap items-center gap-3 rounded-xl border p-3">
          <Input
            value={name}
            maxLength={120}
            aria-label={t("widget.fields.name")}
            className="w-56 font-medium"
            disabled={disabled}
            onChange={(e) => setName(e.target.value)}
            onBlur={() => {
              if (name.trim() && name.trim() !== widget.name)
                void onInstanceSave({ name: name.trim() });
            }}
          />
          <label className="flex items-center gap-2 text-sm">
            <Switch
              checked={widget.status !== "paused"}
              disabled={disabled}
              onCheckedChange={(v) =>
                void onInstanceSave({ status: v ? "active" : "paused" })
              }
            />
            {t("widget.fields.active")}
          </label>
          <div className="w-52">
            <BrandSelect
              id={`widget-brand-${widget.uuid}`}
              value={widget.brand_id ?? null}
              onChange={(b) => void onInstanceSave({ brandId: b })}
              disabled={disabled}
            />
          </div>
          <span className="text-muted-foreground font-mono text-xs">
            {widget.uuid}
          </span>
          <div className="ms-auto flex flex-wrap items-center gap-2">
            <SaveStatus
              state={dirty && saveState !== "saving" ? "idle" : saveState}
              savedAt={savedAt}
              dirty={dirty}
            />
            <Badge
              variant={unpublished ? "outline" : "secondary"}
              data-testid="publish-state"
            >
              {unpublished
                ? t("widget.editor.unpublished")
                : t("widget.editor.live", { version: config.version })}
            </Badge>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setHistoryOpen(true)}
            >
              <History className="size-4" />
              {t("widget.editor.history")}
            </Button>
            {canManage && config.draft ? (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={discard.isPending}
                onClick={async () => {
                  if (
                    await confirm({
                      title: t("widget.editor.discard_confirm"),
                      variant: "destructive",
                    })
                  )
                    discard.mutate();
                }}
              >
                <RotateCcw className="size-4" />
                {t("widget.editor.discard")}
              </Button>
            ) : null}
            {canPublish ? (
              <Button
                type="button"
                size="sm"
                disabled={!canPublishNow || publish.isPending}
                onClick={() => setPublishOpen(true)}
              >
                <Rocket className="size-4" />
                {t("widget.editor.publish")}
              </Button>
            ) : null}
          </div>
        </div>

        {errorCount > 0 ? (
          <div
            role="alert"
            className="border-destructive/40 bg-destructive/5 text-destructive flex items-center gap-2 rounded-lg border px-3 py-2 text-sm"
          >
            <CircleAlert className="size-4" />
            {t("widget.editor.issues", { count: errorCount })}
          </div>
        ) : null}

        <div className="grid gap-4 xl:grid-cols-[220px_minmax(0,1fr)_minmax(380px,0.9fr)]">
          {/* Section navigation with search. */}
          <nav aria-label={t("widget.editor.sections")} className="space-y-3">
            <div className="relative">
              <Search className="text-muted-foreground absolute start-2.5 top-2.5 size-4" />
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={t("widget.editor.search")}
                aria-label={t("widget.editor.search")}
                className="ps-8"
              />
            </div>
            {GROUPS.map((g) => {
              const items = SECTIONS.filter(
                (s) => s.group === g && visibleSections.includes(s.id),
              );
              if (!items.length) return null;
              return (
                <div key={g} className="space-y-1">
                  <p className="text-muted-foreground px-2 text-[11px] font-semibold tracking-wide uppercase">
                    {t(`widget.group.${g}`)}
                  </p>
                  {items.map((s) => {
                    const count = sectionIssues(issues, s.id);
                    return (
                      <button
                        key={s.id}
                        type="button"
                        aria-current={section === s.id ? "page" : undefined}
                        onClick={() => setSection(s.id)}
                        className={cn(
                          "hover:bg-accent flex w-full items-center justify-between rounded-md px-2 py-1.5 text-start text-sm",
                          section === s.id && "bg-accent font-medium",
                        )}
                      >
                        {titles[s.id]}
                        {count ? (
                          <span className="bg-destructive text-destructive-foreground rounded-full px-1.5 text-[10px] font-semibold">
                            {count}
                          </span>
                        ) : null}
                      </button>
                    );
                  })}
                </div>
              );
            })}
            {visibleSections.length === 0 ? (
              <p className="text-muted-foreground px-2 text-sm">
                {t("widget.editor.no_match")}
              </p>
            ) : null}
          </nav>

          <div className="min-w-0 space-y-4" data-testid={`section-${section}`}>
            <div>
              <h2 className="text-lg font-semibold">{titles[section]}</h2>
              <p className="text-muted-foreground text-sm">
                {t(`widget.section_hint.${section}`)}
              </p>
            </div>
            {body}
          </div>

          <div className="xl:sticky xl:top-4 xl:self-start">
            <WidgetPreview
              settings={previewOf?.settings ?? settings}
              name={name || widget.name}
              label={
                previewOf
                  ? t("widget.editor.previewing_version", {
                      version: previewOf.version,
                    })
                  : undefined
              }
            />
          </div>
        </div>
      </div>

      <Dialog open={publishOpen} onOpenChange={setPublishOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("widget.editor.publish_title")}</DialogTitle>
            <DialogDescription>
              {t("widget.editor.publish_hint", { version: config.version + 1 })}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="widget-publish-note">
              {t("widget.editor.note")}
            </Label>
            <Textarea
              id="widget-publish-note"
              value={note}
              maxLength={200}
              onChange={(e) => setNote(e.target.value)}
            />
            {issues
              .filter((i) => i.level === "warning")
              .slice(0, 3)
              .map((i, n) => (
                <p key={n} className="text-xs text-amber-600">
                  {issueText(t, i)} ({i.path})
                </p>
              ))}
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setPublishOpen(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="button"
              disabled={publish.isPending}
              onClick={() => publish.mutate()}
            >
              {publish.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Rocket className="size-4" />
              )}
              {t("widget.editor.publish_now")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <VersionHistory
        open={historyOpen}
        onOpenChange={setHistoryOpen}
        widgetId={widget.uuid}
        current={config.version}
        canPublish={canPublish}
        onPreview={(version, raw) => {
          setPreviewOf(raw ? { version, settings: readSettings(raw) } : null);
        }}
        onRolledBack={async () => {
          setPreviewOf(null);
          await refresh();
        }}
      />
    </EditorProvider>
  );
}

function SaveStatus({
  state,
  savedAt,
  dirty,
}: {
  state: "idle" | "saving" | "saved" | "error";
  savedAt: string | null;
  dirty: boolean;
}) {
  const { t, locale } = useLocale();
  const time = savedAt
    ? new Date(savedAt).toLocaleTimeString(
        locale === "tr" ? "tr-TR" : "en-US",
        { hour: "2-digit", minute: "2-digit" },
      )
    : "";
  return (
    <span
      className="text-muted-foreground flex items-center gap-1 text-xs"
      aria-live="polite"
      data-testid="save-status"
    >
      {state === "saving" ? (
        <>
          <Loader2 className="size-3.5 animate-spin" />
          {t("widget.editor.saving")}
        </>
      ) : state === "error" ? (
        <span className="text-destructive">
          {t("widget.editor.save_failed")}
        </span>
      ) : dirty ? (
        t("widget.editor.pending")
      ) : state === "saved" ? (
        <>
          <CloudCheck className="size-3.5" />
          {t("widget.editor.saved", { time })}
        </>
      ) : null}
    </span>
  );
}
