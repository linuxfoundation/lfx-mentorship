// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { ref, computed } from 'vue';
import { navigateTo, useRoute } from 'nuxt/app';

declare const window: Window & typeof globalThis;

export interface AuthUser {
  sub: string;
  name?: string;
  email?: string;
  picture?: string;
  email_verified?: boolean;
  username?: string;
  intercomJwt?: string;
}

export interface AuthState {
  isAuthenticated: boolean;
  user: AuthUser | null;
  token: string | null;
}

export const authState = ref<AuthState>({
  isAuthenticated: false,
  user: null,
  token: null,
});

export const isAuthLoading = ref(false);
export const isAuthReady = ref(false);

/** Auth0 BFF (`server/api/auth/*`) is not part of this scaffold. */
export const isAuthEnabled = false;

let refreshAuthFn: () => Promise<unknown> = async () => {};
export const setRefreshAuth = (fn: () => Promise<unknown>) => {
  refreshAuthFn = fn;
};

export const login = async (redirectTo?: string, options?: { screenHint?: 'signup' }) => {
  if (!isAuthEnabled) return;

  isAuthLoading.value = true;
  try {
    let currentPath = redirectTo || '/';
    if (!redirectTo && import.meta.client) {
      try {
        const route = useRoute();
        currentPath = route.fullPath || '/';
      } catch {
        currentPath = '/';
      }
    }

    const response = await $fetch<{ success: boolean; authorizationUrl: string }>(
      '/api/auth/login',
      {
        method: 'GET',
        query: {
          ...(currentPath !== '/' ? { redirectTo: currentPath } : {}),
          ...(options?.screenHint ? { screenHint: options.screenHint } : {}),
        },
        credentials: 'include',
      },
    );

    if (response.success && response.authorizationUrl) {
      if (import.meta.client) {
        window.location.href = response.authorizationUrl;
      } else {
        await navigateTo(response.authorizationUrl, { external: true });
      }
    } else {
      isAuthLoading.value = false;
    }
  } catch (error) {
    console.error('Login error:', error);
    isAuthLoading.value = false;
  }
};

export const logout = async () => {
  isAuthLoading.value = true;
  try {
    const response = await $fetch<{ success: boolean; logoutUrl: string }>('/api/auth/logout', {
      method: 'POST',
      body: import.meta.client ? { returnTo: window.location.origin } : undefined,
    });

    if (response.success) {
      authState.value = { isAuthenticated: false, user: null, token: null };

      if (import.meta.client) {
        window.location.href = response.logoutUrl;
      } else {
        await navigateTo(response.logoutUrl, { external: true });
      }
    }
  } catch (error) {
    console.error('Logout error:', error);
  } finally {
    isAuthLoading.value = false;
  }
};

export const useAuth = () => {
  const isAuthenticated = computed(() => authState.value.isAuthenticated);
  const user = computed(() => authState.value.user);

  return {
    isAuthenticated,
    user,
    isLoading: isAuthLoading,
    isReady: isAuthReady,
    isAuthEnabled,
    login,
    logout,
    refreshAuth: () => refreshAuthFn(),
  };
};
