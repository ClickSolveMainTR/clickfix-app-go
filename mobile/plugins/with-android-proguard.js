// Extra R8 rules for the Android release build, appended to
// android/app/proguard-rules.pro on every prebuild.
//
// @livekit/react-native-webrtc keeps `livekit.org.jni_zero.**`, but its AAR
// ships JniZero without the generated JniZeroJni class, so R8 stops with
// "Missing class livekit.org.jni_zero.JniZeroJni" (the rule below is the one
// R8 writes to missing_rules.txt).
const { withDangerousMod } = require('expo/config-plugins');
const fs = require('fs');
const path = require('path');

const MARKER = '# with-android-proguard';
const RULES = `
${MARKER}
-dontwarn livekit.org.jni_zero.JniZeroJni
`;

module.exports = (config) =>
  withDangerousMod(config, [
    'android',
    async (mod) => {
      const file = path.join(mod.modRequest.platformProjectRoot, 'app', 'proguard-rules.pro');
      const current = fs.existsSync(file) ? fs.readFileSync(file, 'utf8') : '';
      if (!current.includes(MARKER)) fs.writeFileSync(file, current + RULES);
      return mod;
    },
  ]);
