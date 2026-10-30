"use client";

import { ArrowLeft, ArrowRight, Bot, BookOpenCheck, X } from "lucide-react";
import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";

import { MarkdownContent } from "@/components/markdown/markdown-content";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { findGuideTarget, usePrefersReducedMotion } from "../lib/dom";
import type { GuideStep } from "../services/guides.service";

const CARD_WIDTH = 340;
const GAP = 12;
const PAD = 6;

type Rect = {
  top: number;
  left: number;
  width: number;
  height: number;
  radius: number;
};

type Props = {
  guideTitle: string;
  step: GuideStep;
  index: number;
  total: number;
  onNext: () => void;
  onBack: () => void;
  onSkip: () => void;
  onAsk: () => void;
  onNavigate: (route: string) => void;
};

function measure(el: Element | null): Rect | null {
  if (!el) return null;
  const r = el.getBoundingClientRect();
  if (r.width === 0 && r.height === 0) return null;
  return {
    top: r.top,
    left: r.left,
    width: r.width,
    height: r.height,
    radius: parseFloat(getComputedStyle(el).borderRadius) || 8,
  };
}

/** Card position next to the target, kept inside the viewport. */
function place(
  rect: Rect | null,
  placement: GuideStep["placement"],
  cardHeight: number,
): { top: number; left: number } {
  const vw = window.innerWidth;
  const vh = window.innerHeight;
  const width = Math.min(CARD_WIDTH, vw - 32);
  if (!rect) {
    return { top: Math.max(16, (vh - cardHeight) / 2), left: (vw - width) / 2 };
  }
  const fits = {
    bottom: rect.top + rect.height + GAP + cardHeight < vh,
    top: rect.top - GAP - cardHeight > 0,
    right: rect.left + rect.width + GAP + width < vw,
    left: rect.left - GAP - width > 0,
  };
  let side = placement === "auto" ? undefined : placement;
  if (!side || !fits[side]) {
    side = (["bottom", "top", "right", "left"] as const).find((s) => fits[s]);
  }
  let top: number;
  let left: number;
  switch (side) {
    case "top":
      top = rect.top - GAP - cardHeight;
      left = rect.left + rect.width / 2 - width / 2;
      break;
    case "right":
      top = rect.top + rect.height / 2 - cardHeight / 2;
      left = rect.left + rect.width + GAP;
      break;
    case "left":
      top = rect.top + rect.height / 2 - cardHeight / 2;
      left = rect.left - GAP - width;
      break;
    case "bottom":
      top = rect.top + rect.height + GAP;
      left = rect.left + rect.width / 2 - width / 2;
      break;
    default:
      // Target fills the screen: float the card at the bottom.
      top = vh - cardHeight - 16;
      left = (vw - width) / 2;
  }
  return {
    top: Math.min(Math.max(16, top), Math.max(16, vh - cardHeight - 16)),
    left: Math.min(Math.max(16, left), vw - width - 16),
  };
}

/**
 * One coach mark: dims the page, cuts a spotlight around the element with
 * `data-guide="<step.target>"` (or shows a centered card when there is no
 * target on the page) and renders the step card. Keyboard: → / Enter next,
 * ← back, Esc skip. Reduced motion turns transitions and smooth scrolling off.
 */
