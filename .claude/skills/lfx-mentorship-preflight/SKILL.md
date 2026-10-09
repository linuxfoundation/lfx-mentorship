---
# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT
name: lfx-mentorship-preflight
description: >
  Mechanical pre-PR validation for the lfx-mentorship Go and Nuxt monorepo.
  In report-only mode checks license headers, formatting, lint, tests, builds,
  and commit state without rewriting tracked files.
context: fork
allowed-tools: Bash, Read, Glob, Grep
---

<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# LFX Mentorship preflight

Args: `[base-ref] --report-only [extra instructions]`; default to `origin/main`.
The lifecycle invokes this action only in `--report-only` mode. Never edit,
format, generate, migrate a database, deploy, commit, reset, push, or write
GitHub state. Build caches and ignored build outputs are allowed; tracked files
must remain unchanged.

First run:

```bash
git status --short
git diff --name-only <base>...HEAD
git log --format='%H %s%n%B' <base>..HEAD
```

Fail if the tree starts dirty, there are no commits to check, or any commit
lacks a `Signed-off-by:` trailer. Save the initial `git status --porcelain` and
compare it after all checks; any tracked-file side effect is a failure and must
be left for the parent session to handle.

## Backend checks

When the range touches `backend/**`, run from `backend/`:

```bash
make license-check
test -z "$(gofmt -l $(git ls-files '*.go'))"
make lint
make test
make build
```

Do not run `make fmt`, `make db-migrate`, `make test-integration`, generators,
Docker builds, Helm installs, or deploy targets. Integration tests require an
explicit disposable database and are outside this default preflight.

## Frontend checks

When the range touches `frontend/**`, require Node 22+ and pnpm, then run from
`frontend/`:

```bash
pnpm format:check
pnpm lint
pnpm build
```

Do not run `pnpm format` or `pnpm lint:fix`. If dependencies are absent, report
the prerequisite rather than installing or changing the lockfile.

## Documentation and policy checks

For every changed non-generated text file, verify the MIT/SPDX header is
present in the file's established comment style. Report changes to migration,
contract, chart, Docker, `CLAUDE.md`, `.claude/skills/**`, and review
knowledge-base paths as protected callouts.

Render PASS, FAIL, SKIP, or WARN for working tree, DCO, license, formatting,
lint, tests, builds, and protected paths. Verdict is `NOT READY` for any FAIL,
`READY WITH CALLOUTS` for WARN only, and `READY FOR PR` otherwise. Include exact
failing commands and concise output; do not fix failures.