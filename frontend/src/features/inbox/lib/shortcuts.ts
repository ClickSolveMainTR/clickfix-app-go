/** Inbox keyboard map, shown by the cheat sheet (`?`). */
export const INBOX_SHORTCUTS = [
  { keys: ["J"], labelKey: "inbox.shortcuts.next" },
  { keys: ["K"], labelKey: "inbox.shortcuts.prev" },
  { keys: ["R"], labelKey: "inbox.shortcuts.reply" },
  { keys: ["N"], labelKey: "inbox.shortcuts.note" },
  { keys: ["⌘", "Enter"], labelKey: "inbox.shortcuts.send" },
  { keys: ["E"], labelKey: "inbox.shortcuts.resolve" },
  { keys: ["A"], labelKey: "inbox.shortcuts.assign" },
  { keys: ["I"], labelKey: "inbox.shortcuts.take" },
  { keys: ["#"], labelKey: "inbox.shortcuts.tag" },
  { keys: ["P"], labelKey: "inbox.shortcuts.priority" },
  { keys: ["S"], labelKey: "inbox.shortcuts.snooze" },
  { keys: ["M"], labelKey: "inbox.shortcuts.macro" },
  { keys: ["/"], labelKey: "inbox.shortcuts.canned" },
  { keys: ["X"], labelKey: "inbox.shortcuts.select" },
  { keys: ["/"], labelKey: "inbox.shortcuts.search" },
  { keys: ["1", "–", "4"], labelKey: "inbox.shortcuts.views" },
  { keys: ["Esc"], labelKey: "inbox.shortcuts.escape" },
  { keys: ["?"], labelKey: "inbox.shortcuts.help" },
] as const;

/** True when the key event comes from a text field (shortcuts stay off). */
export function isTyping(e: KeyboardEvent): boolean {
  const el = e.target as HTMLElement | null;
  if (!el) return false;
  const tag = el.tagName;
  return (
    tag === "INPUT" ||
    tag === "TEXTAREA" ||
    tag === "SELECT" ||
    el.isContentEditable ||
    el.closest("[role=dialog],[role=menu],[cmdk-root]") != null
  );
}
