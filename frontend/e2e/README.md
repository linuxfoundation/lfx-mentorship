<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# E2E tests

Playwright suites that run against a deployed environment, never a local server. They check
what users get from the public site, the backend and Self Serve together. Case numbers in test
titles (`1.3`, `F16`) refer to the E2E test plan, `docs/testing/e2e-test-plan.md` (added in #356).

The Self Serve feature specs, which stub the BFF, live in
`lfx-self-serve/apps/lfx-one/e2e/mentorship-*`; these suites don't repeat them.

| Suite                                                              | Project   | Signs in             | Environments |
| ------------------------------------------------------------------ | --------- | -------------------- | ------------ |
| Smoke: public pages, program detail, directories, legacy redirects | `smoke`   | No; read-only        | PROD, DEV    |
| Journey: one program from enrollment to graduation, several users  | `journey` | Yes, 4 test accounts | DEV only     |

## Running

```bash
pnpm exec playwright install chromium   # once
pnpm test:e2e:smoke                     # PROD
E2E_ENV=dev pnpm test:e2e:smoke         # DEV
E2E_ENV=dev pnpm test:e2e:journey       # DEV; needs the test accounts below
```

`E2E_ENV` selects the environment (`prod` by default); the URLs are in [`env.ts`](env.ts).

## The journey

[`journey/journey.spec.ts`](journey/journey.spec.ts) drives the real Self Serve UI as four DEV
accounts, each in its own browser context:

| Account    | Role in the journey                                                             |
| ---------- | ------------------------------------------------------------------------------- |
| `ADMIN_A`  | Enrolls program A and is its Program Admin; approves program B, then mentors it |
| `ADMIN_B`  | Approves program A, then mentors it; enrolls program B and is its Program Admin |
| `MENTEE_1` | Applies to A, uploads the prerequisites, is accepted, works a task, graduates   |
| `MENTEE_2` | Opens A's Apply link, registering first if it has no mentee profile; declined   |

Both admin accounts must be on the DEV approver team. The setup project signs each account in
through the LFX sign-in page and saves the session to `e2e/.auth/` (gitignored). It signs in on
the dashboard, not a Mentorship page, so a brand-new `MENTEE_2` stays new until it registers;
that step runs only while the account has no mentee profile.

Credentials come from `E2E_<ACCOUNT>_USERNAME` and `E2E_<ACCOUNT>_PASSWORD`: CI secrets, or
`frontend/.env` locally (gitignored). The journey is skipped, not failed, when any is missing.
Never commit them: the setup enters them without Playwright recording them, and records no
traces.

Each run names its programs `E2E Journey A|B <timestamp>`. The teardown runs even when a step
fails: it rejects any journey program still pending, declines every application in the
published ones (walking a graduation back first, so the mentees can apply again), and hides
them. It also cleans up after earlier runs that failed.

### Secrets for CI

Add these repository secrets for
[`.github/workflows/e2e-journey.yml`](../../.github/workflows/e2e-journey.yml):

- `E2E_ADMIN_A_USERNAME`, `E2E_ADMIN_A_PASSWORD`
- `E2E_ADMIN_B_USERNAME`, `E2E_ADMIN_B_PASSWORD`
- `E2E_MENTEE_1_USERNAME`, `E2E_MENTEE_1_PASSWORD`
- `E2E_MENTEE_2_USERNAME`, `E2E_MENTEE_2_PASSWORD`

The accounts should be dedicated test LFIDs with synthetic profiles and no MFA: Self Serve
pages show the account's LFX profile (email, address, phone).

## CI

- [`.github/workflows/e2e.yml`](../../.github/workflows/e2e.yml) runs the smoke suite against
  PROD and DEV every 6 hours, on pull requests that change the suites, and on demand (Actions →
  E2E → Run workflow). A failed run uploads the Playwright report, with traces, as an artifact.
- [`.github/workflows/e2e-journey.yml`](../../.github/workflows/e2e-journey.yml) runs the journey
  on DEV daily and on demand, never on pull requests, since it creates data. Runs queue rather
  than cancel, so a teardown always finishes. It keeps no screenshots or traces and uploads no
  report, because they would show the accounts' profiles: the log names the failed step and
  locator, and a local run has the screenshots.

## Writing tests

- The data is live. Pick records from the public API at run time and assert the page's shape,
  never counts or names known in advance.
- Smoke tests stay signed out and read-only: they run against PROD.
- Journey steps that read something just created retry it (`toPass`): new permissions reach the
  permission store a few seconds after the create.
- Use only synthetic data: no real names, emails or organisations in specs or fixtures. Account
  usernames come from the environment, never from the code.
