import { useCSSVariable } from 'uniwind';
import { type ComponentRef, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Image, NativeModules, Pressable, ScrollView, TextInput, View } from 'react-native';
import { Gesture, GestureDetector, GestureHandlerRootView } from 'react-native-gesture-handler';
import Animated, { useAnimatedStyle, useSharedValue } from 'react-native-reanimated';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import Svg, { Path, Text as SvgText } from 'react-native-svg';
import * as Haptics from 'expo-haptics';
import * as FileSystem from 'expo-file-system/legacy';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import { cn } from '@/ui/cn';
import type { LocalFile } from '../api';
import { hitsStroke, strokePath, type Point, type Stroke } from './image-editing';
import { ImageEditorToolbar } from './image-editor-toolbar';

// Do not import view-shot at module scope: old dev clients must never evaluate its native binding.
export const canMarkup = !!NativeModules.RNViewShot;
const colors = ['#000000', '#FFFFFF', '#FF3B30', '#FFCC00', '#007AFF', '#34C759'];
const tools = ['pen', 'highlighter', 'arrow', 'rectangle', 'text', 'eraser'] as const;
type Tool = (typeof tools)[number];
export function ImageMarkupEditor({
  file,
  onCancel,
  onDone,
}: {
  file: LocalFile;
  onCancel: () => void;
  onDone: (file: LocalFile) => void;
}) {
  const [selectedBorder, border] = useCSSVariable(['--color-primary', '--color-border']);
  const { t } = useTranslation('conversation');
  const insets = useSafeAreaInsets();
  const [size, setSize] = useState({ width: 0, height: 0 });
  const [area, setArea] = useState({ width: 0, height: 0 });
  const [tool, setTool] = useState<Tool>('pen');
  const [width, setWidth] = useState(4);
  const [color, setColor] = useState(colors[2]);
  const [label, setLabel] = useState('');
  const [strokes, setStrokes] = useState<Stroke[]>([]);
  const [draft, setDraft] = useState<Stroke | null>(null);
  const current = useRef<Stroke[]>([]);
  const drawing = useRef<Stroke | null>(null);
  const erased = useRef<Stroke[]>([]);
  const interrupted = useRef(false);
  const [past, setPast] = useState<Stroke[][]>([]);
  const [future, setFuture] = useState<Stroke[][]>([]);
  const [busy, setBusy] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const capture = useRef<ComponentRef<typeof View>>(null);
  const scale = useSharedValue(1),
    savedScale = useSharedValue(1);
  const tx = useSharedValue(0),
    ty = useSharedValue(0);
  const savedX = useSharedValue(0),
    savedY = useSharedValue(0);
  useEffect(() => {
    let active = true;
    Image.getSize(
      file.uri,
      (width, height) => {
        if (active) setSize({ width, height });
      },
      () => toast.error(t('attach.failed')),
    );
    return () => {
      active = false;
    };
  }, [file.uri, t]);
  const fit = size.width && area.width ? Math.min(area.width / size.width, area.height / size.height) : 0;
  const w = size.width * fit,
    h = size.height * fit;
  const imageStyle = useAnimatedStyle(() => ({
    transform: [{ translateX: tx.value }, { translateY: ty.value }, { scale: scale.value }],
  }));
  function replace(next: Stroke[]) {
    current.current = next;
    setStrokes(next);
  }
  function commit(next: Stroke[]) {
    const previous = current.current;
    setPast((p) => [...p, previous]);
    setFuture([]);
    replace(next);
    void Haptics.selectionAsync();
  }
  function point(x: number, y: number): Point {
    return {
      x: Math.max(0, Math.min(size.width, ((x - w / 2 - tx.value) / scale.value + w / 2) / fit)),
      y: Math.max(0, Math.min(size.height, ((y - h / 2 - ty.value) / scale.value + h / 2) / fit)),
    };
  }
  function erase(p: Point) {
    erased.current = erased.current.filter((s) => !hitsStroke(s, p, 16 / fit / scale.value));
    setStrokes(erased.current);
  }
  // Gesture builder registers event handlers; it does not run them during React render.
  /* eslint-disable react-hooks/refs, react-hooks/purity */
  const draw = Gesture.Pan()
    .minDistance(0)
    .maxPointers(1)
    .enabled(!busy && loaded && !!fit)
    .runOnJS(true)
    .onStart((e) => {
      interrupted.current = false;
      const p = point(e.x, e.y);
      if (tool === 'eraser') {
        erased.current = current.current;
        erase(p);
        return;
      }
      if (tool === 'text' && !label.trim()) return;
      drawing.current = {
        id: `${Date.now()}-${Math.random()}`,
        tool: tool as Stroke['tool'],
        color,
        width: width / fit,
        points: [p],
        ...(tool === 'text' ? { text: label.trim() } : {}),
      };
      setDraft(drawing.current);
    })
    .onTouchesDown((e) => {
      if (e.numberOfTouches > 1) {
        interrupted.current = true;
        drawing.current = null;
        setDraft(null);
        setStrokes(current.current);
      }
    })
    .onUpdate((e) => {
      if (e.numberOfPointers !== 1 || interrupted.current) return;
      const p = point(e.x, e.y);
      if (tool === 'eraser') {
        erase(p);
        return;
      }
      const s = drawing.current;
      if (!s) return;
      drawing.current = {
        ...s,
        points:
          s.tool === 'pen' || s.tool === 'highlighter' ? [...s.points, p] : s.tool === 'text' ? [p] : [s.points[0], p],
      };
      setDraft(drawing.current);
    })
    .onEnd(() => {
      if (interrupted.current) return;
      if (tool === 'eraser') {
        if (erased.current.length !== current.current.length) commit(erased.current);
      } else if (drawing.current) commit([...current.current, drawing.current]);
    })
    .onFinalize(() => {
      drawing.current = null;
      setDraft(null);
      setStrokes(current.current);
    });
  const pan = Gesture.Pan()
    .minPointers(2)
    .enabled(!busy)
    .onStart(() => {
      savedX.set(tx.value);
      savedY.set(ty.value);
    })
    .onUpdate((e) => {
      tx.set(savedX.value + e.translationX);
      ty.set(savedY.value + e.translationY);
    });
  const pinch = Gesture.Pinch()
    .enabled(!busy)
    .onStart(() => {
      savedScale.set(scale.value);
    })
    .onUpdate((e) => {
      scale.set(Math.max(1, Math.min(6, savedScale.value * e.scale)));
    });
  /* eslint-enable react-hooks/refs, react-hooks/purity */
  async function save() {
    if (!canMarkup || !loaded || busy || !capture.current || !FileSystem.cacheDirectory) return;
    setBusy(true);
    try {
      scale.set(1);
      tx.set(0);
      ty.set(0);
      // Wait for the transform reset to reach native before taking a complete, uncropped snapshot.
      await new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve())));
      const { captureRef } = await import('react-native-view-shot');
      const ratio = Math.min(1, 2048 / Math.max(size.width, size.height));
      const uri = await captureRef(capture, {
        format: 'png',
        result: 'tmpfile',
        width: Math.round(size.width * ratio),
        height: Math.round(size.height * ratio),
      });
      const name = `markup-${Date.now()}.png`;
      const destination = FileSystem.cacheDirectory + name;
      await FileSystem.copyAsync({ from: uri, to: destination });
      onDone({ uri: destination, name, type: 'image/png' });
    } catch {
      toast.error(t('attach.failed'));
    } finally {
      setBusy(false);
    }
  }
  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <View className="flex-1 bg-background" style={{ paddingTop: insets.top, paddingBottom: insets.bottom }}>
        <ImageEditorToolbar
          title={t('image.draw')}
          cancel={t('image.cancel')}
          done={t('image.done')}
          busy={busy}
          onCancel={onCancel}
          onDone={() => void save()}
        />
        <View
          className="flex-1 items-center justify-center overflow-hidden bg-ink"
          onLayout={(e) => setArea(e.nativeEvent.layout)}>
          {fit > 0 ? (
            <GestureDetector gesture={Gesture.Simultaneous(draw, pan, pinch)}>
              <View style={{ width: w, height: h }}>
                <Animated.View style={[{ width: w, height: h }, imageStyle]}>
                  <View
                    ref={capture}
                    collapsable={false}
                    style={{ width: w, height: h, backgroundColor: 'transparent' }}>
                    <Image
                      source={{ uri: file.uri }}
                      style={{ width: w, height: h }}
                      resizeMode="stretch"
                      onLoad={() => setLoaded(true)}
                      onError={() => toast.error(t('attach.failed'))}
                    />
                    <Svg
                      width={w}
                      height={h}
                      viewBox={`0 0 ${size.width} ${size.height}`}
                      style={{ position: 'absolute' }}>
                      {[...strokes, ...(draft ? [draft] : [])].map((s) =>
                        s.tool === 'text' ? (
                          <SvgText key={s.id} x={s.points[0].x} y={s.points[0].y} fill={s.color} fontSize={s.width * 7}>
                            {s.text}
                          </SvgText>
                        ) : (
                          <Path
                            key={s.id}
                            d={strokePath(s)}
                            stroke={s.color}
                            strokeWidth={s.width * (s.tool === 'highlighter' ? 4 : 1)}
                            opacity={s.tool === 'highlighter' ? 0.35 : 1}
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            fill="none"
                          />
                        ),
                      )}
                    </Svg>
                  </View>
                </Animated.View>
              </View>
            </GestureDetector>
          ) : null}
        </View>
        <Text variant="caption" tone="muted" className="px-3 pt-2">
          {t('image.gesture_hint')}
        </Text>
        {tool === 'text' ? (
          <TextInput
            className="mx-3 my-2 rounded-xl border border-border p-3 text-foreground"
            value={label}
            onChangeText={setLabel}
            placeholder={t('image.label')}
            accessibilityLabel={t('image.label')}
            maxLength={150}
          />
        ) : null}
        <ScrollView
          horizontal
          keyboardShouldPersistTaps="always"
          contentContainerStyle={{ padding: 8, gap: 6 }}
          style={{ flexGrow: 0 }}>
          {tools.map((v) => (
            <Pressable
              key={v}
              accessibilityRole="button"
              accessibilityState={{ selected: tool === v }}
              onPress={() => {
                setTool(v);
                void Haptics.selectionAsync();
              }}
              className={cn('min-h-11 justify-center rounded-xl px-3', tool === v ? 'bg-secondary' : '')}>
              <Text>{t(`image.${v}`)}</Text>
            </Pressable>
          ))}
        </ScrollView>
        <View className="flex-row flex-wrap items-center gap-2 px-3 pb-2">
          {colors.map((c, i) => (
            <Pressable
              key={c}
              accessibilityRole="button"
              accessibilityLabel={t(`image.color_${i}`)}
              accessibilityState={{ selected: color === c }}
              onPress={() => setColor(c)}
              style={{
                width: 32,
                height: 32,
                borderRadius: 16,
                backgroundColor: c,
                borderWidth: color === c ? 3 : 1,
                borderColor: color === c ? selectedBorder : border,
              }}
            />
          ))}
          {[2, 4, 8].map((v, i) => (
            <Pressable
              key={v}
              accessibilityRole="button"
              accessibilityLabel={t('image.width', { width: i + 1 })}
              accessibilityState={{ selected: width === v }}
              onPress={() => setWidth(v)}
              className={cn('size-11 items-center justify-center rounded-xl', width === v ? 'bg-secondary' : '')}>
              <View className="rounded-full bg-foreground" style={{ width: v + 4, height: v + 4 }} />
            </Pressable>
          ))}
        </View>
        <View className="flex-row justify-between px-3 pb-2">
          <Pressable
            accessibilityRole="button"
            disabled={!past.length || busy}
            className="min-h-11 justify-center"
            onPress={() => {
              const previous = past[past.length - 1];
              if (!previous) return;
              const present = current.current;
              setFuture((f) => [...f, present]);
              setPast((p) => p.slice(0, -1));
              replace(previous);
            }}>
            <Text tone={past.length ? 'default' : 'muted'}>{t('image.undo')}</Text>
          </Pressable>
          <Pressable
            accessibilityRole="button"
            className="min-h-11 justify-center"
            onPress={() => {
              scale.set(1);
              tx.set(0);
              ty.set(0);
            }}>
            <Text>{t('image.reset_zoom')}</Text>
          </Pressable>
          <Pressable
            accessibilityRole="button"
            disabled={!future.length || busy}
            className="min-h-11 justify-center"
            onPress={() => {
              const next = future[future.length - 1];
              if (!next) return;
              const present = current.current;
              setPast((p) => [...p, present]);
              setFuture((f) => f.slice(0, -1));
              replace(next);
            }}>
            <Text tone={future.length ? 'default' : 'muted'}>{t('image.redo')}</Text>
          </Pressable>
        </View>
      </View>
    </GestureHandlerRootView>
  );
}
