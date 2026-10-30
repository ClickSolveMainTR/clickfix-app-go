import { useQueryClient } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { type Href, router } from 'expo-router';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AccessibilityInfo, Keyboard, Modal, Pressable, ScrollView, useWindowDimensions, View } from 'react-native';
import Animated, {
  FadeIn,
  useReducedMotion,
  useSharedValue,
  useAnimatedProps,
  withTiming,
} from 'react-native-reanimated';
import Svg, { Defs, Mask, Rect as SvgRect } from 'react-native-svg';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { useOrgSlug } from '@/core/api/org';
import { openAssistant } from '@/features/assistant/open';
import { Button } from '@/ui/button';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Markdown } from '@/ui/markdown';
import { Text } from '@/ui/text';

import { cachedSetupGuides, saveGuideProgress, useHelpAvailability } from './api';
import { cardTop, isVisibleRect, nextSetupGuide, type Rect, spotlightRect } from './core';
import { closeGuide, expectGuideRoute, setGuideStep, startGuide, useGuideTour } from './store';
import { measureTarget } from '@/ui/guide-target';

const CARD_MAX_WIDTH = 420;
/** A target that mounts with its screen (after a navigate step) gets this long to appear. */
const MEASURE_TRIES = 6;
const MEASURE_EVERY_MS = 150;

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));

/** The guide on screen (if any), above every screen, sheet and tab bar. */
export function GuideOverlay() {
  const active = useGuideTour((s) => s.active);
  if (!active) return null;
  return <CoachMark key={active.guide.key} />;
}

/**
 * Coach mark: the screen dims and blurs except for a cut-out around the
 * step's target, and a card beside it explains the step (Next / Back /
 * Skip / "Ask the assistant", progress dots, haptics). A step without a
 * target, or whose target is not on screen, shows the card in the middle.
 * VoiceOver reads the step as it changes; reduce motion turns the fades off.
 */
