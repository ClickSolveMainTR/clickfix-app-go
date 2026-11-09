"use client";

import { Sheet, SheetContent } from "@/components/ui/sheet";

import { AssistantPanel } from "./assistant-panel";

/** The assistant as a right-side overlay (the default mode). */
export function AssistantSheet({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="flex w-full flex-col gap-0 p-0 sm:max-w-xl"
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <AssistantPanel variant="sheet" onClose={() => onOpenChange(false)} />
      </SheetContent>
    </Sheet>
  );
}
