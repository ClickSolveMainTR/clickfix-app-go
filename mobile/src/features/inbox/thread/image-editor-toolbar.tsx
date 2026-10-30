import { View } from 'react-native';
import { Button } from '@/ui/button';
import { Text } from '@/ui/text';
export type EditorToolbarProps = {
  title: string;
  cancel: string;
  done: string;
  busy: boolean;
  onCancel: () => void;
  onDone: () => void;
};
export function ImageEditorToolbar(p: EditorToolbarProps) {
  return (
    <View className="min-h-14 flex-row items-center justify-between border-b border-border px-2">
      <Button title={p.cancel} variant="ghost" disabled={p.busy} onPress={p.onCancel} />
      <Text variant="headline">{p.title}</Text>
      <Button title={p.done} variant="ghost" loading={p.busy} onPress={p.onDone} />
    </View>
  );
}
