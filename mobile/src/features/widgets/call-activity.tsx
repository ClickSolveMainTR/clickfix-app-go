import { useLocalParticipant } from '@livekit/react-native';
import { useEffect, useRef } from 'react';

import { setCallHandlers } from './call-controls';
import { type CallActivityHandle, startCallActivity } from './live-activities';

/**
 * Mirrors the LiveKit call on the Lock Screen and in the Dynamic Island
 * (elapsed time, mute, end) while the call screen is mounted. Rendered inside
 * `<LiveKitRoom>`.
 */
export function CallActivity({
  room,
  title,
  subtitle,
  onLeave,
}: {
  room: string;
  title: string;
  subtitle: string;
  onLeave: () => void;
}) {
  const { localParticipant, isMicrophoneEnabled, isCameraEnabled } = useLocalParticipant();
  const handle = useRef<CallActivityHandle | null>(null);
  const leave = useRef(onLeave);
  const mic = useRef(isMicrophoneEnabled);
  useEffect(() => {
    leave.current = onLeave;
    mic.current = isMicrophoneEnabled;
  });

  useEffect(() => {
    handle.current = startCallActivity({ room, title, subtitle, kind: isCameraEnabled ? 'video' : 'voice' });
    setCallHandlers({
      end: () => leave.current(),
      toggleMute: () => void localParticipant.setMicrophoneEnabled(!mic.current),
    });
    return () => {
      setCallHandlers(null);
      handle.current?.end();
      handle.current = null;
    };
    // Started once per room; the kind is the camera state at join.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [room]);

  useEffect(() => {
    handle.current?.setMuted(!isMicrophoneEnabled);
  }, [isMicrophoneEnabled]);

  return null;
}
