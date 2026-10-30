"use client";

import { Bell, BellOff, LogIn, ThumbsUp } from "lucide-react";
import { useState, useSyncExternalStore } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/** Visitor-facing strings, resolved on the server in the help center language. */
export type ForumLabels = Record<
  | "signin_title"
  | "signin_text"
  | "ask"
  | "reply"
  | "title"
  | "body"
  | "category"
  | "pick_category"
  | "submit"
  | "sending"
  | "pending"
  | "failed"
  | "denied"
  | "rate_limited"
  | "vote"
  | "voted"
  | "subscribe"
  | "unsubscribe"
  | "cancel",
  string
>;

type Visitor = { widgetId: string; token: string };

const TOKEN_KEY = /^clickfix:([0-9a-f-]{36}):token$/i;

/**
 * The web widget SDK keeps its visitor token in this origin's localStorage
 * (`clickfix:<widget id>:token`). The forum API accepts only identified
 * (signed) visitors, so a token is used as-is and an anonymous one simply
 * gets 403 → sign-in prompt.
 */
function readVisitor(): Visitor | null {
  try {
    for (let i = 0; i < window.localStorage.length; i++) {
      const key = window.localStorage.key(i) ?? "";
      const m = TOKEN_KEY.exec(key);
      const token = m ? window.localStorage.getItem(key) : null;
      if (m && token) return { widgetId: m[1]!, token };
    }
  } catch {
    // Storage blocked (private mode): treat as signed out.
  }
  return null;
}

function subscribeStorage(cb: () => void) {
  window.addEventListener("storage", cb);
  return () => window.removeEventListener("storage", cb);
}

let cached: { key: string; value: Visitor | null } | null = null;
function visitorSnapshot(): Visitor | null {
  const v = readVisitor();
  const key = v ? `${v.widgetId}:${v.token}` : "";
  if (!cached || cached.key !== key) cached = { key, value: v };
  return cached.value;
}

function readFlag(key: string): boolean {
  try {
    return window.localStorage.getItem(key) === "1";
  } catch {
    return false;
  }
}

function useVisitor(): Visitor | null {
  return useSyncExternalStore(subscribeStorage, visitorSnapshot, () => null);
}

type Outcome = "ok" | "denied" | "rate_limited" | "failed";

