---
# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT
name: lfx-mentorship-pr-readiness
description: >
  Non-fixing PR shape check for lfx-mentorship. Verifies a clean committed
  branch, base ancestry, DCO signoffs, commit subjects, change size, and
  protected-file callouts before local review.
context: fork
allowed-tools: Bash, Read, Glob, Grep
---

<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# LFX Mentorship PR readiness

Args: `[base-ref] [extra instructions]`; default to `origin/main`. This action
is report-only. Never edit, format, generate, commit, reset, push, or write
GitHub state. Do not fetch: the lifecycle supplies a verified base ref.

Run:

```bash
git status --short
git rev-parse --abbrev-ref HEAD
git rev-parse --verify <base>
git merge-base --is-ancestor <base> HEAD
git diff --shortstat <base>...HEAD
git diff --name-only <base>...HEAD
git log --format='%H%x09%s%x09%D%n%B' <base>..HEAD
```

Stop as `NOT READY` when the tree is dirty, the base is missing, there are no
commits ahead of the base, or the base is not an ancestor of `HEAD`.

Verify every commit has a `Signed-off-by:` trailer, as required by `CLAUDE.md`.
Report unusual or vague subjects, but do not invent a ticket or GPG-signing
requirement that this repository does not declare.

Call out these protected paths without automatically failing them:

- `backend/db/migrations/**` and `backend/db/scripts/**` - schema/data safety.
- `backend/internal/infrastructure/**outbox**`, `**/indexer/**`, and
  `backend/charts/**/templates/ruleset.yaml` - platform contracts.
- `backend/charts/**`, `frontend/charts/**`, and either Dockerfile - deployment.
- `go.mod`, `go.sum`, `frontend/package.json`, `frontend/pnpm-lock.yaml` - dependencies.
- `CLAUDE.md`, `.claude/skills/**`, and `docs/reviews/knowledge-base/**` - review policy.
- `docs/fga-contract.md`, `docs/heimdall-fga-implementation.md`,
  `backend/docs/indexer-contract.md`, and `docs/rewrite/**` - approved contracts.

Render a concise report with branch, base, commit count, diff size, and PASS,
WARN, or FAIL for clean tree, base ancestry, DCO, commit subjects, and protected
paths. Verdict is `NOT READY` for any FAIL, `READY WITH CALLOUTS` for WARN only,
and `READY` otherwise.