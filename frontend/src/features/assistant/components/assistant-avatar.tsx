import { AppMark } from "@/components/brand";
import { cn } from "@/lib/utils";

const sizes = {
  /** Beside a message. */
  sm: "size-7",
  /** Panel header, history. */
  md: "size-9",
  /** Welcome screen, page header. */
  lg: "size-12",
} as const;

/**
 * The assistant's face on every surface (CF-210): the ClickFix mark (ink
 * disc, signal cursor) on a soft tile, the same art as the sidebar and the
 * app icon. Decorative by default; pass `title` where it must be announced.
 */
export function AssistantAvatar({
  size = "sm",
  title,
  className,
}: {
  size?: keyof typeof sizes;
  title?: string;
  className?: string;
}) {
  return (
    <span
      data-slot="assistant-avatar"
      aria-hidden={title ? undefined : true}
      className={cn(
        "bg-signal-soft ring-signal/25 flex shrink-0 items-center justify-center rounded-full ring-1",
        sizes[size],
        className,
      )}
    >
      <AppMark tone="light" title={title ?? ""} className="size-[72%]" />
    </span>
  );
}
