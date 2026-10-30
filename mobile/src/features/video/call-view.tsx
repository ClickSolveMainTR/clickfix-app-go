import {
  AudioSession,
  isTrackReference,
  LiveKitRoom,
  registerGlobals,
  RNE2EEManager,
  RNKeyProvider,
  useChat,
  useLocalParticipant,
  useTracks,
  VideoTrack,
} from '@livekit/react-native';
import { BlurView as RNBlurView } from 'expo-blur';
import { LocalVideoTrack, Room, type RoomOptions, Track } from 'livekit-client';
import { type ReactNode, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, StyleSheet, TextInput, View, useWindowDimensions } from 'react-native';
import { Gesture, GestureDetector } from 'react-native-gesture-handler';
import { KeyboardStickyView, useKeyboardState } from 'react-native-keyboard-controller';
import Animated, { useAnimatedStyle, useSharedValue, withTiming } from 'react-native-reanimated';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { withUniwind } from 'uniwind';

import { CallActivity } from '@/features/widgets/call-activity';
import { EmptyState } from '@/ui/empty-state';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import type { IconName } from '@/ui/icons';
import { Text } from '@/ui/text';

import type { VideoJoinGrant } from './api';

// WebRTC globals for livekit-client. This module is loaded lazily (call
// screens only), so a binary built before the native modules never runs it.
registerGlobals();

const BlurView = withUniwind(RNBlurView);

type Props = { grant: VideoJoinGrant; onLeave: () => void; toolbar?: ReactNode };

/**
 * One LiveKit call on the phone. Encrypted rooms use the shared room key the
 * API hands out after admission (RNKeyProvider + RNE2EEManager, the native
 * counterpart of the panel's ExternalE2EEKeyProvider).
 */
export default function CallView({ grant, onLeave, toolbar }: Props) {
  const { t } = useTranslation('video');
  const insets = useSafeAreaInsets();
  const [room, setRoom] = useState<Room | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let current: Room | undefined;
    let cancelled = false;
    (async () => {
      await AudioSession.startAudioSession();
      const options: RoomOptions = { adaptiveStream: true, dynacast: true };
      if (grant.e2ee_key) {
        const keyProvider = new RNKeyProvider({});
        await keyProvider.setSharedKey(grant.e2ee_key);
        options.encryption = { e2eeManager: new RNE2EEManager(keyProvider) };
      }
      current = new Room(options);
      if (grant.e2ee_key) await current.setE2EEEnabled(true);
      if (!cancelled) setRoom(current);
    })().catch(() => !cancelled && setFailed(true));
    return () => {
      cancelled = true;
      void current?.disconnect();
      void AudioSession.stopAudioSession();
    };
  }, [grant.e2ee_key]);

  if (failed) {
    return (
      <View className="flex-1 items-center justify-center bg-background p-6">
        <EmptyState
          icon="videoOff"
          title={t('call.error.connect_failed')}
          action={{ title: t('call.leave'), icon: 'close', onPress: onLeave }}
        />
      </View>
    );
  }
  if (!room || !grant.token || !grant.server_url) {
    return (
      <View className="flex-1 items-center justify-center bg-background">
        <Text>{t('call.connecting')}</Text>
        <Button title={t('common:close')} variant="ghost" onPress={onLeave} />
      </View>
    );
  }
  return (
    <LiveKitRoom
      room={room}
      serverUrl={grant.server_url}
      token={grant.token}
      connect
      audio
      video={grant.room.allow_camera}
      onDisconnected={onLeave}
      onError={() => setFailed(true)}>
      <CallActivity
        room={grant.room.uuid}
        title={grant.room.name}
        subtitle={grant.room.organization_name}
        onLeave={onLeave}
      />
      {/* A call is always dark, whatever the app theme: video reads best on black. */}
      <View style={StyleSheet.absoluteFill} className="bg-ink">
        <Stage top={insets.top + (toolbar ? 112 : 64)} bottom={insets.bottom + 96} />
        <View className="absolute inset-x-0 top-0 gap-2 px-4" style={{ paddingTop: insets.top + 8 }}>
          <View className="flex-row items-center gap-2">
            <Text variant="headline" className="flex-1 text-on-tile" numberOfLines={1}>
              {grant.room.name}
            </Text>
            {grant.e2ee_key ? <Badge icon="lockClosed" label={t('call.e2ee')} /> : null}
            {grant.room.recording ? <Badge dot label={t('call.recordable')} /> : null}
          </View>
          {toolbar ? <View className="flex-row items-center gap-2">{toolbar}</View> : null}
        </View>
        <Controls
          allowCamera={grant.room.allow_camera}
          allowChat={grant.room.allow_chat}
          bottomInset={insets.bottom}
          onLeave={onLeave}
        />
      </View>
    </LiveKitRoom>
  );
}

