<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship Indexer Contract

## `mentorship_program`

Mentorship publishes one snapshot per program on `lfx.index.mentorship_program`.
The resource ID is the program UUID.

| Field | Value |
| --- | --- |
| `object_type` | `mentorship_program` |
| `access_check_object` | `mentorship_program:{program UUID}` |
| `access_check_relation` | `writer` |
| `public` | `true` only when program status is `published` |
| `name_and_aliases` | Program name and slug |
| `parent_refs` | `project:{project_uid}` when present |
| `tags` | `status:{status}`, `project_uid:{project_uid}` when present |

`data` contains the program card fields required by Mentorship clients: `id`,
`name`, `slug`, `status`, `logo_url`, `project_uid`, `created_on`, and
`updated_on`.

The Query Service uses the access-check fields to include direct and inherited
program writers, including Project Service's `mentorship_program_admin` tuples.