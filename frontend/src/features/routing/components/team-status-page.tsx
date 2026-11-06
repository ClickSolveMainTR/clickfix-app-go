"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Activity, Pencil, Plus, Trash2 } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { Loading } from "@/components/common/loading";
import { EntityTable } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn, DataTable } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { ChannelGlyph } from "@/features/inbox/components/channel-glyph";
import { teamsService } from "@/features/teams/services/teams.service";
import { isApiError } from "@/lib/api";
import { cn, datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  routingService,
  type AgentLoad,
  type RoutingEvent,
  type RoutingRule,
  type RoutingSettings,
} from "../services/routing.service";

const CHANNELS = [
  "widget",
  "email",
  "whatsapp",
  "instagram",
  "messenger",
  "telegram",
];
const STATUS_DOT: Record<string, string> = {
  online: "bg-emerald-500",
  away: "bg-amber-500",
  busy: "bg-red-500",
  offline: "bg-muted-foreground/40",
};

/**
 * Team operations (CF-53, CF-54): live agent status and load for
 * supervisors, routing settings and rules, capacity limits and the routing
 * audit trail.
 */
export function TeamStatusPage() {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const perms = org?.permissions ?? [];
  const canSupervise = perms.includes(Permission.TenantConversationsReadAll);
  const canRead = perms.includes(Permission.TenantTeamsRead);
  const canWrite = perms.includes(Permission.TenantTeamsWrite);
  if (!org) return null;
  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Activity className="size-7" />}
        title={t("routing.title")}
        description={t("routing.description")}
      />
      <Tabs defaultValue={canSupervise ? "live" : "routing"}>
        <TabsList>
          {canSupervise && (
            <TabsTrigger value="live">{t("routing.tabs.live")}</TabsTrigger>
          )}
          {canRead && (
            <TabsTrigger value="routing">
              {t("routing.tabs.routing")}
            </TabsTrigger>
          )}
          {canRead && (
            <TabsTrigger value="history">
              {t("routing.tabs.history")}
            </TabsTrigger>
          )}
        </TabsList>
        {canSupervise && (
          <TabsContent value="live" className="mt-4">
            <LiveTab orgSlug={org.slug} canWrite={canWrite} />
          </TabsContent>
        )}
        {canRead && (
          <TabsContent value="routing" className="mt-4">
            <RoutingTab orgSlug={org.slug} canWrite={canWrite} />
          </TabsContent>
        )}
        {canRead && (
          <TabsContent value="history" className="mt-4">
            <HistoryTab orgSlug={org.slug} />
          </TabsContent>
        )}
      </Tabs>
    </div>
  );
}

function useOnError() {
  const { t } = useLocale();
  return (error: unknown) =>
    appToast.error(
      isApiError(error) ? error.message : t("routing.toast.failed"),
    );
}

