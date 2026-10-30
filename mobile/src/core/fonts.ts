// One module per weight: the package root would ship every file (Metro does not tree-shake).
import { Geist_400Regular } from '@expo-google-fonts/geist/400Regular';
import { Geist_500Medium } from '@expo-google-fonts/geist/500Medium';
import { Geist_600SemiBold } from '@expo-google-fonts/geist/600SemiBold';
import { Geist_700Bold } from '@expo-google-fonts/geist/700Bold';
import { SpaceGrotesk_600SemiBold } from '@expo-google-fonts/space-grotesk/600SemiBold';
import { SpaceGrotesk_700Bold } from '@expo-google-fonts/space-grotesk/700Bold';

import { MATERIAL_FONT } from './material-font';

/**
 * The panel's typefaces (docs/brand.md, "Typography"): Geist for UI text,
 * Space Grotesk for display (titles, the wordmark). Family names are the keys,
 * matched by `--font-sans*` / `--font-display*` in global.css. On Android the
 * icon font loads with them, so no icon draws empty on the first frame.
 */
export const FONTS = {
  Geist_400Regular,
  Geist_500Medium,
  Geist_600SemiBold,
  Geist_700Bold,
  SpaceGrotesk_600SemiBold,
  SpaceGrotesk_700Bold,
  ...(MATERIAL_FONT ? { [MATERIAL_FONT.name]: MATERIAL_FONT.font } : {}),
};
