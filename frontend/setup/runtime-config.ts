// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

const appEnv = process.env.NUXT_PUBLIC_APP_ENV || 'development';
const isProduction = appEnv === 'production';
// NUXT_PUBLIC_APP_URL — must use the NUXT_PUBLIC_ prefix so Nuxt's runtime
// config re-reads it from the container's env at server startup (matches
// public.appUrl below); a differently-prefixed var name is silently ignored.
const appUrl =
  process.env.NUXT_PUBLIC_APP_URL || process.env.NUXT_APP_URL || 'http://localhost:3000';
const selfServeUrl =
  process.env.NUXT_PUBLIC_SELF_SERVE_URL ||
  (isProduction ? 'https://app.lfx.dev' : 'https://app.dev.lfx.dev');
const crowdfundingUrl =
  process.env.NUXT_PUBLIC_CROWDFUNDING_URL ||
  (isProduction
    ? 'https://crowdfunding.lfx.linuxfoundation.org'
    : 'https://crowdfunding.dev.lfx.dev');

export default {
  // Server-only
  apiBaseUrl: process.env.NUXT_API_BASE_URL || 'http://localhost:8080',

  public: {
    apiBase: '/api',
    appEnv,
    appUrl,
    selfServeUrl,
    crowdfundingUrl,
    intercomAppId:
      process.env.NUXT_PUBLIC_INTERCOM_APP_ID || (isProduction ? 'w29sqomy' : 'mxl90k6y'),
  },
};
