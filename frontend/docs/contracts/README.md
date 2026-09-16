<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship UI API contracts

UI-driven contracts for the BFF routes under `/api/mentorship`. Each page in the
mentorship module has its own file so backend work can start from what that
screen actually loads and mutates.

These are the APIs the Angular admin UI needs. Shared TypeScript shapes live in
`packages/shared/src/interfaces/mentorship.interface.ts`. Constants (statuses,
labels, page sizes) live in `packages/shared/src/constants/mentorship*.constants.ts`.

Mentor and mentee page contracts will be added in a later pass.

## Admin pages

| Page             | Route                          | Contract                                             |
| ---------------- | ------------------------------ | ---------------------------------------------------- |
| Programs list    | `/mentorship/admin`            | [admin-programs-list.md](./admin-programs-list.md)   |
| Enroll a program | `/mentorship/admin/enroll`     | [admin-enroll-program.md](./admin-enroll-program.md) |
| Program detail   | `/mentorship/admin/:programId` | [admin-program-detail.md](./admin-program-detail.md) |

## How endpoints are split

- **One list GET per tab.** A page with underline tabs does not load every tab in
  one payload. The page header has its own GET; each tab has its own list GET.
  Current Mentees, Past Mentees, and Applicants all use `GET .../mentees` with
  `type=current|past|all` — there is no `/applicants` collection.
- **Status-changing row actions share one PATCH.** Accept / decline / withdraw /
  graduate / close / re-open are the same operation: `PATCH` with `{ "status": "..." }`.
  Do not add `/accept`, `/decline`, `/graduate` routes. Applicant status writes
  use the mentee PATCH.
- **Every other table or toolbar action is its own endpoint.** Invite, remove,
  create task, list tasks, save note, create / edit / delete a term.
- **Downloads are UI-only.** No export APIs and no task view/download APIs — the
  UI uses `task.file`.
- **`:programId` accepts the program `id` or `slug`**, matching
  `/mentorship/admin/:programId`.

## Status vocabulary

These contracts use the **backend's status values** (the source of truth). The
Self Serve Angular UI maps them to display labels in its own BFF/constants.

| Domain         | Wire values (used here)                                             | Self Serve UI display                                                     |
| -------------- | ------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| Program        | `pending`, `published`, `hidden`                                    | Pending Review, Open, Completed                                           |
| Application    | `pending`, `accepted`, `declined`, `withdrawn`, `graduated`, `hold` | same (+ `applied`/`tasks-completed` split of `pending` in Applicants tab) |
| Program member | `pending`, `approved`, `declined`, `withdrawn`                      | Invited, Accepted, Declined, Withdrawn                                    |
| Task           | `pending`, `in_progress`, `submitted`, `completed`                  | same                                                                      |
| Term           | `open`, `closed`                                                    | same                                                                      |

## Shared conventions

| Concern         | Rule                                                                                                          |
| --------------- | ------------------------------------------------------------------------------------------------------------- |
| Auth            | Session required (`getUsernameFromAuth`). Unauthenticated → `401`.                                            |
| Impersonation   | Reads allowed. Writes use `blockDuringImpersonation` → `403 IMPERSONATION_READ_ONLY`.                         |
| List pagination | `{ data, total }` with `offset` + `limit`. Cap `limit` at 50.                                                 |
| Search          | Case-insensitive match on the fields named in each contract.                                                  |
| Dates           | Date-only fields are `YYYY-MM-DD`. Timestamps are ISO-8601.                                                   |
| Errors          | `400` validation, `404` unknown resource, `409` conflict (duplicate name, cannot close term, max open terms). |
