import { useCSSVariable } from 'uniwind';
import type { IconName } from './icons';
export function channelIcon(channel: string): IconName {
  switch (channel) {
    case 'email':
      return 'mail';
    case 'whatsapp':
    case 'phone':
    case 'voice':
      return 'phone';
    case 'instagram':
      return 'camera';
    case 'api':
      return 'channel';
    default:
      return 'chat';
  }
}
/** Channel identity fills share the named palette used by native icon tiles. */
export function useChannelColor() {
  const [whatsapp, email, instagram, phone, fallback] = useCSSVariable([
    '--color-channel-whatsapp', '--color-tile-blue', '--color-channel-instagram', '--color-tile-green', '--color-tile-indigo',
  ]);
  return (channel: string): string | undefined => {
    const color = channel === 'whatsapp' ? whatsapp : channel === 'email' ? email
      : channel === 'instagram' ? instagram : channel === 'voice' || channel === 'phone' ? phone : fallback;
    return typeof color === 'string' ? color : undefined;
  };
}
