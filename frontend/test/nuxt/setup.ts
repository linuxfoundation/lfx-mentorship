// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { enableAutoUnmount } from '@vue/test-utils';
import { afterEach } from 'vitest';

// mountSuspended wrappers share one Nuxt app per file. Unmount them after each
// test so onUnmounted hooks release scroll locks and window listeners, then
// clear what teleported overlays left in <body>, so no test depends on order.
let unmountAll: () => void = () => {};
enableAutoUnmount((hook) => {
  unmountAll = hook;
});

afterEach(() => {
  unmountAll();
  document.body.innerHTML = '';
});
