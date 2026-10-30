"use client";

import { useState } from "react";
import { MessageSquareText, Plus, Trash2, Zap } from "lucide-react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { useLocale } from "@/providers/locale-provider";

import {
  useCannedMutations,
  useCannedResponses,
  useMacroMutations,
  useMacros,
  useMacrosScope,
} from "../hooks/use-macros";
import type { Canned, Macro, MacroAction } from "../services/macros.service";

const VARIABLES = [
  "{{contact.first_name}}",
  "{{contact.name}}",
  "{{contact.email}}",
  "{{company.name}}",
  "{{agent.first_name}}",
  "{{agent.name}}",
  "{{conversation.number}}",
];

/** Canned responses and macros (CF-48). */
export function RepliesPage() {
  const { t } = useLocale();
  const scope = useMacrosScope();
  if (!scope.canRead) {
    return (
      <p className="text-muted-foreground p-8 text-sm">
        {t("macros.no_permission")}
      </p>
    );
  }
  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-4">
      <PageHeader
        title={t("macros.title")}
        description={t("macros.description")}
      />
      <Tabs defaultValue="canned">
        <TabsList>
          <TabsTrigger value="canned">
            <MessageSquareText className="size-4" />
            {t("macros.tabs.canned")}
          </TabsTrigger>
          <TabsTrigger value="macros">
            <Zap className="size-4" />
            {t("macros.tabs.macros")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="canned" className="mt-4">
          <CannedList />
        </TabsContent>
        <TabsContent value="macros" className="mt-4">
          <MacroList />
        </TabsContent>
      </Tabs>
    </div>
  );
}

function ScopeBadge({ scope }: { scope: string }) {
  const { t } = useLocale();
  return (
    <span className="bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[10px] font-medium tracking-wide uppercase">
      {t(`macros.scope.${scope}`)}
    </span>
  );
}

