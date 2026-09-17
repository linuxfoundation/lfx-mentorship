<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Admin — Programs list

**Page:** `/mentorship/admin` (`AdminComponent`)
**Types:** `MentorshipProgram`, `MentorshipProgramsResponse`, `MentorshipProgramStatus`

Card list of programs the signed-in admin can manage. Search and status filter
are server-side; **Load more** appends the next page. Clicking a card navigates
to `/mentorship/admin/:programId` using the program `id`. **Enroll a Program**
is client navigation only.

---

## 1. List programs

```http
GET /api/mentorship/programs
```

**Backend:** `GET /v1/me/managed-programs` — self-scoped collection with
per-result program `writer` filtering. Includes non-public programs the caller
administers. The BFF forwards the session JWT.

### Query

| Param    | Type                                 | Default                                       | Notes                               |
| -------- | ------------------------------------ | --------------------------------------------- | ----------------------------------- |
| `search` | string                               | omitted                                       | Case-insensitive match on `name`.   |
| `status` | `published` \| `pending` \| `hidden` | omitted                                       | Exact match. Invalid value → `400`. |
| `offset` | number                               | `0`                                           |                                     |
| `limit`  | number                               | UI sends `2` (`MENTORSHIP_PROGRAM_PAGE_SIZE`) | Server default `50`, max `50`.      |

### Success `200`

```json
{
  "data": [
    {
      "id": "mp_gridflow_fall26",
      "slug": "gridflow-time-series-ingestion-pipeline",
      "name": "GridFlow: Time-Series Ingestion Pipeline",
      "projectName": "LF Energy",
      "activeTerm": { "id": "term_id", "name": "Fall 2026" },
      "status": "published",
      "stats": { "mentors": 4, "mentees": 2, "graduated": 6 },
      "logoUrl": "https://…/icon.svg"
    }
  ],
  "total": 4
}
```

### Fields the card renders

| Field                                                 | UI                                                                                               |
| ----------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| `logoUrl`                                             | Avatar. Omit to show an initials tile from `name`.                                               |
| `projectName` + `activeTerm.name`                     | Season line, e.g. `LF Energy · Fall 2026`.                                                      |
| `status`                                              | Badge: Published / Pending / Hidden (UI maps to display labels).                                 |
| `name`                                                | Card title.                                                                                      |
| `stats.mentors` / `stats.mentees` / `stats.graduated` | Metric columns.                                                                                  |
| `id`                                                  | Navigation target. `slug` is kept so detail URLs can resolve either.                             |

`stats.mentees` is the live enrolled count (accepted). `stats.graduated` is the
lifetime graduated count for the program.

### Errors

| Status | When                                                     |
| ------ | -------------------------------------------------------- |
| `400`  | `status` is not one of `published`, `pending`, `hidden`. |
| `401`  | No session.                                              |

---

## Not an API on this page

- **Enroll a Program** → navigate to `/mentorship/admin/enroll`.
- Card click → navigate to `/mentorship/admin/:programId`. No extra fetch on this page.