function LiveTab({
  orgSlug,
  canWrite,
}: {
  orgSlug: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useOnError();
  const key = ["tenant", orgSlug, "capacity"] as const;
  const { data, isLoading } = useQuery({
    queryKey: key,
    queryFn: routingService.capacity,
    refetchInterval: 15_000,
  });
  const save = useMutation({
    mutationFn: (p: { user: string | null; max: string }) =>
      routingService.setLimits(
        p.user,
        p.max.trim() === "" ? {} : { "*": Math.max(0, Number(p.max)) },
      ),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("routing.toast.saved"));
    },
    onError,
  });
  const { mutate: saveLimit } = save;

  const columns = useMemo(
    () =>
      [
        createColumn<AgentLoad>({
          id: "agent",
          accessorFn: (row) => `${row.name} ${row.skills.join(" ")}`,
          labelKey: "routing.live.agent",
          gridPrimary: true,
          enableHiding: false,
          cell: ({ row }) => (
            <div>
              <div className="font-medium">{row.original.name}</div>
              {row.original.skills.length > 0 && (
                <div className="text-muted-foreground text-xs font-normal">
                  {row.original.skills.join(", ")}
                </div>
              )}
            </div>
          ),
        }),
        createColumn<AgentLoad>({
          accessorKey: "status",
          labelKey: "routing.live.status",
          searchable: false,
          cell: ({ row }) => {
            const a = row.original;
            return (
              <div>
                <span className="inline-flex items-center gap-1.5">
                  <span
                    className={cn("size-2 rounded-full", STATUS_DOT[a.status])}
                  />
                  {t(`routing.status.${a.status}`)}
                </span>
                {a.last_seen_at && a.status === "offline" && (
                  <div className="text-muted-foreground text-xs">
                    {datetime(a.last_seen_at, "dd.MM HH:mm")}
                  </div>
                )}
              </div>
            );
          },
        }),
        createColumn<AgentLoad>({
          id: "teams",
          accessorFn: (row) => row.teams.map((x) => x.name).join(", "),
          labelKey: "routing.live.teams",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs">
              {row.original.teams.map((x) => x.name).join(", ") || "—"}
            </span>
          ),
        }),
        createColumn<AgentLoad>({
          accessorKey: "open_total",
          labelKey: "routing.live.open",
          searchable: false,
          cell: ({ row }) => <OpenLoad agent={row.original} />,
        }),
        createColumn<AgentLoad>({
          id: "limit",
          accessorFn: (row) => row.limits["*"] ?? Number.POSITIVE_INFINITY,
          labelKey: "routing.live.limit",
          searchable: false,
          cell: ({ row }) => (
            <LimitCell
              agent={row.original}
              canWrite={canWrite}
              onSave={(max, done) =>
                saveLimit(
                  { user: row.original.user_uuid, max },
                  { onSuccess: done },
                )
              }
            />
          ),
        }),
      ] as ColumnDef<AgentLoad, unknown>[],
    [canWrite, saveLimit, t],
  );

  if (isLoading || !data) return <Loading label={t("common.loading")} />;
  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-3">
        <Stat label={t("routing.live.online")} value={data.online} />
        <Stat label={t("routing.live.queue")} value={data.queue} />
        <Stat
          label={t("routing.live.routing")}
          value={data.settings.enabled ? t("routing.on") : t("routing.off")}
        />
      </div>
      <DataTable
        columns={columns}
        data={data.agents}
        getRowId={(row) => row.user_uuid}
        initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
        features={{
          persistKey: "tenant-team-status-live-v1",
          // The capacity snapshot holds every agent: search, sort and paging
          // run in the browser.
          columnFilters: false,
          rowSelection: false,
        }}
      />
      {canWrite && <DefaultLimits orgSlug={orgSlug} />}
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string | number }) {
  return (
    <Card>
      <CardContent className="py-4">
        <div className="text-muted-foreground text-xs">{label}</div>
        <div className="text-2xl font-semibold tabular-nums">{value}</div>
      </CardContent>
    </Card>
  );
}

/** Open conversations of an agent, per channel, with the load bar. */
function OpenLoad({ agent: a }: { agent: AgentLoad }) {
  const { t } = useLocale();
  const pct = a.utilization != null ? Math.round(a.utilization * 100) : null;
  return (
    <div className="tabular-nums">
      <span className="font-medium">{a.open_total}</span>
      {Object.keys(a.open).length > 1 && (
        <span className="text-muted-foreground ml-1 text-xs">
          (
          {Object.entries(a.open)
            .map(([k, v]) => `${t(`routing.channel.${k}`)} ${v}`)
            .join(", ")}
          )
        </span>
      )}
      {pct != null && (
        <div className="bg-muted mt-1 h-1.5 w-24 overflow-hidden rounded-full">
          <div
            className={cn(
              "h-full",
              pct >= 100
                ? "bg-red-500"
                : pct >= 80
                  ? "bg-amber-500"
                  : "bg-emerald-500",
            )}
            style={{ width: `${Math.min(100, pct)}%` }}
          />
        </div>
      )}
    </div>
  );
}

