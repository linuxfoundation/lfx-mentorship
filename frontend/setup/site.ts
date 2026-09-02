// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Prefer NUXT_PUBLIC_SITE_URL — @nuxtjs/sitemap / nuxt-site-config re-read that
// name at runtime. Fall back to the app URL used by canonical tags.
const appUrl = (
  process.env.NUXT_PUBLIC_SITE_URL ||
  process.env.NUXT_SITE_URL ||
  process.env.NUXT_PUBLIC_APP_URL ||
  process.env.NUXT_APP_URL ||
  'http://localhost:3000'
).replace(/\/$/, '');

export default {
  url: appUrl,
  name: 'LFX Mentorship',
};