function Badge({ label, icon, dot }: { label: string; icon?: IconName; dot?: boolean }) {
  return (
    <View className="flex-row items-center gap-1 rounded-full bg-on-tile/15 px-2 py-1">
      {icon ? <Icon name={icon} size={12} tone="on-tile" /> : null}
      {dot ? <View className="size-2 rounded-full bg-destructive" /> : null}
      <Text variant="caption" className="text-on-tile">
        {label}
      </Text>
    </View>
  );
}

type Ref = ReturnType<typeof useTracks>[number];

function Tile({ item, compact }: { item: Ref; compact?: boolean }) {
  const { t } = useTranslation('video');
  const name = item.participant.isLocal ? t('call.you') : item.participant.name || item.participant.identity;
  return (
    <View className="flex-1 overflow-hidden rounded-2xl bg-on-tile/10">
      {isTrackReference(item) && !item.publication.isMuted ? (
        <VideoTrack
          trackRef={item}
          style={{ flex: 1 }}
          objectFit={item.source === Track.Source.ScreenShare ? 'contain' : 'cover'}
          mirror={item.participant.isLocal && item.source === Track.Source.Camera}
        />
      ) : (
        <View className="flex-1 items-center justify-center gap-2">
          <View className={`items-center justify-center rounded-full bg-on-tile/15 ${compact ? 'size-10' : 'size-20'}`}>
            <Text variant={compact ? 'headline' : 'title'} className="text-on-tile">
              {name.slice(0, 1).toUpperCase()}
            </Text>
          </View>
        </View>
      )}
      {compact ? null : (
        <Text variant="footnote" className="absolute bottom-3 left-3 rounded-full bg-ink/60 px-2 py-1 text-on-tile">
          {name}
          {item.source === Track.Source.ScreenShare ? ` · ${t('call.screen')}` : ''}
        </Text>
      )}
    </View>
  );
}

const trackKey = (r: Ref) => `${r.participant.identity}:${r.source}`;

/** Pinch to zoom and drag a shared screen; double tap resets. */
function ZoomTile({ item }: { item: Ref }) {
  const scale = useSharedValue(1),
    savedScale = useSharedValue(1),
    tx = useSharedValue(0),
    ty = useSharedValue(0),
    savedX = useSharedValue(0),
    savedY = useSharedValue(0);
  const pinch = Gesture.Pinch()
    .onStart(() => savedScale.set(scale.value))
    .onUpdate((e) => scale.set(Math.max(1, Math.min(5, savedScale.value * e.scale))));
  const pan = Gesture.Pan()
    .onStart(() => {
      savedX.set(tx.value);
      savedY.set(ty.value);
    })
    .onUpdate((e) => {
      if (scale.value <= 1) return;
      tx.set(savedX.value + e.translationX);
      ty.set(savedY.value + e.translationY);
    });
  const reset = Gesture.Tap()
    .numberOfTaps(2)
    .onEnd(() => {
      scale.set(withTiming(1));
      tx.set(withTiming(0));
      ty.set(withTiming(0));
    });
  const style = useAnimatedStyle(() => ({
    transform: [{ translateX: tx.value }, { translateY: ty.value }, { scale: scale.value }],
  }));
  return (
    <GestureDetector gesture={Gesture.Simultaneous(pinch, pan, reset)}>
      <Animated.View style={[{ flex: 1 }, style]}>
        <Tile item={item} />
      </Animated.View>
    </GestureDetector>
  );
}

