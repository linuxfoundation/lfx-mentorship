// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

export default {
  autoImport: false,
  components: {
    prefix: 'pv',
    // Module defaults exclude to ["Editor", "Chart"] unless overridden — clear so Editor can register.
    exclude: ['Chart'],
    include: ['Avatar', 'AvatarGroup', 'DatePicker', 'Editor', 'SelectButton', 'Toast'],
  },
  options: {
    theme: 'none',
  },
};
