"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowDown,
  ArrowUp,
  BookOpenCheck,
  Pencil,
  Plus,
  Sparkles,
  Trash2,
} from "lucide-react";
import {
  useState,
  useId,
  cloneElement,
  isValidElement,
  type ReactElement,
  type ReactNode,
} from "react";

import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  platformGuidesService,
  type AdminGuide,
  type AdminGuideInput,
  type GuideStep,
} from "../services/guides.service";
import { useDialogs } from "@/providers/dialog-provider";

const KEY = ["platform-guides"] as const;
const LIMIT = 50;
const ALL = "all";
const PLACEMENTS = ["auto", "top", "bottom", "left", "right"] as const;

type Editing = { uuid: string | null; input: AdminGuideInput };

function emptyInput(): AdminGuideInput {
  return {
    key: "",
    locale: "tr",
    surface: "web",
    route: "/",
    kind: "page",
    order: 100,
    setup_order: 0,
    title: "",
    summary: "",
    steps: [],
    is_published: true,
  };
}

function toInput(g: AdminGuide): AdminGuideInput {
  return {
    key: g.key,
    locale: g.locale,
    surface: g.surface,
    route: g.route,
    kind: g.kind,
    order: g.order,
    setup_order: g.setup_order,
    title: g.title,
    summary: g.summary,
    steps: g.steps,
    is_published: g.is_published,
  };
}

/**
 * Module guide editor (platform.guides.manage): every page guide and tour
 * per locale, the set-up tour order and AI drafting from route metadata.
 */
