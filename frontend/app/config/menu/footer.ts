// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { AppRoute, selfServeMenteeUrl, selfServeMentorUrl } from '~/config/routes';
import type { FooterMenuSection } from '~/types/footer.types';

export function lfxFooterMenu(selfServeUrl: string): FooterMenuSection[] {
  return [
    {
      title: 'Platform',
      links: [
        { name: 'Find a Program', link: AppRoute.FindProgram },
        { name: 'Enroll a Program', link: AppRoute.EnrollProgram },
        { name: 'Mentees', link: selfServeMenteeUrl(selfServeUrl) },
        { name: 'Mentors', link: selfServeMentorUrl(selfServeUrl) },
        { name: 'Contact support', intercom: true },
      ],
    },
    {
      title: 'The Linux Foundation',
      links: [
        { name: 'LFX Self Serve', link: 'https://lfx.linuxfoundation.org' },
        { name: 'LFX Insights', link: 'https://insights.lfx.linuxfoundation.org' },
        { name: 'LFX Crowdfunding', link: 'https://crowdfunding.lfx.linuxfoundation.org' },
        { name: 'About the LF', link: 'https://www.linuxfoundation.org/about' },
      ],
    },
  ];
}
