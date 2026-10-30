import * as Device from 'expo-device';
import { Platform } from 'react-native';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { APP_VERSION } from '@/core/config';
import { noteMissing } from '@/features/guides/api';
import { uploadAttachment } from '@/features/inbox/api';

/** Product feedback (CF-142 mobile, CF-141 API): `POST /v1/tenant/product-feedback`. */
export type FeedbackKind = import('@/core/api/schema').components['schemas']['ProductFeedbackInput']['kind'];

export type FeedbackInput = {
  message: string;
  kind: FeedbackKind;
  route: string;
  /** Local screenshot to upload first (attachments flow), or null. */
  screenshot: string | null;
  /** The phone's model and OS go along only when the member leaves this on. */
  includeDevice: boolean;
};

/** "Apple iPhone 16 · iOS 26.1": what the phone says about itself, nothing that identifies the member. */
export function deviceSummary(): string {
  const model = [Device.manufacturer, Device.modelName].filter(Boolean).join(' ');
  const os = `${Platform.OS === 'ios' ? 'iOS' : 'Android'} ${Device.osVersion ?? ''}`.trim();
  return [model, os].filter(Boolean).join(' · ');
}

export async function sendFeedback(input: FeedbackInput): Promise<{ uuid: string }> {
  const attachment_uuids: string[] = [];
  if (input.screenshot) {
    const file = await uploadAttachment({
      uri: input.screenshot,
      name: `clickfix-feedback-${Date.now()}.jpg`,
      type: 'image/jpeg',
    });
    attachment_uuids.push(file.uuid);
  }
  try {
    return await unwrap(
      api.POST('/v1/tenant/product-feedback', {
        params: { header: orgHeader() },
        body: {
          message: input.message,
          kind: input.kind,
          surface: 'mobile',
          route: input.route,
          app_version: APP_VERSION,
          ...(input.includeDevice ? { device: deviceSummary() } : {}),
          ...(attachment_uuids.length ? { attachment_uuids } : {}),
        },
      }),
    );
  } catch (e) {
    noteMissing('feedback', e);
    throw e;
  }
}
