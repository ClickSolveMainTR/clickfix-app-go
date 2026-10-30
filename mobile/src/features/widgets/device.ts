import { storage } from '@/core/storage';

import { newDeviceId } from './tokens-core';

const DEVICE_KEY = 'widgets.device_id';

/** A stable id for this installation (widget and Live Activity tokens are per device). */
export function deviceId(): string {
  let id = storage.getString(DEVICE_KEY);
  if (!id) {
    id = newDeviceId();
    storage.set(DEVICE_KEY, id);
  }
  return id;
}
