"use client";

import { ArrowRight, Bot, Check, Globe, Loader2, Plus } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";

import { BrandLogo, isBrandSlug } from "@/components/brand-logos";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { BRAND_EASE, usePrefersReducedMotion } from "@/lib/motion";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import type { SetupAnswers } from "../../services/setup.service";

type Industry = NonNullable<SetupAnswers["industry"]>;
type Channel = NonNullable<SetupAnswers["channels"]>[number];

const INDUSTRIES: Industry[] = [
  "saas",
  "ecommerce",
  "agency",
  "it_helpdesk",
  "fintech",
  "education",
  "healthcare",
  "field_service",
  "other",
];
const TEAM_SIZES = ["1", "2-5", "6-20", "21-50", "50+"] as const;
const CHANNELS: Channel[] = [
  "widget",
  "email",
  "whatsapp",
  "instagram",
  "messenger",
  "telegram",
  "voice",
];
const HOURS = ["weekdays_9_18", "weekdays_8_20", "extended", "24_7"] as const;
const LANGUAGES = ["tr", "en", "de", "ar", "ru", "fr"] as const;
const TONES = ["friendly", "formal", "neutral"] as const;

/** Request-type suggestions per industry (copy keys). */
const REQUEST_HINTS: Record<Industry, string[]> = {
  saas: ["bug", "account", "billing", "integration", "feature"],
  ecommerce: ["order", "shipping", "return", "payment", "stock"],
  agency: ["revision", "new_project", "maintenance", "billing"],
  it_helpdesk: ["access", "password", "hardware", "software", "network"],
  fintech: ["dispute", "kyc", "card", "transfer", "fraud"],
  education: ["enrolment", "payment", "course_access", "certificate"],
  healthcare: ["appointment", "results", "insurance", "billing"],
  field_service: ["fault", "installation", "maintenance", "warranty"],
  other: ["question", "complaint", "billing", "account"],
};

type QuestionKey =
  | "website"
  | "industry"
  | "team_size"
  | "channels"
  | "support_hours"
  | "languages"
  | "request_types"
  | "tone";

const QUESTIONS: QuestionKey[] = [
  "website",
  "industry",
  "team_size",
  "channels",
  "support_hours",
  "languages",
  "request_types",
  "tone",
];

export type DiscoveryResult = { website: string; answers: SetupAnswers };

function Bubble({
  from,
  children,
}: {
  from: "bot" | "user";
  children: ReactNode;
}) {
  const reduced = usePrefersReducedMotion();
  return (
    <motion.div
      initial={reduced ? false : { opacity: 0, y: 10, scale: 0.98 }}
      animate={{ opacity: 1, y: 0, scale: 1 }}
      transition={{ duration: 0.35, ease: BRAND_EASE }}
      className={cn(
        "flex items-end gap-2",
        from === "user" && "flex-row-reverse",
      )}
    >
      {from === "bot" ? (
        <span
          className="grid size-8 shrink-0 place-items-center rounded-full text-white"
          style={{ background: "var(--setup-accent)" }}
          aria-hidden
        >
          <Bot className="size-4" />
        </span>
      ) : null}
      <div
        className={cn(
          "max-w-[85%] rounded-2xl px-4 py-2.5 text-sm leading-relaxed",
          from === "bot"
            ? "bg-muted rounded-bl-sm"
            : "rounded-br-sm text-white",
        )}
        style={
          from === "user" ? { background: "var(--setup-accent)" } : undefined
        }
      >
        {children}
      </div>
    </motion.div>
  );
}

function Typing() {
  return (
    <div className="flex items-end gap-2" aria-hidden>
      <span className="bg-muted grid size-8 place-items-center rounded-full">
        <Bot className="text-muted-foreground size-4" />
      </span>
      <div className="bg-muted flex gap-1 rounded-2xl rounded-bl-sm px-4 py-3">
        {[0, 1, 2].map((i) => (
          <motion.span
            key={i}
            className="bg-muted-foreground/60 size-1.5 rounded-full"
            animate={{ y: [0, -3, 0] }}
            transition={{ duration: 0.8, repeat: Infinity, delay: i * 0.15 }}
          />
        ))}
      </div>
    </div>
  );
}

