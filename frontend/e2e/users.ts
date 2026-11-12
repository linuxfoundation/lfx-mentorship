// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// The DEV test accounts the journey suite signs in as. Credentials come only from the
// environment: CI secrets, or frontend/.env (gitignored) locally. Never commit them.

export const USERS = ['ADMIN_A', 'ADMIN_B', 'MENTEE_1', 'MENTEE_2'] as const;

export type User = (typeof USERS)[number];

export interface Credentials {
  username: string;
  password: string;
}

export function credentials(user: User): Credentials | undefined {
  const username = process.env[`E2E_${user}_USERNAME`];
  const password = process.env[`E2E_${user}_PASSWORD`];
  return username && password ? { username, password } : undefined;
}

export function missingUsers(): User[] {
  return USERS.filter((user) => !credentials(user));
}

// Where the setup project saves each user's signed-in session (gitignored).
export function storageStatePath(user: User): string {
  return `e2e/.auth/${user.toLowerCase()}.json`;
}
