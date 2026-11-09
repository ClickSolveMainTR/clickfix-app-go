"use client";

import { AssistantPanel } from "./assistant-panel";
import { useOptionalAssistant } from "./assistant-provider";

/**
 * The assistant docked beside the page (split view): a right column of the
 * workspace shell that pushes the content instead of covering it. Rendered
 * by the shell only while the member chose the docked mode and the panel is
 * open; narrow screens fall back to the sheet.
 */
export function AssistantDock() {
  const assistant = useOptionalAssistant();
  if (!assistant || assistant.effectiveMode !== "docked" || !assistant.open)
    return null;
  return (
    <aside
      aria-label="ClickFix Asistan"
      className="bg-background sticky top-0 hidden h-svh w-[26rem] shrink-0 flex-col border-l lg:flex xl:w-[30rem]"
      data-guide="assistant-dock"
    >
      <AssistantPanel
        variant="docked"
        onClose={() => assistant.setOpen(false)}
      />
    </aside>
  );
}
