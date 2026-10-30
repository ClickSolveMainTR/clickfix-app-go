"use client";

import { Bot, CircleHelp, ListChecks, PlayCircle } from "lucide-react";
import { useState } from "react";

import { MarkdownContent } from "@/components/markdown/markdown-content";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Progress } from "@/components/ui/progress";
import { useOptionalAssistant } from "@/features/assistant/components/assistant-provider";
import { useLocale } from "@/providers/locale-provider";

import { useGuides } from "./guides-provider";

/**
 * Header "?" button: what this page is for, replay its tour, ask the
 * assistant, and the set-up tour progress.
 */
export function PageHelpButton() {
  const { t } = useLocale();
  const { pageGuide, pageGuides, setup, startTour, startSetupTour } =
    useGuides();
  const assistant = useOptionalAssistant();
  const [open, setOpen] = useState(false);
  const tour = pageGuide ?? pageGuides[0] ?? null;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="size-9 rounded-lg"
          aria-label={t("guides.help.button")}
          data-guide="header-help"
        >
          <CircleHelp className="size-[1.1rem]" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 space-y-3">
        <div>
          <p className="text-muted-foreground text-[11px] font-semibold tracking-wide uppercase">
            {t("guides.help.what_is_this")}
          </p>
          {pageGuide ? (
            <>
              <h3 className="mt-1 text-sm font-semibold">{pageGuide.title}</h3>
              <MarkdownContent
                markdown={pageGuide.summary}
                className="text-muted-foreground mt-1 text-sm [&_p]:my-1"
              />
            </>
          ) : (
            <p className="text-muted-foreground mt-1 text-sm">
              {t("guides.help.no_guide")}
            </p>
          )}
        </div>
        <div className="flex flex-wrap gap-2">
          {tour && tour.steps.length ? (
            <Button
              size="sm"
              onClick={() => {
                setOpen(false);
                startTour(tour);
              }}
            >
              <PlayCircle className="size-4" />
              {t("guides.help.replay")}
            </Button>
          ) : null}
          {assistant ? (
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setOpen(false);
                assistant.ask(t("guides.help.ask_prompt"), pageGuide?.key);
              }}
            >
              <Bot className="size-4" />
              {t("guides.tour.ask")}
            </Button>
          ) : null}
        </div>
        {setup && setup.total > 0 ? (
          <div className="border-t pt-3">
            <div className="mb-1.5 flex items-center justify-between text-xs">
              <span className="font-medium">{t("guides.setup.title")}</span>
              <span className="text-muted-foreground">
                {setup.completed}/{setup.total}
              </span>
            </div>
            <Progress value={(setup.completed / setup.total) * 100} />
            <Button
              size="sm"
              variant="ghost"
              className="mt-2 -ml-2"
              onClick={() => {
                setOpen(false);
                startSetupTour();
              }}
            >
              <ListChecks className="size-4" />
              {setup.completed > 0
                ? t("guides.setup.continue")
                : t("guides.setup.start")}
            </Button>
          </div>
        ) : null}
      </PopoverContent>
    </Popover>
  );
}
