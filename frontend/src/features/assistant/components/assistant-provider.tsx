"use client";

import { AssistantRuntimeProvider } from "@assistant-ui/react";
import { useQueryClient } from "@tanstack/react-query";
import { usePathname } from "next/navigation";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { permissions } from "@/config/permissions";
import { useOptionalTenant } from "@/features/tenant";
import { useIsMobile } from "@/hooks/use-mobile";
import { useLocalStorage } from "@/hooks/use-local-storage";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { PendingMessages } from "../lib/queue";
import {
  AssistantSurfaceContext,
  type AssistantSurface,
} from "../runtime/surface";
import { useAssistantRuntime } from "../runtime/use-assistant-runtime";

import { AssistantSheet } from "./assistant-sheet";
import { FocusHighlighter } from "./focus-highlighter";
import { SupportWidget } from "./support-widget";

/** Where the panel lives: an overlay sheet or a docked column beside the page. */
export type AssistantMode = "drawer" | "docked";

type AssistantContextValue = {
  open: boolean;
  setOpen: (open: boolean) => void;
  toggle: () => void;
  /** Opens the panel; with a prompt, sends it as the user's question. */
  ask: (prompt?: string, guideKey?: string) => void;
  /** The member's choice; `docked` falls back to the sheet on small screens. */
  mode: AssistantMode;
  setMode: (mode: AssistantMode) => void;
  /** The mode in effect for this viewport. */
  effectiveMode: AssistantMode;
};

const AssistantContext = createContext<AssistantContextValue | null>(null);

/**
 * ClickFix Asistan of the workspace shell (CF-141, CF-200): one assistant-ui
 * runtime for the organization, shared by the sheet (header button or
 * Cmd/Ctrl + J), the docked split view and the `/assistant` page. Members
 * without `tenant.assistant.use` get no panel.
 */
export function AssistantProvider({ children }: { children: ReactNode }) {
  const allowed = usePermission().can(permissions.assistant.use);
  if (!allowed) return <>{children}</>;
  return <Provider>{children}</Provider>;
}

function Provider({ children }: { children: ReactNode }) {
  const { t, locale } = useLocale();
  const slug = useOptionalTenant()?.slug ?? "";
  const pathname = usePathname() || "/";
  const qc = useQueryClient();
  const mobile = useIsMobile();
  const [open, setOpen] = useState(false);
  const [mode, setMode] = useLocalStorage<AssistantMode>(
    "assistant.mode",
    "drawer",
  );
  const [guideKey, setGuideKey] = useState<string | undefined>();
  const effectiveMode: AssistantMode =
    mode === "docked" && !mobile ? "docked" : "drawer";

  const refreshDocuments = useCallback(
    () =>
      void qc.invalidateQueries({ queryKey: ["assistant", slug, "documents"] }),
    [qc, slug],
  );
  const { runtime, decisions, ensureThread } = useAssistantRuntime({
    slug,
    locale,
    route: pathname,
    guideKey,
    onDocumentUploaded: refreshDocuments,
    translate: t,
  });
  const [queue] = useState(() => new PendingMessages());
  const surface = useMemo<AssistantSurface>(
    () => ({
      slug,
      locale,
      compact: false,
      decisions,
      queue,
      ensureThread,
      refreshDocuments,
    }),
    [slug, locale, decisions, queue, ensureThread, refreshDocuments],
  );

  const toggle = useCallback(() => setOpen((o) => !o), []);
  // A question from a tour step or the "?" popover opens a fresh thread.
  const pendingPrompt = useRef<string | undefined>(undefined);
  const ask = useCallback(
    (prompt?: string, guide?: string) => {
      setGuideKey(guide);
      pendingPrompt.current = prompt;
      setOpen(true);
      void (async () => {
        await runtime.threads.switchToNewThread();
        const text = pendingPrompt.current;
        pendingPrompt.current = undefined;
        if (text) runtime.thread.append(text);
      })();
    },
    [runtime],
  );

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "j") {
        e.preventDefault();
        setOpen((o) => !o);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // Closing the panel stops a running answer (the API keeps the partial text).
  useEffect(() => {
    if (open) return;
    if (runtime.thread.getState().isRunning) runtime.thread.cancelRun();
  }, [open, runtime]);

  const value = useMemo<AssistantContextValue>(
    () => ({ open, setOpen, toggle, ask, mode, setMode, effectiveMode }),
    [ask, effectiveMode, mode, open, setMode, toggle],
  );

  return (
    <AssistantContext.Provider value={value}>
      <AssistantRuntimeProvider runtime={runtime}>
        <AssistantSurfaceContext.Provider value={surface}>
          {children}
          <FocusHighlighter />
          <SupportWidget />
          {effectiveMode === "drawer" ? (
            <AssistantSheet open={open} onOpenChange={setOpen} />
          ) : null}
        </AssistantSurfaceContext.Provider>
      </AssistantRuntimeProvider>
    </AssistantContext.Provider>
  );
}

/** Assistant context, or null when the member has no assistant. */
export function useOptionalAssistant(): AssistantContextValue | null {
  return useContext(AssistantContext);
}
