// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

export default {
  titleTemplate: '%s | LFX Mentorship',
  htmlAttrs: { lang: 'en' },
  meta: [
    { charset: 'utf-8' },
    { name: 'viewport', content: 'width=device-width, initial-scale=1' },
    { name: 'description', content: 'LFX Mentorship' },
  ],
  link: [
    {
      rel: 'icon',
      type: 'image/x-icon',
      href: 'https://cdn.platform.linuxfoundation.org/assets/lf-favicon.png',
    },
    { rel: 'preconnect', href: 'https://fonts.googleapis.com' },
    { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: 'anonymous' },
    { rel: 'dns-prefetch', href: 'https://kit.fontawesome.com' },
    {
      rel: 'stylesheet',
      href: 'https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&family=Roboto+Slab:wght@300;400;600&display=swap',
    },
  ],
  script: [
    // Same Font Awesome kit as LFX platform apps (required by uikit icons)
    { src: 'https://kit.fontawesome.com/0c49a28643.js', crossorigin: 'anonymous', async: true },
  ],
};
