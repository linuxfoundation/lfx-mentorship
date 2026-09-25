<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship Indexer Contract

## `mentorship_program`

Mentorship publishes one snapshot per program on `lfx.index.mentorship_program`.
The resource ID is the program UUID.

| Field | Value |
| --- | --- |
| `object_type` | `mentorship_program` |
| `object_ref` | `mentorship_program:{program UUID}` |
| `access_check_object` | `mentorship_program:{program UUID}` |
| `access_check_relation` | `viewer` |
| `public` | `true` only when program status is `published` |
| `name_and_aliases` | Program name and slug |
| `parent_refs` | `project:{project_uid}` when present |
| `tags` | `status:{status}`, `project_uid:{project_uid}` when present |

`data` contains the program card fields required by Mentorship clients: `id`,
`name`, `slug`, `status`, `logo_url`, `project_uid`, `created_on`, and
`updated_on`.

The Query Service uses the access-check fields to include direct and inherited
program viewers, including Project Service's `mentorship_program_admin` tuples.

The DynamoDB importer queues one current-state snapshot for every program in
`index_outbox`; the normal index relay publishes those snapshots. Stored
authorization is redacted, while `x-on-behalf-of` is retained. At publish time
the relay obtains a cached client-credentials token and replaces the redacted
authorization value, allowing the indexer to authenticate the service and
attribute the initiating user.

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
