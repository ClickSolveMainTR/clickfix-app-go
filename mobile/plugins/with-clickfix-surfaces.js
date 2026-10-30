// CF-152: native wiring for the OS surfaces (widgets, Live Activities, App
// Intents, Control Center). Every identifier is derived from
// `ios.bundleIdentifier`, so `expo prebuild --clean` reproduces the project
// and nothing here pins a team or a store id. See docs/mobile/widgets.md.
const {
  createRunOncePlugin,
  withEntitlementsPlist,
  withInfoPlist,
  withPlugins,
  withMod,
  withXcodeProject,
} = require('expo/config-plugins');
const fs = require('fs');
const path = require('path');

/** Defined only on the main app target: `_shared` Swift files guard app-only code with it. */
const APP_CONDITION = 'CLICKFIX_APP';

function ids(config) {
  const bundleId = config.ios && config.ios.bundleIdentifier;
  if (!bundleId) throw new Error('with-clickfix-surfaces: ios.bundleIdentifier is required');
  return {
    bundleId,
    appGroup: `group.${bundleId}`,
    ownKeychain: `$(AppIdentifierPrefix)${bundleId}`,
    sharedKeychain: `$(AppIdentifierPrefix)${bundleId}.shared`,
  };
}

const unique = (list) => [...new Set(list)];

/**
 * App Group for the snapshot; Keychain groups with the app's own group first,
 * so items saved without an explicit group (the refresh token) stay private
 * and only the widget token is written to the shared one.
 */
const withEntitlements = (config) =>
  withEntitlementsPlist(config, (mod) => {
    const { appGroup, ownKeychain, sharedKeychain } = ids(mod);
    const e = mod.modResults;
    e['com.apple.security.application-groups'] = unique([
      ...(e['com.apple.security.application-groups'] || []),
      appGroup,
    ]);
    const rest = (e['keychain-access-groups'] || []).filter((g) => g !== ownKeychain && g !== sharedKeychain);
    e['keychain-access-groups'] = [ownKeychain, sharedKeychain, ...rest];
    return mod;
  });

const withPlist = (config) =>
  withInfoPlist(config, (mod) => {
    const { appGroup, sharedKeychain } = ids(mod);
    mod.modResults.ClickFixAppIdentifierPrefix = '$(AppIdentifierPrefix)';
    mod.modResults.ClickFixAppGroup = appGroup;
    mod.modResults.ClickFixKeychainGroup = sharedKeychain;
    // Live Activities (call, urgent conversation) are rendered by targets/widgets.
    mod.modResults.NSSupportsLiveActivities = true;
    return mod;
  });

/**
 * `CLICKFIX_APP` on the app target (App Shortcuts provider, UIApplication
 * calls) and Turkish in `knownRegions`, so the String Catalogs of the widget
 * and the intents compile their Turkish strings.
 */
const withXcode = (config) =>
  withXcodeProject(config, (mod) => {
    const project = mod.modResults;
    const { bundleId } = ids(mod);
    const configs = project.pbxXCBuildConfigurationSection();
    for (const key of Object.keys(configs)) {
      const settings = configs[key] && configs[key].buildSettings;
      if (!settings) continue;
      const id = String(settings.PRODUCT_BUNDLE_IDENTIFIER || '').replace(/"/g, '');
      if (id !== bundleId) continue;
      const current = String(settings.SWIFT_ACTIVE_COMPILATION_CONDITIONS || '$(inherited)')
        .replace(/"/g, '')
        .split(/\s+/)
        .filter(Boolean);
      if (!current.includes(APP_CONDITION)) current.push(APP_CONDITION);
      settings.SWIFT_ACTIVE_COMPILATION_CONDITIONS = `"${current.join(' ')}"`;
    }
    const root = project.getFirstProject().firstProject;
    const regions = (root.knownRegions || []).map((r) => String(r).replace(/"/g, ''));
    for (const r of ['en', 'tr', 'Base']) if (!regions.includes(r)) regions.push(r);
    root.knownRegions = regions;
    return mod;
  });

/**
 * App Shortcuts phrases ship as `_shared/<lang>.lproj/AppShortcuts.strings`:
 * an `AppShortcuts.xcstrings` catalog needs iOS 17 and the app targets 16.4.
 * @bacons/apple-targets lists `_shared/*` as membership exceptions, which
 * puts the lproj folders themselves there and makes Xcode fail with "PIF GUID
 * already registered". Xcode expects localized files as
 * `/Localized/_shared/<file>`, so rewrite them. Runs on the same mod as
 * apple-targets and is registered before it, so it runs after it.
 */
const withLocalizedSharedFiles = (config) =>
  withMod(config, {
    platform: 'ios',
    mod: 'xcodeProjectBeta2',
    action: (mod) => {
      const widgetDir = path.join(mod.modRequest.projectRoot, 'targets', 'widgets');
      for (const object of mod.modResults.values()) {
        const list = object && object.props && object.props.membershipExceptions;
        if (!Array.isArray(list)) continue;
        const out = [];
        for (const entry of list) {
          const m = /^(_shared)\/[^/]+\.lproj$/.exec(entry);
          if (!m) {
            out.push(entry);
            continue;
          }
          for (const file of fs.readdirSync(path.join(widgetDir, entry))) {
            out.push(`/Localized/${m[1]}/${file}`);
          }
        }
        object.props.membershipExceptions = unique(out).sort();
      }
      return mod;
    },
  });

const withClickFixSurfaces = (config) => {
  ids(config);
  return withPlugins(config, [withEntitlements, withPlist, withXcode, withLocalizedSharedFiles]);
};

module.exports = createRunOncePlugin(withClickFixSurfaces, 'with-clickfix-surfaces', '1.0.0');
