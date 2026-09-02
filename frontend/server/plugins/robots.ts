// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { defineNitroPlugin } from 'nitropack/runtime';

// The `robots` module config in nuxt.config is resolved at build time, so it
// can't see the per-environment NUXT_PUBLIC_APP_ENV set on the deployed pod
// (the same image is reused across dev/staging). Deciding here instead, via
// the module's runtime hook, reads the real env at request time.
export default defineNitroPlugin((nitroApp) => {
  nitroApp.hooks.hook('robots:config', (ctx) => {
    const appEnv = process.env.NUXT_PUBLIC_APP_ENV;
    const isProduction = appEnv === 'production';
    // Unset or `development` is local; deployed non-prod (staging/dev) stays noindex.
    const isLocal = !appEnv || appEnv === 'development';

    if (isProduction || isLocal) return;

    ctx.groups = [{ userAgent: ['*'], disallow: ['/'], allow: [], comment: [] }];
  });
});