/** Capacity limit of an agent; the pencil opens an inline editor. */
function LimitCell({
  agent: a,
  canWrite,
  onSave,
}: {
  agent: AgentLoad;
  canWrite: boolean;
  onSave: (max: string, done: () => void) => void;
}) {
  const { t } = useLocale();
  const [editing, setEditing] = useState(false);
  const [limit, setLimit] = useState("");
  const max = a.limits["*"];
  if (editing) {
    return (
      <div className="flex items-center gap-1">
        <Input
          className="h-8 w-20"
          type="number"
          min={0}
          max={1000}
          value={limit}
          placeholder="∞"
          onChange={(e) => setLimit(e.target.value)}
        />
        <Button
          size="sm"
          onClick={() => onSave(limit, () => setEditing(false))}
        >
          {t("common.save")}
        </Button>
        <Button size="sm" variant="ghost" onClick={() => setEditing(false)}>
          {t("common.cancel")}
        </Button>
      </div>
    );
  }
  return (
    <div className="flex items-center gap-1">
      <span className="tabular-nums">{max ?? "∞"}</span>
      {canWrite && (
        <Button
          size="icon"
          variant="ghost"
          aria-label={t("routing.live.edit_limit")}
          onClick={() => {
            setLimit(max != null ? String(max) : "");
            setEditing(true);
          }}
        >
          <Pencil className="size-3.5" />
        </Button>
      )}
    </div>
  );
}

function DefaultLimits({ orgSlug }: { orgSlug: string }) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useOnError();
  const key = ["tenant", orgSlug, "capacity-limits"] as const;
  const { data } = useQuery({ queryKey: key, queryFn: routingService.limits });
  const [draft, setDraft] = useState<Record<string, string> | null>(null);
  const values =
    draft ??
    Object.fromEntries(
      ["*", ...CHANNELS].map((k) => [
        k,
        data?.defaults[k] != null ? String(data.defaults[k]) : "",
      ]),
    );
  const save = useMutation({
    mutationFn: () =>
      routingService.setLimits(
        null,
        Object.fromEntries(
          Object.entries(values)
            .filter(([, v]) => v.trim() !== "")
            .map(([k, v]) => [k, Math.max(0, Number(v))]),
        ),
      ),
    onSuccess: async () => {
      setDraft(null);
      await qc.invalidateQueries({ queryKey: ["tenant", orgSlug] });
      appToast.success(t("routing.toast.saved"));
    },
    onError,
  });
  return (
    <Card>
      <CardContent className="space-y-3 py-4">
        <div>
          <h3 className="text-sm font-semibold">{t("routing.limits.title")}</h3>
          <p className="text-muted-foreground text-xs">
            {t("routing.limits.hint")}
          </p>
        </div>
        <div className="grid gap-2 sm:grid-cols-4">
          {["*", ...CHANNELS].map((k) => (
            <label key={k} className="grid gap-1 text-xs">
              <span className="flex items-center gap-1.5">
                {k === "*" ? null : (
                  <ChannelGlyph type={k} className="size-3.5" />
                )}
                {k === "*"
                  ? t("routing.limits.all")
                  : t(`routing.channel.${k}`)}
              </span>
              <Input
                type="number"
                min={0}
                max={1000}
                value={values[k]}
                placeholder="∞"
                onChange={(e) => setDraft({ ...values, [k]: e.target.value })}
              />
            </label>
          ))}
        </div>
        <Button
          size="sm"
          disabled={!draft || save.isPending}
          onClick={() => save.mutate()}
        >
          {t("common.save")}
        </Button>
      </CardContent>
    </Card>
  );
}

