// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { rmSync } from 'node:fs';
import { expect, test as setup } from '@playwright/test';
import { env } from './env';
import { signIn } from './support/auth';
import { credentials, missingUsers, storageStatePath, USERS } from './users';

// Signs each journey user in through the real LFX sign-in page and saves the session, so the
// journey runs against the real Auth0, Heimdall and backend chain rather than a token shortcut.
for (const user of USERS) {
  setup(`sign in ${user}`, async ({ page }) => {
    // A session saved by an earlier run would let the journey run without this one signing in.
    rmSync(storageStatePath(user), { force: true });
    setup.skip(env.name !== 'dev', 'the journey runs on DEV only');
    setup.skip(missingUsers().length > 0, `missing credentials for ${missingUsers().join(', ')}`);

    // From the dashboard, not a Mentorship page: a Mentorship call would provision MENTEE_2, which
    // the journey registers as a brand-new user.
    await page.goto(env.selfServeUrl);
    await signIn(page, credentials(user)!);
    await expect(page).toHaveURL(new RegExp(`^${env.selfServeUrl.replaceAll('.', '\\.')}`));

    await page.context().storageState({ path: storageStatePath(user) });
  });
}
