import { FlexWidget, TextWidget } from 'react-native-android-widget';

import { i18n } from '@/core/i18n';

import type { PresenceStatus, WidgetSnapshot } from '../snapshot-core';

/**
 * Android home screen widgets (CF-152): Inbox and Presence, drawn from the
 * same snapshot as the iOS widgets. RemoteViews have no SF Symbols or
 * dynamic colours, so each widget is rendered for light and dark.
 */

type Palette = { bg: string; fg: string; muted: string; signal: string; chip: string; chipActive: string };

const LIGHT: Palette = {
  bg: '#FFFFFF',
  fg: '#0C110D',
  muted: '#5E655F',
  signal: '#087313',
  chip: '#EFEEE9',
  chipActive: '#0E9F22',
};
const DARK: Palette = {
  bg: '#111412',
  fg: '#E8ECE9',
  muted: '#9AA39C',
  signal: '#03FF13',
  chip: '#1A1E1B',
  chipActive: '#03FF13',
};

const STATUS_DOT: Record<PresenceStatus, string> = {
  online: '#22C55E',
  away: '#F59E0B',
  busy: '#EF4444',
  offline: '#9CA3AF',
};

const t = (key: string, opts?: Record<string, unknown>) => i18n.t(`widgets:${key}`, opts);
type Hex = `#${string}`;
const hex = (c: string) => c as Hex;

function timeAgo(iso: string, now: number): string {
  const min = Math.max(0, Math.round((now - Date.parse(iso)) / 60_000));
  if (min < 60) return `${min}${i18n.language === 'en' ? 'm' : 'dk'}`;
  const h = Math.round(min / 60);
  if (h < 24) return `${h}${i18n.language === 'en' ? 'h' : 'sa'}`;
  return `${Math.round(h / 24)}${i18n.language === 'en' ? 'd' : 'g'}`;
}

function Message({ p, text }: { p: Palette; text: string }) {
  return (
    <FlexWidget
      clickAction="OPEN_APP"
      style={{
        height: 'match_parent',
        width: 'match_parent',
        backgroundColor: hex(p.bg),
        borderRadius: 24,
        padding: 16,
        justifyContent: 'center',
      }}>
      <TextWidget text="clickfix" style={{ fontSize: 14, fontWeight: '700', color: hex(p.signal) }} />
      <TextWidget text={text} style={{ fontSize: 14, color: hex(p.fg), marginTop: 6 }} maxLines={3} />
      <TextWidget text={t('open_app')} style={{ fontSize: 12, color: hex(p.muted), marginTop: 6 }} />
    </FlexWidget>
  );
}

function notReady(snap: WidgetSnapshot | null, p: Palette, need: 'inbox' | 'presence') {
  if (!snap || snap.state === 'signed_out') return <Message p={p} text={t('signed_out')} />;
  if (snap.state === 'no_org') return <Message p={p} text={t('no_org')} />;
  if (snap.state === 'no_permission' || (need === 'inbox' && !snap.inbox))
    return <Message p={p} text={t('no_permission')} />;
  return null;
}

function Count({ p, value, label }: { p: Palette; value: number; label: string }) {
  return (
    <FlexWidget style={{ flex: 1 }}>
      <TextWidget text={String(value)} style={{ fontSize: 26, fontWeight: '700', color: hex(p.fg) }} />
      <TextWidget text={label} style={{ fontSize: 12, color: hex(p.muted) }} maxLines={1} />
    </FlexWidget>
  );
}

