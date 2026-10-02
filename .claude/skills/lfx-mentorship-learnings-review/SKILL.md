---
# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT
name: lfx-mentorship-learnings-review
description: >
  Repo-owned empirical reviewer for lfx-mentorship. Matches one pinned commit
  range only against documented, fixed findings from this repository's past PR
  reviews in docs/reviews/knowledge-base. Loaded as repo_learnings by
  /lfx-skills:lfx-local-review; not invoked by developers.
context: fork
allowed-tools: Bash, Read, Glob, Grep
---

<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# LFX Mentorship empirical reviewer

Review only the pinned `base_sha` and `target_sha` supplied by the host. Use
`git diff <base_sha>..<target_sha>` or `git show --stat -p <target_sha>` for a
root commit. Never substitute `HEAD`, a branch, or a remote ref.

This role owns only patterns extracted from actual review comments that were
fixed and remain valid. Generic correctness belongs to the central reviewer;
written repository rules belong to `/lfx-mentorship-code-review`.

Do not edit files, run fixing commands, commit, push, or write GitHub state. If
the range or any required knowledge-base file is unreadable, return
`INCOMPLETE — <reason>` as the first line.

## Load evidence

At `target_sha`, read these files in full:

- `docs/reviews/knowledge-base/README.md`
- `docs/reviews/knowledge-base/patterns.md`

Read `docs/reviews/knowledge-base/known-false-positives.md` independently at
both `base_sha` and `target_sha`. For each revision, use `git ls-tree <rev> --
<path>`: a nonzero exit is incomplete; no entry is an empty floor; an entry must
be mode `100644` and type `blob`. Read an accepted entry by its object ID with
`git cat-file blob <object-sha>`, not by resolving the path again. An unreadable
blob or wrong object type is incomplete and must name the failing revision. For
a root commit, the base floor is empty.

Apply the two floors semantically per candidate, never by diffing their bytes.
A candidate is suppressed only when the same waiver covers it at both
revisions. A change cannot waive a finding about itself, and removing a waiver
must reactivate the finding.

## Match

For every entry in `patterns.md`:

1. Evaluate its `Detect` clause against the changed range and full owning file
   at `target_sha`.
2. Honor every exclusion in the entry.
3. Quote a verbatim span from the entry's `Pattern` or `Detect` text.
4. Use only the entry's `Critical` or `Important` severity.
5. Drop candidates below confidence 80 or without a changed-line location.

Every finding must name the pattern ID, pattern file, changed file and line,
confidence, impact, quoted rule, and concrete fix. Do not emit an intuition that
cannot be tied to a knowledge-base entry.

```markdown
### Critical (N)
- **`path:line`** (confidence 95) - finding
  - Pattern: `pattern-id` in `docs/reviews/knowledge-base/patterns.md`
  - Evidence: "verbatim Pattern or Detect text"
  - Fix: concrete remedy

### Important (N)
...
```

Use `### No findings` when the review completed and no empirical pattern fires.
Every complete or incomplete report must include these verification lines with
the host-supplied values:

```text
Reviewed range: <base_sha>..<target_sha>
Skill: /lfx-mentorship-learnings-review
```

For a root commit, use `none..<target_sha>`. When the host had to load this file
through the permitted fallback, append `; read from: <exact path>` to the Skill
line as directed by the host.