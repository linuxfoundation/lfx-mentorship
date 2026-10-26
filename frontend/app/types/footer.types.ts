// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

export type FooterMenuLink =
  | { name: string; link: string; action?: never; support?: never }
  | { name: string; action: () => void; link?: never; support?: never }
  | { name: string; support: true; link?: never; action?: never };

declare global {
  interface Window {
    // Transcend airgap.js API (loaded in setup/head.ts)
    transcend?: { showConsentManager?: () => void };
  }
}

export interface FooterMenuSection {
  title: string;
  links: FooterMenuLink[];
}
