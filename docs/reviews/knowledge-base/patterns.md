<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Empirical review patterns

## `migration/derived-state-seed` - Critical

**Pattern:** Bulk-imported domain rows must enqueue reconstructable FGA and
index derived state; transition-only relays cannot publish state for rows that
predate those transitions.

**Detect:** Flag a bulk import or removal of seed tooling when existing
programs, applications, tasks, approver memberships, or program index records
can be inserted without idempotently enqueueing their corresponding outbox
markers. Exclude ordinary runtime writes that already enqueue in the same
transaction and entities that are explicitly reported as unreconstructable.

**Empirical citation:** `copilot-pull-request-reviewer`, PR #175,
https://github.com/linuxfoundation/lfx-mentorship/pull/175#discussion_r4097847409:
"The DynamoDB ETL inserts domain rows directly and does not populate either
`fga_outbox` or `index_outbox`, so transition-only relays will never publish FGA
tuples or search documents for pre-existing programs/applications/tasks."
Fixed by `d09ab1606ec426ba33141083dbb48e0762be462c`; current importer seed logic
retains the fix.

**Failure message:** Imported rows can remain absent from authorization or
search after cutover.

**Fix:** Enqueue idempotent current-state markers through the normal outboxes
and report every row whose required parent or identity prevents reconstruction.

## `outbox/dead-letter-preservation-and-repair` - Critical

**Pattern:** Re-runnable seed/import code must preserve dead-letter state and
diagnostics, and hard-delete events must retain an exact-key operator repair
path because the source row no longer exists.

**Detect:** Flag an import/upsert that changes an existing `dead_letter` marker
to pending, clears its attempts/retry timestamp/error, advances its generation,
or replaces a retained delete operation. Also flag removal of exact-key repair
for retained deletion markers. Exclude explicit operator-selected repair of one
marker and new/non-dead-letter rows.

**Empirical citations:**

- `copilot-pull-request-reviewer`, PR #175,
  https://github.com/linuxfoundation/lfx-mentorship/pull/175#discussion_r4100762453:
  "Preserve `dead_letter`, its attempt count, and `last_error` here; only the
  explicit repair command should revive it."
- `copilot-pull-request-reviewer`, PR #175,
  https://github.com/linuxfoundation/lfx-mentorship/pull/175#discussion_r4097847475:
  "Re-running the domain write is impossible for a dead-lettered `delete_access`
  marker because the source row has already been hard-deleted."

Fixed by `289e3a91b9c34632b0706e09f0f50a9dd2c452f4` and
`d09ab1606ec426ba33141083dbb48e0762be462c`; current importer and
`cmd/outbox-repair` retain the fixes.

**Failure message:** A routine importer rerun can revive every unresolved
failure, while a failed deletion can leave stale authorization permanently.

**Fix:** Preserve dead-letter metadata during idempotent seeding and allow only
bounded exact-key operator repair, never a broad replay or direct publish.

## `migration/complete-unmapped-report` - Important

**Pattern:** Migration output must identify every unmapped row with the IDs
operators need to repair or quarantine it; counts and samples are insufficient.

**Detect:** Flag a migration path that truncates unresolved output, emits only a
count, or omits identifiers needed to locate the source and intended parent or
assignee. Exclude aggregate summary lines when complete per-row warnings are
also emitted.

**Empirical citations:**

- `copilot-pull-request-reviewer`, PR #175,
  https://github.com/linuxfoundation/lfx-mentorship/pull/175#discussion_r4097847524:
  "This only prints the first ten program IDs, but the migration contract
  requires reporting every unmapped program."
- `copilot-pull-request-reviewer`, PR #175,
  https://github.com/linuxfoundation/lfx-mentorship/pull/175#discussion_r4097847562:
  "The count does not identify which tasks need remediation."

Fixed by `d09ab1606ec426ba33141083dbb48e0762be462c`; current importer emits
per-row `UNMAPPED_*` warnings with identifying context.

**Failure message:** Operators cannot completely repair or quarantine imported
rows before authorization enforcement.

**Fix:** Emit one warning per unresolved row with its source ID and all known
program, term, user, parent, or assignee identifiers, plus an aggregate summary.

## `indexer/publisher-authorization-test` - Important

**Pattern:** The index relay's machine authorization must overwrite any stored
authorization value at publish time while preserving actor attribution, and a
relay test must assert both headers on the emitted envelope.

**Detect:** Flag a change to index authorization, header sanitization, envelope
construction, or relay tests when no test proves configured authorization
overwrites an empty or sanitized stored value and `x-on-behalf-of` survives.
Exclude unrelated payload-only index changes.

**Empirical citation:** `copilot-pull-request-reviewer`, PR #175,
https://github.com/linuxfoundation/lfx-mentorship/pull/175#discussion_r4100762487:
"Add a relay test that starts with sanitized/empty headers, calls
`SetAuthorization`, and checks the emitted header value."
Fixed by `289e3a91b9c34632b0706e09f0f50a9dd2c452f4`; current authorization
provider and relay tests retain the fix.

**Failure message:** The relay can acknowledge publication while downstream
indexing rejects an unauthenticated envelope or loses actor attribution.

**Fix:** Restore publish-time machine authorization and a relay test asserting
authorization replacement plus preservation of `x-on-behalf-of`.