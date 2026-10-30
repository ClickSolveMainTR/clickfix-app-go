"use client";

// Ported from otopoly-go frontend/src/features/calendar/components/calendar-page.tsx:
// same views, toolbar, layer toggles, keyboard shortcuts and drag-and-drop.
// ClickFix changes: appointment / shift / time-off layers, host / team /
// type / status filters, booking dialog instead of free events, realtime
// refresh on `appointment.changed`, tenant-scoped requests.

import {
  addDays,
  addMonths,
  addWeeks,
  endOfWeek,
  format,
  isSameDay,
  isSameMonth,
  parseISO,
  startOfWeek,
  subDays,
} from "date-fns";
import {
  ChevronLeft,
  ChevronRight,
  Filter,
  Keyboard,
  Layers,
  Plus,
} from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { AppointmentDialog } from "@/features/calendar/components/appointment-dialog";
import { BookDialog } from "@/features/calendar/components/book-dialog";
import { ItemDialog } from "@/features/calendar/components/item-dialog";
import { CalendarViews } from "@/features/calendar/components/views/calendar-views";
import {
  COLOR_CLASSES,
  visibleRange,
  type CalendarView,
  type CalendarViewItem,
} from "@/features/calendar/components/views/types";
import { useCalendarFilterOptions } from "@/features/calendar/hooks/use-filter-options";
import {
  useCalendarFeed,
  useCalendarLive,
  useCalendarMeta,
  useCalendarMutations,
} from "@/features/calendar/hooks/use-calendar";
import { localDay, toViewItem } from "@/features/calendar/lib/feed-items";
import { LAYER_DEFS, STATUS_KEYS } from "@/features/calendar/lib/layers";
import {
  APPOINTMENT_STATUSES,
  CALENDAR_LAYERS,
  type AppointmentStatus,
  type CalendarFeedItem,
  type CalendarFilters,
  type CalendarLayer,
} from "@/features/calendar/types";
import { useLocalStorage } from "@/hooks/use-local-storage";
import { cn } from "@/lib/utils";
import { dateFnsLocale, dayPickerLocale } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const VIEWS: CalendarView[] = ["month", "week", "day", "agenda"];
const VIEW_LABEL_KEYS: Record<CalendarView, string> = {
  month: "calendar.views.month",
  week: "calendar.views.week",
  day: "calendar.views.day",
  agenda: "calendar.views.agenda",
};
const isView = (v: unknown): v is CalendarView =>
  typeof v === "string" && (VIEWS as string[]).includes(v);

const DAY_RE = /^\d{4}-\d{2}-\d{2}$/;
const ALL = "__all";

/** Keyboard shortcuts (shown in the help popover). */
const SHORTCUTS: { keys: string[]; labelKey: string }[] = [
  { keys: ["T"], labelKey: "calendar.shortcuts.today" },
  { keys: ["←", "→"], labelKey: "calendar.shortcuts.prev_next" },
  { keys: ["1", "2", "3", "4"], labelKey: "calendar.shortcuts.views" },
  { keys: ["N"], labelKey: "calendar.shortcuts.new_appointment" },
];

const EMPTY_FILTERS: CalendarFilters = {
  hostUuids: [],
  teamUuid: "",
  typeUuids: [],
  statuses: [],
};

function shift(view: CalendarView, date: Date, dir: 1 | -1) {
  switch (view) {
    case "month":
      return addMonths(date, dir);
    case "week":
      return addWeeks(date, dir);
    case "day":
      return addDays(date, dir);
    case "agenda":
      return dir > 0 ? addDays(date, 30) : subDays(date, 30);
  }
}

function isTypingTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return false;
  return (
    target.isContentEditable ||
    ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName) ||
    target.closest("[role=dialog],[role=menu],[role=listbox]") !== null
  );
}