function Chip({
  selected,
  onClick,
  children,
}: {
  selected?: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={selected}
      className={cn(
        "focus-visible:ring-ring/50 inline-flex items-center gap-1.5 rounded-full border px-3.5 py-1.5 text-sm transition-all outline-none focus-visible:ring-[3px]",
        selected
          ? "border-transparent text-white shadow-sm"
          : "bg-card hover:bg-muted",
      )}
      style={selected ? { background: "var(--setup-accent)" } : undefined}
    >
      {selected ? <Check className="size-3.5" aria-hidden /> : null}
      {children}
    </button>
  );
}

/**
 * The discovery questionnaire as a short chat: the assistant asks one
 * question at a time (typing indicator, then the bubble), the owner
 * answers with chips or a field, and every answer stays in the thread.
 */
export function DiscoveryChat({
  companyName,
  onDone,
  busy,
}: {
  companyName: string;
  onDone: (result: DiscoveryResult) => void;
  busy: boolean;
}) {
  const { t, locale } = useLocale();
  const reduced = usePrefersReducedMotion();
  const [step, setStep] = useState(0);
  const [typing, setTyping] = useState(false);
  const [website, setWebsite] = useState("");
  const [answers, setAnswers] = useState<SetupAnswers>({
    company_name: companyName,
    channels: [],
    languages: [locale === "en" ? "en" : "tr"],
    request_types: [],
  });
  const [custom, setCustom] = useState("");
  const endRef = useRef<HTMLDivElement>(null);
  const question = QUESTIONS[step];

  useEffect(() => {
    endRef.current?.scrollIntoView({
      behavior: reduced ? "auto" : "smooth",
      block: "end",
    });
  }, [step, typing, reduced]);

  const next = () => {
    if (step >= QUESTIONS.length - 1) return;
    if (reduced) {
      setStep((s) => s + 1);
      return;
    }
    setTyping(true);
    window.setTimeout(() => {
      setTyping(false);
      setStep((s) => s + 1);
    }, 550);
  };

  const set = <K extends keyof SetupAnswers>(key: K, value: SetupAnswers[K]) =>
    setAnswers((a) => ({ ...a, [key]: value }));

  const toggle = (key: "channels" | "languages" | "request_types", v: string) =>
    setAnswers((a) => {
      const list = (a[key] ?? []) as string[];
      return {
        ...a,
        [key]: list.includes(v) ? list.filter((x) => x !== v) : [...list, v],
      };
    });

  const finish = (tone: (typeof TONES)[number]) => {
    const final = { ...answers, tone };
    setAnswers(final);
    onDone({ website: website.trim(), answers: final });
  };

  const hints = useMemo(
    () => REQUEST_HINTS[answers.industry ?? "other"],
    [answers.industry],
  );

  const answered = (key: QuestionKey): ReactNode => {
    switch (key) {
      case "website":
        return website.trim() || t("onboarding.setup.chat.no_website");
      case "industry":
        return t(`onboarding.setup.industries.${answers.industry}`);
      case "team_size":
        return t("onboarding.setup.chat.team_size_value", {
          size: answers.team_size ?? "",
        });
      case "channels":
        return (answers.channels ?? [])
          .map((c) => t(`onboarding.setup.channels.${c}`))
          .join(", ");
      case "support_hours":
        return t(`onboarding.setup.hours.${answers.support_hours}`);
      case "languages":
        return (answers.languages ?? [])
          .map((l) => t(`onboarding.setup.languages.${l}`))
          .join(", ");
      case "request_types":
        return (answers.request_types ?? []).join(", ");
      case "tone":
        return t(`onboarding.setup.tones.${answers.tone}`);
    }
  };

  const submitWebsite = (event: FormEvent) => {
    event.preventDefault();
    next();
  };

  const addCustom = (event: FormEvent) => {
    event.preventDefault();
    const v = custom.trim();
    if (v && !(answers.request_types ?? []).includes(v)) {
      toggle("request_types", v);
    }
    setCustom("");
  };

  return (
    <div className="flex flex-col gap-4" aria-live="polite">
      <Bubble from="bot">
        {t("onboarding.setup.chat.hello", { name: companyName })}
      </Bubble>
      {QUESTIONS.slice(0, step + 1).map((key, index) => (
        <div key={key} className="flex flex-col gap-3">
          <Bubble from="bot">{t(`onboarding.setup.chat.q_${key}`)}</Bubble>
          {index < step || (busy && key === "tone") ? (
            <Bubble from="user">{answered(key)}</Bubble>
          ) : null}
        </div>
      ))}
      {typing ? <Typing /> : null}

      <AnimatePresence mode="wait" initial={false}>
        {!typing && !busy ? (
          <motion.div
            key={question}
            initial={reduced ? false : { opacity: 0, y: 8 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -4 }}
            transition={{ duration: 0.25, ease: BRAND_EASE }}
            className="flex flex-wrap gap-2 pl-10"
          >
            {question === "website" ? (
              <form onSubmit={submitWebsite} className="flex w-full gap-2">
                <div className="relative flex-1">
                  <Globe
                    className="text-muted-foreground absolute top-1/2 left-3 size-4 -translate-y-1/2"
                    aria-hidden
                  />
                  <Input
                    autoFocus
                    inputMode="url"
                    autoComplete="url"
                    aria-label={t("onboarding.setup.chat.website_label")}
                    placeholder="ornek.com.tr"
                    className="pl-9"
                    value={website}
                    maxLength={300}
                    onChange={(e) => setWebsite(e.target.value)}
                  />
                </div>
                <Button type="submit">
                  {website.trim()
                    ? t("onboarding.setup.chat.continue")
                    : t("onboarding.setup.chat.skip")}
                  <ArrowRight />
                </Button>
              </form>
            ) : null}
            {question === "industry"
              ? INDUSTRIES.map((v) => (
                  <Chip
                    key={v}
                    selected={answers.industry === v}
                    onClick={() => {
                      set("industry", v);
                      next();
                    }}
                  >
                    {t(`onboarding.setup.industries.${v}`)}
                  </Chip>
                ))
              : null}
            {question === "team_size"
              ? TEAM_SIZES.map((v) => (
                  <Chip
                    key={v}
                    selected={answers.team_size === v}
                    onClick={() => {
                      set("team_size", v);
                      next();
                    }}
                  >
                    {t("onboarding.setup.chat.team_size_value", { size: v })}
                  </Chip>
                ))
              : null}
            {question === "channels" ? (
              <>
                {CHANNELS.map((v) => (
                  <Chip
                    key={v}
                    selected={answers.channels?.includes(v)}
                    onClick={() => toggle("channels", v)}
                  >
                    {isBrandSlug(v) ? (
                      <BrandLogo slug={v} className="size-3.5" decorative />
                    ) : null}
                    {t(`onboarding.setup.channels.${v}`)}
                  </Chip>
                ))}
                <Button
                  size="sm"
                  className="rounded-full"
                  disabled={!answers.channels?.length}
                  onClick={next}
                >
                  {t("onboarding.setup.chat.continue")}
                  <ArrowRight />
                </Button>
              </>
            ) : null}
            {question === "support_hours"
              ? HOURS.map((v) => (
                  <Chip
                    key={v}
                    selected={answers.support_hours === v}
                    onClick={() => {
                      set("support_hours", v);
                      next();
                    }}
                  >
                    {t(`onboarding.setup.hours.${v}`)}
                  </Chip>
                ))
              : null}
            {question === "languages" ? (
              <>
                {LANGUAGES.map((v) => (
                  <Chip
                    key={v}
                    selected={answers.languages?.includes(v)}
                    onClick={() => toggle("languages", v)}
                  >
                    {t(`onboarding.setup.languages.${v}`)}
                  </Chip>
                ))}
                <Button
                  size="sm"
                  className="rounded-full"
                  disabled={!answers.languages?.length}
                  onClick={next}
                >
                  {t("onboarding.setup.chat.continue")}
                  <ArrowRight />
                </Button>
              </>
            ) : null}
            {question === "request_types" ? (
              <>
                {hints.map((h) => {
                  const label = t(`onboarding.setup.requests.${h}`);
                  return (
                    <Chip
                      key={h}
                      selected={answers.request_types?.includes(label)}
                      onClick={() => toggle("request_types", label)}
                    >
                      {label}
                    </Chip>
                  );
                })}
                {(answers.request_types ?? [])
                  .filter(
                    (v) =>
                      !hints.some(
                        (h) => t(`onboarding.setup.requests.${h}`) === v,
                      ),
                  )
                  .map((v) => (
                    <Chip
                      key={v}
                      selected
                      onClick={() => toggle("request_types", v)}
                    >
                      {v}
                    </Chip>
                  ))}
                <form onSubmit={addCustom} className="flex w-full gap-2 pt-1">
                  <Input
                    aria-label={t("onboarding.setup.chat.custom_request")}
                    placeholder={t("onboarding.setup.chat.custom_request")}
                    value={custom}
                    maxLength={60}
                    onChange={(e) => setCustom(e.target.value)}
                  />
                  <Button
                    type="submit"
                    variant="outline"
                    size="icon"
                    disabled={!custom.trim()}
                  >
                    <Plus />
                    <span className="sr-only">
                      {t("onboarding.setup.chat.add")}
                    </span>
                  </Button>
                  <Button
                    type="button"
                    disabled={!answers.request_types?.length}
                    onClick={next}
                  >
                    {t("onboarding.setup.chat.continue")}
                    <ArrowRight />
                  </Button>
                </form>
              </>
            ) : null}
            {question === "tone"
              ? TONES.map((v) => (
                  <Chip key={v} onClick={() => finish(v)}>
                    {t(`onboarding.setup.tones.${v}`)}
                  </Chip>
                ))
              : null}
          </motion.div>
        ) : null}
      </AnimatePresence>
      {busy ? <Thinking hasWebsite={Boolean(website.trim())} /> : null}
      <div ref={endRef} />
    </div>
  );
}

