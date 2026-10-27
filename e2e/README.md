<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# P1 E2E tests

Playwright tests for [docs/testing/p1-test-plan.md](../docs/testing/p1-test-plan.md). They run against a deployed environment (DEV by default) with real accounts; nothing is mocked.

## Setup

Needs Node 22+ and pnpm.

```bash
cd e2e
pnpm install
pnpm exec playwright install chromium
cp .env.example .env   # then fill in the passwords
```

`.env` is gitignored. Never commit it or paste its values anywhere.

## Run

```bash
pnpm test                    # sign in all users, then run every spec
pnpm test --grep "1.7"       # one case, by plan number
pnpm test --grep ADM-04-01   # one case, by workbook scenario ID
pnpm test:headed             # watch the browser
pnpm report                  # open the last HTML report
```

The `setup` project signs in each user once and saves the session to `e2e/.auth/<user>.json` (gitignored). Specs reuse those sessions. A user with no credentials in `.env` is skipped, and so are the specs that need it.

## Users

| Key | Account | Roles |
|---|---|---|
| `adminA` | sameh100 | Program Admin A, approver, then Mentor 2 |
| `adminB` | samehlol16 | Approver, Program Admin B, Mentor 1 |
| `mentee1` | sameh001 | Mentee 1, non-approver |
| `mentee2` | samehhotmail | Brand-new user, then mentee from an Apply link, then Mentee 2 |

`mentee2` signs in on the dashboard, so it makes no Mentorship call before case 12.6. Keep it that way until 12.6 has run.

## Writing specs

- One file per plan section, numbered so they run in plan order: `01-admin-programs.spec.ts`, `12-api-auth.spec.ts`.
- Title each test with the plan case and the workbook IDs: `test('1.7 Program page [ADM-04-01]', …)`.
- Use `asUser('adminA')` from `support/fixtures.ts` to get a signed-in page. Call it more than once for multi-user flows; each user gets their own browser context.
- Prefer the app's `data-testid`s (`mentorship-program-detail-title`, `mentorship-program-card-<id>`, …).
- Put shared IDs in `support/test-data.ts`, and prefix anything you create with `E2E`.
- Specs run one at a time, because the plan uses up state (registering, graduating). Follow the plan's [suggested order](../docs/testing/p1-test-plan.md#suggested-order) when you add files.
