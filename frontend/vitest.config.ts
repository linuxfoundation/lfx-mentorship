// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { fileURLToPath } from 'node:url';
import { defineVitestProject } from '@nuxt/test-utils/config';
import { defineConfig } from 'vitest/config';

// Two projects: `*.nuxt.test.ts` runs in the Nuxt runtime (components, pages,
// vue-query composables); everything else runs in plain Node. One Nuxt-wide
// config is not an option, because the Nuxt auto-import transform overrides the
// `$fetch`/`useRuntimeConfig`/`createError` globals the BFF tests stub.
const NUXT_TESTS = '**/*.nuxt.test.ts';
const exclude = ['**/node_modules/**', '**/.nuxt/**', '**/.output/**'];

export default defineConfig({
  test: {
    coverage: {
      provider: 'v8',
      include: ['app/**/*.{ts,vue}', 'server/**/*.ts'],
      exclude: ['**/*.test.ts'],
    },
    projects: [
      {
        // Nuxt sets this at build time; app code under test runs as the client would.
        define: { 'import.meta.client': 'true' },
        resolve: { alias: { '~': fileURLToPath(new URL('./app', import.meta.url)) } },
        test: {
          name: 'unit',
          environment: 'node',
          include: ['**/*.test.ts'],
          exclude: [...exclude, NUXT_TESTS],
        },
      },
      await defineVitestProject({
        test: {
          name: 'nuxt',
          environment: 'nuxt',
          environmentOptions: { nuxt: { domEnvironment: 'happy-dom' } },
          setupFiles: ['./test/nuxt/setup.ts'],
          include: [NUXT_TESTS],
          exclude,
        },
      }),
    ],
  },
});
