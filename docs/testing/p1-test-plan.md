<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# LFX Mentorship: P1 Test Plan

The P1 test cases for the Mentorship module in LFX Self Serve (frontend and BFF) against the real lfx-mentorship backend. It covers all 178 P1 scenarios in [`Mentorship_QA_Use_Cases.xlsx`](Mentorship_QA_Use_Cases.xlsx), grouped into cases the same way as the [E2E test plan](e2e-test-plan.md).

The E2E plan runs the full program lifecycle. This plan goes deeper into Self Serve: every P1 screen state, impersonation, cross-user access (IDOR), BFF authentication and log redaction. Where the E2E run already covered a case, the **Ref** column gives its number and result (`e2e 3.5 ✅`), so you can re-confirm it quickly rather than test it from scratch.

| | DEV | PROD |
|---|---|---|
| Public site | https://mentorship.dev.lfx.dev | https://mentorship.linuxfoundation.org |
| Self Serve | https://app.dev.lfx.dev/mentorship | https://app.lfx.dev/mentorship |
| BFF API | https://app.dev.lfx.dev/api/mentorship | https://app.lfx.dev/api/mentorship |

**Last run:** in progress, started 2026-10-26 on DEV. Program A is enrolled and approved; see [Test data created on DEV](#test-data-created-on-dev).

- **DEV:** every case.
- **PROD:** the read-only and access-control subset in [section 14](#14-prod).

---

## How to run

1. Follow steps 1–5 of [How to run](e2e-test-plan.md#how-to-run) in the E2E plan: Playwright MCP with `--isolated`, one browser context per test user, sign in by hand, save storage state to a scratch directory, keep fixtures under `.playwright-mcp/`, and prefix test data with `E2E`.
2. **API cases** (marked "API:") go straight to the BFF. Use the signed-in context's `appSession` cookie with curl, Postman or `context.request`. Never paste a cookie or token into the chat or this file.
3. **Impersonation cases** ([section 10](#10-impersonation)) need a session started by the impersonator user, then pointed at the target user.
4. **Log cases** ([section 13](#13-logs)) need Datadog access to the DEV Self Serve and mentorship-api logs.
5. **Fault cases** (5.7, 9.6) need a failing request: block it in devtools, or run Self Serve locally with `LFX_V2_SERVICE` pointing at a stub.
6. The [Endpoints](Mentorship_QA_Use_Cases.xlsx) sheet lists all 61 BFF endpoints with the role each one needs and whether it is a write. Cases 10.6, 10.7 and 11.1 run through it.
7. Record results here, and in the workbook's Status column using the scenario IDs in **Ref**.

### Test users

Four accounts plus a logged-out browser. Each account plays several roles, and the two fresh accounts use up their "new user" state on their first steps, so follow the [suggested order](#suggested-order).

| User | Roles in this run | State before the run | Used by |
|---|---|---|---|
| `sameh100` | **Program Admin A**, approver, then **Mentor 2**; possibly the **impersonator** | Approver. Program Admin of several programs, including Program A, the new program enrolled for this run. No mentor profile; its inbox is readable. Self Serve shows it an "Impersonate user" button | 1.x–4.x (admin of Program A), 5.4 (race), 6.1–6.13 (registers as a mentor, then gets invited by email and in the app), 11.6 and 11.7 |
| `samehlol16` | **Approver**, **Program Admin B**, **Mentor 1** | Approver. Program Admin and mentor of several programs | 5.1–5.4 (approves and rejects), 6.4 (opens sameh100's invite link), 6.14 and 7.x (mentor of Program A), 11.2 and 11.3. Its own programs act as "Program B" in 11.7 |
| `sameh001` | **Mentee 1**; non-approver | Has a mentee profile and one withdrawn application | 2.5, 2.8, 8.4–8.8, 9.x (full lifecycle on Program A, graduated last), 5.5, 5.6, 11.1, 11.4–11.5 |
| `samehhotmail` | **Brand-new user**, then the **mentee from an Apply link**, then **Mentee 2** | Has never opened Mentorship | 12.6 (first visit), 8.3 (registers from an Apply link), then 2.6–2.7, 2.9, 3.10, 8.7, 9.3 (pending applications), and the other mentee in 11.4–11.5 |
| Logged out | Anonymous | No session | 5.8, 12.1 |

**Not covered by these accounts**

| Cases | Needs | Plan |
|---|---|---|
| 8.1 Register as a mentee directly | One more fresh account (samehhotmail's "no profile" state goes to 8.3, which has a known bug, e2e F14) | Rely on e2e 5.1 ✅, unless you add a fresh account |
| 5.9 First-time approver | An approver that has never opened Mentorship | Skip |
| 10.x Impersonation | An account with the Auth0 claim `http://lfx.dev/claims/can_impersonate: true` | sameh100 shows an "Impersonate user" button, so it probably has the claim. Confirm by starting an impersonation of sameh001 before section 10 |
| 1.5 Load more | A Program Admin of 13 or more programs | Run as sameh100 or samehlol16 if either has 13 or more (the pending enrollments in section 4 count); otherwise skip |

**Account setup**

- **Feature flag:** every account must be targeted by the `mentorship-enabled` LaunchDarkly flag (required on PROD; check DEV). Otherwise Self Serve redirects to the dashboard.
- **Approvers:** sameh100 and samehlol16 are already in the approver team (`approverTeam.members` in `values/{dev,prod}/lfx-mentorship-backend.yaml` in lfx-v2-argocd).
- **Inbox:** the invite emails in 6.1–6.4 go to sameh100. In the E2E run, plus-addresses on Gmail didn't arrive from DEV, so the account's base address must be a real inbox.
- **Datadog:** read access to DEV logs for section 13.

### Test data to prepare

| Item | Owner | Needed for |
|---|---|---|
| **Program A** `E2E P1 Program 2026-10-26`: full details, logo, 2 open terms accepting applications, all 5 standard prerequisites plus 1 custom. Enrolled by sameh100, approved by samehlol16 | sameh100 | Most admin, mentor and mentee cases |
| **Program B**: any published program of samehlol16 with an open term | samehlol16 | 11.7 |
| 3 more published programs with an open term accepting applications (samehlol16's or other DEV programs) | any | 8.7 (samehhotmail: 3 pending plus a 4th) |
| 3 more pending programs from section 4 enrollments (`E2E` prefix) | sameh100 | 5.3 (reject), 5.4 (race), 1.4 |
| 1 pending program enrolled by sameh001, at the very end of the run | sameh001 | 5.6 (a program's own admin who is not an approver can't approve it). Reject it as samehlol16 afterwards. Done last, so sameh001 has no admin role during sections 2–11 |
| A program of sameh100 with no logo | sameh100 | 1.6 |
| sameh100 programs that are Completed (only closed terms), Hidden and Rejected | sameh100 | 1.4, 2.13 |
| Mentors on Program A: samehlol16 **requests** to mentor and sameh100 **approves** (3.2); sameh100 **invites** sameh001 and **revokes** it (3.4) | sameh100, samehlol16 | 3.1–3.4, 7.x |
| Mentors on samehlol16's programs: sameh100 **requests** one and samehlol16 **declines** (3.3); samehlol16 **invites** sameh100 to others by email and in the app (6.1–6.2, 6.12–6.13); samehlol16 **removes** sameh100 from one (3.5) | samehlol16, sameh100 | 3.3, 3.5, 6.x |
| Terms on Program A: one with pending applications (to close), one closed (to re-open), one with no applications (to delete) | sameh100 | 3.10–3.12 |
| Applications on Program A in each state: Applied, Tasks Completed, Accepted, Declined, Withdrawn, Graduated. sameh001 gives Accepted and Graduated; samehhotmail's applications give the rest (a withdrawn mentee can reapply, e2e 5.6) | sameh001, samehhotmail | 2.x, 9.1–9.2 |
| Tasks: file-required, not file-required, due today (UTC), due in the past (the UI accepts past dates, see F-14), submitted, approved | sameh100, samehlol16 | 7.x, 9.x, 12.9 |

### Test data created on DEV

| Item | ID | State |
|---|---|---|
| Program A: E2E P1 Program 2026-10-26 (slug `e2e-p1-program-2026-10-26`), project Academy Software Foundation (ASWF) | `97059425-e2cb-4b9a-b432-411b78678c47` | Open. Enrolled by sameh100, approved by samehlol16. Logo stored. Skills Go and Kubernetes; CII ID 1. All 5 standard prerequisites are required, plus the custom "E2E Intro Video" (file required, due 2026-12-15). No mentors or mentees yet |
| E2E Term A Nov 2026 (Nov 2026 – Feb 2027; applications 2026-10-26 to 2026-11-30) | `f426493e-d188-456a-8ee2-6fa54f5e69cf` | Open |
| E2E Term B Dec 2026 (Dec 2026 – Mar 2027; applications 2026-10-26 to 2026-12-15) | `26a7efd4-a373-4ea4-b1b5-b77371ce6b05` | Open |

Apply link for 8.3: `https://app.dev.lfx.dev/mentorship/mentee/apply?programId=97059425-e2cb-4b9a-b432-411b78678c47&programTermId=f426493e-d188-456a-8ee2-6fa54f5e69cf`

**Fixtures** (in `.playwright-mcp/`, as in the E2E run): PNG under 2 MB; PNG over 2 MB; small PDF, DOC, DOCX and TXT; 21 MB PDF; a JSON body over 15 MB (12.13); and payload strings `<script>alert(1)</script>` and `<img src=x onerror=alert(1)>` (12.16).

### Suggested order

Some cases use up state, so run them in this order:

1. **samehhotmail first visit (12.6)**, before it touches Mentorship in any other way. Then it opens an Apply link to Program A and registers (8.3).
2. **Enrollment and review (4, 5.1–5.5, 5.7–5.8, 5.10):** sameh100 enrolls Program A and the extra pending programs; samehlol16 approves Program A.
3. **Mentor setup (3, 6):** sameh100 registers as a mentor (6.5) before it is invited anywhere.
4. **Mentees apply (8):** sameh001 to Program A; samehhotmail to Program A and 3 other programs, then a 4th.
5. **Applications and tasks (2, 7, 9).** Graduate sameh001 last: a graduate can't apply again, and an accepted mentee can't apply elsewhere (e2e 5.7, 5.12).
6. **Access and API (11, 12).**
7. **Logs (13).**
8. **5.6 last:** sameh001 enrolls a throwaway program and tries to approve it.

---

## Status legend

| Symbol | Meaning |
|---|---|
| ✅ | Passed |
| ❌ | Failed (see [Findings](#findings)) |
| ⚠️ | Passed with a caveat, or a known limitation |
| — | Not tested |

`F-xx` refers to the workbook's Findings sheet; `e2e Fxx` refers to a [finding in the E2E plan](e2e-test-plan.md#findings).

---

## 1. Admin: access, programs list, program page

| # | Case | Expected | Ref | DEV |
|---|---|---|---|---|
| 1.1 | Open `/mentorship` with the flag on | Redirects to `/mentorship/admin`; "Admin" header and "Enroll a Program" button | ADM-01-01 | — |
| 1.2 | Programs list | Cards only for programs the user administers, sorted by name; logo or initial avatar, project name, status badge, name, Mentors / Mentees / Graduated counts | ADM-02-01 | — |
| 1.3 | Search | Debounced about 200 ms; matches program or project name, case-insensitive | ADM-02-02 | — |
| 1.4 | Status filter | Open, Pending Review, Completed, Rejected and Hidden each filter the list; clearing with × restores it | ADM-02-03 | — |
| 1.5 | Load more (13+ programs) | Pages of 12 appended; Load more hidden once all are loaded | ADM-02-04 | — |
| 1.6 | Add logo from a card | "Logo missing" hint; PNG or JPEG up to 2 MB gives "Logo added." and the list reloads with the logo | ADM-03-01 · e2e 2.5 (e2e F2, fixed on DEV) | — |
| 1.7 | Program page | Header with name, project, status badge, View Public Page (new tab to `<mentorship>/programs/<id>`) and Edit Program; tabs Current Mentees, Past Mentees, Mentors and Terms with counts | ADM-04-01 | ✅ Program A: Open badge; counts 0 / 0 / 0 / 2 |
| 1.8 | Hide | ⋯ > Hide > confirm: "Program hidden."; badge Hidden | ADM-05-01 · e2e 3.5 ✅ | — |
| 1.9 | Unhide | "Program unhidden."; previous badge restored | ADM-05-02 · e2e 3.5 ✅ | — |

## 2. Admin: mentees and applications

| # | Case | Expected | Ref | DEV |
|---|---|---|---|---|
| 2.1 | Current Mentees tab | 10 rows a page; "Showing {first} to {last} of {total}"; columns Mentee (with note link), Term, Status, Application Dates, Other Active Applications, Actions | ADM-06-01 | — |
| 2.2 | Search and filters | Search debounced 300 ms; status and term filters apply; any change goes back to page 1 | ADM-06-02 | — |
| 2.3 | Filter options | Status: All, Applied, Tasks Completed, Accepted, Declined, Withdrawn, Graduated. Term: "All open terms" plus open terms only | ADM-06-03, ADM-06-04 | — |
| 2.4 | Expand a row | First expand shows "Loading tasks..." and then the application's tasks | ADM-07-01 | — |
| 2.5 | Accept | Full time or Part time required; "Application accepted"; table and counts refresh; the mentee sees Accepted | ADM-08-01 · e2e 6.1 ✅ | — |
| 2.6 | Decline | Pending or accepted row: "Application declined" | ADM-09-01 · e2e 6.7 (PROD ✅) | — |
| 2.7 | Withdraw for the mentee | Confirm says "This can't be undone."; "Application withdrawn" | ADM-09-02 · e2e 6.6 ✅ | — |
| 2.8 | Graduate | Accepted rows only: "Mentee graduated" | ADM-09-03 · e2e 6.5 ✅ | — |
| 2.9 | Decline by Term | A single open term is preselected; "N applications declined" (or "1 application declined"); pending applications in the term become Declined; accepted ones untouched | ADM-10-01 · e2e 6.2 ✅ | — |
| 2.10 | Reviewer note | Save: "Note saved", kept after reload. Clear and save: "Note cleared" | ADM-11-01, ADM-11-02 | — |
| 2.11 | Create a task | Name, description, optional due date, file-required flag; the mentee sees the task | ADM-12-01 · e2e 6.3 ✅ | — |
| 2.12 | Download CSV | All or one status; file `current-mentees-<status or all>.csv` (or `past-…`); columns Name, Email, Status, Term, Tasks Submitted, Applied On, Updated On; UTF-8 BOM; opens cleanly in Excel | ADM-13-01 · e2e 6.4 ✅ | — |
| 2.13 | Past Mentees tab | Read-only rows (Mentee, Term, Status) with no actions, notes or tasks; 10 a page. Status filter: All, Pending, Accepted, Declined, Withdrawn, Graduated. Term filter: "All closed terms" plus closed terms | ADM-14-01, ADM-14-02, ADM-14-03 | — |

## 3. Admin: mentors and terms

| # | Case | Expected | Ref | DEV |
|---|---|---|---|---|
| 3.1 | Mentors tab | Columns Mentor (avatar and name), Status, Invitation/Application date (or "-"), Profile Created? (Yes/No), Actions; search 300 ms; 6-status filter. No mentor email anywhere | ADM-15-01, ADM-15-09 | — |
| 3.2 | Approve a request | Requested or Pending row: "Mentor approved." | ADM-15-02 · e2e 3.4 | — |
| 3.3 | Decline a request | "Mentor declined." | ADM-15-03 · e2e 3.4 ✅ | — |
| 3.4 | Revoke an invite | Invited row: "Invite revoked."; status Declined | ADM-15-04 | — |
| 3.5 | Remove a mentor | Approved row: "Mentor removed."; status Withdrawn | ADM-15-05 · e2e 3.7 ✅ | — |
| 3.6 | Invite a mentor | Type 2+ characters (name, LF username or full email); results show name and LFID only; Invite disabled until a pick; "Invitation sent."; field clears; row shows Invited | ADM-16-01 · e2e 3.3 ✅ | — |
| 3.7 | Terms tab | Columns Term, Status, Pending, Declined, Accepted, Graduated, Dates, Application Dates, Actions | ADM-17-01 | — |
| 3.8 | Create a term | Name up to 50 (with counter), start and end month-year, application start and end dates: "Term created." | ADM-17-02, ADM-18-01 · e2e 3.2 ✅ | — |
| 3.9 | Edit a term | "Term updated." | ADM-17-03 · e2e 3.2 ✅ | — |
| 3.10 | Close a term | No accepted mentees; confirm warns that pending applications are declined; "Term closed."; pending become Declined | ADM-17-04 · e2e 3.2, 3.9 ✅ | — |
| 3.11 | Re-open a term | "Term re-opened." | ADM-17-05 | — |
| 3.12 | Delete a term | No applications: "Term deleted." | ADM-17-06 | — |

## 4. Enrollment and edit

| # | Case | Expected | Ref | DEV |
|---|---|---|---|---|
| 4.1 | Step 1 | Name 3–100, LF project, technologies, description, repo URL, PNG or JPEG logo up to 2 MB: Next goes to Setup | ADM-19-01 · e2e 2.1 ✅ | ✅ Playwright: Step 1 filled (1×1 PNG logo); Next goes to Setup |
| 4.2 | CII ID | A valid CII ID shows the badge | ADM-19-09 | ✅ Playwright: ID 1 shows the badge |
| 4.3 | LF project picker | Search and scroll through pages; Retry after a failure recovers | ADM-19-10 | ✅ Playwright: scrolling reads the next page; search finds ASWF; Retry after a blocked read recovers |
| 4.4 | Import from an existing program | Details, technologies, skills and prerequisites filled; terms empty; logo and T&C not copied | ADM-19-12 | ✅ Playwright: imported Program A; terms empty; logo and T&C not copied |
| 4.5 | Step 2 | At least 1 skill and 1–4 terms (add, edit, delete with confirm): Next proceeds | ADM-20-01 | ✅ Playwright: Next with no skill shows "Add at least one skill."; Add Term disabled at 4; delete asks to confirm |
| 4.6 | Step 3 prerequisites | Toggle Resume, Cover Letter, School Enrollment, Permission, Coding Challenge; add a custom one; all kept in the form | ADM-20-03 | ✅ Playwright: all 5 toggled plus a custom one; kept after Back and Next |
| 4.7 | Submit | Program created as pending; logo uploaded; "Program submitted for review."; listed as Pending Review; review email sent | ADM-21-01 · e2e 2.4 ✅, 2.5 (e2e F2) | ✅ Playwright: 4 programs created; logo uploaded; toast shown; card and API say Pending Review. Email not checked |
| 4.8 | Leave the wizard with no changes | Leaves without a prompt | ADM-22-01 | ✅ Playwright: Cancel leaves with no prompt |
| 4.9 | Edit program | Opens `/admin/enroll?programId=…` titled "Edit program", prefilled; open terms editable, closed read-only; Import and T&C hidden; logo optional | ADM-23-01 · e2e 3.1 ✅ | ✅ Playwright: on Program A |
| 4.10 | Update | "Program updated."; back on the program page | ADM-23-02 · e2e 3.1 ✅ | ✅ Playwright: website changed; "Program updated."; back on the program page |

## 5. Program review

| # | Case | Expected | Ref | DEV | PROD |
|---|---|---|---|---|---|
| 5.1 | Approve | `/mentorship/program-review/<id>?decision=approve`: "Approve this program?" with the name; Confirm sends one POST; "Program approved"; program published and visible; the admin's card goes from Pending Review to Open | PR-01-01, PR-01-05 · e2e 2.6 ✅ | ✅ Playwright: samehlol16 approved an `E2E P1 Approve` program; one POST; card Open, then hidden by the clean-up. Program A also approved by hand | — |
| 5.2 | Open the link only | Only a GET; no POST (safe against mail-scanner prefetch) | PR-01-02 | ✅ Playwright: no write sent on opening the link; program still pending. The read runs during server render | — |
| 5.3 | Reject | `?decision=reject`: red Reject button; "Program rejected"; the admin sees Rejected | PR-02-01 · e2e 2.8 ✅ | ✅ Playwright: red Reject; "Program rejected"; card Rejected | — |
| 5.4 | Two approvers at once (sameh100 and samehlol16) | Both open the page; the second Confirm gets 409 "Another reviewer decided this program before you confirmed." | PR-04-02 | ✅ Playwright: second Confirm got 409 and the "Another reviewer…" message. Program hidden by the clean-up | — |
| 5.5 | Non-approver (sameh001) opens the link | 403 "You don't have permission to review programs"; no confirm card | PR-05-01 · e2e 2.7 ✅ | ✅ Playwright: sameh001 got 403 and the no-permission state; no confirm card | — |
| 5.6 | Program's own admin, not an approver, confirms (sameh001 on the program it enrolls at the end of the run) | The confirm card may show; POST gets 403; status unchanged | PR-05-02 · e2e 7.5 ✅ | — Automated; runs last with `E2E_RUN_LAST=1` | — |
| 5.7 | Timeout, then retry | Fault case. The UI says the decision was not recorded; check the real status; a retry may get 409 (F-28) | PR-07-03 | — `test.fixme`: needs a stubbed backend | — |
| 5.8 | Logged out | Auth0 login; returnTo keeps `?decision` | PR-08-02 · e2e 7.1 ✅ | ✅ Playwright: Auth0 login returned to the link with `?decision=approve` | — |
| 5.9 | First-time approver (skipped: no such account) | Expected to work. May fail with a generic error, because the review page skips the user-setup retry (F-27) | PR-08-04 | — `test.fixme`: no first-time approver | — |
| 5.10 | Cross-site form POST to `/program-review/:id/decision` | No session cookie sent (SameSite=Lax), so 401. There is no CSRF token | PR-08-06 | ✅ Playwright: cross-site form POST got 401; status still pending | — |

## 6. Mentor: invitations, registration, profile

| # | Case | Expected | Ref | DEV |
|---|---|---|---|---|
| 6.1 | Accept from the email link | `/mentorship/mentor/invites?token=…` > Accept: "Invitation accepted" with "Go to My Programs"; the program is in My Programs | MTR-01-01 · e2e 4.2 — | — |
| 6.2 | Decline from the email link | No confirm; "Invitation declined"; button goes to `/mentorship/mentor/programs` | MTR-01-02 | — |
| 6.3 | Open the link only | No API request sent (safe against mail-scanner prefetch) | MTR-01-03 | — |
| 6.4 | Open sameh100's invite link signed in as samehlol16 | "This invitation is for a different account" | MTR-01-06 | — |
| 6.5 | Register as a mentor | Introduction, 1+ skill, compliance and terms ticked: "Profile created"; picked programs requested; lands on My Programs | MTR-02-01 · e2e 4.3 ⚠️ (e2e F4) | — |
| 6.6 | Program picker on the form | First 20 published programs, scroll loads more. A pick adds a local Pending row and sends nothing; picking again does nothing; withdrawing a local pick removes it with no API call | MTR-03-01, MTR-03-02, MTR-03-03 | — |
| 6.7 | Submit with several picks | One "Requests sent" toast listing the programs | MTR-03-07 | — |
| 6.8 | Profile tab | LFX profile card, About Me (sanitised), skill chips, Mentoring History (program, term, N mentees, In Progress or Completed), in-progress first | MTR-05-01 | — |
| 6.9 | Edit profile | PATCH sends only changed fields; "Profile updated"; drawer closes; page updates | MTR-06-01 | — |
| 6.10 | Request to mentor from the drawer | Sent at once; "Request sent"; shows Pending | MTR-07-01 · e2e 4.4 ✅ | — |
| 6.11 | Withdraw a request | "Request withdrawn" | MTR-07-04 | — |
| 6.12 | Accept an invitation in the app | "Invited" badge; Accept with no confirm; "Invitation accepted / You are now a mentor of this program."; history refreshes | MTR-08-01 · e2e 4.1 ✅ | — |
| 6.13 | Decline an invitation in the app | Confirm "...Only a program administrator can invite you again."; "Invitation declined" | MTR-08-02 | — |
| 6.14 | My Programs | One card per program: project, status (Open or Completed), name, logo or initial, Mentees / Tasks to Review / Applicants | MTR-04-01 · e2e 4.5 ✅ | — |

## 7. Mentor: program work

| # | Case | Expected | Ref | DEV |
|---|---|---|---|---|
| 7.1 | Program page | Header with name, project and status; Tasks / Mentees / Applicants tabs with counts; opens on Tasks | MTR-09-01 | — |
| 7.2 | Applicants tab | Pending pill by default; Pending / Accepted / Declined / All; 10 a page (10, 25, 50); search by name or email, case-insensitive, back to page 1 | MTR-10-01, MTR-10-02 · e2e 4.6 ✅ | — |
| 7.3 | Current Mentees tab | Accepted and graduated rows; progress is the % of completed non-prerequisite tasks, or "-". "+" on an accepted row opens Create Task; Create Group Task preselects every accepted mentee and is disabled when there are none | MTR-11-01, MTR-11-02, MTR-11-03 | — |
| 7.4 | Create a task | Name up to 120, description up to 1000, optional due date, file-required flag: "Task created"; the mentee sees it | MTR-12-01 · e2e 4.9 ✅ | — |
| 7.5 | Group task | N mentees selected: "N mentees were given the task." | MTR-12-02 | — |
| 7.6 | Edit a task | Non-prerequisite task; only changed fields sent; "Task updated" | MTR-13-01 | — |
| 7.7 | Inline status dropdown | PATCH sent at once, no toast; reverts on failure | MTR-13-02 | — |
| 7.8 | Dropdown to Completed, or reopening an approved or prerequisite task | Skips the review step (F-15). Record what the backend allows | MTR-13-06 | — |
| 7.9 | Tasks tab | "Awaiting Review" by default; Approved and All pills; newest first; relative times | MTR-14-01 | — |
| 7.10 | Approve a task | "Task approved"; the mentee sees Completed | MTR-14-02 · e2e 4.10 ✅, 5.9 ⚠️ (e2e F6) | — |
| 7.11 | Request Changes | "Changes requested"; the mentee sees To Do; there is no comment field | MTR-14-03 · e2e 4.10 ⚠️ (e2e F5) | — |
| 7.12 | Download a submission | Saved with the server's filename (or "submission") | MTR-15-01 · e2e 4.8 ✅ | — |
| 7.13 | Reviewer note | Counter out of 2000; trimmed; "Note saved", both tabs update. Clearing gives "Note cleared" | MTR-16-01, MTR-16-02 · e2e 4.7 ✅ | — |

## 8. Mentee: registration and applying

| # | Case | Expected | Ref | DEV |
|---|---|---|---|---|
| 8.1 | Register directly (needs a fresh account; otherwise rely on e2e 5.1) | All required fields: "Profile created / Your mentee profile has been saved."; lands on `/mentee/overview` | MTE-01-01 · e2e 5.1 ✅ | — |
| 8.2 | Demographics consent | Ticking a row enables its dropdown; the answer is sent | MTE-02-01 | — |
| 8.3 | Apply link with no profile (samehhotmail) | `/mentee/apply?programId=A&programTermId=B` redirects to `/mentee?programId=A&programTermId=B`; after registering, back on the apply page with the checklist hidden and Submit enabled | MTE-04-01 · e2e F14 (lfx-self-serve#4232) | — |
| 8.4 | Has a profile, opens `/mentorship/mentee` | Redirects to `/mentee/overview`, or to `/apply` when both ids are present | MTE-04-03 · e2e 5.1 ✅ | — |
| 8.5 | Apply page | "Apply to {program}", "{project} - {term}", profile card, profile details, demographics summary, Before You Apply checklist | MTE-05-01 · e2e 5.2 ✅ | — |
| 8.6 | Submit an application | Tick all 5 confirmations ("N remaining" reaches 0): "Application submitted"; new card on the overview; the admin sees Applied | MTE-06-01 · e2e 5.3 ⚠️ (e2e F2) | — |
| 8.7 | A 4th application with 3 pending (samehhotmail) | Should be refused. Record the backend response and the message shown (F-18) | MTE-06-09 · e2e 5.14 ❌ (e2e F15) | — |
| 8.8 | Edit demographics on the apply page | Change one answer: only that group is sent; "Demographics updated"; summary updates | MTE-07-01 | — |

## 9. Mentee: overview, tasks, profile

| # | Case | Expected | Ref | DEV |
|---|---|---|---|---|
| 9.1 | Overview cards | Ordered Active, Graduated, Awaiting Review, In Progress; progress "x of y"; "Decision expected {date}" on non-accepted cards | MTE-08-01 | — |
| 9.2 | Past Applications | Declined shows "Not selected", withdrawn "Withdrawn", hold "On Hold"; newest first; UTC dates | MTE-08-03 | — |
| 9.3 | Withdraw | From the card or a history row: "Application withdrawn"; lists refresh; the admin sees Withdrawn | MTE-09-01 · e2e 5.5 ✅ | — |
| 9.4 | My Tasks | Accepted and graduated applications first, with "x% - a of b submitted" and All / To Do / In Progress / Submitted chips; then pending applications' Prerequisite Tasks; A–Z | MTE-10-01 | — |
| 9.5 | "N open" badge | Shows the open-task count; hidden if the read fails | MTE-10-02 | — |
| 9.6 | One application's task read fails | Fault case. The whole overview and My Tasks fail (F-22) | MTE-10-06 | — |
| 9.7 | Change task status | To Do → In Progress → Submitted, "Task updated" each time, spinner while saving. Submitted → In Progress before the due date is allowed | MTE-11-01, MTE-11-02 · e2e 5.8 ✅ | — |
| 9.8 | Date column | "Due MMM d, yyyy" (UTC), "Submitted on …" or "No Due Date" | MTE-12-01 | — |
| 9.9 | Due-date cutoff | Task due today: locks exactly at the end of the UTC day. Check with the browser in a UTC+ and a UTC− zone | MTE-12-03 | — |
| 9.10 | Upload a file | PDF, DOC, DOCX or TXT up to 20 MB: "File uploaded / …task marked as submitted"; row Submitted | MTE-13-01 · e2e 5.4 ✅ | — |
| 9.11 | Download, replace, remove | Download uses the server filename; Replace works until complete or past due; Remove only before submitting, no confirm, "File removed" | MTE-13-02 · e2e 5.4 ✅ | — |
| 9.12 | Profile tab | Profile card, About Me (sanitised), Skills, Areas to Improve, Additional Notes, country name | MTE-14-01 | — |
| 9.13 | Application History | Ordered graduated, accepted, pending, then others; labels In Review, Accepted, Not Selected, Withdrawn, Graduated, On Hold; View opens the program page in a new tab | MTE-14-02 | — |
| 9.14 | Edit profile | Skills, notes, country, introduction: only changed groups sent; "Profile updated" | MTE-15-01 · e2e 5.10 ✅ | — |
| 9.15 | LFX profile card | Name, emails, linked accounts. Edit LFX Profile saves and syncs; a sync failure shows a warning toast | MTE-16-01 | — |
| 9.16 | Connect GitHub or LinkedIn | OAuth round trip; success or error toast shown once; query params removed | MTE-16-03 | — |
| 9.17 | Tabs and keyboard | Overview / My Tasks / Mentee Profile; arrow, Home and End keys move between tabs | MTE-17-01 | — |

## 10. Impersonation

**Blocked:** none of the test accounts has the `can_impersonate` claim. Once one does, it impersonates the user in brackets. Every write must be refused with 403 `IMPERSONATION_READ_ONLY` and change nothing; reads must work.

| # | Case | Expected | Ref | DEV |
|---|---|---|---|---|
| 10.1 | Admin writes (sameh100): add logo, hide, save note, invite, submit enrollment | Each refused with the server's read-only message; nothing created or changed. Mentor candidate search still works | ADM-03-09, ADM-05-06, ADM-11-07, ADM-16-10, ADM-21-08 | — |
| 10.2 | Admin CSV download (sameh100) | Allowed | ADM-13-07 | — |
| 10.3 | Mentor (samehlol16; for the invite link and register form, a user with an open invite or no mentor profile) | Invite link shows "You're viewing as another user" read-only state; register form shows a sticky read-only banner and saves nothing; profile edit shows the read-only message; submission download allowed | MTR-01-07, MTR-02-10, MTR-06-06, MTR-15-05 | — |
| 10.4 | Mentee (sameh001; for the register form, a user with no mentee profile) | Register shows "You are viewing as another user, so registration is read-only."; Submit Application shows an error toast and the form stays; Connect GitHub/LinkedIn disabled with no sync call | MTE-03-03, MTE-06-05, MTE-16-04 | — |
| 10.5 | Review page (samehlol16) | GET works; Confirm gets 403. The UI shows the approver-team message instead of a read-only one (F-29) | PR-08-01 | — |
| 10.6 | API: every write endpoint (W in the Endpoints sheet) | 403 `IMPERSONATION_READ_ONLY`; a warning log names the impersonator and the target | BFF-09-01 | — |
| 10.7 | API: all GETs, `POST /mentor-candidates`, CSV export, file download | Work, using the target's token | BFF-09-02 | — |
| 10.8 | Impersonation session expires | Session cleared; the support user's own token is used | BFF-09-03 | — |

## 11. Access control and IDOR

| # | Case | Expected | Ref | DEV | PROD |
|---|---|---|---|---|---|
| 11.1 | API: every endpoint as a user without the role (sameh001 and samehhotmail on admin and mentor routes; samehlol16 on Program A's admin routes) | Backend 403 or 404 passed through unchanged; no data leaked; no change | BFF-02-01 · e2e 7.3 ✅ (one endpoint) | — | — |
| 11.2 | sameh001, and samehlol16 (mentor but not admin of Program A), open Program A's admin page or `GET /admin/programs/<A>` | 403 or 404. If samehlol16 gets 200, check whether its approver role grants the read, and record it | BFF-02-02 · e2e 7.2 ✅ | — | — |
| 11.3 | Mentor opens a program they don't mentor (page and API) | 404 | BFF-02-03, MTR-09-04 · e2e 3.7 ✅ | — | — |
| 11.4 | API: sameh001 withdraws samehhotmail's application | 403 or 404; no change | BFF-03-01, MTE-09-06 | — | — |
| 11.5 | API: samehhotmail uses sameh001's task id to change status, upload, delete or download a file | 403 or 404; no change | BFF-03-02, MTE-13-10 · e2e 7.4 ✅ | — | — |
| 11.6 | API: samehlol16 uses sameh100's mentor request id or invitation id | 404; no change | BFF-03-03, MTR-07-07 | — | — |
| 11.7 | API: sameh100 calls `/admin/programs/<A>/terms/<term of B>` or `/mentors/<member of B>`, where B is a samehlol16 program sameh100 doesn't administer | Refused by the backend; the BFF only checks the id is a UUID (F-30) | BFF-03-04 | — | — |
| 11.8 | API: `PATCH /me/lfx-profile` with another user's email | Email ignored; only the caller's data changes | BFF-03-05 | — | — |
| 11.9 | API: register (mentor and mentee) with a made-up email or GitHub value | Ignored; resolved from the signed-in identity | MTR-02-11, MTE-03-06 | — | — |
| 11.10 | API: create a task for a non-accepted, graduated or mentor-role application | 400 | MTR-12-08 | — | — |

## 12. API and hardening

| # | Case | Expected | Ref | DEV | PROD |
|---|---|---|---|---|---|
| 12.1 | No session on any `/api/mentorship/*` | 401 JSON; no backend call | BFF-01-01, ADM-01-06 · e2e 7.1 ✅ | ✅ admin/programs, mentor/programs, mentee/has-profile (fetch with no cookies) | — |
| 12.2 | Expired access token, valid refresh token | Silent refresh; 200 | BFF-01-02 · see e2e F12 | — | — |
| 12.3 | Expired token and the refresh fails | 401 for API calls; a page load logs the user out | BFF-01-03 | — | — |
| 12.4 | Tampered `appSession` cookie | 401 | BFF-01-04 | — | — |
| 12.5 | Flag off for the user, API called directly | Still answered: the flag only hides the UI (F-32). Confirm with product that this is acceptable | ADM-01-05, BFF-10-02 | — | — |
| 12.6 | First Mentorship call by samehhotmail | Backend 401 "not provisioned", then `PUT /me`, one retry, 200. The user never sees an error | BFF-08-01 | — | — |
| 12.7 | `POST /mentee/apply` without the 5 confirmations | Accepted: the confirmations are only checked in the browser (F-17) | MTE-06-08 | — | — |
| 12.8 | `POST /tasks` with more than 100 ids, a non-UUID, or due date `2026-02-31` | 400; owner or assignee sent by the client ignored | MTR-12-09 | — | — |
| 12.9 | Mentee task status `complete` or `pending`; another user's task | 400; 403 | MTE-11-09 | — | — |
| 12.10 | Past due: set Submitted, upload or delete after the cutoff | 400 `TASK_PAST_DUE`; mapped toast in the UI | MTE-12-06 | — | — |
| 12.11 | Task upload without `X-File-Name`; as `multipart/form-data`; over 20 MB | 400; 415; 413. Never 500 | MTE-13-09 | — | — |
| 12.12 | Mentee profile PATCH with an unknown key, `profile_links`, `null`, `{}` or a blank skill | 400 | MTE-15-06 | — | — |
| 12.13 | Non-UUID task id on download; non-UUID application id on withdraw; withdraw while impersonating | 400; 400; 403 read-only | MTR-15-04, MTE-09-07 | — | — |
| 12.14 | JSON body over 15 MB | 413 expected; probably 500 today (F-31) | BFF-06-04 | — | — |
| 12.15 | Any error response | No backend host or URL in the body | BFF-07-07 | — | — |
| 12.16 | File download headers (mentor, admin and mentee) | `Content-Disposition: attachment`; `X-Content-Type-Options: nosniff` | MTR-15-03, MTE-13-11 · e2e 7.7 ✅ | — | — |
| 12.17 | Stored HTML: `<script>` or `onerror` in the mentor and mentee introduction; a term named `<img src=x onerror=alert(1)>` in Decline by Term | Rendered sanitised or escaped; nothing runs | MTR-05-05, MTE-14-05, ADM-10-04 · e2e 7.6 ✅ | — | — |
| 12.18 | Mentor candidate search | Search text sent in the POST body, never in the URL or logs | ADM-16-09 | — | — |

## 13. Logs

| # | Case | Expected | Ref | DEV |
|---|---|---|---|---|
| 13.1 | Self Serve logs after running sections 2–12 | No bearer token, cookie, invite token, name or email search text, task or note text, or profile values | BFF-11-01 | — |
| 13.2 | Logs for 6.1–6.4 | Invite token redacted in the page URL and in the backend path | MTR-01-10 | — |

## 14. PROD

Run only cases that read, or that act on `E2E` data you created. Every PROD account must be in the `mentorship-enabled` flag, and sameh100 and samehlol16 must be in the PROD approver team.

| Cases | What |
|---|---|
| 1.1–1.4, 1.7 | Admin list and program page (sameh100) |
| 4.7, 5.1, 5.2, 5.5, 5.8 | Enroll one `E2E … (do not apply)` program, open the review link only, check a non-approver is refused, then approve it |
| 11.1–11.3 | Wrong-role reads only |
| 12.1–12.4, 12.15, 12.16 | Authentication and response checks |

---

## Known issues to watch

These are open from the E2E run or the workbook, and P1 cases above touch them. Check their state before marking a result.

| Issue | Summary | Cases |
|---|---|---|
| e2e F2 | Right after applying, the overview shows "Forbidden" for about 10–15 s while access syncs | 8.6 |
| e2e F5, F6 | Request Changes has no feedback text; an approved task still shows Submitted to the mentee | 7.10, 7.11 |
| e2e F12 | Mentorship pages forward to the dashboard on an idle session (expired token not refreshed during server render) | 12.2, every UI case |
| e2e F14 | A first-time applicant from an Apply link stays on the registration form | 8.3 |
| e2e F15 | A 4th active application is accepted. Fixed on `main` by [#370](https://github.com/linuxfoundation/lfx-mentorship/pull/370) (cap at three pending applications), so 8.7 should now pass on DEV | 8.7 |
| F-15 | Task status dropdown can skip mentor review | 7.8 |
| F-17 | Before You Apply confirmations are only checked in the browser | 12.7 |
| F-22 | One failed task read breaks the whole mentee overview | 9.6 |
| F-27 | First-time approver may get a generic error | 5.9 |
| F-28 | After a timeout, the review page says "not recorded" without knowing | 5.7 |
| F-29 | Impersonation on the review page shows the wrong message | 10.5 |
| F-30 | Child ids are only checked to be UUIDs in the BFF | 11.7 |
| F-31 | Oversize JSON returns 500 | 12.14 |
| F-32 | The flag does not gate the API | 12.5 |

### Open questions that affect expected results

| Workbook # | Question | Status |
|---|---|---|
| 10 | Can a user accept an invitation without a mentor profile? | Answered: yes (e2e 4.1, PROD 3.3) |
| 11 | Can mentors set Completed with a plain task PATCH? | Open, answered by 7.8 |
| 13 | When does an invite token expire, and is there a distinct error? | Open, check during 6.1–6.4 |
| 19, 20 | Apply refusal messages and limits | Partly answered: graduates and accepted mentees are refused (e2e 5.7, 5.12); the 3-application limit is not enforced (e2e F15) |
| 25 | Can a program's own admin approve it? | Answered: only approver-team membership is checked, so an admin who is also an approver can (e2e Info) |
| 28 | Is the decision POST idempotent after a timeout? | Open, answered by 5.7 |

## Findings

New findings from this run. Number them P-1, P-2, … so they don't clash with the E2E plan's F-numbers or the workbook's F-xx.

| # | Finding | Env | Severity | Ticket |
|---|---|---|---|---|
| | | | | |
