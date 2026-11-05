// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { vi, describe, it, expect } from 'vitest';
import * as h3 from 'h3';
import type { H3Event } from 'h3';
import securityHeaders, { CONTENT_SECURITY_POLICY } from './security-headers';

vi.mock('h3', async (importOriginal) => ({
  ...(await importOriginal<typeof import('h3')>()),
  defineEventHandler: (fn: unknown) => fn,
  setResponseHeader: vi.fn(),
}));

const handler = securityHeaders as unknown as (event: H3Event) => unknown;

describe('security-headers middleware', () => {
  it('sets a Content-Security-Policy that restricts form targets, base URL and plugins', () => {
    const event = {} as H3Event;
    handler(event);
    expect(h3.setResponseHeader).toHaveBeenCalledWith(
      event,
      'Content-Security-Policy',
      CONTENT_SECURITY_POLICY,
    );
    expect(CONTENT_SECURITY_POLICY.split('; ')).toEqual([
      "form-action 'self'",
      "base-uri 'self'",
      "object-src 'none'",
    ]);
  });
});