export function CalendarPage() {
  const { t, locale } = useLocale();
  const dfLocale = dateFnsLocale(locale);
  const { hasPermission } = usePermission();
  const canRead = hasPermission(permissions.tenant.appointments.read);
  const canWrite = hasPermission(permissions.tenant.appointments.write);
  const router = useRouter();
  const searchParams = useSearchParams();

  // Remembered per browser (useLocalStorage tolerates blocked storage).
  const [storedView, setView] = useLocalStorage<CalendarView>(
    "calendar:view",
    "week",
  );
  const [hidden, setHidden] = useLocalStorage<CalendarLayer[]>(
    "calendar:hidden-layers",
    [],
  );
  const [storedFilters, setFilters] = useLocalStorage<CalendarFilters>(
    "calendar:filters",
    EMPTY_FILTERS,
  );
  const filters = { ...EMPTY_FILTERS, ...(storedFilters ?? {}) };
  const view = isView(storedView) ? storedView : "week";
  const [date, setDate] = useState(() => {
    const d = searchParams.get("date");
    return d && DAY_RE.test(d) ? localDay(d) : new Date();
  });
  const [booking, setBooking] = useState<{ day: Date | null } | null>(null);
  const [selected, setSelected] = useState<{
    item: CalendarFeedItem;
    view: CalendarViewItem;
  } | null>(null);
  const linked = searchParams.get("appointment");
  const [openUuid, setOpenUuid] = useState<string | null>(linked);
  const { move } = useCalendarMutations();
  useCalendarLive(canRead);

  // Deep links: ?date=YYYY-MM-DD&view=day and ?appointment=<uuid>.
  const linkedView = searchParams.get("view");
  useEffect(() => {
    if (isView(linkedView)) setView(linkedView);
  }, [linkedView, setView]);
  const [handledLink, setHandledLink] = useState<string | null>(null);
  if (linked && handledLink !== linked) {
    setHandledLink(linked);
    setOpenUuid(linked);
  }

  const meta = useCalendarMeta(canRead);
  const options = useCalendarFilterOptions(canRead);
  const available = useMemo<CalendarLayer[]>(
    () => meta.data?.layers.map((l) => l.key) ?? ["appointments"],
    [meta.data],
  );
  const hiddenSet = useMemo(
    () => new Set(Array.isArray(hidden) ? hidden : []),
    [hidden],
  );
  const layers = useMemo(
    () =>
      CALENDAR_LAYERS.filter((l) => available.includes(l) && !hiddenSet.has(l)),
    [available, hiddenSet],
  );

  const range = useMemo(() => visibleRange(view, date), [view, date]);
  const from = format(range.from, "yyyy-MM-dd");
  const to = format(range.to, "yyyy-MM-dd");
  const feedQuery = useCalendarFeed(from, to, layers, canRead, filters);
  const feed = feedQuery.data;

  const { items, byId } = useMemo(() => {
    const map = new Map<string, CalendarFeedItem>();
    const out: CalendarViewItem[] = [];
    if (layers.length === 0) return { items: out, byId: map };
    for (const item of feed?.items ?? []) {
      if (!layers.includes(item.layer)) continue;
      map.set(item.id, item);
      out.push(toViewItem(item));
    }
    return { items: out, byId: map };
  }, [feed, layers]);

  const todayItems = useMemo(() => {
    const now = new Date();
    return items
      .filter((i) => !i.muted)
      .filter((i) =>
        i.allDay ? i.start <= now && now < i.end : isSameDay(i.start, now),
      )
      .filter((i) => byId.get(i.id)?.layer === "appointments")
      .sort((a, b) => a.start.getTime() - b.start.getTime());
  }, [items, byId]);

  const title = useMemo(() => {
    switch (view) {
      case "month":
        return format(date, "LLLL yyyy", { locale: dfLocale });
      case "week": {
        const s = startOfWeek(date, { weekStartsOn: 1 });
        const e = endOfWeek(date, { weekStartsOn: 1 });
        return isSameMonth(s, e)
          ? `${format(s, "d")} – ${format(e, "d MMMM yyyy", { locale: dfLocale })}`
          : `${format(s, "d MMM", { locale: dfLocale })} – ${format(e, "d MMM yyyy", { locale: dfLocale })}`;
      }
      case "day":
        return format(date, "EEEE, d MMMM yyyy", { locale: dfLocale });
      case "agenda":
        return `${format(range.from, "d MMM", { locale: dfLocale })} – ${format(subDays(range.to, 1), "d MMM yyyy", { locale: dfLocale })}`;
    }
  }, [view, date, dfLocale, range]);

  const openBooking = useCallback(
    (day: Date | null) => {
      if (canWrite) setBooking({ day });
    },
    [canWrite],
  );

  const onItemClick = useCallback(
    (viewItem: CalendarViewItem) => {
      const item = byId.get(viewItem.id);
      if (!item) return;
      if (item.layer === "appointments" && item.ref_uuid) {
        setOpenUuid(item.ref_uuid);
      } else {
        setSelected({ item, view: viewItem });
      }
    },
    [byId],
  );

  const onItemMove = useCallback(
    (viewItem: CalendarViewItem, start: Date, end: Date) => {
      const item = byId.get(viewItem.id);
      if (!item?.movable || !item.ref_uuid || item.layer !== "appointments") {
        return;
      }
      move.mutate(
        {
          id: item.id,
          body: {
            layer: "appointments",
            ref_uuid: item.ref_uuid,
            starts_at: start.toISOString(),
            ends_at: end.toISOString(),
          },
        },
        { onSuccess: () => appToast.success(t("calendar.moved")) },
      );
    },
    [byId, move, t],
  );

  const toggleLayer = (layer: CalendarLayer, on: boolean) => {
    const next = new Set(hiddenSet);
    if (on) next.delete(layer);
    else next.add(layer);
    setHidden([...next]);
  };
  const allOn = layers.length === available.length;

  const patchFilters = (patch: Partial<CalendarFilters>) =>
    setFilters({ ...filters, ...patch });
  const toggleStatus = (status: AppointmentStatus, on: boolean) =>
    patchFilters({
      statuses: on
        ? [...filters.statuses, status]
        : filters.statuses.filter((s) => s !== status),
    });
  const activeFilters =
    (filters.hostUuids.length > 0 ? 1 : 0) +
    (filters.teamUuid ? 1 : 0) +
    (filters.typeUuids.length > 0 ? 1 : 0) +
    (filters.statuses.length > 0 ? 1 : 0);

  // Keyboard shortcuts (ignored while typing or with a dialog open).
  const dialogOpen = booking !== null || selected !== null || openUuid !== null;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (dialogOpen || e.metaKey || e.ctrlKey || e.altKey) return;
      if (isTypingTarget(e.target)) return;
      const key = e.key.toLowerCase();
      const viewByKey: Record<string, CalendarView> = {
        "1": "month",
        "2": "week",
        "3": "day",
        "4": "agenda",
      };
      if (key === "t") setDate(new Date());
      else if (key === "arrowleft") setDate((d) => shift(view, d, -1));
      else if (key === "arrowright") setDate((d) => shift(view, d, 1));
      else if (viewByKey[key]) setView(viewByKey[key]);
      else if (key === "n" || key === "c") openBooking(null);
      else return;
      e.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [dialogOpen, view, setView, openBooking]);

  const layerList = (
    <div className="space-y-0.5">
      {available.map((layer) => {
        const def = LAYER_DEFS[layer];
        const Icon = def.icon;
        const on = !hiddenSet.has(layer);
        const count = feed?.counts?.[layer];
        return (
          <label
            key={layer}
            className={cn(
              "hover:bg-muted flex cursor-pointer items-center gap-2.5 rounded-md px-2 py-1.5 text-sm",
              !on && "text-muted-foreground",
            )}
          >
            <Checkbox
              checked={on}
              onCheckedChange={(v) => toggleLayer(layer, v === true)}
              aria-label={t(def.labelKey)}
            />
            <span
              aria-hidden
              className={cn(
                "size-2 shrink-0 rounded-full",
                COLOR_CLASSES[def.color].dot,
                !on && "opacity-40",
              )}
            />
            <Icon aria-hidden className="size-3.5 shrink-0" />
            <span className="min-w-0 flex-1 truncate">{t(def.labelKey)}</span>
            {on && count ? (
              <Badge
                variant="secondary"
                className="h-5 px-1.5 text-[10px] tabular-nums"
              >
                {count}
              </Badge>
            ) : null}
          </label>
        );
      })}
    </div>
  );

  const toggleAllButton = (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="h-7 px-2 text-xs"
      onClick={() => setHidden(allOn ? [...available] : [])}
    >
      {allOn ? t("calendar.layers.none") : t("calendar.layers.all")}
    </Button>
  );

  const filterControls = (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label htmlFor="calendar-filter-host" className="text-xs">
          {t("calendar.filters.host")}
        </Label>
        <Select
          value={filters.hostUuids[0] ?? ALL}
          onValueChange={(v) =>
            patchFilters({ hostUuids: v === ALL ? [] : [v] })
          }
        >
          <SelectTrigger id="calendar-filter-host" className="h-8 w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>
              {t("calendar.filters.everyone")}
            </SelectItem>
            {options.members.map((m) => (
              <SelectItem key={m.uuid} value={m.uuid}>
                {m.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="calendar-filter-team" className="text-xs">
          {t("calendar.filters.team")}
        </Label>
        <Select
          value={filters.teamUuid || ALL}
          onValueChange={(v) => patchFilters({ teamUuid: v === ALL ? "" : v })}
        >
          <SelectTrigger id="calendar-filter-team" className="h-8 w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>
              {t("calendar.filters.all_teams")}
            </SelectItem>
            {options.teams.map((team) => (
              <SelectItem key={team.uuid} value={team.uuid}>
                {team.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="calendar-filter-type" className="text-xs">
          {t("calendar.filters.type")}
        </Label>
        <Select
          value={filters.typeUuids[0] ?? ALL}
          onValueChange={(v) =>
            patchFilters({ typeUuids: v === ALL ? [] : [v] })
          }
        >
          <SelectTrigger id="calendar-filter-type" className="h-8 w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>
              {t("calendar.filters.all_types")}
            </SelectItem>
            {(meta.data?.types ?? []).map((type) => (
              <SelectItem key={type.uuid} value={type.uuid}>
                {type.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <fieldset className="space-y-1">
        <legend className="text-xs font-medium">
          {t("calendar.filters.status")}
        </legend>
        {APPOINTMENT_STATUSES.map((status) => (
          <label
            key={status}
            className="hover:bg-muted flex cursor-pointer items-center gap-2 rounded-md px-1 py-1 text-sm"
          >
            <Checkbox
              checked={filters.statuses.includes(status)}
              onCheckedChange={(v) => toggleStatus(status, v === true)}
            />
            {t(STATUS_KEYS[status])}
          </label>
        ))}
      </fieldset>
      {activeFilters > 0 ? (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="w-full"
          onClick={() => setFilters(EMPTY_FILTERS)}
        >
          {t("calendar.filters.clear")}
        </Button>
      ) : null}
    </div>
  );

  const showEmptyHint =
    !feedQuery.isFetching &&
    feedQuery.isSuccess &&
    items.length === 0 &&
    layers.length > 0 &&
    view !== "agenda";

  return (
    <EntityPage
      title={t("calendar.title")}
      description={t("calendar.subtitle")}
      permission={permissions.tenant.appointments.read}
      actions={
        canWrite ? (
          <Button onClick={() => openBooking(null)}>
            <Plus aria-hidden />
            {t("calendar.new_appointment")}
          </Button>
        ) : null
      }
    >
      <div className="grid grid-cols-[minmax(0,1fr)] gap-4 lg:grid-cols-[17rem_minmax(0,1fr)]">
        <aside className="order-2 space-y-4 lg:order-1">
          <Card className="hidden gap-2 py-3 shadow-none lg:flex">
            <CardContent className="p-2">
              <Calendar
                mode="single"
                locale={dayPickerLocale(locale)}
                selected={date}
                month={date}
                onMonthChange={setDate}
                onSelect={(d) => d && setDate(d)}
                weekStartsOn={1}
                className="w-full"
              />
            </CardContent>
          </Card>

          <Card className="hidden gap-2 py-3 shadow-none lg:flex">
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-semibold">
                {t("calendar.layers.title")}
              </CardTitle>
              {toggleAllButton}
            </CardHeader>
            <CardContent className="px-2 pb-3">{layerList}</CardContent>
          </Card>

          <Card className="hidden gap-2 py-3 shadow-none lg:flex">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-semibold">
                {t("calendar.filters.title")}
              </CardTitle>
            </CardHeader>
            <CardContent className="px-3 pb-3">{filterControls}</CardContent>
          </Card>

          <Card className="gap-2 py-3 shadow-none">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-semibold">
                {t("calendar.today")} ·{" "}
                {format(new Date(), "d MMMM", { locale: dfLocale })}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-1 px-2 pb-3">
              {todayItems.length === 0 ? (
                <p className="text-muted-foreground px-2 text-sm">
                  {t("calendar.today_empty")}
                </p>
              ) : (
                todayItems.slice(0, 8).map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => onItemClick(item)}
                    className="hover:bg-muted focus-visible:ring-ring flex w-full items-start gap-2 rounded-md px-2 py-1.5 text-left text-sm outline-none focus-visible:ring-2"
                  >
                    <span
                      aria-hidden
                      className={cn(
                        "mt-1.5 size-2 shrink-0 rounded-full",
                        COLOR_CLASSES[item.color].dot,
                      )}
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate">{item.title}</span>
                      <span className="text-muted-foreground block truncate text-xs">
                        {format(item.start, "HH:mm")}
                        {item.subtitle ? ` · ${item.subtitle}` : ""}
                      </span>
                    </span>
                  </button>
                ))
              )}
              {todayItems.length > 8 ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="w-full text-xs"
                  onClick={() => {
                    setDate(new Date());
                    setView("day");
                  }}
                >
                  {t("calendar.more", { count: todayItems.length - 8 })}
                </Button>
              ) : null}
            </CardContent>
          </Card>
        </aside>

        <Card className="order-1 min-w-0 gap-0 self-start overflow-hidden py-0 shadow-none lg:order-2">
          <div className="flex flex-wrap items-center justify-between gap-2 border-b p-2 sm:gap-3 sm:p-3">
            <div className="flex min-w-0 items-center gap-1.5 sm:gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setDate(new Date())}
              >
                {t("calendar.today")}
              </Button>
              <div className="flex">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t("calendar.prev")}
                  onClick={() => setDate((d) => shift(view, d, -1))}
                >
                  <ChevronLeft aria-hidden />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t("calendar.next")}
                  onClick={() => setDate((d) => shift(view, d, 1))}
                >
                  <ChevronRight aria-hidden />
                </Button>
              </div>
              <h2 className="truncate text-base font-semibold first-letter:uppercase sm:text-lg">
                {title}
              </h2>
            </div>
            <div className="flex items-center gap-1.5">
              <Popover>
                <PopoverTrigger asChild>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="lg:hidden"
                  >
                    <Filter aria-hidden />
                    <span className="sr-only sm:not-sr-only">
                      {t("calendar.filters.title")}
                    </span>
                    {activeFilters > 0 ? (
                      <Badge
                        variant="secondary"
                        className="h-4 px-1 text-[10px] tabular-nums"
                      >
                        {activeFilters}
                      </Badge>
                    ) : null}
                  </Button>
                </PopoverTrigger>
                <PopoverContent align="end" className="w-72 p-3">
                  {filterControls}
                </PopoverContent>
              </Popover>
              <Popover>
                <PopoverTrigger asChild>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="lg:hidden"
                  >
                    <Layers aria-hidden />
                    <span className="sr-only sm:not-sr-only">
                      {t("calendar.layers.title")}
                    </span>
                    {!allOn ? (
                      <Badge
                        variant="secondary"
                        className="h-4 px-1 text-[10px] tabular-nums"
                      >
                        {layers.length}/{available.length}
                      </Badge>
                    ) : null}
                  </Button>
                </PopoverTrigger>
                <PopoverContent align="end" className="w-72 p-2">
                  <div className="flex items-center justify-between px-2 pb-1">
                    <span className="text-sm font-semibold">
                      {t("calendar.layers.title")}
                    </span>
                    {toggleAllButton}
                  </div>
                  {layerList}
                </PopoverContent>
              </Popover>
              <Popover>
                <PopoverTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    className="hidden md:inline-flex"
                    aria-label={t("calendar.shortcuts.title")}
                  >
                    <Keyboard aria-hidden />
                  </Button>
                </PopoverTrigger>
                <PopoverContent align="end" className="w-64">
                  <p className="mb-2 text-sm font-semibold">
                    {t("calendar.shortcuts.title")}
                  </p>
                  <ul className="space-y-1.5 text-sm">
                    {SHORTCUTS.map((s) => (
                      <li
                        key={s.labelKey}
                        className="flex items-center justify-between gap-3"
                      >
                        <span className="text-muted-foreground">
                          {t(s.labelKey)}
                        </span>
                        <span className="flex gap-1">
                          {s.keys.map((k) => (
                            <kbd
                              key={k}
                              className="bg-muted rounded border px-1.5 font-mono text-[11px]"
                            >
                              {k}
                            </kbd>
                          ))}
                        </span>
                      </li>
                    ))}
                  </ul>
                </PopoverContent>
              </Popover>
              <ToggleGroup
                type="single"
                variant="outline"
                size="sm"
                value={view}
                onValueChange={(next) => {
                  if (isView(next)) setView(next);
                }}
                aria-label={t("calendar.views.label")}
              >
                {VIEWS.map((v) => (
                  <ToggleGroupItem key={v} value={v} className="px-2.5">
                    {t(VIEW_LABEL_KEYS[v])}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
            </div>
          </div>

          {layers.length === 0 && meta.isSuccess ? (
            <div className="bg-muted/40 text-muted-foreground flex flex-wrap items-center justify-between gap-2 border-b px-3 py-2 text-sm">
              <span>{t("calendar.layers.all_hidden")}</span>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setHidden([])}
              >
                {t("calendar.layers.all")}
              </Button>
            </div>
          ) : null}
          {showEmptyHint ? (
            <div className="bg-muted/40 text-muted-foreground border-b px-3 py-2 text-sm">
              {t("calendar.empty")}
              {canWrite ? ` ${t("calendar.empty_hint")}` : ""}
            </div>
          ) : null}

          <div className="h-[calc(100dvh-15rem)] min-h-[30rem] sm:min-h-[34rem]">
            {meta.isError ? (
              <ErrorState
                className="m-4"
                title={t("calendar.disabled.title")}
                description={t("calendar.disabled.description")}
              />
            ) : feedQuery.isError && !feed ? (
              <ErrorState
                className="m-4"
                title={t("calendar.error.title")}
                description={t("calendar.error.description")}
                onRetry={() => void feedQuery.refetch()}
                retryLabel={t("calendar.error.retry")}
              />
            ) : (
              <CalendarViews
                items={items}
                view={view}
                date={date}
                loading={feedQuery.isFetching || meta.isLoading}
                onDateChange={setDate}
                onViewChange={setView}
                onItemClick={onItemClick}
                onSlotClick={(start) => openBooking(start)}
                onItemMove={onItemMove}
              />
            )}
          </div>
        </Card>
      </div>

      {booking ? (
        <BookDialog
          open
          onOpenChange={(open) => {
            if (!open) setBooking(null);
          }}
          draftDay={booking.day}
          onBooked={(a) => setDate(parseISO(a.starts_at))}
        />
      ) : null}
      <AppointmentDialog
        uuid={openUuid}
        onOpenChange={(open) => {
          if (open) return;
          setOpenUuid(null);
          if (linked) router.replace(routes.cms.calendar.root);
        }}
      />
      <ItemDialog
        item={selected?.item ?? null}
        view={selected?.view ?? null}
        onOpenChange={(open) => {
          if (!open) setSelected(null);
        }}
      />
    </EntityPage>
  );
}