function RoutingTab({
  orgSlug,
  canWrite,
}: {
  orgSlug: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useOnError();
  const settings = useQuery({
    queryKey: ["tenant", orgSlug, "routing-settings"],
    queryFn: routingService.settings,
  });
  const rules = useQuery({
    queryKey: ["tenant", orgSlug, "routing-rules"],
    queryFn: routingService.rules,
  });
  const teams = useQuery({
    queryKey: ["tenant", orgSlug, "routing-teams"],
    queryFn: () => teamsService.list(orgSlug, { limit: 100, offset: 0 }),
  });
  const saveSettings = useMutation({
    mutationFn: (body: RoutingSettings) => routingService.saveSettings(body),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["tenant", orgSlug] });
      appToast.success(t("routing.toast.saved"));
    },
    onError,
  });
  const removeRule = useMutation({
    mutationFn: (uuid: string) => routingService.removeRule(uuid),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: ["tenant", orgSlug, "routing-rules"] }),
    onError,
  });
  const [editing, setEditing] = useState<RoutingRule | "new" | null>(null);
  if (!settings.data) return <Loading label={t("common.loading")} />;
  const s = settings.data;
  const set = (patch: Partial<RoutingSettings>) =>
    saveSettings.mutate({ ...s, ...patch });
  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="grid gap-4 py-4 sm:grid-cols-2">
          <Toggle
            label={t("routing.settings.enabled")}
            hint={t("routing.settings.enabled_hint")}
            checked={s.enabled}
            disabled={!canWrite}
            onChange={(v) => set({ enabled: v })}
          />
          <div className="grid gap-1.5">
            <span className="text-sm">{t("routing.settings.strategy")}</span>
            <Select
              value={s.strategy}
              disabled={!canWrite}
              onValueChange={(v) =>
                set({ strategy: v as RoutingSettings["strategy"] })
              }
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="round_robin">
                  {t("routing.strategy.round_robin")}
                </SelectItem>
                <SelectItem value="load_balanced">
                  {t("routing.strategy.load_balanced")}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <Toggle
            label={t("routing.settings.respect_capacity")}
            checked={s.respect_capacity}
            disabled={!canWrite}
            onChange={(v) => set({ respect_capacity: v })}
          />
          <Toggle
            label={t("routing.settings.vip")}
            hint={t("routing.settings.vip_hint")}
            checked={s.vip_account_owner}
            disabled={!canWrite}
            onChange={(v) => set({ vip_account_owner: v })}
          />
          <Toggle
            label={t("routing.settings.offline")}
            hint={t("routing.settings.offline_hint")}
            checked={s.reassign_on_offline}
            disabled={!canWrite}
            onChange={(v) => set({ reassign_on_offline: v })}
          />
          <div className="grid gap-1.5">
            <span className="text-sm">{t("routing.settings.unanswered")}</span>
            <Input
              type="number"
              min={1}
              max={1440}
              disabled={!canWrite}
              defaultValue={s.unanswered_minutes ?? ""}
              placeholder={t("routing.settings.unanswered_never")}
              onBlur={(e) => {
                const v = e.target.value.trim();
                const next = v === "" ? null : Number(v);
                if (next !== s.unanswered_minutes)
                  set({ unanswered_minutes: next });
              }}
            />
          </div>
        </CardContent>
      </Card>

      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold">{t("routing.rules.title")}</h3>
          <p className="text-muted-foreground text-xs">
            {t("routing.rules.hint")}
          </p>
        </div>
        {canWrite && (
          <Button size="sm" onClick={() => setEditing("new")}>
            <Plus className="size-4" />
            {t("routing.rules.new")}
          </Button>
        )}
      </div>
      {(rules.data ?? []).length === 0 && (
        <p className="text-muted-foreground text-sm">
          {t("routing.rules.empty")}
        </p>
      )}
      {(rules.data ?? []).map((r) => (
        <Card key={r.uuid}>
          <CardContent className="flex items-center gap-3 py-3">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <span className="font-medium">{r.name}</span>
                {!r.is_active && (
                  <Badge variant="secondary">
                    {t("routing.rules.inactive")}
                  </Badge>
                )}
              </div>
              <div className="text-muted-foreground text-xs">
                {[
                  r.conditions.channel_types
                    ?.map((c) => t(`routing.channel.${c}`))
                    .join(", "),
                  r.conditions.priorities
                    ?.map((p) => t(`inbox.priority.${p}`))
                    .join(", "),
                ]
                  .filter(Boolean)
                  .join(" · ") || t("routing.rules.all")}
                {" → "}
                {r.team?.name ?? t("routing.rules.any_team")}
                {r.skills.length > 0 &&
                  ` · ${t("routing.rules.skills")}: ${r.skills.join(", ")}`}
              </div>
            </div>
            {canWrite && (
              <div className="flex gap-1">
                <Button
                  size="icon"
                  variant="ghost"
                  aria-label={t("routing.rules.edit")}
                  onClick={() => setEditing(r)}
                >
                  <Pencil className="size-4" />
                </Button>
                <Button
                  disabled={removeRule.isPending}
                  size="icon"
                  variant="ghost"
                  aria-label={t("routing.rules.delete")}
                  onClick={() => removeRule.mutate(r.uuid)}
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
            )}
          </CardContent>
        </Card>
      ))}
      {editing && (
        <RuleEditor
          rule={editing === "new" ? null : editing}
          teams={(teams.data?.items ?? []).map((x) => ({
            uuid: x.uuid,
            name: x.name,
          }))}
          onDone={async () => {
            setEditing(null);
            await qc.invalidateQueries({
              queryKey: ["tenant", orgSlug, "routing-rules"],
            });
          }}
        />
      )}
    </div>
  );
}