function CoachMark() {
  const { t } = useTranslation('help');
  const active = useGuideTour((s) => s.active);
  const firstSave = useRef(true);
  const win = useWindowDimensions();
  const insets = useSafeAreaInsets();
  const reduceMotion = useReducedMotion();
  const qc = useQueryClient();
  const org = useOrgSlug();
  const assistantOn = useHelpAvailability((s) => s.assistant);
  const [measured, setMeasured] = useState<{ key: string; rect: Rect | null } | null>(null);
  const [cardHeight, setCardHeight] = useState(0);
  const [keyboardTop, setKeyboardTop] = useState<number | null>(null);
  useEffect(() => {
    const show = Keyboard.addListener('keyboardDidShow', (e) => setKeyboardTop(e.endCoordinates.screenY));
    const hide = Keyboard.addListener('keyboardDidHide', () => setKeyboardTop(null));
    return () => {
      show.remove();
      hide.remove();
    };
  }, []);
  const availableHeight = Math.min(win.height, keyboardTop ?? win.height);

  const guide = active?.guide;
  const index = active?.index ?? 0;
  const step = guide?.steps[index];
  const total = guide?.steps.length ?? 0;
  const stepKey = `${guide?.key}:${step?.id}:${index}:${win.width}x${win.height}`;
  const rect = measured?.key === stepKey ? measured.rect : undefined;

  useEffect(() => {
    if (!step) return;
    let cancelled = false;
    void (async () => {
      for (let i = 0; i < MEASURE_TRIES && step.target; i++) {
        const r = await measureTarget(step.target);
        if (cancelled) return;
        if (isVisibleRect(r, win)) return setMeasured({ key: stepKey, rect: r });
        await wait(MEASURE_EVERY_MS);
      }
      if (!cancelled) setMeasured({ key: stepKey, rect: null });
    })();
    return () => {
      cancelled = true;
    };
  }, [stepKey, step, win]);

  useEffect(() => {
    if (!guide || !step) return;
    void saveGuideProgress(
      qc,
      org,
      guide.key,
      'in_progress',
      index,
      false,
      firstSave.current && active?.manual === true,
    );
    firstSave.current = false;
    AccessibilityInfo.announceForAccessibility(`${t('guides.step_of', { current: index + 1, total })}. ${step.title}`);
  }, [qc, org, guide, step, index, total, t, active?.manual]);

  if (!guide || !step) return null;
  const last = index >= total - 1;
  const visibleRect = rect === undefined ? measured?.rect : rect;
  const hole = visibleRect ? spotlightRect(visibleRect, win) : null;
  const width = Math.min(win.width - 32, CARD_MAX_WIDTH);
  const left = (win.width - width) / 2;
  const top = hole
    ? cardTop(hole, cardHeight, { ...win, height: availableHeight }, insets, step.placement).top
    : Math.max((availableHeight - cardHeight) / 2, insets.top + 16);
  const ready = rect !== undefined && cardHeight > 0;

  const finish = (status: 'completed' | 'skipped') => {
    void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    void saveGuideProgress(qc, org, guide.key, status, index, active?.source === 'setup');
    const source = active?.source;
    closeGuide();
    // The setup tour goes on with its next item once one is done.
    if (source === 'setup' && status === 'completed') {
      const next = nextSetupGuide(cachedSetupGuides(qc, org));
      if (next) setTimeout(() => startGuide(next, 'setup', active?.manual !== true), 400);
    }
  };

  const next = () => {
    void Haptics.selectionAsync();
    const action = step.action;
    if (action?.kind === 'navigate' && action.route) {
      expectGuideRoute(action.route);
      router.push(action.route as Href);
    }
    if (last) return finish('completed');
    setGuideStep(index + 1);
  };

  const ask = () => {
    void Haptics.selectionAsync();
    finish('skipped');
    openAssistant({ route: guide.route, guide: guide.key, prompt: step.title });
  };

  const below = !!hole && top >= hole.y + hole.height;
  const above = !!hole && top + cardHeight <= hole.y;
  const arrowX = hole ? Math.max(24, Math.min(width - 24, hole.x + hole.width / 2 - left)) : 24;

  return (
    <Modal
      visible
      transparent
      statusBarTranslucent
      navigationBarTranslucent
      animationType={reduceMotion ? 'none' : 'fade'}
      onRequestClose={() => finish('skipped')}>
      <View className="flex-1" accessibilityViewIsModal>
        <Pressable
          accessibilityLabel={t('guides.skip')}
          onPress={() => finish('skipped')}
          style={{ position: 'absolute', inset: 0 }}
        />
        <Spotlight
          hole={hole ? { ...hole, radius: (visibleRect?.radius ?? 12) + 6 } : null}
          win={win}
          reduced={!!reduceMotion}
        />
        <Animated.View
          key={stepKey}
          entering={reduceMotion ? undefined : FadeIn.duration(180)}
          onLayout={(e) => setCardHeight(e.nativeEvent.layout.height)}
          className={cn('absolute rounded-2xl border border-border bg-card p-4', !ready && 'opacity-0')}
          style={{
            left,
            top,
            width,
            maxHeight: availableHeight - insets.top - insets.bottom - 32,
            borderCurve: 'continuous',
            boxShadow: '0 12px 32px rgba(2, 6, 24, 0.28)',
          }}>
          {hole && (below || above) ? (
            <View
              pointerEvents="none"
              className="absolute size-4 rotate-45 border-border bg-card"
              style={{
                left: arrowX - 8,
                ...(below
                  ? { top: -8, borderLeftWidth: 1, borderTopWidth: 1 }
                  : { bottom: -8, borderRightWidth: 1, borderBottomWidth: 1 }),
              }}
            />
          ) : null}
          <View className="flex-row items-start gap-3">
            <View className="size-10 items-center justify-center rounded-xl bg-secondary">
              <Icon name="help" size={22} tone="primary" />
            </View>
            <View className="flex-1 gap-1">
              <Text variant="caption" tone="muted">
                {guide.title}
              </Text>
              <Text variant="headline" accessibilityRole="header">
                {step.title}
              </Text>
            </View>
            <Pressable
              accessibilityRole="button"
              accessibilityLabel={t('guides.skip')}
              hitSlop={10}
              onPress={() => finish('skipped')}>
              <Icon name="close" tone="muted-foreground" size={20} />
            </Pressable>
          </View>
          <ScrollView className="mt-2 shrink" bounces={false} contentContainerClassName="pb-1">
            <Markdown source={step.body} />
          </ScrollView>
          {assistantOn ? (
            <Pressable
              accessibilityRole="button"
              onPress={ask}
              className={cn('mt-3 min-h-10 flex-row items-center gap-1.5 self-start py-1.5 active:opacity-70')}>
              <Icon name="sparkles" size={15} tone="muted-foreground" />
              <Text variant="footnote" className={cn('font-sans-medium text-muted-foreground')}>
                {t('guides.ask_assistant')}
              </Text>
            </Pressable>
          ) : null}
          <View className="mt-4 flex-row items-center gap-2">
            <View
              className="flex-1 flex-row items-center gap-1.5"
              accessible
              accessibilityLabel={t('guides.step_of', { current: index + 1, total })}>
              <Text variant="footnote" tone="muted" className="tabular-nums">
                {index + 1}/{total}
              </Text>
            </View>
            <Button variant="ghost" title={t('guides.skip')} onPress={() => finish('skipped')} />
            <Button title={last ? t('guides.done') : t('guides.next')} onPress={next} />
          </View>
        </Animated.View>
      </View>
    </Modal>
  );
}

