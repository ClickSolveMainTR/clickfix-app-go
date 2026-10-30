import { Host, Text, TextButton, TopAppBar } from '@expo/ui/jetpack-compose';
import type { EditorToolbarProps } from './image-editor-toolbar';
export function ImageEditorToolbar(p: EditorToolbarProps) {
  return (
    <Host style={{ height: 64 }}>
      <TopAppBar windowInsets={{}}>
        <TopAppBar.Title>
          <Text>{p.title}</Text>
        </TopAppBar.Title>
        <TopAppBar.NavigationIcon>
          <TextButton onClick={p.onCancel} enabled={!p.busy}>
            <Text>{p.cancel}</Text>
          </TextButton>
        </TopAppBar.NavigationIcon>
        <TopAppBar.Actions>
          <TextButton onClick={p.onDone} enabled={!p.busy}>
            <Text>{p.done}</Text>
          </TextButton>
        </TopAppBar.Actions>
      </TopAppBar>
    </Host>
  );
}
