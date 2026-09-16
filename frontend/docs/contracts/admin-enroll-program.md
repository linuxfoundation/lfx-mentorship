<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Admin — Enroll a program

**Page:** `/mentorship/admin/enroll` (`EnrollProgramComponent`)
**Types:** `MentorshipEnrollRequest`, `MentorshipEnrollForm`, `MentorshipProgram`,
`MentorshipLfProject`, `MentorshipNameAvailability`, `MentorshipPrerequisite`,
`MentorshipProgramTerm`

Three-step wizard: Program Details → Program Setup → Prerequisites. Step
validation also runs on the server so a client cannot persist a partial body.
On success the UI toasts and returns to the programs list. The created program
lands as `pending`.

The BFF already mocks list/name/CII/project/create. Logo bytes and the import
copy payload (section 2) are still missing. CII lookup stays on the existing
`GET /api/mentorship/cii/:projectId` — do not add another CII endpoint.

---

## Import from an existing program (two calls)

The details step has a dropdown **Import from existing program**. Helper text:
"Copies details, skills and prerequisites from a program you have run before."

That is **not** creating a program. It only pre-fills the wizard so the admin
does not retype name, project, description, URLs, skills, and prerequisites.

| When                   | API                                        | Why                                                                                              |
| ---------------------- | ------------------------------------------ | ------------------------------------------------------------------------------------------------ |
| Page opens             | `GET /programs`                            | Fill the dropdown with `{ id, name }` options.                                                   |
| Admin picks one option | `GET /programs/:programId/enroll-template` | Fill the form fields. The list card does not have description, skills, CII id, or prerequisites. |

Today the second call does not exist. `onImportProgram()` copies from a
client-side mock (`formFromImportedMentorshipProgram`). Section 2 is the API
that replaces that mock.

---

## 1. Programs for the import dropdown

```http
GET /api/mentorship/programs
```

Same contract as [admin-programs-list.md](./admin-programs-list.md). The UI
currently calls it without `limit` (server default page). Return every program
the admin may copy from, or page it the same way as the list.

Only `id` and `name` are used in the dropdown.

---

## 2. Copy that program into the wizard

Fired when the import dropdown changes (`onImportProgram()`).

```http
GET /api/mentorship/programs/:programId/enroll-template
```

`:programId` is the `id` chosen in the dropdown.

This is **not** submit-enroll and **not** the program-detail header GET. It
returns the enroll-form fields so the wizard can `patchValue` them. Skip logo
and terms: the admin always picks a new logo, and the wizard always starts
with one default term (`createDefaultMentorshipTerm()`).

### Success `200`

Body is `MentorshipEnrollRequest` **without** `termsAccepted` and **without** a
logo:

```json
{
  "importProgramId": "mp_gridflow_fall26",
  "name": "GridFlow: Time-Series Ingestion Pipeline",
  "project": { "id": "proj-gridflow", "name": "GridFlow", "logoUrl": "https://…" },
  "technologies": ["GO", "Kubernetes", "GraphQL"],
  "description": "<p>Build a time-series ingestion pipeline…</p>",
  "repositoryUrl": "https://github.com/lfenergy/gridflow",
  "websiteUrl": "https://lfenergy.org",
  "ciiProjectId": "1842",
  "codeOfConductUrl": "https://www.contributor-covenant.org/version/2/1/code_of_conduct/",
  "logoFileName": "",
  "skills": ["GO", "Kubernetes"],
  "terms": [],
  "prerequisites": [
    {
      "id": "prereq-resume",
      "name": "Resume",
      "description": "Upload the most recent version of your resume.",
      "required": true,
      "requireFile": true,
      "custom": false
    }
  ]
}
```

`project` is the full LF project object (`MentorshipLfProject`), not a bare
`projectId`. The wizard writes `project.id` into the form's `projectId` control
and keeps `name` / `logoUrl` for the selected option.

Do not copy `terms`. The wizard always starts with one default term from
`createDefaultMentorshipTerm()`. `terms: []` tells the UI to keep that default.

### Errors

| Status | When                                            |
| ------ | ----------------------------------------------- |
| `404`  | Unknown program, or the admin cannot import it. |

---

## 3. Search Linux Foundation projects

```http
GET /api/mentorship/lf-projects?search=&offset=0&limit=10
```

Powers the project picker (`MENTORSHIP_LF_PROJECT_PAGE_SIZE` = 10). Filter is
typeahead on `name`; the picker lazy-loads the next page.

### Success `200` — `MentorshipLfProjectsResponse`

```json
{
  "data": [{ "id": "proj-gridflow", "name": "GridFlow", "logoUrl": "https://…" }],
  "total": 20
}
```

`logoUrl` is optional. The selected option must remain resolvable after search
results change (return the selected project even when it is not on the current
page, or the UI keeps a local cache).

---

## 4. Unique program name

```http
GET /api/mentorship/programs/name-available?name=GridFlow:%20Time-Series
```

Debounced on the details step. The wizard will not leave Details until the
trimmed name is `available`. Comparison is case-insensitive against existing
program names.

### Success `200`

```json
{ "available": true }
```

