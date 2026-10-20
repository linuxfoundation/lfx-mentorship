// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { afterEach, describe, expect, it, vi } from 'vitest';
import robots from './robots';

vi.mock('nitropack/runtime', () => ({ defineNitroPlugin: (fn: unknown) => fn }));

type RobotsContext = { groups: unknown[] };

// Runs the plugin's robots:config hook against a context the module would have built.
function resolveRobots(): RobotsContext {
  let hook: ((ctx: RobotsContext) => void) | undefined;
  const nitroApp = { hooks: { hook: (_name: string, fn: typeof hook) => (hook = fn) } };
  (robots as unknown as (app: typeof nitroApp) => void)(nitroApp);
  const ctx: RobotsContext = { groups: [{ userAgent: ['*'], allow: ['/'] }] };
  hook!(ctx);
  return ctx;
}

afterEach(() => {
  vi.unstubAllEnvs();
});

describe('robots plugin', () => {
  it('leaves production indexable', () => {
    vi.stubEnv('NUXT_PUBLIC_APP_ENV', 'production');
    expect(resolveRobots().groups).toEqual([{ userAgent: ['*'], allow: ['/'] }]);
  });

  it.each(['development', 'staging', ''])('blocks crawlers when the env is %j', (env) => {
    vi.stubEnv('NUXT_PUBLIC_APP_ENV', env);
    expect(resolveRobots().groups).toEqual([
      { userAgent: ['*'], disallow: ['/'], allow: [], comment: [] },
    ]);
  });
});