const THINKING = ["reading", "understanding", "drafting", "polishing"] as const;

/** "AI at work" lines while the plan is drafted (10–30 s). */
function Thinking({ hasWebsite }: { hasWebsite: boolean }) {
  const { t } = useLocale();
  const reduced = usePrefersReducedMotion();
  const lines = hasWebsite ? THINKING : THINKING.slice(1);
  const [index, setIndex] = useState(0);
  useEffect(() => {
    const id = window.setInterval(
      () => setIndex((i) => Math.min(i + 1, lines.length - 1)),
      3500,
    );
    return () => window.clearInterval(id);
  }, [lines.length]);
  return (
    <div className="flex items-center gap-3 pl-10" role="status">
      <span
        className="relative grid size-9 place-items-center rounded-full text-white"
        style={{ background: "var(--setup-accent)" }}
      >
        {reduced ? null : (
          <motion.span
            className="absolute inset-0 rounded-full"
            style={{ background: "var(--setup-accent)" }}
            animate={{ scale: [1, 1.6], opacity: [0.5, 0] }}
            transition={{ duration: 1.4, repeat: Infinity }}
          />
        )}
        <Loader2 className="relative size-4 animate-spin" aria-hidden />
      </span>
      <AnimatePresence mode="wait">
        <motion.p
          key={lines[index]}
          initial={reduced ? false : { opacity: 0, y: 6 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -6 }}
          className="text-muted-foreground text-sm"
        >
          {t(`onboarding.setup.thinking.${lines[index]}`)}
        </motion.p>
      </AnimatePresence>
    </div>
  );
}
