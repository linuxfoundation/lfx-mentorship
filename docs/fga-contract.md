<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship FGA Contract

Mentorship publishes derived authorization state to `lfx-v2-fga-sync` through
its transactional outbox and JetStream relay. The generic envelope and subject
semantics are defined by the [fga-sync contract](https://github.com/linuxfoundation/lfx-v2-fga-sync/blob/main/docs/fga-sync-contract.md).

## Object Types

| Object type | Operations | Source |
|---|---|---|
| `mentorship_program` | `update_access`, `delete_access`, `member_put`, `member_remove` | `programs`, `program_members` |
| `mentorship_application` | `update_access`, `delete_access` | `applications` |
| `mentorship_task` | `update_access`, `delete_access` | `tasks` |
| `mentorship_approver_team` | `member_put`, `member_remove` | `mentorship_approver_team_members` |

All UIDs are canonical PostgreSQL IDs. User values in `relations` and
`username` are LFIDs, not local user UUIDs.

## Program Full Sync

A program `update_access` is derived from the current program, its project
link, and effective memberships:

```json
{
  "object_type": "mentorship_program",
  "operation": "update_access",
  "data": {
    "uid": "<program-uid>",
    "public": true,
    "relations": {
      "writer": ["<program-admin-lfid>"],
      "mentor": ["<accepted-mentor-lfid>"]
    },
    "references": {
      "project": ["<project-uid>"],
      "global_mentorship_approver": ["mentorship_approver_team:global#member"]
    },
    "exclude_relations": []
  }
}
```

`project` is a typed parent reference. The approver-team userset belongs in
`references`, not `relations`, because it is not an LFID.

## Child Full Syncs

Applications reference their program and carry the applicant relation:

```json
{
  "object_type": "mentorship_application",
  "operation": "update_access",
  "data": {
    "uid": "<application-uid>",
    "public": false,
    "relations": {"mentee": ["<applicant-lfid>"]},
    "references": {"mentorship_program": ["<program-uid>"]}
  }
}
```

Tasks reference their application and carry the assignee relation:

```json
{
  "object_type": "mentorship_task",
  "operation": "update_access",
  "data": {
    "uid": "<task-uid>",
    "public": false,
    "relations": {"assignee": ["<assignee-lfid>"]},
    "references": {"mentorship_application": ["<application-uid>"]}
  }
}
```

`delete_access` is emitted for hard deletion. Status changes such as decline,
withdrawal, graduation, submission, and review do not delete authorization
tuples.

Relay failures are retried with generation guards. After the configured maximum
attempt count, a marker moves to the `dead_letter` state and remains in the
outbox with its last error for operator inspection and replay. Dead-lettered
markers are never silently discarded.

Operators should monitor pending outbox age, `fga_relay_dead_lettered`, and
tombstones whose `last_reconciled_on` is stale. Replay a dead-lettered marker
with `fga-replay <marker-id>` after correcting its underlying data or dependency
failure; do not delete the marker manually.

## Membership Mutations

Accepted mentor and program-admin changes use precise membership messages:

```json
{
  "object_type": "mentorship_program",
  "operation": "member_put",
  "data": {
    "uid": "<program-uid>",
    "username": "<mentor-lfid>",
    "relations": ["mentor"]
  }
}
```

Removal names the exact relation and never relies on an empty relation list for
Mentorship-owned flows. Global approver-team membership uses the
`mentorship_approver_team:global` object and `member` relation. Project-wide
`mentorship_program_admin` membership is owned and emitted by Project Service.

Global approver roster management is restricted to the platform-issued
`manage:mentorship:approvers` scope. Project-wide program-admin storage and
assignment are owned by Project Service; Mentorship consumes the inherited
`project#mentorship_program_admin` relation and does not write competing
project-level role tuples.