async function forumWrite(
  org: string,
  path: string,
  visitor: Visitor,
  body: unknown,
): Promise<{ outcome: Outcome; data?: unknown }> {
  try {
    const res = await fetch(
      `/api/v1/public/help/${encodeURIComponent(org)}/forum/${path}`,
      {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${visitor.token}`,
          "X-Widget-ID": visitor.widgetId,
        },
        body: JSON.stringify(body),
      },
    );
    if (res.status === 401 || res.status === 403) return { outcome: "denied" };
    if (res.status === 429) return { outcome: "rate_limited" };
    if (!res.ok) return { outcome: "failed" };
    const json = (await res.json()) as { data?: unknown };
    return { outcome: "ok", data: json.data };
  } catch {
    return { outcome: "failed" };
  }
}

/** "Sign in to take part" card shown instead of write controls. */
export function ForumSignIn({ labels }: { labels: ForumLabels }) {
  return (
    <div className="border-border bg-muted/40 flex items-start gap-3 rounded-xl border p-4 text-sm">
      <LogIn
        aria-hidden
        className="text-muted-foreground mt-0.5 size-5 shrink-0"
      />
      <div className="space-y-1">
        <p className="font-medium">{labels.signin_title}</p>
        <p className="text-muted-foreground">{labels.signin_text}</p>
      </div>
    </div>
  );
}

/**
 * New topic or reply form. Every submission waits for moderation; the
 * visitor is told so instead of seeing the post appear.
 */
export function ForumComposer({
  org,
  labels,
  topicSlug,
  categories,
  defaultCategory,
}: {
  org: string;
  labels: ForumLabels;
  /** Set for a reply; topics need a category instead. */
  topicSlug?: string;
  categories?: { slug: string; name: string }[];
  defaultCategory?: string;
}) {
  const visitor = useVisitor();
  const isReply = Boolean(topicSlug);
  const [open, setOpen] = useState(isReply);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [category, setCategory] = useState(
    defaultCategory ?? categories?.[0]?.slug ?? "",
  );
  const [state, setState] = useState<"idle" | "sending" | Outcome>("idle");

  if (!visitor || state === "denied") return <ForumSignIn labels={labels} />;
  if (state === "ok")
    return (
      <p
        role="status"
        className="rounded-xl border border-emerald-500/40 bg-emerald-500/5 p-4 text-sm"
      >
        {labels.pending}
      </p>
    );
  if (!open) return <Button onClick={() => setOpen(true)}>{labels.ask}</Button>;

  const invalid = !body.trim() || (!isReply && (!title.trim() || !category));
  const submit = async () => {
    setState("sending");
    const res = isReply
      ? await forumWrite(
          org,
          `topics/${encodeURIComponent(topicSlug!)}/replies`,
          visitor,
          { body: body.trim() },
        )
      : await forumWrite(org, "topics", visitor, {
          title: title.trim(),
          body: body.trim(),
          category,
        });
    setState(res.outcome);
  };

  return (
    <form
      className="border-border bg-card space-y-3 rounded-xl border p-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (!invalid && state !== "sending") void submit();
      }}
    >
      <h2 className="font-medium">{isReply ? labels.reply : labels.ask}</h2>
      {!isReply && (
        <>
          {categories && categories.length > 1 && !defaultCategory && (
            <label className="block space-y-1 text-sm">
              <span className="font-medium">{labels.category}</span>
              <select
                value={category}
                onChange={(e) => setCategory(e.target.value)}
                className="border-input bg-background h-9 w-full rounded-md border px-2"
              >
                {categories.map((c) => (
                  <option key={c.slug} value={c.slug}>
                    {c.name}
                  </option>
                ))}
              </select>
            </label>
          )}
          <label className="block space-y-1 text-sm">
            <span className="font-medium">{labels.title}</span>
            <input
              value={title}
              maxLength={200}
              required
              onChange={(e) => setTitle(e.target.value)}
              className="border-input bg-background h-9 w-full rounded-md border px-3"
            />
          </label>
        </>
      )}
      <label className="block space-y-1 text-sm">
        <span className="font-medium">{labels.body}</span>
        <textarea
          value={body}
          rows={5}
          maxLength={16000}
          required
          onChange={(e) => setBody(e.target.value)}
          className="border-input bg-background w-full rounded-md border px-3 py-2"
        />
      </label>
      {(state === "failed" || state === "rate_limited") && (
        <p role="alert" className="text-destructive text-sm">
          {state === "rate_limited" ? labels.rate_limited : labels.failed}
        </p>
      )}
      <div className="flex flex-wrap justify-end gap-2">
        {!isReply && (
          <Button
            type="button"
            variant="outline"
            onClick={() => setOpen(false)}
          >
            {labels.cancel}
          </Button>
        )}
        <Button type="submit" disabled={invalid || state === "sending"}>
          {state === "sending" ? labels.sending : labels.submit}
        </Button>
      </div>
    </form>
  );
}

/** Upvote of a topic (no `replyUuid`) or a reply. Signed-in visitors only. */
export function ForumVote({
  org,
  topicSlug,
  replyUuid,
  votes,
  labels,
}: {
  org: string;
  topicSlug: string;
  replyUuid?: string;
  /** Unknown for a topic on its own page (the API returns it in lists only). */
  votes?: number;
  labels: ForumLabels;
}) {
  const visitor = useVisitor();
  const storageKey = `clickfix:forum-vote:${org}:${topicSlug}:${replyUuid ?? ""}`;
  // Remembered locally: public reads do not say who voted.
  const remembered = useSyncExternalStore(
    subscribeStorage,
    () => readFlag(storageKey),
    () => false,
  );
  const [override, setOverride] = useState<boolean | null>(null);
  const active = override ?? remembered;
  const [delta, setDelta] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const count = votes === undefined ? null : votes + delta;

  const toggle = async () => {
    if (!visitor || busy) return;
    setBusy(true);
    setError(null);
    const res = await forumWrite(
      org,
      `topics/${encodeURIComponent(topicSlug)}/votes`,
      visitor,
      { ...(replyUuid ? { reply_uuid: replyUuid } : {}), remove: active },
    );
    setBusy(false);
    if (res.outcome !== "ok") {
      setError(
        res.outcome === "denied"
          ? labels.denied
          : res.outcome === "rate_limited"
            ? labels.rate_limited
            : labels.failed,
      );
      return;
    }
    const now = Boolean((res.data as { active?: boolean })?.active);
    if (now !== active) setDelta((d) => d + (now ? 1 : -1));
    setOverride(now);
    try {
      if (now) window.localStorage.setItem(storageKey, "1");
      else window.localStorage.removeItem(storageKey);
    } catch {
      // Storage blocked.
    }
  };

  return (
    <span className="inline-flex items-center gap-2">
      <button
        type="button"
        onClick={() => void toggle()}
        disabled={!visitor || busy}
        aria-pressed={active}
        title={
          visitor ? (active ? labels.voted : labels.vote) : labels.signin_title
        }
        className={cn(
          "border-border inline-flex h-8 items-center gap-1.5 rounded-full border px-3 text-sm tabular-nums transition-colors outline-none focus-visible:ring-2",
          active
            ? "border-primary bg-primary/10 text-primary"
            : "text-muted-foreground hover:bg-muted",
          !visitor && "cursor-default",
        )}
      >
        <ThumbsUp aria-hidden className="size-3.5" />
        {count === null ? (
          <span>{active ? labels.voted : labels.vote}</span>
        ) : (
          <>
            <span className="sr-only">{labels.vote}</span>
            {count}
          </>
        )}
      </button>
      {error && (
        <span role="alert" className="text-destructive text-xs">
          {error}
        </span>
      )}
    </span>
  );
}

/** Follow a topic: replies land in the visitor's widget notification inbox. */
export function ForumSubscribe({
  org,
  topicSlug,
  labels,
}: {
  org: string;
  topicSlug: string;
  labels: ForumLabels;
}) {
  const visitor = useVisitor();
  const [active, setActive] = useState(false);
  const [busy, setBusy] = useState(false);
  if (!visitor) return null;
  const toggle = async () => {
    setBusy(true);
    const res = await forumWrite(
      org,
      `topics/${encodeURIComponent(topicSlug)}/subscribe`,
      visitor,
      { remove: active },
    );
    setBusy(false);
    if (res.outcome === "ok")
      setActive(Boolean((res.data as { active?: boolean })?.active));
  };
  return (
    <Button
      variant="outline"
      size="sm"
      disabled={busy}
      onClick={() => void toggle()}
    >
      {active ? <BellOff className="size-4" /> : <Bell className="size-4" />}
      {active ? labels.unsubscribe : labels.subscribe}
    </Button>
  );
}
