<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# E2E tests

Playwright suites that run against a deployed environment, never a local server. They check
what users get from the public site, the backend and Self Serve together. Case numbers in test
titles (`1.3`, `F16`) refer to the [E2E test plan](../../docs/testing/e2e-test-plan.md).

The Self Serve feature specs, which stub the BFF, live in
`lfx-self-serve/apps/lfx-one/e2e/mentorship-*`; these suites don't repeat them.

| Suite                                                              | Project | Signs in      | Environments |
| ------------------------------------------------------------------ | ------- | ------------- | ------------ |
| Smoke: public pages, program detail, directories, legacy redirects | `smoke` | No; read-only | PROD, DEV    |

## Running

```bash
pnpm exec playwright install chromium   # once
pnpm test:e2e:smoke                     # PROD
E2E_ENV=dev pnpm test:e2e:smoke         # DEV
```

`E2E_ENV` selects the environment (`prod` by default); the URLs are in [`env.ts`](env.ts).

## CI

[`.github/workflows/e2e.yml`](../../.github/workflows/e2e.yml) runs the smoke suite against PROD
and DEV every 6 hours, on pull requests that change the suites, and on demand (Actions → E2E →
Run workflow). A failed run uploads the Playwright report, with traces, as an artifact.

## Writing tests

- The data is live. Pick records from the public API at run time and assert the page's shape,
  never counts or names known in advance.
- Smoke tests stay signed out and read-only: they run against PROD.
- Use only synthetic data: no real names, emails or organisations in specs or fixtures.