/**
 * One or two people: the other side fills the screen and you sit in a corner
 * window (FaceTime). Three or more: a grid; tapping a tile (a shared screen,
 * say) opens it full screen with pinch to zoom.
 */
function Stage({ top, bottom }: { top: number; bottom: number }) {
  const { t } = useTranslation('video');
  const { width } = useWindowDimensions();
  const [focus, setFocus] = useState<string | null>(null);
  const tracks = useTracks(
    [
      { source: Track.Source.Camera, withPlaceholder: true },
      { source: Track.Source.ScreenShare, withPlaceholder: false },
    ],
    { onlySubscribed: false },
  );
  const local = tracks.find((r) => r.participant.isLocal && r.source === Track.Source.Camera);
  const others = tracks.filter((r) => r !== local);
  const focused = focus ? tracks.find((r) => trackKey(r) === focus) : undefined;

  if (focused || others.length <= 1) {
    const main = focused ?? others[0] ?? local;
    const zoomable = !!focused || main?.source === Track.Source.ScreenShare;
    return (
      <View style={StyleSheet.absoluteFill}>
        {main ? zoomable ? <ZoomTile key={trackKey(main)} item={main} /> : <Tile item={main} /> : null}
        {main && local && main !== local ? (
          <View
            pointerEvents="none"
            className="absolute right-4 h-44 w-28 overflow-hidden rounded-2xl border border-on-tile/20"
            style={{ top }}>
            <Tile item={local} compact />
          </View>
        ) : null}
        {focused ? (
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={t('common:close')}
            onPress={() => setFocus(null)}
            hitSlop={8}
            className="absolute left-4 size-10 items-center justify-center rounded-full bg-ink/60 active:opacity-70"
            style={{ top }}>
            <Icon name="close" size={18} tone="on-tile" />
          </Pressable>
        ) : null}
        {zoomable ? (
          <Text
            variant="caption"
            pointerEvents="none"
            className="absolute self-center rounded-full bg-ink/60 px-3 py-1 text-on-tile"
            style={{ bottom: bottom + 8 }}>
            {t('call.zoom_hint')}
          </Text>
        ) : null}
      </View>
    );
  }
  const columns = 2;
  const size = (width - 16 * 2 - 8) / columns;
  return (
    <FlatList
      style={StyleSheet.absoluteFill}
      contentContainerStyle={{ paddingTop: top, paddingBottom: bottom, paddingHorizontal: 16, gap: 8 }}
      columnWrapperStyle={{ gap: 8 }}
      data={tracks}
      numColumns={columns}
      keyExtractor={trackKey}
      renderItem={({ item }) => (
        <Pressable
          accessibilityRole="button"
          onPress={() => setFocus(trackKey(item))}
          style={{ width: size, height: size * 1.33 }}>
          <Tile item={item} />
        </Pressable>
      )}
    />
  );
}

function Control({
  icon,
  label,
  onPress,
  active = true,
  danger,
}: {
  icon: IconName;
  label: string;
  onPress: () => void;
  active?: boolean;
  danger?: boolean;
}) {
  const fill = danger ? 'bg-destructive' : active ? 'bg-on-tile/20' : 'bg-on-tile';
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityState={{ selected: !active }}
      onPress={onPress}
      hitSlop={6}
      className={`size-14 items-center justify-center rounded-full active:opacity-70 ${fill}`}>
      <Icon name={icon} size={24} tone={danger || active ? 'on-tile' : 'ink'} />
    </Pressable>
  );
}

