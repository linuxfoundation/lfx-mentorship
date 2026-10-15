// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

type FakeWindow = {
  setTimeout: typeof setTimeout;
  dataLayer?: Record<string, unknown>[];
  airgap?: { getConsent?: () => { purposes?: Record<string, boolean | string> } | undefined };
  Intercom?: (...args: unknown[]) => void;
};

let win: FakeWindow;
let appEnv: string;

// The module remembers events it fired once, so each test gets a fresh copy.
const load = () => import('./useFunnelAnalytics');

beforeEach(() => {
  vi.resetModules();
  vi.useFakeTimers();
  win = { setTimeout: ((fn: () => void, ms: number) => setTimeout(fn, ms)) as typeof setTimeout };
  appEnv = 'development';
  vi.stubGlobal('window', win);
  vi.stubGlobal('useRuntimeConfig', () => ({ public: { appEnv } }));
  vi.stubGlobal('onMounted', (fn: () => void) => fn());
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const consent = (analytics: boolean | string) => ({
  getConsent: () => ({ purposes: { Analytics: analytics } }),
});

describe('trackFunnelEvent', () => {
  it('sends to the dataLayer and Intercom once analytics consent is given', async () => {
    const { trackFunnelEvent, FunnelEvent } = await load();
    win.airgap = consent(true);
    win.Intercom = vi.fn();
    trackFunnelEvent(FunnelEvent.ProgramDetailViewed, { program_id: 'p1' });
    expect(win.dataLayer).toEqual([{ event: 'program_detail_viewed', program_id: 'p1' }]);
    expect(win.Intercom).toHaveBeenCalledWith('trackEvent', 'program_detail_viewed', {
      program_id: 'p1',
    });
  });

  it.each([
    ['declined', consent(false)],
    ['a non-boolean answer', consent('Auto')],
    ['no consent record', { getConsent: () => undefined }],
    ['no consent API', {}],
  ])('sends nothing when analytics consent is %s', async (_label, airgap) => {
    const { trackFunnelEvent, FunnelEvent } = await load();
    win.airgap = airgap;
    trackFunnelEvent(FunnelEvent.ApplyStarted);
    expect(win.dataLayer).toBeUndefined();
  });

  it('keeps the dataLayer event when Intercom throws', async () => {
    const { trackFunnelEvent, FunnelEvent } = await load();
    win.airgap = consent(true);
    win.Intercom = () => {
      throw new Error('not booted');
    };
    trackFunnelEvent(FunnelEvent.ApplyStarted);
    expect(win.dataLayer).toHaveLength(1);
  });

  it('fires a once-keyed event only once', async () => {
    const { trackFunnelEvent, FunnelEvent } = await load();
    win.airgap = consent(true);
    trackFunnelEvent(FunnelEvent.DirectoryViewed, { directory: 'mentors' }, 'dir:mentors');
    trackFunnelEvent(FunnelEvent.DirectoryViewed, { directory: 'mentors' }, 'dir:mentors');
    trackFunnelEvent(FunnelEvent.DirectoryViewed, { directory: 'mentees' }, 'dir:mentees');
    expect(win.dataLayer).toHaveLength(2);
  });

  it('waits for the consent manager to load before deciding', async () => {
    const { trackFunnelEvent, FunnelEvent } = await load();
    trackFunnelEvent(FunnelEvent.ApplyStarted);
    vi.advanceTimersByTime(400);
    expect(win.dataLayer).toBeUndefined();
    win.airgap = consent(true);
    vi.advanceTimersByTime(200);
    expect(win.dataLayer).toEqual([{ event: 'apply_started' }]);
  });

  it.each([
    ['development', 1],
    ['production', 0],
  ])(
    'without a consent manager in %s, sends %i events after giving up waiting',
    async (env, sent) => {
      const { trackFunnelEvent, FunnelEvent } = await load();
      appEnv = env;
      trackFunnelEvent(FunnelEvent.ApplyStarted);
      vi.advanceTimersByTime(1800);
      expect(win.dataLayer).toBeUndefined();
      vi.advanceTimersByTime(200);
      expect(win.dataLayer?.length ?? 0).toBe(sent);
    },
  );
});

describe('useFunnelPageView', () => {
  it('tracks on mount, once per event by default', async () => {
    const { useFunnelPageView, FunnelEvent } = await load();
    win.airgap = consent(true);
    useFunnelPageView(FunnelEvent.DirectoryViewed, { directory: 'programs' });
    useFunnelPageView(FunnelEvent.DirectoryViewed, { directory: 'programs' });
    useFunnelPageView(FunnelEvent.ProgramDetailViewed, { program_slug: 'a' }, 'program:a');
    expect(win.dataLayer).toEqual([
      { event: 'directory_viewed', directory: 'programs' },
      { event: 'program_detail_viewed', program_slug: 'a' },
    ]);
  });
});
