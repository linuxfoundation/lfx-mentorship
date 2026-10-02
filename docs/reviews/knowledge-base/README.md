<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Review knowledge base

This directory is the canonical empirical input for
`/lfx-mentorship-learnings-review`. Entries are accepted only when a reviewer
raised the issue on this repository, a later commit fixed it, and the fix still
exists in current code.

`patterns.md` records those findings and their operational detection rules.
`known-false-positives.md` is the explicit suppression floor. Written design
rules, generic correctness advice, and unresolved review opinions do not belong
here.

Each pattern must include:

- severity (`Critical` or `Important`);
- `Pattern` and precise `Detect` clauses with exclusions;
- reviewer name, stable review-comment URL, and verbatim quote;
- exact fixing commit and current-code status;
- failure message and concrete fix.

Before changing a pattern, re-audit the cited comment, fixing commit, current
implementation, and exclusions. A new waiver cannot suppress a finding about
the same reviewed change because the learnings reviewer requires matching
waivers at both the base and target revisions.