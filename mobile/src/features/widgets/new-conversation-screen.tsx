import { Redirect } from 'expo-router';

/**
 * `clickfix:///new-conversation` (Quick Actions widget, Control Center,
 * Siri). A conversation starts from a contact, so this opens the contact list.
 */
export default function NewConversationScreen() {
  return <Redirect href="/contacts" />;
}