function Controls({
  allowCamera,
  allowChat,
  bottomInset,
  onLeave,
}: {
  allowCamera: boolean;
  allowChat: boolean;
  bottomInset: number;
  onLeave: () => void;
}) {
  const { t } = useTranslation('video');
  const keyboardVisible = useKeyboardState((state) => state.isVisible);
  const { localParticipant, isMicrophoneEnabled, isCameraEnabled } = useLocalParticipant();
  const [front, setFront] = useState(true);
  const [chat, setChat] = useState(false);
  const flip = async () => {
    const pub = localParticipant.getTrackPublication(Track.Source.Camera);
    if (pub?.track instanceof LocalVideoTrack) {
      await pub.track.restartTrack({ facingMode: front ? 'environment' : 'user' });
      setFront(!front);
    }
  };
  return (
    // Rides on top of the keyboard while chatting; the video behind never resizes.
    <KeyboardStickyView
      offset={{ closed: 0, opened: bottomInset }}
      style={{ position: 'absolute', left: 0, right: 0, bottom: 0 }}>
      <View className="gap-3 px-4" style={{ paddingBottom: keyboardVisible ? 8 : Math.max(bottomInset, 12) }}>
        {chat ? <ChatPanel /> : null}
        <BlurView
          tint="systemChromeMaterialDark"
          intensity={60}
          className="flex-row items-center justify-evenly self-center overflow-hidden rounded-full px-3 py-3"
          style={{ gap: 12 }}>
          <Control
            icon={isMicrophoneEnabled ? 'mic' : 'micOff'}
            label={t('call.toggle_mic')}
            active={isMicrophoneEnabled}
            onPress={() => void localParticipant.setMicrophoneEnabled(!isMicrophoneEnabled)}
          />
          {allowCamera ? (
            <Control
              icon={isCameraEnabled ? 'video' : 'videoOff'}
              label={t('call.toggle_camera')}
              active={isCameraEnabled}
              onPress={() => void localParticipant.setCameraEnabled(!isCameraEnabled)}
            />
          ) : null}
          {allowCamera && isCameraEnabled ? (
            <Control icon="flipCamera" label={t('call.flip_camera')} onPress={() => void flip()} />
          ) : null}
          {allowChat ? (
            <Control icon="chat" label={t('call.chat')} active={!chat} onPress={() => setChat(!chat)} />
          ) : null}
          <Control icon="hangUp" label={t('call.leave')} onPress={onLeave} danger />
        </BlurView>
      </View>
    </KeyboardStickyView>
  );
}

function ChatPanel() {
  const { t } = useTranslation('video');
  const { chatMessages, send, isSending } = useChat();
  const [text, setText] = useState('');
  return (
    <BlurView tint="systemChromeMaterialDark" intensity={60} className="max-h-72 overflow-hidden rounded-3xl px-4 py-3">
      <FlatList
        keyboardDismissMode="interactive"
        keyboardShouldPersistTaps="handled"
        data={chatMessages}
        keyExtractor={(m) => m.id}
        renderItem={({ item }) => (
          <Text variant="footnote" className="py-0.5 text-on-tile">
            <Text variant="footnote" className="font-sans-semibold text-on-tile/70">
              {item.from?.name || item.from?.identity}:{' '}
            </Text>
            {item.message}
          </Text>
        )}
      />
      <TextInput
        className="mt-2 rounded-full bg-on-tile/15 px-4 py-2 text-on-tile"
        accessibilityLabel={t('call.chat_placeholder')}
        placeholder={t('call.chat_placeholder')}
        placeholderTextColorClassName="accent-on-tile/60"
        value={text}
        editable={!isSending}
        onChangeText={setText}
        onSubmitEditing={() => {
          if (!text.trim()) return;
          void send(text.trim());
          setText('');
        }}
        returnKeyType="send"
      />
    </BlurView>
  );
}
