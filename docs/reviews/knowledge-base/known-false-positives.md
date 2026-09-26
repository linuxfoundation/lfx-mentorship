<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Known false positives

This is the suppression floor for the empirical reviewer. A candidate is
suppressed only when the same active entry covers it at both the reviewed base
and target revisions. Adding a waiver cannot approve the change that adds it;
removing a waiver immediately reactivates the finding.

There are currently no known false-positive waivers.

Future entries must name the pattern ID, exact path/symbol or condition, reason,
owner, approval reference, and expiry or revalidation condition. Broad path-only
or repository-wide waivers are not valid.