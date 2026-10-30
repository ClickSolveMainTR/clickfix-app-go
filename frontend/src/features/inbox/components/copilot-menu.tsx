"use client";

import {
  Languages,
  ListChecks,
  MessageCircleQuestion,
  Minimize2,
  Maximize2,
  PenLine,
  Sparkles,
  SpellCheck2,
  Wand2,
  type LucideIcon,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useLocale } from "@/providers/locale-provider";

import type { CopilotRequest } from "../hooks/use-copilot";
import { COPILOT_LANGUAGES, COPILOT_TONES } from "../lib/copilot";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Agent's draft; transforms are disabled while it is empty. */
  draft: string;
  disabled?: boolean;
  /** Why the menu is disabled (AI off, not configured, not in the plan). */
  disabledReason?: string;
  pending?: boolean;
  onRun: (req: CopilotRequest) => void;
  onAsk: () => void;
};

/** Copilot actions of the composer toolbar (`⌘ .`). */
export function CopilotMenu({
  open,
  onOpenChange,
  draft,
  disabled,
  disabledReason,
  pending,
  onRun,
  onAsk,
}: Props) {
  const { t } = useLocale();
  const text = draft.trim();
  const hasDraft = text.length > 0;
  const tooLong = text.length > 4000;
  const transformsOff = !hasDraft || tooLong;

  const item = (
    Icon: LucideIcon,
    label: string,
    onSelect: () => void,
    shortcut?: string,
    off?: boolean,
  ) => (
    <DropdownMenuItem disabled={off} onSelect={onSelect}>
      <Icon className="size-4" />
      {label}
      {shortcut && <DropdownMenuShortcut>{shortcut}</DropdownMenuShortcut>}
    </DropdownMenuItem>
  );

  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange}>
      <DropdownMenuTrigger asChild disabled={disabled}>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={disabled}
          title={disabledReason}
          className="text-muted-foreground hover:text-foreground h-8 gap-1.5 px-2 disabled:pointer-events-auto disabled:cursor-not-allowed"
          aria-label={
            disabledReason
              ? `${t("inbox.copilot.title")}: ${disabledReason}`
              : t("inbox.copilot.title")
          }
        >
          <Sparkles
            className={
              pending ? "text-signal-text size-4 animate-pulse" : "size-4"
            }
          />
          <span className="hidden text-xs sm:inline">
            {t("inbox.copilot.title")}
          </span>
          <kbd className="text-muted-foreground/70 hidden font-mono text-[10px] md:inline">
            ⌘.
          </kbd>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" side="top" className="w-64">
        <DropdownMenuLabel className="text-muted-foreground text-xs font-normal">
          {t("inbox.copilot.menu_conversation")}
        </DropdownMenuLabel>
        {item(
          Sparkles,
          t("inbox.copilot.op.suggest_reply"),
          () => onRun({ op: "suggest_reply" }),
          "G",
        )}
        {item(
          ListChecks,
          t("inbox.copilot.op.summarize"),
          () => onRun({ op: "summarize" }),
          "U",
        )}
        {item(Wand2, t("inbox.copilot.op.next_action"), () =>
          onRun({ op: "next_action" }),
        )}
        {item(MessageCircleQuestion, t("inbox.copilot.op.ask"), onAsk, "Q")}
        <DropdownMenuSeparator />
        <DropdownMenuLabel className="text-muted-foreground text-xs font-normal">
          {tooLong
            ? t("inbox.copilot.draft_too_long")
            : hasDraft
              ? t("inbox.copilot.menu_draft")
              : t("inbox.copilot.menu_draft_empty")}
        </DropdownMenuLabel>
        {item(
          SpellCheck2,
          t("inbox.copilot.op.rewrite"),
          () => onRun({ op: "rewrite", text }),
          undefined,
          transformsOff,
        )}
        {item(
          Minimize2,
          t("inbox.copilot.op.shorten"),
          () => onRun({ op: "shorten", text }),
          undefined,
          transformsOff,
        )}
        {item(
          Maximize2,
          t("inbox.copilot.op.expand"),
          () => onRun({ op: "expand", text }),
          undefined,
          transformsOff,
        )}
        <DropdownMenuSub>
          <DropdownMenuSubTrigger disabled={transformsOff}>
            <PenLine className="size-4" />
            {t("inbox.copilot.op.tone")}
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            {COPILOT_TONES.map((tone) => (
              <DropdownMenuItem
                key={tone}
                onSelect={() => onRun({ op: "tone", text, tone })}
              >
                {t(`inbox.copilot.tone.${tone}`)}
              </DropdownMenuItem>
            ))}
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger disabled={transformsOff}>
            <Languages className="size-4" />
            {t("inbox.copilot.op.translate")}
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            {COPILOT_LANGUAGES.map((lang) => (
              <DropdownMenuItem
                key={lang}
                onSelect={() => onRun({ op: "translate", text, locale: lang })}
              >
                {t(`inbox.copilot.language.${lang}`)}
              </DropdownMenuItem>
            ))}
          </DropdownMenuSubContent>
        </DropdownMenuSub>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