export function TourOverlay({
  guideTitle,
  step,
  index,
  total,
  onNext,
  onBack,
  onSkip,
  onAsk,
  onNavigate,
}: Props) {
  const { t } = useLocale();
  const reduced = usePrefersReducedMotion();
  const titleId = useId();
  const cardRef = useRef<HTMLDivElement>(null);
  // The overlay is keyed per step, so the first measure happens on mount.
  const [rect, setRect] = useState<Rect | null>(() =>
    measure(findGuideTarget(step.target)),
  );
  const [cardHeight, setCardHeight] = useState(220);

  const update = useCallback(() => {
    setRect(measure(findGuideTarget(step.target)));
    if (cardRef.current) setCardHeight(cardRef.current.offsetHeight);
  }, [step.target]);

  useLayoutEffect(() => {
    const el = findGuideTarget(step.target);
    el?.scrollIntoView({
      block: "center",
      inline: "nearest",
      behavior: reduced ? "auto" : "smooth",
    });
    let frame = requestAnimationFrame(update);
    const onChange = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(update);
    };
    window.addEventListener("resize", onChange);
    window.addEventListener("scroll", onChange, true);
    const observer =
      typeof ResizeObserver !== "undefined" && el
        ? new ResizeObserver(onChange)
        : null;
    if (el) observer?.observe(el);
    // Late-rendering pages: look for the target again shortly.
    const retry = window.setTimeout(onChange, 400);
    return () => {
      cancelAnimationFrame(frame);
      window.clearTimeout(retry);
      window.removeEventListener("resize", onChange);
      window.removeEventListener("scroll", onChange, true);
      observer?.disconnect();
    };
  }, [step.target, reduced, update]);

  useEffect(() => {
    const previous =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    cardRef.current?.focus({ preventScroll: true });
    return () => {
      if (previous?.isConnected) previous.focus({ preventScroll: true });
    };
  }, [step.id]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented) return;
      if (e.key === "Tab") {
        const card = cardRef.current;
        const controls = card?.querySelectorAll<HTMLElement>(
          'button:not(:disabled), a[href], [tabindex="0"]',
        );
        const first = controls?.[0];
        const last = controls?.[controls.length - 1];
        if (
          e.shiftKey &&
          (document.activeElement === first || document.activeElement === card)
        ) {
          e.preventDefault();
          last?.focus();
        } else if (
          !e.shiftKey &&
          (document.activeElement === last ||
            !card?.contains(document.activeElement))
        ) {
          e.preventDefault();
          first?.focus();
        }
        return;
      }
      const tag = (e.target as HTMLElement | null)?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA") return;
      if (e.key === "Escape") {
        e.preventDefault();
        onSkip();
      } else if (
        e.key === "ArrowRight" ||
        (e.key === "Enter" && e.target === cardRef.current)
      ) {
        e.preventDefault();
        onNext();
      } else if (e.key === "ArrowLeft" && index > 0) {
        e.preventDefault();
        onBack();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [index, onBack, onNext, onSkip]);

  if (typeof document === "undefined") return null;
  const pos = place(rect, step.placement, cardHeight);
  const last = index === total - 1;
  const navigate =
    step.action?.kind === "navigate" && step.action.route
      ? step.action.route
      : null;

  return createPortal(
    <div className="fixed inset-0 z-[90]" data-guide-overlay>
      <button
        type="button"
        className="absolute inset-0 size-full"
        aria-label={t("guides.tour.skip")}
        onClick={onSkip}
      />
      {rect ? (
        <div
          aria-hidden
          className={cn(
            "pointer-events-none fixed rounded-lg ring-2 ring-white/80",
            !reduced && "transition-all duration-200 ease-out",
          )}
          style={{
            top: rect.top - PAD,
            left: rect.left - PAD,
            width: rect.width + PAD * 2,
            height: rect.height + PAD * 2,
            borderRadius: rect.radius + PAD,
            boxShadow: "0 0 0 9999px rgb(0 0 0 / 0.55)",
          }}
        />
      ) : (
        <div
          aria-hidden
          className="pointer-events-none fixed inset-0 bg-black/55"
        />
      )}
      <div
        ref={cardRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className={cn(
          "bg-popover text-popover-foreground fixed max-h-[calc(100dvh-2rem)] overflow-y-auto rounded-2xl border p-5 shadow-2xl outline-none",
          !reduced && "transition-[top,left] duration-200 ease-out",
        )}
        style={{
          top: pos.top,
          left: pos.left,
          width: Math.min(CARD_WIDTH, window.innerWidth - 32),
        }}
      >
        <div className="mb-3 flex items-start justify-between gap-2">
          <BookOpenCheck aria-hidden className="text-primary size-5 shrink-0" />
          <p className="text-muted-foreground text-[11px] font-semibold tracking-wide uppercase">
            {guideTitle}
          </p>
          <button
            type="button"
            onClick={onSkip}
            className="text-muted-foreground hover:text-foreground focus-visible:ring-ring -mt-1 -mr-1 rounded-md p-1.5 focus-visible:ring-2 focus-visible:outline-none"
            aria-label={t("guides.tour.skip")}
          >
            <X className="size-4" />
          </button>
        </div>
        <h2 id={titleId} className="text-base font-semibold">
          {step.title}
        </h2>
        {step.body ? (
          <MarkdownContent
            markdown={step.body}
            className="text-muted-foreground mt-1.5 text-sm [&_p]:my-1"
          />
        ) : null}
        <p
          aria-live="polite"
          className="text-muted-foreground mt-4 text-xs tabular-nums"
        >
          {t("guides.tour.progress", { current: index + 1, total })}
        </p>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <Button size="sm" variant="ghost" onClick={onAsk}>
            <Bot className="size-4" />
            {t("guides.tour.ask")}
          </Button>
          <div className="ms-auto flex items-center gap-1">
            {index > 0 ? (
              <Button
                size="sm"
                variant="outline"
                onClick={onBack}
                aria-label={t("guides.tour.back")}
              >
                <ArrowLeft className="size-4" />
              </Button>
            ) : (
              <Button size="sm" variant="ghost" onClick={onSkip}>
                {t("guides.tour.skip")}
              </Button>
            )}
            {navigate ? (
              <Button
                size="sm"
                variant="outline"
                onClick={() => onNavigate(navigate)}
              >
                {t("guides.tour.go")}
              </Button>
            ) : null}
            <Button size="sm" onClick={onNext}>
              {last ? t("guides.tour.done") : t("guides.tour.next")}
              {last ? null : <ArrowRight className="size-4" />}
            </Button>
          </div>
        </div>
      </div>
    </div>,
    document.body,
  );
}
