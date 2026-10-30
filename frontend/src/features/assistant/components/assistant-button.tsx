"use client";

import { Bot } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

import { useOptionalAssistant } from "./assistant-provider";

/** Header button that opens ClickFix Asistan (also Cmd/Ctrl + J). */
export function AssistantButton() {
  const { t } = useLocale();
  const assistant = useOptionalAssistant();
  if (!assistant) return null;
  return (
    <Button
      variant="ghost"
      size="icon"
      className="size-9 rounded-lg"
      onClick={assistant.toggle}
      aria-label={t("assistant.open")}
      aria-expanded={assistant.open}
      title={`${t("assistant.title")} (⌘J)`}
      data-guide="header-assistant"
    >
      <Bot className="size-[1.1rem]" />
    </Button>
  );
}
