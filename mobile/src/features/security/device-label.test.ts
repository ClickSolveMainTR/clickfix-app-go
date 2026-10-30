import assert from 'node:assert/strict';
import { test } from 'node:test';

import { deviceLabel } from './device-label.ts';

test('deviceLabel names the app, browsers and unknown agents', () => {
  assert.deepEqual(deviceLabel('ClickFixMobile/0.1.0 (ios)'), { kind: 'app', label: 'ClickFix iOS 0.1.0' });
  assert.deepEqual(deviceLabel('ClickFixMobile/2.3.1 (android)'), { kind: 'app', label: 'ClickFix Android 2.3.1' });
  assert.equal(
    deviceLabel(
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15',
    ).label,
    'Safari · macOS',
  );
  assert.equal(
    deviceLabel(
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36',
    ).label,
    'Chrome · Windows',
  );
  assert.equal(
    deviceLabel('Mozilla/5.0 (Windows NT 10.0) Chrome/130.0 Safari/537.36 Edg/130.0').label,
    'Edge · Windows',
  );
  assert.deepEqual(deviceLabel(null), { kind: 'unknown', label: '' });
  assert.equal(deviceLabel('curl/8.0').label, 'curl/8.0');
});
