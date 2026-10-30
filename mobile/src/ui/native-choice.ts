import { ActionSheetIOS, Alert, Platform } from 'react-native';
/** A native choice dialog, including a paged Android fallback for more than three actions. */
export function nativeChoice(title: string, choices: { label: string; onPress: () => void }[], cancelLabel: string) {
  if (Platform.OS === 'ios')
    return ActionSheetIOS.showActionSheetWithOptions(
      { title, options: [...choices.map((c) => c.label), cancelLabel], cancelButtonIndex: choices.length },
      (index) => choices[index]?.onPress(),
    );
  const show = (offset: number) =>
    Alert.alert(
      title,
      undefined,
      [
        ...choices.slice(offset, offset + 2).map((choice) => ({ text: choice.label, onPress: choice.onPress })),
        ...(offset + 2 < choices.length
          ? [{ text: '…', onPress: () => show(offset + 2) }]
          : [{ text: cancelLabel, style: 'cancel' as const }]),
      ],
      { cancelable: true },
    );
  show(0);
}