export function PlatformGuidesPage() {
  const { confirmDelete } = useDialogs();
  const { t } = useLocale();
  const qc = useQueryClient();
  const [surface, setSurface] = useState<string>(ALL);
  const [locale, setLocale] = useState<string>("tr");
  const [q, setQ] = useState("");
  const [offset, setOffset] = useState(0);
  const [editing, setEditing] = useState<Editing | null>(null);
  const [generating, setGenerating] = useState(false);
  const params = {
    surface: surface === ALL ? undefined : surface,
    locale: locale === ALL ? undefined : locale,
    q: q.trim() || undefined,
    limit: LIMIT,
    offset,
  };
  const list = useQuery({
    queryKey: [...KEY, params],
    queryFn: () => platformGuidesService.list(params),
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => platformGuidesService.remove(uuid),
    onSuccess: () => {
      appToast.success(t("guides.platform.deleted"));
      void qc.invalidateQueries({ queryKey: KEY });
    },
  });
  const total = list.data?.total ?? 0;

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<BookOpenCheck className="size-7" />}
        title={t("guides.platform.title")}
        description={t("guides.platform.description")}
        actions={
          <>
            <Button variant="outline" onClick={() => setGenerating(true)}>
              <Sparkles className="size-4" />
              {t("guides.platform.generate")}
            </Button>
            <Button
              onClick={() => setEditing({ uuid: null, input: emptyInput() })}
            >
              <Plus className="size-4" />
              {t("guides.platform.new")}
            </Button>
          </>
        }
      />
      <div className="flex flex-wrap gap-2">
        <Input
          className="w-64"
          value={q}
          placeholder={t("guides.platform.search")}
          aria-label={t("guides.platform.search")}
          onChange={(e) => {
            setQ(e.target.value);
            setOffset(0);
          }}
        />
        <Select
          value={surface}
          onValueChange={(v) => {
            setSurface(v);
            setOffset(0);
          }}
        >
          <SelectTrigger
            className="w-32"
            aria-label={t("guides.platform.field.surface")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>{t("guides.all")}</SelectItem>
            <SelectItem value="web">{t("guides.surface.web")}</SelectItem>
            <SelectItem value="mobile">{t("guides.surface.mobile")}</SelectItem>
          </SelectContent>
        </Select>
        <Select
          value={locale}
          onValueChange={(v) => {
            setLocale(v);
            setOffset(0);
          }}
        >
          <SelectTrigger
            className="w-32"
            aria-label={t("guides.platform.field.locale")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>{t("guides.all")}</SelectItem>
            <SelectItem value="tr">Türkçe</SelectItem>
            <SelectItem value="en">English</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-sm">
          <thead className="bg-muted/50 text-muted-foreground text-left text-xs">
            <tr>
              <th className="px-3 py-2">{t("guides.platform.col.title")}</th>
              <th className="px-3 py-2">{t("guides.platform.col.route")}</th>
              <th className="px-3 py-2">{t("guides.platform.col.kind")}</th>
              <th className="px-3 py-2">{t("guides.platform.col.steps")}</th>
              <th className="px-3 py-2">{t("guides.platform.col.source")}</th>
              <th className="px-3 py-2" />
            </tr>
          </thead>
          <tbody className="divide-y">
            {list.isPending || list.isError || !list.data?.items.length ? (
              <tr>
                <td colSpan={6} className="px-6 py-12 text-center">
                  {list.isPending ? (
                    <p role="status" className="text-muted-foreground">
                      {t("common.loading")}
                    </p>
                  ) : list.isError ? (
                    <div role="alert" className="space-y-3">
                      <p className="text-destructive">
                        {t("common.error_generic")}
                      </p>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => void list.refetch()}
                      >
                        {t("common.retry")}
                      </Button>
                    </div>
                  ) : (
                    <div className="text-muted-foreground space-y-2">
                      <BookOpenCheck className="mx-auto size-8" />
                      <p>{t("common.empty_title")}</p>
                      <p className="text-xs">{t("common.empty_description")}</p>
                    </div>
                  )}
                </td>
              </tr>
            ) : null}
            {(list.data?.items ?? []).map((g) => (
              <tr key={g.uuid}>
                <td className="px-3 py-2">
                  <p className="font-medium">{g.title}</p>
                  <p className="text-muted-foreground text-xs">
                    {g.key} · {g.locale}
                    {g.is_published ? "" : ` · ${t("guides.platform.draft")}`}
                  </p>
                </td>
                <td className="px-3 py-2 font-mono text-xs">
                  {g.surface}:{g.route}
                </td>
                <td className="px-3 py-2">
                  <Badge variant="secondary">
                    {g.setup_order > 0
                      ? t("guides.platform.setup_n", { n: g.setup_order })
                      : t(`guides.kind.${g.kind}`)}
                  </Badge>
                </td>
                <td className="px-3 py-2 tabular-nums">{g.steps.length}</td>
                <td className="px-3 py-2 text-xs">
                  {t(`guides.source.${g.source}`)}
                </td>
                <td className="px-3 py-2 text-right whitespace-nowrap">
                  <Button
                    size="icon"
                    variant="ghost"
                    aria-label={t("guides.platform.edit")}
                    onClick={() =>
                      setEditing({ uuid: g.uuid, input: toInput(g) })
                    }
                  >
                    <Pencil className="size-4" />
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    aria-label={t("guides.platform.delete")}
                    disabled={remove.isPending}
                    onClick={async () => {
                      if (
                        await confirmDelete({
                          title: t("guides.platform.delete_confirm"),
                        })
                      ) {
                        remove.mutate(g.uuid);
                      }
                    }}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {total > LIMIT ? (
        <div className="flex items-center justify-end gap-2">
          <span className="text-muted-foreground text-xs">
            {offset + 1}–{Math.min(offset + LIMIT, total)} / {total}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={offset === 0}
            onClick={() => setOffset(Math.max(0, offset - LIMIT))}
          >
            {t("guides.prev")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={offset + LIMIT >= total}
            onClick={() => setOffset(offset + LIMIT)}
          >
            {t("guides.next")}
          </Button>
        </div>
      ) : null}
      {editing ? (
        <GuideEditor
          editing={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            void qc.invalidateQueries({ queryKey: KEY });
          }}
        />
      ) : null}
      <GenerateDialog
        open={generating}
        onOpenChange={setGenerating}
        onDraft={(input) => {
          setGenerating(false);
          setEditing({ uuid: null, input });
        }}
      />
    </div>
  );
}

function GuideEditor({
  editing,
  onClose,
  onSaved,
}: {
  editing: Editing;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useLocale();
  const [input, setInput] = useState<AdminGuideInput>(editing.input);
  const steps = input.steps ?? [];
  const save = useMutation({
    mutationFn: () =>
      editing.uuid
        ? platformGuidesService.update(editing.uuid, input)
        : platformGuidesService.create(input),
    onSuccess: () => {
      appToast.success(t("guides.platform.saved"));
      onSaved();
    },
  });
  const setStep = (i: number, patch: Partial<GuideStep>) =>
    setInput({
      ...input,
      steps: steps.map((s, j) => (j === i ? { ...s, ...patch } : s)),
    });
  const move = (i: number, d: -1 | 1) => {
    const next = [...steps];
    const [s] = next.splice(i, 1);
    next.splice(i + d, 0, s);
    setInput({ ...input, steps: next });
  };

  return (
    <Dialog open onOpenChange={(o) => !o && !save.isPending && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {editing.uuid
              ? t("guides.platform.edit")
              : t("guides.platform.new")}
          </DialogTitle>
          <DialogDescription>
            {t("guides.platform.description")}
          </DialogDescription>
        </DialogHeader>
        {save.isError ? (
          <p
            role="alert"
            className="text-destructive bg-destructive/5 rounded-lg p-3 text-sm"
          >
            {t("common.error_generic")}
          </p>
        ) : null}
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={t("guides.platform.field.key")}>
            <Input
              value={input.key ?? ""}
              disabled={Boolean(editing.uuid)}
              onChange={(e) => setInput({ ...input, key: e.target.value })}
              placeholder="web.inbox"
            />
          </Field>
          <Field label={t("guides.platform.field.locale")}>
            <Select
              value={input.locale ?? "tr"}
              disabled={Boolean(editing.uuid)}
              onValueChange={(v) =>
                setInput({ ...input, locale: v as AdminGuideInput["locale"] })
              }
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="tr">Türkçe</SelectItem>
                <SelectItem value="en">English</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <Field label={t("guides.platform.field.surface")}>
            <Select
              value={input.surface}
              onValueChange={(v) =>
                setInput({ ...input, surface: v as AdminGuideInput["surface"] })
              }
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="web">{t("guides.surface.web")}</SelectItem>
                <SelectItem value="mobile">
                  {t("guides.surface.mobile")}
                </SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <Field label={t("guides.platform.field.route")}>
            <Input
              value={input.route}
              onChange={(e) => setInput({ ...input, route: e.target.value })}
              placeholder="/teams/[uuid]"
            />
          </Field>
          <Field label={t("guides.platform.field.kind")}>
            <Select
              value={input.kind ?? "page"}
              onValueChange={(v) =>
                setInput({ ...input, kind: v as AdminGuideInput["kind"] })
              }
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="page">{t("guides.kind.page")}</SelectItem>
                <SelectItem value="tour">{t("guides.kind.tour")}</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label={t("guides.platform.field.order")}>
              <Input
                type="number"
                value={input.order ?? 0}
                onChange={(e) =>
                  setInput({ ...input, order: Number(e.target.value) })
                }
              />
            </Field>
            <Field label={t("guides.platform.field.setup_order")}>
              <Input
                type="number"
                min={0}
                max={100}
                value={input.setup_order ?? 0}
                onChange={(e) =>
                  setInput({ ...input, setup_order: Number(e.target.value) })
                }
              />
            </Field>
          </div>
          <Field label={t("guides.platform.field.title")} wide>
            <Input
              value={input.title}
              maxLength={200}
              onChange={(e) => setInput({ ...input, title: e.target.value })}
            />
          </Field>
          <Field label={t("guides.platform.field.summary")} wide>
            <Textarea
              rows={3}
              maxLength={2000}
              value={input.summary ?? ""}
              onChange={(e) => setInput({ ...input, summary: e.target.value })}
            />
          </Field>
          <div className="flex items-center gap-2 sm:col-span-2">
            <Switch
              id="guide-published"
              checked={input.is_published ?? true}
              onCheckedChange={(v) => setInput({ ...input, is_published: v })}
            />
            <Label htmlFor="guide-published" className="font-normal">
              {t("guides.platform.field.published")}
            </Label>
          </div>
        </div>
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold">
              {t("guides.platform.steps")}
            </h3>
            <Button
              size="sm"
              variant="outline"
              disabled={steps.length >= 20}
              onClick={() =>
                setInput({
                  ...input,
                  steps: [
                    ...steps,
                    {
                      id: crypto.randomUUID(),
                      target: "",
                      title: "",
                      body: "",
                      placement: "auto",
                    },
                  ],
                })
              }
            >
              <Plus className="size-4" />
              {t("guides.platform.add_step")}
            </Button>
          </div>
          {steps.length === 0 ? (
            <p className="text-muted-foreground rounded-xl border border-dashed p-6 text-center text-sm">
              {t("common.empty_description")}
            </p>
          ) : null}
          {steps.map((s, i) => (
            <div key={i} className="space-y-2 rounded-lg border p-3">
              <div className="grid gap-2 sm:grid-cols-[1fr_1fr_8rem_auto]">
                <Input
                  value={s.title}
                  aria-label={`${i + 1}. ${t("guides.platform.field.step_title")}`}
                  placeholder={t("guides.platform.field.step_title")}
                  onChange={(e) => setStep(i, { title: e.target.value })}
                />
                <Input
                  value={s.target}
                  aria-label={`${i + 1}. ${t("guides.platform.field.target")}`}
                  placeholder={t("guides.platform.field.target")}
                  className="font-mono text-xs"
                  onChange={(e) => setStep(i, { target: e.target.value })}
                />
                <Select
                  value={s.placement}
                  onValueChange={(v) =>
                    setStep(i, { placement: v as GuideStep["placement"] })
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {PLACEMENTS.map((p) => (
                      <SelectItem key={p} value={p}>
                        {p}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <div className="flex">
                  <Button
                    size="icon"
                    variant="ghost"
                    disabled={i === 0}
                    aria-label={t("guides.platform.move_up")}
                    onClick={() => move(i, -1)}
                  >
                    <ArrowUp className="size-4" />
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    disabled={i === steps.length - 1}
                    aria-label={t("guides.platform.move_down")}
                    onClick={() => move(i, 1)}
                  >
                    <ArrowDown className="size-4" />
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    aria-label={t("guides.platform.delete")}
                    onClick={() =>
                      setInput({
                        ...input,
                        steps: steps.filter((_, j) => j !== i),
                      })
                    }
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              </div>
              <Textarea
                rows={2}
                value={s.body}
                aria-label={`${i + 1}. ${t("guides.platform.field.body")}`}
                placeholder={t("guides.platform.field.body")}
                onChange={(e) => setStep(i, { body: e.target.value })}
              />
            </div>
          ))}
          <p className="text-muted-foreground text-xs">
            {t("guides.platform.target_hint")}
          </p>
        </div>
        <DialogFooter>
          <Button variant="ghost" disabled={save.isPending} onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={save.isPending || !input.title.trim() || !input.route}
            onClick={() => save.mutate()}
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function GenerateDialog({
  open,
  onOpenChange,
  onDraft,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDraft: (input: AdminGuideInput) => void;
}) {
  const { t } = useLocale();
  const [route, setRoute] = useState("/");
  const [locale, setLocale] = useState<"tr" | "en">("tr");
  const [description, setDescription] = useState("");
  const [strings, setStrings] = useState("");
  const [targets, setTargets] = useState("page-header, page-actions");
  const generate = useMutation({
    mutationFn: () =>
      platformGuidesService.generate({
        surface: "web",
        route,
        locale,
        description,
        strings: strings
          .split("\n")
          .map((s) => s.trim())
          .filter(Boolean)
          .slice(0, 200),
        targets: targets
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean)
          .slice(0, 50),
      }),
    onSuccess: (res) =>
      onDraft({ ...res.draft, order: 100, setup_order: 0, is_published: true }),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("guides.platform.generate")}</DialogTitle>
          <DialogDescription>
            {t("guides.platform.generate_hint")}
          </DialogDescription>
        </DialogHeader>
        {generate.isError ? (
          <p role="alert" className="text-destructive text-sm">
            {t("common.error_generic")}
          </p>
        ) : null}
        <div className="space-y-3">
          <p className="text-muted-foreground text-sm">
            {t("guides.platform.generate_hint")}
          </p>
          <div className="grid grid-cols-[1fr_8rem] gap-2">
            <Field label={t("guides.platform.field.route")}>
              <Input value={route} onChange={(e) => setRoute(e.target.value)} />
            </Field>
            <Field label={t("guides.platform.field.locale")}>
              <Select
                value={locale}
                onValueChange={(v) => setLocale(v as "tr" | "en")}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="tr">Türkçe</SelectItem>
                  <SelectItem value="en">English</SelectItem>
                </SelectContent>
              </Select>
            </Field>
          </div>
          <Field label={t("guides.platform.field.description")}>
            <Textarea
              rows={2}
              maxLength={2000}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </Field>
          <Field label={t("guides.platform.field.strings")}>
            <Textarea
              rows={4}
              value={strings}
              onChange={(e) => setStrings(e.target.value)}
            />
          </Field>
          <Field label={t("guides.platform.field.targets")}>
            <Input
              value={targets}
              className="font-mono text-xs"
              onChange={(e) => setTargets(e.target.value)}
            />
          </Field>
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={generate.isPending || !route.startsWith("/")}
            onClick={() => generate.mutate()}
          >
            <Sparkles className="size-4" />
            {generate.isPending
              ? t("guides.platform.generating")
              : t("guides.platform.generate")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Field({
  label,
  wide,
  children,
}: {
  label: string;
  wide?: boolean;
  children: ReactNode;
}) {
  const id = useId();
  const labelControl = (child: ReactNode): ReactNode => {
    if (!isValidElement(child)) return child;
    const element = child as ReactElement<{
      id?: string;
      children?: ReactNode;
      "aria-labelledby"?: string;
    }>;
    if (
      element.type === Input ||
      element.type === Textarea ||
      element.type === SelectTrigger
    ) {
      return cloneElement(element, { id, "aria-labelledby": `${id}-label` });
    }
    if (element.type === Select)
      return cloneElement(element, {
        children: Array.isArray(element.props.children)
          ? element.props.children.map(labelControl)
          : labelControl(element.props.children),
      });
    return element;
  };
  return (
    <div className={wide ? "space-y-1.5 sm:col-span-2" : "space-y-1.5"}>
      <Label id={`${id}-label`} htmlFor={id}>
        {label}
      </Label>
      {labelControl(children)}
    </div>
  );
}
