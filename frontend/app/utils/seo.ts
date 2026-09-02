// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { plainTextFromHtml } from '~/utils/html-text';

export const SITE_NAME = 'LFX Mentorship';
export const ORGANIZATION_NAME = 'The Linux Foundation';
export const ORGANIZATION_URL = 'https://www.linuxfoundation.org';
export const DEFAULT_OG_IMAGE_PATH = '/og-image.png';
export const META_DESCRIPTION_MAX = 160;

export function siteOrigin(appUrl: string): string {
  return appUrl.replace(/\/$/, '');
}

export function toAbsoluteUrl(url: string | undefined, origin: string): string | undefined {
  const trimmed = url?.trim();
  if (!trimmed) return undefined;
  if (/^https?:\/\//i.test(trimmed)) return trimmed;
  return `${origin}${trimmed.startsWith('/') ? '' : '/'}${trimmed}`;
}

export function defaultOgImage(origin: string): string {
  return `${origin}${DEFAULT_OG_IMAGE_PATH}`;
}

export function absoluteOgImage(url: string | undefined, origin: string): string {
  return toAbsoluteUrl(url, origin) ?? defaultOgImage(origin);
}

export function truncateMetaDescription(raw: string, fallback: string): string {
  const text = plainTextFromHtml(raw);
  if (!text) return fallback;
  if (text.length <= META_DESCRIPTION_MAX) return text;
  return `${text.slice(0, META_DESCRIPTION_MAX - 3)}...`;
}

export function organizationJsonLd(origin: string) {
  return {
    '@context': 'https://schema.org',
    '@type': 'Organization',
    name: ORGANIZATION_NAME,
    url: ORGANIZATION_URL,
    logo: defaultOgImage(origin),
    sameAs: [
      'https://www.linkedin.com/company/the-linux-foundation',
      'https://twitter.com/linuxfoundation',
      'https://github.com/linuxfoundation',
    ],
  };
}

export function courseJsonLd(input: {
  name: string;
  description: string;
  url: string;
  image: string;
}) {
  return {
    '@context': 'https://schema.org',
    '@type': 'Course',
    name: input.name,
    description: input.description,
    url: input.url,
    image: input.image,
    provider: {
      '@type': 'Organization',
      name: ORGANIZATION_NAME,
      url: ORGANIZATION_URL,
    },
  };
}