function CannedList() {
  const { t } = useLocale();
  const scope = useMacrosScope();
  const list = useCannedResponses();
  const [editing, setEditing] = useState<Canned | "new" | null>(null);
  const editable = (c: Canned) => c.mine || scope.canWrite;
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-muted-foreground text-sm">
          {t("macros.canned_hint")}
        </p>
        <Button size="sm" onClick={() => setEditing("new")}>
          <Plus className="size-4" />
          {t("macros.new_canned")}
        </Button>
      </div>
      {list.isLoading && <Skeleton className="h-24 rounded-xl" />}
      {!list.isLoading && (list.data ?? []).length === 0 && (
        <p className="text-muted-foreground rounded-xl border border-dashed p-8 text-center text-sm">
          {t("macros.empty_canned")}
        </p>
      )}
      <ul className="divide-y rounded-xl border">
        {(list.data ?? []).map((c) => (
          <li key={c.uuid}>
            <button
              type="button"
              disabled={!editable(c)}
              onClick={() => setEditing(c)}
              className="hover:bg-muted/40 flex w-full flex-col gap-1 px-4 py-3 text-left disabled:cursor-default"
            >
              <span className="flex items-center gap-2">
                <span className="text-muted-foreground font-mono text-xs">
                  /{c.shortcut}
                </span>
                <span className="font-medium">{c.title}</span>
                <ScopeBadge scope={c.scope} />
                <span className="text-muted-foreground ml-auto text-xs tabular-nums">
                  {t("macros.used", { count: c.usage_count })}
                </span>
              </span>
              <span className="text-muted-foreground line-clamp-2 text-sm">
                {c.content}
              </span>
            </button>
          </li>
        ))}
      </ul>
      {editing && (
        <CannedDialog
          key={editing === "new" ? "new" : editing.uuid}
          item={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  );
}

function CannedDialog({
  item,
  onClose,
}: {
  item: Canned | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const scope = useMacrosScope();
  const m = useCannedMutations();
  const [deleting, setDeleting] = useState(false);
  const [form, setForm] = useState({
    title: item?.title ?? "",
    shortcut: item?.shortcut ?? "",
    category: item?.category ?? "",
    content: item?.content ?? "",
    scope: (item?.scope ?? "personal") as "personal" | "organization",
  });
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {item ? item.title : t("macros.new_canned")}
          </DialogTitle>
          <DialogDescription>
            {t("macros.canned_form_description")}
          </DialogDescription>
        </DialogHeader>
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            const done = { onSuccess: onClose };
            if (item) m.update.mutate({ uuid: item.uuid, body: form }, done);
            else m.create.mutate(form, done);
          }}
        >
          <div className="grid grid-cols-[1fr_9rem] gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="cr-title">{t("macros.fields.title")}</Label>
              <Input
                id="cr-title"
                value={form.title}
                onChange={(e) =>
                  setForm((f) => ({ ...f, title: e.target.value }))
                }
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="cr-shortcut">{t("macros.fields.shortcut")}</Label>
              <Input
                id="cr-shortcut"
                value={form.shortcut}
                placeholder="iade"
                onChange={(e) =>
                  setForm((f) => ({
                    ...f,
                    shortcut: e.target.value
                      .toLowerCase()
                      .replace(/[^a-z0-9_-]/g, ""),
                  }))
                }
              />
            </div>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="cr-content">{t("macros.fields.content")}</Label>
            <Textarea
              id="cr-content"
              rows={6}
              value={form.content}
              onChange={(e) =>
                setForm((f) => ({ ...f, content: e.target.value }))
              }
            />
            <div className="flex flex-wrap gap-1">
              {VARIABLES.map((v) => (
                <button
                  type="button"
                  key={v}
                  className="bg-muted hover:bg-muted/70 rounded px-1.5 py-0.5 font-mono text-[11px]"
                  onClick={() =>
                    setForm((f) => ({ ...f, content: f.content + v }))
                  }
                >
                  {v}
                </button>
              ))}
            </div>
          </div>
          {!item && scope.canWrite && (
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={form.scope === "organization"}
                onChange={(e) =>
                  setForm((f) => ({
                    ...f,
                    scope: e.target.checked ? "organization" : "personal",
                  }))
                }
              />
              {t("macros.share_org")}
            </label>
          )}
          <DialogFooter className="sm:items-center">
            {item && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="text-destructive"
                onClick={() => setDeleting(true)}
              >
                <Trash2 className="size-4" />
                {t("macros.delete")}
              </Button>
            )}
            <Button
              type="submit"
              className="ml-auto"
              disabled={
                !form.title ||
                !form.shortcut ||
                !form.content ||
                m.create.isPending ||
                m.update.isPending
              }
            >
              {t("macros.save")}
            </Button>
          </DialogFooter>
        </form>
        {item && (
          <ConfirmDialog
            open={deleting}
            title={t("macros.delete_title")}
            variant="destructive"
            isPending={m.remove.isPending}
            onCancel={() => setDeleting(false)}
            onConfirm={() => m.remove.mutate(item.uuid, { onSuccess: onClose })}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

const ACTION_TYPES = [
  "reply",
  "note",
  "status",
  "priority",
  "assign",
  "tag",
  "untag",
] as const;
const STATUS_VALUES = ["open", "pending", "resolved", "closed"];
const PRIORITY_VALUES = ["low", "normal", "high", "urgent"];

function MacroList() {
  const { t } = useLocale();
  const scope = useMacrosScope();
  const list = useMacros();
  const [editing, setEditing] = useState<Macro | "new" | null>(null);
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-muted-foreground text-sm">
          {t("macros.macros_hint")}
        </p>
        <Button size="sm" onClick={() => setEditing("new")}>
          <Plus className="size-4" />
          {t("macros.new_macro")}
        </Button>
      </div>
      {!list.isLoading && (list.data ?? []).length === 0 && (
        <p className="text-muted-foreground rounded-xl border border-dashed p-8 text-center text-sm">
          {t("macros.empty_macros")}
        </p>
      )}
      <div className="grid gap-3 sm:grid-cols-2">
        {(list.data ?? []).map((m) => (
          <button
            type="button"
            key={m.uuid}
            disabled={!(m.mine || scope.canWrite)}
            onClick={() => setEditing(m)}
            className="bg-card hover:border-foreground/20 flex flex-col gap-2 rounded-xl border p-4 text-left disabled:cursor-default"
          >
            <span className="flex items-center gap-2">
              <Zap className="size-4 text-amber-500" />
              <span className="font-medium">{m.name}</span>
              <ScopeBadge scope={m.scope} />
            </span>
            <span className="flex flex-wrap gap-1">
              {m.actions.map((a, i) => (
                <span
                  key={i}
                  className="bg-muted rounded px-1.5 py-0.5 text-[11px]"
                >
                  {t(`macros.actions.${a.type}`)}
                  {a.type !== "reply" && a.type !== "note" && a.value
                    ? `: ${a.value}`
                    : ""}
                </span>
              ))}
            </span>
            <span className="text-muted-foreground text-xs">
              {t("macros.used", { count: m.usage_count })}
            </span>
          </button>
        ))}
      </div>
      {editing && (
        <MacroDialog
          key={editing === "new" ? "new" : editing.uuid}
          item={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  );
}

function MacroDialog({
  item,
  onClose,
}: {
  item: Macro | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const scope = useMacrosScope();
  const m = useMacroMutations();
  const [deleting, setDeleting] = useState(false);
  const [name, setName] = useState(item?.name ?? "");
  const [shared, setShared] = useState(
    item ? item.scope === "organization" : false,
  );
  const [actions, setActions] = useState<MacroAction[]>(
    item?.actions ?? [{ type: "reply", value: "" }],
  );
  const set = (i: number, patch: Partial<MacroAction>) =>
    setActions((a) => a.map((x, j) => (j === i ? { ...x, ...patch } : x)));
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{item ? item.name : t("macros.new_macro")}</DialogTitle>
          <DialogDescription>
            {t("macros.macro_form_description")}
          </DialogDescription>
        </DialogHeader>
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            const body = {
              name,
              actions,
              scope: shared ? ("organization" as const) : ("personal" as const),
            };
            const done = { onSuccess: onClose };
            if (item) m.update.mutate({ uuid: item.uuid, body }, done);
            else m.create.mutate(body, done);
          }}
        >
          <div className="grid gap-1.5">
            <Label htmlFor="mc-name">{t("macros.fields.name")}</Label>
            <Input
              id="mc-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("macros.fields.actions")}</Label>
            {actions.map((a, i) => (
              <div key={i} className="flex items-start gap-2">
                <select
                  aria-label={t("macros.fields.action_type")}
                  value={a.type}
                  onChange={(e) =>
                    set(i, {
                      type: e.target.value as MacroAction["type"],
                      value: "",
                    })
                  }
                  className="bg-background h-9 rounded-md border px-2 text-sm"
                >
                  {ACTION_TYPES.map((type) => (
                    <option key={type} value={type}>
                      {t(`macros.actions.${type}`)}
                    </option>
                  ))}
                </select>
                {a.type === "status" || a.type === "priority" ? (
                  <select
                    aria-label={t("macros.fields.action_value")}
                    value={a.value}
                    onChange={(e) => set(i, { value: e.target.value })}
                    className="bg-background h-9 flex-1 rounded-md border px-2 text-sm"
                  >
                    <option value="">—</option>
                    {(a.type === "status"
                      ? STATUS_VALUES
                      : PRIORITY_VALUES
                    ).map((v) => (
                      <option key={v} value={v}>
                        {t(
                          a.type === "status"
                            ? `inbox.status.${v}`
                            : `inbox.priority.${v}`,
                        )}
                      </option>
                    ))}
                  </select>
                ) : a.type === "assign" ? (
                  <select
                    aria-label={t("macros.fields.action_value")}
                    value={a.value}
                    onChange={(e) => set(i, { value: e.target.value })}
                    className="bg-background h-9 flex-1 rounded-md border px-2 text-sm"
                  >
                    <option value="me">{t("macros.assign_me")}</option>
                    <option value="">{t("macros.unassign")}</option>
                  </select>
                ) : a.type === "reply" || a.type === "note" ? (
                  <Textarea
                    rows={2}
                    className="flex-1"
                    value={a.value}
                    onChange={(e) => set(i, { value: e.target.value })}
                  />
                ) : (
                  <Input
                    className="flex-1"
                    value={a.value}
                    placeholder={t("macros.tag_name")}
                    onChange={(e) => set(i, { value: e.target.value })}
                  />
                )}
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={t("macros.remove_action")}
                  onClick={() => setActions((x) => x.filter((_, j) => j !== i))}
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
            ))}
            {actions.length < 10 && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="justify-self-start"
                onClick={() =>
                  setActions((x) => [
                    ...x,
                    { type: "status", value: "resolved" },
                  ])
                }
              >
                <Plus className="size-4" />
                {t("macros.add_action")}
              </Button>
            )}
          </div>
          {scope.canWrite && (
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={shared}
                disabled={Boolean(item)}
                onChange={(e) => setShared(e.target.checked)}
              />
              {t("macros.share_org")}
            </label>
          )}
          <DialogFooter className="sm:items-center">
            {item && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="text-destructive"
                onClick={() => setDeleting(true)}
              >
                <Trash2 className="size-4" />
                {t("macros.delete")}
              </Button>
            )}
            <Button
              type="submit"
              className="ml-auto"
              disabled={
                !name.trim() ||
                actions.length === 0 ||
                m.create.isPending ||
                m.update.isPending
              }
            >
              {t("macros.save")}
            </Button>
          </DialogFooter>
        </form>
        {item && (
          <ConfirmDialog
            open={deleting}
            title={t("macros.delete_title")}
            variant="destructive"
            isPending={m.remove.isPending}
            onCancel={() => setDeleting(false)}
            onConfirm={() => m.remove.mutate(item.uuid, { onSuccess: onClose })}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
