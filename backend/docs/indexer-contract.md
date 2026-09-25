<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship Indexer Contract

## `mentorship_program`

Mentorship publishes one snapshot per program on `lfx.index.mentorship_program`.
The resource ID is the program UUID.

| Field | Value |
| --- | --- |
| `object_type` | `mentorship_program` |
| `object_ref` | Derived by the indexer from the NATS subject and `object_id` |
| `access_check_object` | `mentorship_program:{program UUID}` |
| `access_check_relation` | `viewer` |
| `public` | `true` only when program status is `published` |
| `name_and_aliases` | Program name and slug |
| `parent_refs` | `project:{project_uid}` when present |
| `tags` | `status:{status}`, `project_uid:{project_uid}` when present |

`data` contains the program card fields required by Mentorship clients: `id`,
`name`, `slug`, `status`, `logo_url`, `project_uid`, `project_slug`,
`project_name`, `created_on`, and
`updated_on`, plus public enrollment statistics:

```json
"stats": {
	"mentors": 4,
	"mentees": 2,
	"graduated": 6
}
```

The stats are rebuilt from PostgreSQL in the same transaction as the index
outbox snapshot. Application lifecycle changes and active mentor membership
changes re-enqueue the program snapshot so counts do not remain stale.

The Query Service uses the access-check fields to include direct and inherited
program viewers, including Project Service's `mentorship_program_admin` tuples.

The DynamoDB importer queues one current-state snapshot for every program in
`index_outbox`; the normal index relay publishes those snapshots. Stored
authorization is redacted, and client-supplied `x-on-behalf-of` metadata is
discarded before it reaches the outbox. At publish time the relay obtains a
cached client-credentials token and replaces the redacted authorization value,
allowing the indexer to authenticate the service without trusting caller-
controlled actor attribution.

Failed publishes increment attempts once, wait for `INDEX_RELAY_RETRY_DELAY`,
and dead-letter after `INDEX_RELAY_MAX_ATTEMPTS`. A newer generation arriving
during a failed publish is requeued immediately. After fixing the cause of a
dead-lettered record, an operator can requeue one exact record without
publishing outside the relay:

```bash
/app/outbox-repair \
	--outbox=index \
	--object-type=mentorship_program \
	--object-uid=<uuid>
```

## `mentorship_application`

Mentorship publishes one snapshot per application on
`lfx.index.mentorship_application`. The resource ID is the application UUID.

| Field | Value |
| --- | --- |
| `object_type` | `mentorship_application` |
| `object_ref` | Derived by the indexer from the NATS subject and `object_id` |
| `access_check_object` | `mentorship_application:{application UUID}` |
| `access_check_relation` | `auditor` |
| `history_check_object` | `mentorship_program:{program UUID}` |
| `history_check_relation` | `auditor` |
| `public` | `false` |
| `tags` | `role:{role}`, `status:{status}` |

The document contains the application ID, program-term ID, applicant user ID,
role, status, term status, and creation/update timestamps. The application and
its parent program are resolved in the same transaction that writes the index
outbox record. Application create, update, bulk decline, reapply, and delete
transactions all update or delete the corresponding index record.

## `mentorship_task`

Mentorship publishes one snapshot per task on `lfx.index.mentorship_task`. The
resource ID is the task UUID.

| Field | Value |
| --- | --- |
| `object_type` | `mentorship_task` |
| `object_ref` | Derived by the indexer from the NATS subject and `object_id` |
| `access_check_object` | `mentorship_task:{task UUID}` |
| `access_check_relation` | `auditor` |
| `history_check_object` | `mentorship_application:{application UUID}` |
| `history_check_relation` | `auditor` |
| `public` | `false` |
| `tags` | `status:{status}`, `category:{category}`, `assignee_id:{user UUID}` |

The document contains the task ID, application ID, assignee ID, name,
description, category, `prerequisite`, status, application/term status, custom
flag, `submit_file`, submission `file`, due date, and creation/update
timestamps. These fields cover the task rows described by the mentee, mentor,
and admin UI contracts. Tasks created with an application, updated, deleted,
or created as part of an application/reapply transaction are coalesced through
the same generation-guarded index outbox.

Reviewer notes are intentionally not part of the shared application projection:
the UI contract makes them reviewer-only, while Query Service projections do not
provide field-level redaction for an applicant who can otherwise audit the
application.

Application and task index records use the same publish-time M2M
authorization, actor-attribution headers, retry policy, and exact dead-letter
repair command as program records. They remain non-public and are intended for
Query Service collection and direct-grant filtering after the corresponding
resource projections are enabled.