### Errors

| Status | When                     |
| ------ | ------------------------ |
| `400`  | `name` missing or blank. |

---

## 5. Upload program logo

There is no upload path today. The wizard only stores `logoFileName` and a
browser `blob:` preview. Bytes must be persisted before or with create.

```http
POST /api/mentorship/programs/logo
Content-Type: multipart/form-data
```

| Part   | Rules                                                              |
| ------ | ------------------------------------------------------------------ |
| `file` | JPG or PNG only (no SVG). Max 2 MB. Intended display size 420×420. |

### Success `201`

```json
{ "logoFileName": "gridflow.png", "logoUrl": "https://…" }
```

Create (endpoint 6) then sends `logoFileName` and may also send `logoUrl` if
you return it here. If you prefer a single multipart create, skip this
endpoint and accept `file` on `POST /programs` instead — the UI will switch to
whichever lands.

---

## 6. Submit enrollment

```http
POST /api/mentorship/programs
```

Blocked during impersonation.

### Body — `MentorshipEnrollRequest`

```json
{
  "importProgramId": "",
  "name": "GridFlow: Time-Series Ingestion Pipeline",
  "projectId": "proj-gridflow",
  "technologies": ["GO", "Kubernetes"],
  "description": "<p>…</p>",
  "repositoryUrl": "https://github.com/lfenergy/gridflow",
  "websiteUrl": "https://lfenergy.org",
  "ciiProjectId": "1842",
  "codeOfConductUrl": "https://www.contributor-covenant.org/version/2/1/code_of_conduct/",
  "logoFileName": "gridflow.png",
  "skills": ["GO", "Kubernetes"],
  "terms": [
    {
      "id": "term-1-2026",
      "name": "Term 1 - 2026",
      "startDate": "2026-12-01",
      "endDate": "2027-02-01",
      "applicationStartDate": "2026-09-17",
      "applicationEndDate": "2026-11-30"
    }
  ],
  "prerequisites": [
    {
      "id": "prereq-resume",
      "name": "Resume",
      "description": "Upload the most recent version of your resume.",
      "required": true,
      "requireFile": true
    },
    {
      "id": "prereq-coding",
      "name": "Coding Challenge",
      "description": "Complete a code challenge",
      "required": false,
      "challengeUrl": "https://example.com/challenge"
    },
    {
      "id": "prereq-custom-1",
      "name": "Blog draft",
      "description": "Write a short intro post.",
      "required": true,
      "custom": true,
      "dueDate": "2026-10-01",
      "requireFile": true
    }
  ],
  "termsAccepted": true
}
```

### Field rules (also enforced by `getMentorshipEnrollStepErrors`)

| Field                                               | Rule                                                                                                                                                        |
| --------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `name`                                              | Required, trimmed length 3–100, unique (case-insensitive).                                                                                                  |
| `projectId`                                         | Required. Must exist in the LF project catalog.                                                                                                             |
| `description`                                       | HTML; visible text ≤ 3000.                                                                                                                                  |
| `repositoryUrl` / `websiteUrl` / `codeOfConductUrl` | Valid URL when present.                                                                                                                                     |
| `ciiProjectId`                                      | Empty or numeric. When present, must exist at CII.                                                                                                          |
| `logoFileName`                                      | Required; `jpg` / `jpeg` / `png`.                                                                                                                           |
| `skills`                                            | At least one. Catalog is client-side (`MENTORSHIP_SKILL_OPTIONS`); no API.                                                                                  |
| `terms`                                             | At least one. At most 4 open terms (`MENTORSHIP_MAX_OPEN_TERMS`). Date windows must be valid (see enroll term dialog).                                      |
| `prerequisites`                                     | Each selected (`required: true`) item needs a description. Coding challenge needs `challengeUrl` when required. Custom items: name ≤ 20, description ≤ 500. |
| `termsAccepted`                                     | Must be `true`.                                                                                                                                             |

Term dates:

- `startDate` / `endDate` — ISO `YYYY-MM-01` (month precision).
- `applicationStartDate` / `applicationEndDate` — ISO `YYYY-MM-DD`.
- Application window must fall before the term start; end ≥ start.

### Success `201` — `MentorshipProgram`

Same card shape as the programs list. `status` is `pending`. `term` is
the first term's `name`. `stats` are zeros. `projectName` is resolved from
`projectId`.

### Errors

| Status | When                                                       |
| ------ | ---------------------------------------------------------- |
| `400`  | Wizard validation failed. Message is the first step error. |
| `409`  | Name or slug already taken.                                |

---

## Not an API on this page

- **Skills / technologies catalog** — `MENTORSHIP_SKILL_OPTIONS` in shared constants.
- **Default prerequisites** — `MENTORSHIP_DEFAULT_PREREQUISITES`; the client sends the edited list on create.
- **Add / edit / delete term in the wizard** — local form state until submit.
- **Policy links / CoC template / CII apply URL** — static constants.
- **CII Best Practices lookup** — already `GET /api/mentorship/cii/:projectId`. No new API.
- **Cancel** — client confirm, then navigate back. No draft-save API.
