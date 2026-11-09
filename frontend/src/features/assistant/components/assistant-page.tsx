"use client";

import { Bot } from "lucide-react";

import { PageHeader } from "@/components/layout/page-header";
import { useLocale } from "@/providers/locale-provider";

import { AssistantPanel, ThreadHistory } from "./assistant-panel";

/**
 * The assistant as a full page (`/assistant`): past chats on the left, the
 * conversation on the right. Same runtime as the sheet and the dock, so a
 * chat started from the header continues here.
 */
export function AssistantPage() {
  const { t } = useLocale();
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4" data-layout="fixed">
      <PageHeader
        icon={<Bot className="size-7" />}
        title={t("assistant.title")}
        description={t("assistant.page.description")}
      />
      <div className="bg-card flex min-h-0 flex-1 overflow-hidden rounded-xl border">
        <aside
          aria-label={t("assistant.history")}
          className="hidden w-72 shrink-0 flex-col border-r md:flex"
        >
          <ThreadHistory />
        </aside>
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          <AssistantPanel variant="page" />
        </div>
      </div>
    </div>
  );
}
