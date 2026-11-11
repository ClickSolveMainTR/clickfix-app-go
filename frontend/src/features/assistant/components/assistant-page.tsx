"use client";

import { ThreadListPrimitive } from "@assistant-ui/react";
import { Plus } from "lucide-react";

import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

import { AssistantAvatar } from "./assistant-avatar";
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
        icon={<AssistantAvatar size="md" />}
        title={t("assistant.title")}
        description={t("assistant.page.description")}
      />
      <div className="bg-card flex min-h-0 flex-1 overflow-hidden rounded-xl border shadow-xs">
        <aside
          aria-label={t("assistant.history")}
          className="bg-background/40 hidden w-72 shrink-0 flex-col border-r md:flex"
        >
          <ThreadHistory
            header={
              <div className="flex h-12 items-center justify-between gap-2 border-b px-4">
                <p className="font-display truncate text-sm font-semibold tracking-tight">
                  {t("assistant.history.title")}
                </p>
                <ThreadListPrimitive.New asChild>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t("assistant.new_chat")}
                    title={t("assistant.new_chat")}
                  >
                    <Plus className="size-4" />
                  </Button>
                </ThreadListPrimitive.New>
              </div>
            }
          />
        </aside>
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          <AssistantPanel variant="page" />
        </div>
      </div>
    </div>
  );
}
