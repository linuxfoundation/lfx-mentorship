// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { defineNuxtPlugin } from 'nuxt/app';
import { authState, isAuthLoading, isAuthReady } from '~/composables/useAuth';

/**
 * Lightweight auth bootstrap for menv3.
 * Marks auth as ready in signed-out state. Wire real Auth0 BFF later
 * (wire real Auth0 BFF later via `plugins/auth.client.ts` + `server/api/auth/*`).
 */
export default defineNuxtPlugin(() => {
  authState.value = { isAuthenticated: false, user: null, token: null };
  isAuthLoading.value = false;
  isAuthReady.value = true;
});
