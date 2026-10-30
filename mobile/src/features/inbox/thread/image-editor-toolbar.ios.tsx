import { Button, Host, NavigationStack, RNHostView, Toolbar, ToolbarItem } from '@expo/ui/swift-ui';
import { disabled, navigationTitle, navigationBarTitleDisplayMode } from '@expo/ui/swift-ui/modifiers';
import { View } from 'react-native';
import type { EditorToolbarProps } from './image-editor-toolbar';
export function ImageEditorToolbar(p: EditorToolbarProps) {
  return (
    <Host style={{ height: 56 }}>
      <NavigationStack>
        <Toolbar modifiers={[navigationTitle(p.title), navigationBarTitleDisplayMode('inline')]}>
          <RNHostView>
            <View />
          </RNHostView>
          <Toolbar.Content>
            <ToolbarItem placement="cancellationAction">
              <Button label={p.cancel} onPress={p.onCancel} modifiers={[disabled(p.busy)]} />
            </ToolbarItem>
            <ToolbarItem placement="confirmationAction">
              <Button label={p.done} onPress={p.onDone} modifiers={[disabled(p.busy)]} />
            </ToolbarItem>
          </Toolbar.Content>
        </Toolbar>
      </NavigationStack>
    </Host>
  );
}