function InboxBody({ snap, p, rows, now }: { snap: WidgetSnapshot | null; p: Palette; rows: number; now: number }) {
  const blocked = notReady(snap, p, 'inbox');
  if (blocked || !snap?.inbox) return blocked;
  const inbox = snap.inbox;
  return (
    <FlexWidget
      clickAction="OPEN_URI"
      clickActionData={{ uri: 'clickfix:///inbox' }}
      style={{
        height: 'match_parent',
        width: 'match_parent',
        backgroundColor: hex(p.bg),
        borderRadius: 24,
        padding: 14,
      }}>
      <FlexWidget style={{ flexDirection: 'row', width: 'match_parent', alignItems: 'center' }}>
        <TextWidget text={t('inbox')} style={{ fontSize: 13, fontWeight: '600', color: hex(p.signal) }} />
        <FlexWidget style={{ flex: 1 }} />
        <TextWidget text={snap.org?.name ?? ''} style={{ fontSize: 11, color: hex(p.muted) }} maxLines={1} />
      </FlexWidget>
      <FlexWidget style={{ flexDirection: 'row', width: 'match_parent', marginTop: 6 }}>
        <Count p={p} value={inbox.unread} label={t('unread')} />
        <Count p={p} value={inbox.mine} label={t('mine')} />
      </FlexWidget>
      {rows > 0 ? (
        inbox.top.length ? (
          inbox.top.slice(0, rows).map((c) => (
            <FlexWidget
              key={c.uuid}
              clickAction="OPEN_URI"
              clickActionData={{ uri: `clickfix:///conversation/${c.uuid}` }}
              style={{
                width: 'match_parent',
                marginTop: 8,
                padding: 8,
                borderRadius: 12,
                backgroundColor: hex(p.chip),
              }}>
              <FlexWidget style={{ flexDirection: 'row', width: 'match_parent' }}>
                <TextWidget
                  text={c.contact}
                  style={{ fontSize: 13, fontWeight: c.unread ? '700' : '500', color: hex(p.fg) }}
                  maxLines={1}
                />
                <FlexWidget style={{ flex: 1 }} />
                <TextWidget text={timeAgo(c.at, now)} style={{ fontSize: 11, color: hex(p.muted) }} />
              </FlexWidget>
              <TextWidget text={c.preview} style={{ fontSize: 12, color: hex(p.muted) }} maxLines={1} />
            </FlexWidget>
          ))
        ) : (
          <TextWidget text={t('no_conversations')} style={{ fontSize: 12, color: hex(p.muted), marginTop: 10 }} />
        )
      ) : null}
    </FlexWidget>
  );
}

const CHOICES: PresenceStatus[] = ['online', 'away', 'busy'];

function PresenceBody({ snap, p }: { snap: WidgetSnapshot | null; p: Palette }) {
  const blocked = notReady(snap, p, 'presence');
  if (blocked || !snap) return blocked;
  const status = snap.presence;
  return (
    <FlexWidget
      style={{
        height: 'match_parent',
        width: 'match_parent',
        backgroundColor: hex(p.bg),
        borderRadius: 24,
        padding: 14,
      }}>
      <TextWidget text={t('presence')} style={{ fontSize: 13, fontWeight: '600', color: hex(p.signal) }} />
      <FlexWidget style={{ flexDirection: 'row', alignItems: 'center', marginTop: 6 }}>
        <FlexWidget
          style={{
            width: 10,
            height: 10,
            borderRadius: 5,
            backgroundColor: hex(status ? STATUS_DOT[status] : STATUS_DOT.offline),
          }}
        />
        <TextWidget
          text={t(status ? `status.${status}` : 'status.unknown')}
          style={{ fontSize: 18, fontWeight: '700', color: hex(p.fg), marginLeft: 8 }}
        />
      </FlexWidget>
      <FlexWidget style={{ flexDirection: 'row', width: 'match_parent', marginTop: 10 }}>
        {CHOICES.map((s, i) => (
          <FlexWidget
            key={s}
            clickAction="SET_PRESENCE"
            clickActionData={{ status: s }}
            accessibilityLabel={t(`status.${s}`)}
            style={{
              flex: 1,
              marginLeft: i ? 6 : 0,
              paddingVertical: 8,
              borderRadius: 16,
              alignItems: 'center',
              backgroundColor: hex(s === status ? p.chipActive : p.chip),
            }}>
            <TextWidget
              text={t(`status.${s}`)}
              style={{ fontSize: 12, fontWeight: '600', color: hex(s === status ? '#000000' : p.fg) }}
              maxLines={1}
            />
          </FlexWidget>
        ))}
      </FlexWidget>
    </FlexWidget>
  );
}

/** Rows that fit the widget's current height (dp). */
export function inboxRows(heightDp: number): number {
  if (heightDp < 140) return 0;
  return Math.max(1, Math.min(3, Math.floor((heightDp - 110) / 56)));
}

export function renderInbox(snap: WidgetSnapshot | null, heightDp: number, now = Date.now()) {
  const rows = inboxRows(heightDp);
  return {
    light: <InboxBody snap={snap} p={LIGHT} rows={rows} now={now} />,
    dark: <InboxBody snap={snap} p={DARK} rows={rows} now={now} />,
  };
}

export function renderPresence(snap: WidgetSnapshot | null) {
  return { light: <PresenceBody snap={snap} p={LIGHT} />, dark: <PresenceBody snap={snap} p={DARK} /> };
}
