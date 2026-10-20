// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { describe, expect, it } from 'vitest';
import {
  absoluteOgImage,
  courseJsonLd,
  organizationJsonLd,
  siteOrigin,
  toAbsoluteUrl,
  truncateMetaDescription,
} from './seo';

const ORIGIN = 'https://mentorship.example.org';

describe('seo helpers', () => {
  it('strips a trailing slash from the site origin', () => {
    expect(siteOrigin(`${ORIGIN}/`)).toBe(ORIGIN);
    expect(siteOrigin(ORIGIN)).toBe(ORIGIN);
  });

  it.each([
    [undefined, undefined],
    ['  ', undefined],
    ['https://cdn.example.org/logo.png', 'https://cdn.example.org/logo.png'],
    ['HTTP://cdn.example.org/a.png', 'HTTP://cdn.example.org/a.png'],
    ['/logo.png', `${ORIGIN}/logo.png`],
    ['logo.png', `${ORIGIN}/logo.png`],
  ])('toAbsoluteUrl(%j)', (url, want) => {
    expect(toAbsoluteUrl(url, ORIGIN)).toBe(want);
  });

  it('falls back to the default OG image', () => {
    expect(absoluteOgImage(undefined, ORIGIN)).toBe(`${ORIGIN}/og-image.png`);
    expect(absoluteOgImage('/logo.png', ORIGIN)).toBe(`${ORIGIN}/logo.png`);
  });

  it('turns rich text into a bounded meta description', () => {
    expect(truncateMetaDescription('<p>Learn <b>Go</b> &amp; Rust</p>', 'fallback')).toBe(
      'Learn Go & Rust',
    );
    expect(truncateMetaDescription('<p><br></p>', 'fallback')).toBe('fallback');
    const long = truncateMetaDescription('x'.repeat(200), '');
    expect(long).toHaveLength(160);
    expect(long.endsWith('...')).toBe(true);
    expect(truncateMetaDescription('y'.repeat(160), '')).toBe('y'.repeat(160));
  });

  it('builds structured data', () => {
    expect(organizationJsonLd(ORIGIN)).toMatchObject({
      '@type': 'Organization',
      logo: `${ORIGIN}/og-image.png`,
    });
    expect(courseJsonLd({ name: 'P', description: 'D', url: 'U', image: 'I' })).toMatchObject({
      '@type': 'Course',
      name: 'P',
      provider: { '@type': 'Organization', name: 'The Linux Foundation' },
    });
  });
});
