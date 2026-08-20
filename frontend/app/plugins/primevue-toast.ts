// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import ToastService from 'primevue/toastservice';

export default defineNuxtPlugin((nuxtApp) => {
  nuxtApp.vueApp.use(ToastService);
});
