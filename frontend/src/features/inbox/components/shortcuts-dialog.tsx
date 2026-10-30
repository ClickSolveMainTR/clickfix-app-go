"use client";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useLocale } from "@/providers/locale-provider";

import { COPILOT_SHORTCUTS } from "../lib/copilot";
import { INBOX_SHORTCUTS } from "../lib/shortcuts";

/** Keyboard cheat sheet (`?`). */
export function ShortcutsDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
}) {
  const { t } = useLocale();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t("inbox.shortcuts.title")}</DialogTitle>
          <DialogDescription>
            {t("inbox.shortcuts.description")}
          </DialogDescription>
        </DialogHeader>
        <ul className="grid gap-1.5">
          {[...INBOX_SHORTCUTS, ...COPILOT_SHORTCUTS].map((s) => (
            <li
              key={s.labelKey}
              className="flex items-center justify-between gap-4 text-sm"
            >
              <span>{t(s.labelKey)}</span>
              <span className="flex gap-1">
                {s.keys.map((k) => (
                  <kbd
                    key={k}
                    className="bg-muted min-w-6 rounded border px-1.5 py-0.5 text-center font-mono text-[11px]"
                  >
                    {k}
                  </kbd>
                ))}
              </span>
            </li>
          ))}
        </ul>
      </DialogContent>
    </Dialog>
  );
}
