// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

export type SelfServeSection = 'mentee' | 'mentor' | 'admin';

export interface SelfServeNavItem {
  id: SelfServeSection;
  label: string;
  icon: string;
  to: string;
}
