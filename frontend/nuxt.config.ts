// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT
// https://nuxt.com/docs/api/configuration/nuxt-config

import head from './setup/head';
import modules from './setup/modules';
import primevue from './setup/primevue';
import runtimeConfig from './setup/runtime-config';
import tailwindcss from './setup/tailwind';
import vite from './setup/vite';

export default defineNuxtConfig({
  app: { head },
  compatibilityDate: '2025-07-15',
  devtools: { enabled: true },
  devServer: {
    port: 3000,
  },
  experimental: { typedPages: true },
  modules,
  // Register uikit as lfx-* (e.g. button/button.vue → LfxButton)
  components: [
    {
      path: '~/components/uikit',
      prefix: 'Lfx',
      pathPrefix: false,
    },
    {
      path: '~/components/modules',
      pathPrefix: true,
    },
    {
      path: '~/components/shared',
      pathPrefix: true,
    },
  ],
  plugins: [
    '~/plugins/vue-query.ts',
    '~/plugins/primevue-toast.ts',
    '~/plugins/lfx-ui-core.client.ts',
    '~/plugins/auth.client.ts',
  ],
  css: ['~/assets/styles/main.scss'],
  primevue,
  runtimeConfig,
  tailwindcss,
  vite,
});
