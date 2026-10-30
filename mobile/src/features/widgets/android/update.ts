import { requestWidgetUpdate } from 'react-native-android-widget';

import { readAndroidSnapshot } from '../sync';
import { renderInbox, renderPresence } from './widgets';

/** Widget names, as declared to the config plugin in app.json. */
export const ANDROID_WIDGETS = { inbox: 'Inbox', presence: 'Presence' } as const;

/** Redraws every placed ClickFix widget from the stored snapshot. */
export async function updateAndroidWidgets() {
  const snap = readAndroidSnapshot();
  await Promise.all([
    requestWidgetUpdate({
      widgetName: ANDROID_WIDGETS.inbox,
      renderWidget: (info) => renderInbox(snap, info.height),
    }),
    requestWidgetUpdate({ widgetName: ANDROID_WIDGETS.presence, renderWidget: () => renderPresence(snap) }),
  ]);
}
