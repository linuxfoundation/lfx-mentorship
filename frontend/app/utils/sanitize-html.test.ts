// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { describe, expect, it, vi } from 'vitest';

vi.mock('isomorphic-dompurify', async (importOriginal) => {
  const actual = await importOriginal<typeof import('isomorphic-dompurify')>();
  return { ...actual, clearWindow: vi.fn(actual.clearWindow) };
});

// The call counter is module state, so each test reloads sanitize-html and
// starts from a spy with no calls.
const load = async () => {
  vi.resetModules();
  const { clearWindow } = await import('isomorphic-dompurify');
  vi.mocked(clearWindow).mockClear();
  return { ...(await import('./sanitize-html')), clearWindow };
};

describe('sanitizeHtml', () => {
  it('sanitizes with the given config', async () => {
    const { sanitizeHtml } = await load();
    expect(sanitizeHtml('<p onclick="x()">hi</p><script>alert(1)</script>')).toBe('<p>hi</p>');
    expect(
      sanitizeHtml('<a href="https://example.org" target="_blank">a</a>', {
        ADD_ATTR: ['target'],
      }),
    ).toBe('<a href="https://example.org" target="_blank">a</a>');
  });

  it('replaces the jsdom window once every SANITIZE_CALLS_PER_WINDOW calls', async () => {
    const { sanitizeHtml, clearWindow, SANITIZE_CALLS_PER_WINDOW } = await load();

    for (let i = 1; i < SANITIZE_CALLS_PER_WINDOW; i++) sanitizeHtml('<h2 id="a">a</h2>');
    expect(clearWindow).not.toHaveBeenCalled();

    sanitizeHtml('<h2 id="a">a</h2>');
    expect(clearWindow).toHaveBeenCalledTimes(1);

    for (let i = 0; i < SANITIZE_CALLS_PER_WINDOW; i++) sanitizeHtml('<h2 id="a">a</h2>');
    expect(clearWindow).toHaveBeenCalledTimes(2);
  });

  it('keeps sanitizing correctly after the window is replaced', async () => {
    const { sanitizeHtml, clearWindow, SANITIZE_CALLS_PER_WINDOW } = await load();
    const config = { ALLOWED_TAGS: ['p'] };

    for (let i = 0; i < SANITIZE_CALLS_PER_WINDOW; i++) sanitizeHtml('<p>x</p>', config);
    expect(clearWindow).toHaveBeenCalledTimes(1);

    expect(sanitizeHtml('<p id="x">kept</p><img src=x onerror="alert(1)">', config)).toBe(
      '<p id="x">kept</p>',
    );
  });
});
