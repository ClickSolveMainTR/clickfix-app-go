// CF-152: WidgetKit extension (home / lock screen widgets, iOS 18 controls).
// Files in `_shared/` are compiled into this extension AND the main app
// (App Intents, App Shortcuts, snapshot model). See docs/mobile/widgets.md.

/** @type {import('@bacons/apple-targets/app.plugin').ConfigFunction} */
module.exports = (config) => {
  const bundleId = config.ios?.bundleIdentifier ?? 'com.getclickfix.app';

  return {
    type: 'widget',
    name: 'widgets',
    displayName: 'ClickFix',
    bundleIdentifier: '.widgets',
    deploymentTarget: '17.0',
    frameworks: ['SwiftUI', 'WidgetKit', 'AppIntents', 'ActivityKit'],
    colors: {
      // Tint for widget edit UI and interactive controls.
      $accent: { color: '#0E9F22', darkColor: '#03FF13' },
      $widgetBackground: { color: '#FFFFFF', darkColor: '#111412' },
      // Signal green is a fill colour; text uses the darker signal-text shade
      // on light backgrounds (docs/brand.md).
      brandSignal: { color: '#0E9F22', darkColor: '#03FF13' },
      brandSignalText: { color: '#087313', darkColor: '#3DFF4B' },
      brandInk: { color: '#0C110D', darkColor: '#E8ECE9' },
      widgetBackground: { color: '#FFFFFF', darkColor: '#111412' },
    },
    entitlements: {
      'com.apple.security.application-groups': [`group.${bundleId}`],
      // Only the shared group: the extension reads the widget token, never
      // the app's own Keychain items (refresh token).
      'keychain-access-groups': [`$(AppIdentifierPrefix)${bundleId}.shared`],
    },
  };
};
