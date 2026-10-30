// Background tasks must be defined before anything renders, also in the headless JS run
// Android starts for a notification action (CF-81 quick reply) or a home screen widget
// (CF-152). Then the expo-router app.
import { Platform } from 'react-native';

import '@/features/push/quick-reply';
import 'expo-router/entry';

if (Platform.OS === 'android') require('@/features/widgets/android/task-handler');
