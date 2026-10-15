// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const runtime = vi.hoisted(() => ({ intercomAppId: 'app123' as string | undefined }));
vi.mock('nuxt/app', () => ({
  useRuntimeConfig: () => ({ public: { intercomAppId: runtime.intercomAppId } }),
}));

type Script = { src?: string; onload?: () => void; onerror?: () => void };
type FakeWindow = {
  Intercom?: ReturnType<typeof vi.fn> | ((...args: unknown[]) => void);
  intercomSettings?: Record<string, unknown>;
};

let win: FakeWindow;
let scripts: Script[];

// Intercom state is module-level, so each test loads a fresh copy.
const load = async () => (await import('./useIntercom')).useIntercom();

/** Completes the script load the way the real widget does: it replaces the queue stub. */
function finishScriptLoad(): ReturnType<typeof vi.fn> {
  const intercom = vi.fn();
  win.Intercom = intercom;
  scripts.at(-1)!.onload!();
  vi.advanceTimersByTime(100);
  return intercom;
}

beforeEach(() => {
  vi.resetModules();
  vi.useFakeTimers();
  runtime.intercomAppId = 'app123';
  scripts = [];
  win = {};
  vi.stubGlobal('window', win);
  vi.stubGlobal('document', {
    createElement: () => {
      const script: Script = {};
      scripts.push(script);
      return script;
    },
    getElementsByTagName: () => [],
    head: { appendChild: vi.fn() },
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('useIntercom.boot', () => {
  it('rejects without a window', async () => {
    const intercom = await load();
    vi.stubGlobal('window', undefined);
    await expect(intercom.boot({})).rejects.toThrow('Window is undefined');
  });

  it('rejects without an app ID', async () => {
    runtime.intercomAppId = undefined;
    await expect((await load()).boot({})).rejects.toThrow('No Intercom app ID');
    expect(scripts).toHaveLength(0);
  });

  it('loads the widget, then boots an anonymous visitor', async () => {
    const intercom = await load();
    const booted = intercom.boot({ name: 'Visitor' });
    expect(scripts[0]!.src).toBe('https://widget.intercom.io/widget/app123');
    expect(win.intercomSettings).toEqual({
      api_base: 'https://api-iam.intercom.io',
      app_id: 'app123',
    });
    const widget = finishScriptLoad();
    await booted;
    expect(widget.mock.calls).toEqual([
      ['boot', { api_base: 'https://api-iam.intercom.io', app_id: 'app123', name: 'Visitor' }],
    ]);
  });

  it('keeps the identity JWT out of the boot call', async () => {
    const intercom = await load();
    const booted = intercom.boot({
      user_id: 'u1',
      name: 'Ada',
      email: 'a@b.org',
      intercom_user_jwt: 'jwt',
    });
    expect(win.intercomSettings?.intercom_user_jwt).toBe('jwt');
    const widget = finishScriptLoad();
    await booted;
    expect(widget).toHaveBeenCalledWith(
      'boot',
      expect.not.objectContaining({ intercom_user_jwt: expect.anything() }),
    );
    expect(widget).toHaveBeenCalledWith('update', { user_id: 'u1', name: 'Ada', email: 'a@b.org' });
  });

  it('only updates when booted again in the same mode', async () => {
    const intercom = await load();
    const first = intercom.boot({ user_id: 'u1' });
    const widget = finishScriptLoad();
    await first;
    widget.mockClear();
    await intercom.boot({ user_id: 'u1', name: 'Ada', intercom_user_jwt: 'jwt', app_id: 'other' });
    expect(widget.mock.calls).toEqual([['update', { user_id: 'u1', name: 'Ada' }]]);
  });

  it('shuts down and reboots to identify an anonymous visitor', async () => {
    const intercom = await load();
    const anonymous = intercom.boot({});
    const widget = finishScriptLoad();
    await anonymous;
    widget.mockClear();
    const identified = intercom.boot({ user_id: 'u1' });
    vi.advanceTimersByTime(100);
    await identified;
    expect(widget.mock.calls.map((c) => c[0])).toEqual(['shutdown', 'boot', 'update']);
  });

  it('reuses an Intercom function already on the page', async () => {
    const existing = vi.fn();
    win.Intercom = existing;
    const booted = (await load()).boot({});
    expect(existing).toHaveBeenCalledWith('reattach_activator');
    finishScriptLoad();
    await booted;
  });

  it('rejects when the widget never loads', async () => {
    const booted = (await load()).boot({});
    scripts[0]!.onerror!();
    vi.advanceTimersByTime(10000);
    await expect(booted).rejects.toThrow('Script failed to load');
  });

  it('rejects and stays unbooted when the boot call throws', async () => {
    const intercom = await load();
    const booted = intercom.boot({});
    win.Intercom = () => {
      throw new Error('boom');
    };
    scripts[0]!.onload!();
    vi.advanceTimersByTime(100);
    await expect(booted).rejects.toThrow('boom');
    const widget = vi.fn();
    win.Intercom = widget;
    intercom.show();
    expect(widget).not.toHaveBeenCalled();
  });
});

describe('useIntercom update, show, and shutdown', () => {
  it('does nothing before boot', async () => {
    const intercom = await load();
    const widget = vi.fn();
    win.Intercom = widget;
    intercom.update({ name: 'x' });
    intercom.show();
    intercom.shutdown();
    expect(widget).not.toHaveBeenCalled();
  });

  it('forwards commands once booted and forgets the JWT on shutdown', async () => {
    const intercom = await load();
    const booted = intercom.boot({ user_id: 'u1', intercom_user_jwt: 'jwt' });
    const widget = finishScriptLoad();
    await booted;
    widget.mockClear();
    intercom.update();
    intercom.show();
    intercom.shutdown();
    expect(widget.mock.calls).toEqual([['update', {}], ['show'], ['shutdown']]);
    expect(win.intercomSettings?.intercom_user_jwt).toBeUndefined();
    intercom.show();
    expect(widget).toHaveBeenCalledTimes(3);
  });

  it('logs rather than throws when the widget fails', async () => {
    const intercom = await load();
    const booted = intercom.boot({});
    finishScriptLoad();
    await booted;
    win.Intercom = () => {
      throw new Error('widget error');
    };
    expect(() => {
      intercom.update({ name: 'x' });
      intercom.show();
      intercom.shutdown();
    }).not.toThrow();
    expect(console.error).toHaveBeenCalledTimes(3);
  });

  it('is a no-op without a window', async () => {
    const intercom = await load();
    vi.stubGlobal('window', undefined);
    expect(() => intercom.shutdown()).not.toThrow();
  });
});
