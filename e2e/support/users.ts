// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import path from 'node:path';

export type UserKey = 'adminA' | 'adminB' | 'mentee1' | 'mentee2';

export interface TestUser {
  envPrefix: string;
  roles: string;
}

// Roles follow the "Test users" table in docs/testing/p1-test-plan.md.
export const USERS: Record<UserKey, TestUser> = {
  adminA: { envPrefix: 'E2E_ADMIN_A', roles: 'Program Admin A, approver, then Mentor 2' },
  adminB: { envPrefix: 'E2E_ADMIN_B', roles: 'Approver, Program Admin B, Mentor 1' },
  mentee1: { envPrefix: 'E2E_MENTEE_1', roles: 'Mentee 1, non-approver' },
  mentee2: { envPrefix: 'E2E_MENTEE_2', roles: 'Brand-new user, then mentee from an Apply link, then Mentee 2' },
};

export const USER_KEYS = Object.keys(USERS) as UserKey[];

const AUTH_DIR = path.resolve(__dirname, '../.auth');

export function storageStatePath(key: UserKey): string {
  return path.join(AUTH_DIR, `${key}.json`);
}

export interface Credentials {
  username: string;
  password: string;
}

/** Returns the user's credentials, or undefined when neither half is set. Half a pair is an error. */
export function credentialsFor(key: UserKey): Credentials | undefined {
  const { envPrefix } = USERS[key];
  const username = process.env[`${envPrefix}_USERNAME`] ?? '';
  const password = process.env[`${envPrefix}_PASSWORD`] ?? '';

  if (!username && !password) {
    return undefined;
  }
  if (!username || !password) {
    throw new Error(`Set both ${envPrefix}_USERNAME and ${envPrefix}_PASSWORD, or neither to skip ${key}.`);
  }
  return { username, password };
}