const AnimatedRect = Animated.createAnimatedComponent(SvgRect);

/** Animate the mask itself, so rounded corners remain clear while moving between targets. */
function Spotlight({
  hole,
  win,
  reduced,
}: {
  hole: Rect | null;
  win: { width: number; height: number };
  reduced: boolean;
}) {
  const x = useSharedValue(hole?.x ?? win.width / 2);
  const y = useSharedValue(hole?.y ?? win.height / 2);
  const width = useSharedValue(hole?.width ?? 0);
  const height = useSharedValue(hole?.height ?? 0);
  const radius = useSharedValue(hole?.radius ?? 18);
  useEffect(() => {
    const config = { duration: reduced ? 0 : 240 };
    x.value = withTiming(hole?.x ?? win.width / 2, config);
    y.value = withTiming(hole?.y ?? win.height / 2, config);
    width.value = withTiming(hole?.width ?? 0, config);
    height.value = withTiming(hole?.height ?? 0, config);
    radius.value = withTiming(hole?.radius ?? 18, config);
  }, [
    hole?.x,
    hole?.y,
    hole?.width,
    hole?.height,
    hole?.radius,
    reduced,
    win.width,
    win.height,
    x,
    y,
    width,
    height,
    radius,
  ]);
  const props = useAnimatedProps(() => ({
    x: x.value,
    y: y.value,
    width: width.value,
    height: height.value,
    rx: radius.value,
    ry: radius.value,
  }));
  return (
    <Svg
      width={win.width}
      height={win.height}
      style={{ position: 'absolute' }}
      pointerEvents="none"
      accessibilityElementsHidden>
      <Defs>
        <Mask id="guide-spotlight" x="0" y="0" width={win.width} height={win.height} maskUnits="userSpaceOnUse">
          <SvgRect width={win.width} height={win.height} fill="white" />
          <AnimatedRect animatedProps={props} fill="black" />
        </Mask>
      </Defs>
      <SvgRect width={win.width} height={win.height} fill="rgba(2,6,24,0.56)" mask="url(#guide-spotlight)" />
    </Svg>
  );
}