function Toggle({
  label,
  hint,
  checked,
  disabled,
  onChange,
}: {
  label: string;
  hint?: string;
  checked: boolean;
  disabled?: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <label className="flex items-start gap-3">
      <Switch
        checked={checked}
        disabled={disabled}
        onCheckedChange={onChange}
      />
      <span>
        <span className="block text-sm">{label}</span>
        {hint && (
          <span className="text-muted-foreground block text-xs">{hint}</span>
        )}
      </span>
    </label>
  );
}

const ANY = "__any__";

function RuleEditor({
  rule,
  teams,
  onDone,
}: {
  rule: RoutingRule | null;
  teams: { uuid: string; name: string }[];
  onDone: () => void;
}) {
  const { t } = useLocale();
  const onError = useOnError();
  const [name, setName] = useState(rule?.name ?? "");
  const [channels, setChannels] = useState<string[]>(
    rule?.conditions.channel_types ?? [],
  );
  const [team, setTeam] = useState(rule?.team?.uuid ?? ANY);
  const [skills, setSkills] = useState((rule?.skills ?? []).join(", "));
  const [active, setActive] = useState(rule?.is_active ?? true);
  const save = useMutation({
    mutationFn: () => {
      const body = {
        name: name.trim(),
        is_active: active,
        conditions: { channel_types: channels },
        team_uuid: team === ANY ? null : team,
        skills: skills
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
      };
      return rule
        ? routingService.updateRule(rule.uuid, body)
        : routingService.createRule(body);
    },
    onSuccess: onDone,
    onError,
  });
  return (
    <Card className="border-primary/40">
      <CardContent className="grid gap-3 py-4">
        <Input
          value={name}
          maxLength={120}
          placeholder={t("routing.rules.name")}
          onChange={(e) => setName(e.target.value)}
        />
        <div className="flex flex-wrap gap-1.5">
          {CHANNELS.map((c) => (
            <button
              key={c}
              type="button"
              className={cn(
                "flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs",
                channels.includes(c)
                  ? "border-primary bg-primary/10"
                  : "text-muted-foreground",
              )}
              onClick={() =>
                setChannels((l) =>
                  l.includes(c) ? l.filter((x) => x !== c) : [...l, c],
                )
              }
            >
              <ChannelGlyph type={c} className="size-3.5" />
              {t(`routing.channel.${c}`)}
            </button>
          ))}
        </div>
        <div className="grid gap-2 sm:grid-cols-2">
          <Select value={team} onValueChange={setTeam}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ANY}>{t("routing.rules.any_team")}</SelectItem>
              {teams.map((x) => (
                <SelectItem key={x.uuid} value={x.uuid}>
                  {x.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Input
            value={skills}
            placeholder={t("routing.rules.skills_placeholder")}
            onChange={(e) => setSkills(e.target.value)}
          />
        </div>
        <div className="flex items-center gap-3">
          <label className="flex items-center gap-2 text-sm">
            <Switch checked={active} onCheckedChange={setActive} />
            {t("routing.rules.active")}
          </label>
          <div className="ml-auto flex gap-2">
            <Button variant="outline" size="sm" onClick={onDone}>
              {t("common.cancel")}
            </Button>
            <Button
              size="sm"
              disabled={!name.trim() || save.isPending}
              onClick={() => save.mutate()}
            >
              {t("common.save")}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

const EMPTY_EVENTS: RoutingEvent[] = [];

function HistoryTab({ orgSlug }: { orgSlug: string }) {
  const { t } = useLocale();
  const events = useQuery({
    queryKey: ["tenant", orgSlug, "routing-events"],
    queryFn: routingService.events,
    refetchInterval: 30_000,
  });

  const columns = useMemo(
    () =>
      [
        createColumn<RoutingEvent>({
          accessorKey: "created_at",
          labelKey: "routing.history.columns.when",
          searchable: false,
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs tabular-nums">
              {datetime(row.original.created_at, "dd.MM HH:mm:ss")}
            </span>
          ),
        }),
        createColumn<RoutingEvent>({
          accessorKey: "number",
          labelKey: "routing.history.columns.conversation",
          gridPrimary: true,
          enableHiding: false,
          cell: ({ row }) => (
            <Link
              className="font-medium hover:underline"
              href={`/inbox?c=${row.original.conversation_uuid}`}
            >
              #{row.original.number}
            </Link>
          ),
        }),
        createColumn<RoutingEvent>({
          accessorKey: "reason",
          labelKey: "routing.history.columns.reason",
          searchable: false,
          cell: ({ row }) => (
            <Badge variant="outline">
              {t(`routing.reason.${row.original.reason}`)}
            </Badge>
          ),
        }),
        createColumn<RoutingEvent>({
          id: "route",
          accessorFn: (row) =>
            [row.from?.name, row.to?.name, row.team].filter(Boolean).join(" "),
          labelKey: "routing.history.columns.route",
          enableSorting: false,
          cell: ({ row }) => {
            const e = row.original;
            return (
              <span className="text-muted-foreground">
                {e.from ? `${e.from.name} → ` : ""}
                {e.to?.name ?? t("routing.history.queue")}
                {e.team ? ` · ${e.team}` : ""}
              </span>
            );
          },
        }),
      ] as ColumnDef<RoutingEvent, unknown>[],
    [t],
  );

  return (
    <EntityTable
      columns={columns}
      data={events.data ?? EMPTY_EVENTS}
      // Events carry no id of their own.
      getRowId={(row, index) =>
        `${row.created_at}-${row.conversation_uuid}-${index}`
      }
      isLoading={events.isLoading}
      isError={events.isError}
      onRetry={() => void events.refetch()}
      emptyDescription={t("routing.history.empty")}
      // The endpoint returns the recent events in one response: search, sort
      // and paging run in the browser.
      manual={{ filtering: false, sorting: false, pagination: false }}
      initialState={{ pagination: { pageIndex: 0, pageSize: 25 } }}
      features={{
        persistKey: "tenant-routing-history-v1",
        columnFilters: false,
        rowSelection: false,
      }}
    />
  );
}
