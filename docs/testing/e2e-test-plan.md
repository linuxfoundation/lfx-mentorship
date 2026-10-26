<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# LFX Mentorship: E2E Test Plan and Results

End-to-end test cases for the new Mentorship platform (the public site plus the Mentorship module in LFX Self Serve), with the results of the last run.

| | DEV | PROD |
|---|---|---|
| Public site | https://mentorship.dev.lfx.dev | https://mentorship.linuxfoundation.org |
| Self Serve | https://app.dev.lfx.dev/mentorship | https://app.lfx.dev/mentorship |

**Last run:** 2026-10-22, with re-tests of fixes on 2026-10-23 ([section 12](#12-re-tests-2026-10-23)). Driven through Playwright MCP.

- **DEV:** full program lifecycle.
- **PROD:** public site, plus a shorter lifecycle in Self Serve (see [PROD](#9-prod)).

---

## How to run

1. Start Playwright MCP with `--isolated`, so that sessions don't share a browser profile.
2. Give each test user their own browser context: `browser.newContext()` in `browser_run_code_unsafe`. A banner injected into each context names the user to sign in as. Sign in by hand; never put credentials in the chat or in this file.
3. Save each signed-in context with `context.storageState()` to a scratch directory, and keep only LFX and Auth0 cookies. Do not commit these files.
4. Upload fixtures must live under `.playwright-mcp/`, which is gitignored. The run used these generated files:

   | File | Purpose |
   |---|---|
   | 420×420 PNG | Valid program logo |
   | 1200×1200 PNG, 4.3 MB | Logo over the 2 MB limit |
   | SVG | Logo of a disallowed type |
   | Small PDF | Valid task file |
   | 21 MB PDF | Task file over the 20 MB limit |
   | TXT | Task file (plain text is allowed) |

5. Prefix all test data with `E2E` so it is easy to find and clean up.

### Test users

| User | DEV role in the run | Notes |
|---|---|---|
| `michal` | Program Admin, approver | Admin of Fruit Program and of the E2E program. Approver team member. |
| `mlehotskyh` | Mentor | Already a graduate, so it cannot apply as a mentee. |
| `mlehotskynet` | Graduate (eligibility check) | Already a graduate. |
| `mlehotskyh2` | Mentee 1 (full lifecycle) | Had no mentee profile before the run. |
| `mlehotskyh3` | Mentor request, non-approver, mentee 2 (declined) | Had no profiles before the run. |

On PROD, `michal` was Program Admin and approver, `mlehotskyh` mentor, and `mlehotsky1` and `mlehotsky2` mentees (accepted then graduated, and declined). Every PROD test account must be in the `mentorship-enabled` LaunchDarkly flag, or Self Serve redirects it to the dashboard.

The test accounts use external inboxes, so the run did not check emails.

### Test data created on DEV

| Item | ID | State after the run |
|---|---|---|
| E2E Test Program 2026-10-21 | `a444c9eb-3183-4fc2-b563-8492e23bfac4` | Published. `mlehotskyh` mentor, `mlehotskyh2` graduated, `mlehotskyh3` declined. Reusable for the next run. |
| E2E Pending Program 2026-10-22 | `89355b93-6692-4d82-b217-e2b064df1b53` | Rejected |
| E2E Term Oct 2026 on Fruit Program | `902ea779-2974-432d-8057-d40d37848737` | Closed |

### Test data created on PROD

| Item | ID | State after the run |
|---|---|---|
| E2E Test Program (do not apply) 2026-10-22 | `456c2394-3041-49a9-b2eb-270d407932bc` | Published, term closed, so it is listed as Completed. It can't be hidden (a program with graduates stays visible, by design). PROD data is truncated before the final DB import. |

---

## Status legend

| Symbol | Meaning |
|---|---|
| ✅ | Passed |
| ❌ | Failed (see [Findings](#findings)) |
| ⚠️ | Passed with a caveat, or a known limitation |
| — | Not tested |

---

## 1. Public site (logged out)

| # | Case | Expected | DEV | PROD |
|---|---|---|---|---|
| 1.1 | Landing page | Stats from `/api/summary`; Become a Mentor and My Mentorship link to Self Serve | ✅ | ✅ |
| 1.2 | Programs list | Published programs only; All / Accepting / In progress / Completed tabs; search and empty state; skill filter; sort; Load More pages by 15 with no duplicates | ✅ | ✅ (counts and first page only) |
| 1.3 | Program detail | Opens by slug and by UUID; Overview, Terms, Mentors, Mentees and Sponsors tabs; no emails shown | ❌ Sponsors tab ([F1](#findings)) | ✅ |
| 1.4 | Apply while logged out | "Sign in to apply" dialog, then Self Serve `/mentorship/mentee/apply` with `programId` and `programTermId` | ✅ | — |
| 1.5 | Donate | Opens the program's Crowdfunding initiative | ❌ 404 for programs without an initiative ([F1](#findings)) | — |
| 1.6 | Mentor and mentee directories | Search, Load More, mentee status tabs, profile pages, no contact details; an unknown ID gives 404 | ✅ | ✅ |
| 1.7 | Enroll page | "Enroll a Program in LFX" opens Self Serve `/mentorship/admin/enroll` | ✅ | — |
| 1.8 | Unknown program | 404 page | ✅ | ✅ |
| 1.9 | Mobile, 390 px wide | No horizontal scroll on landing, programs, detail, mentors, mentees and enroll | ✅ | — |
| 1.10 | Hidden program | 404 by slug and by UUID; visible again after unhide | ✅ | — |
| 1.11 | Pending program | Not public | ⚠️ 404 by slug, but readable by UUID ([F3](#findings), known) | — |

## 2. Enrollment and approval

| # | Case | Expected | DEV |
|---|---|---|---|
| 2.1 | Step 1 validation | Required fields flagged; SVG logo rejected ("not the right file type"); logo over 2 MB rejected; PNG accepted and previewed | ✅ |
| 2.2 | Term dialog validation | Required fields; application end date before start date rejected | ✅ |
| 2.3 | Step 3 validation | Submit is blocked without the terms accepted and without at least one prerequisite | ⚠️ A missing prerequisite shows only the generic banner ([F7](#findings)) |
| 2.4 | Submit | Program created as `pending`; owner's list shows "Pending Review" | ✅ |
| 2.5 | Logo upload after submit | Logo stored in the same submit | ❌ 403 in 1 of 2 enrollments; Retry later succeeds ([F2](#findings)) |
| 2.6 | Approve from the review page | Approver confirms; program `published` and public | ✅ |
| 2.7 | Approve as a non-approver | 403 and "You don't have permission to review programs" | ✅ |
| 2.8 | Reject | Approver confirms; program `rejected`; owner sees "Rejected" | ✅ |
| 2.9 | Review page for a reviewed program | "This program has already been reviewed" | ✅ |

## 3. Program management (Program Admin)

| # | Case | Expected | DEV |
|---|---|---|---|
| 3.1 | Edit program | Stepper opens with the program's data; Update saves | ✅ |
| 3.2 | Terms | Create, edit and close a term; closing declines its pending applications | ✅ |
| 3.3 | Invite a mentor | Typeahead finds by name or LFID; row shows Invited | ✅ |
| 3.4 | Mentor request | Requested row offers Approve and Decline; Decline sets Declined | ✅ |
| 3.5 | Hide and unhide | Hidden program returns 404 publicly; unhide restores it | ✅ |
| 3.6 | Hide with active applications | Refused with 409 and a clear message | ✅ |
| 3.7 | Remove a mentor | Mentor shows as Withdrawn and loses access (404); re-inviting resets them to Invited, and access returns after they accept | ✅ |
| 3.8 | Delete a program | — | — (no Delete in the UI) |

## 4. Mentor

| # | Case | Expected | DEV |
|---|---|---|---|
| 4.1 | Accept an invitation in Self Serve | Edit Mentor Profile drawer lists the invite; Accept sets Approved; program appears in My Programs | ✅ |
| 4.2 | Accept an invitation from the email link | `/mentorship/mentor/invites?token=` | — (test inboxes not reachable) |
| 4.3 | Become a Mentor with a program request | Profile saved and the picked program requested | ⚠️ Request not sent after Withdraw and re-pick ([F4](#findings), unconfirmed) |
| 4.4 | Request to join from the profile drawer | Request sent; Requested on the admin's Mentors tab | ✅ |
| 4.5 | My Programs | Cards show mentees, tasks to review and applicants | ✅ |
| 4.6 | Applicants tab | Status pills; reviewer note and View Tasks only, no accept or decline | ✅ |
| 4.7 | Reviewer note | Saved and visible to the Program Admin | ✅ |
| 4.8 | Applicant files | Download returns the mentee's file | ✅ |
| 4.9 | Create a task for a mentee | Task created | ✅ |
| 4.10 | Review tasks | Approve completes the task; Request Changes returns it to the mentee | ⚠️ No feedback text ([F5](#findings)) |

## 5. Mentee

| # | Case | Expected | DEV |
|---|---|---|---|
| 5.1 | Register | Validation; Submit lands on the overview; `/mentorship/mentee` then redirects to the overview | ✅ |
| 5.2 | Apply page | Shows the LFX profile, mentee profile, demographics, Before You Apply and terms; Submit stays disabled until all are confirmed | ✅ |
| 5.3 | Submit an application | Overview shows the application under review | ⚠️ "Forbidden" for about 10 s right after submitting ([F2](#findings)) |
| 5.4 | Prerequisite uploads | PNG rejected (type); 21 MB rejected; PDF and TXT accepted; upload marks the task submitted; Replace works | ✅ |
| 5.5 | Withdraw a pending application | Moves to Past Applications as Withdrawn | ✅ |
| 5.6 | Reapply after withdrawing | New application with fresh prerequisite tasks | ✅ |
| 5.7 | Apply as a graduate | Refused: "You have already graduated from a mentorship" | ✅ (shown only after Submit) |
| 5.8 | Work on tasks once accepted | Program shows Active; upload; To Do → In Progress → Submitted | ✅ |
| 5.9 | Approved task | Shows as completed | ⚠️ Shows "Submitted" ([F6](#findings)) |
| 5.10 | Edit mentee profile | Saved and shown | ✅ |
| 5.11 | Graduation on the public site | Graduate listed on the program's Mentees tab and in the directory | ✅ |

## 6. Admin decisions

| # | Case | Expected | DEV |
|---|---|---|---|
| 6.1 | Accept | Attendance type required; status Accepted | ✅ |
| 6.2 | Decline by Term | Declines every pending application in the term; accepted ones untouched | ✅ |
| 6.3 | Create a task | Name, description, due date, file required | ✅ |
| 6.4 | Download By Status | CSV with name, email, status, term and tasks submitted | ✅ |
| 6.5 | Graduate | Status Graduated | ✅ |
| 6.6 | Withdraw for a mentee | Application withdrawn by the admin | ✅ |
| 6.7 | Decline a single application | Status Declined; the confirmation says the mentee is told | — (PROD ✅) |

## 7. Access control

| # | Case | Expected | DEV | PROD |
|---|---|---|---|---|
| 7.1 | Logged out, Self Serve Mentorship routes | Redirect to Auth0 or SSO login | ✅ | ✅ |
| 7.2 | Mentee opens a program's admin page | "No access to this program" (403) | ✅ | — |
| 7.3 | Mentor calls the admin-only status change | 403 | ✅ | — |
| 7.4 | Mentee downloads another mentee's task file | 403 | ✅ | — |
| 7.5 | Non-approver records a program decision | 403 | ✅ | — |

## 8. Emails (PROD)

PROD sends only to `EMAIL_ALLOWED_RECIPIENTS`. A plus-address matches its base address, so `mlehotsky+1@` and `mlehotsky+2@` are allowed through `mlehotsky@`.

| # | Email | Expected | PROD |
|---|---|---|---|
| 8.1 | Program submitted: to the admin, and to the review inbox with View, Approve and Reject links | Sent; links point to PROD domains | ✅ |
| 8.2 | Program approved; mentor accepted the invitation | Sent to the admin | ✅ |
| 8.3 | New application with tasks complete; mentee accepted (admin and HR) | Sent, naming the mentee | ⚠️ Sent, but the mentee is "A mentee" ([F10](#findings)) |
| 8.4 | Mentee emails: profile created, application received, accepted, declined, task, graduated | Sent to the mentee and copied to `EMAIL_COPY_INBOX` | ❌ None sent ([F10](#findings)) |
| 8.5 | Mentor invite email link (`/mentorship/mentor/invites?token=`) | — | — (the mentor's inbox is not reachable) |

## 9. PROD

**Public site:** cases 1.1–1.3, 1.6, 1.8 and 7.1 pass. Totals at run time: 1,303 programs, 2 accepting, 1,002 mentors.

**Self Serve:** Mentorship is gated by the `mentorship-enabled` LaunchDarkly flag ([`mentorship-enabled.guard.ts`](https://github.com/linuxfoundation/lfx-self-serve/blob/main/apps/lfx-one/src/app/shared/guards/mentorship-enabled.guard.ts)). Accounts outside the flag are redirected to the dashboard.

| # | Case | PROD |
|---|---|---|
| 2.4 | Enroll; pending program not listed publicly, slug gives 404 | ✅ |
| 2.5 | Logo upload after submit | ❌ 403 on all four tries; Retry later succeeds ([F2](#findings)) |
| 2.6 | Approve | ✅ `michal` is in the PROD approver team |
| 3.3, 4.1 | Invite a mentor; mentor accepts from the profile drawer without a mentor profile | ✅ |
| 3.5 | Hide while published with no applications; public 404; unhide | ✅ |
| 3.6 | Hide with a graduated mentee | ✅ Refused, by design |
| 3.2 | Close the term | ✅ |
| 5.1, 5.2 | Register two new mentees and apply | ✅ |
| 5.3 | Overview right after applying | ⚠️ "Forbidden" for both mentees ([F2](#findings)) |
| 5.4 | Prerequisite upload | ✅ |
| 6.1 | Accept (part time) | ✅ |
| 6.7 | Decline a single application | ✅ |
| 6.3, 5.8, 4.10, 6.5 | Create task, submit, mentor approves, graduate | ✅ |
| 5.11 | Graduate on the public site | ❌ Listed as "Mentee" with no name ([F10](#findings)) |
| 4.6, 6.1 | Mentee names in the mentor's and admin's lists | ❌ Blank ([F10](#findings)) |
| 7.2 | Mentee opens the program's admin page | ✅ "No access to this program" |

## 10. Migrated data (PROD vs legacy)

Compared 25 programs sampled across the PROD list with the legacy API (`api.mentorship.lfx.linuxfoundation.org`). Program IDs are kept by the import.

| Check | Result |
|---|---|
| Name, term count, logo present, logo URL loads | ✅ 25 of 25 |
| Slug | ✅ 25 of 25. Elsewhere, `meshery` became `meshery-1`; old links use the ID, so they still resolve |
| Mentor names | ✅ 23 of 25. The 2 differences are a duplicate legacy row and a mentor that legacy `/mentors` omits but `/member` counts |
| Skills | ⚠️ Mostly equal (legacy skills contain commas). One program lost "Profiling Tools"; unconfirmed |

## 11. Legacy URL redirects (PROD)

| Legacy path | Result |
|---|---|
| `/project/{id}`, `/project/{id}/detail` | ✅ 301 to `/programs/{id}` |
| `/mentee/{id}`, `/mentee/{id},{programId}`, `/mentor/{id}` | ✅ 301 to the new profile |
| `/participate/*`, `/profile`, `/email/process-approval?…` | ✅ 301 to `/` |
| `/mentee`, `/mentor` with no ID | ⚠️ 404 ([F13](#findings)) |

---

## 12. Re-tests (2026-10-23)

PROD runs `v0.1.10` (backend, cron jobs and frontend), which includes [#358](https://github.com/linuxfoundation/lfx-mentorship/pull/358) only. DEV runs `main`.

| Fix | DEV result |
|---|---|
| [#359](https://github.com/linuxfoundation/lfx-mentorship/pull/359) (F1) | ✅ No initiative means `200` with no sponsors, "No sponsors listed yet", and Donate disabled with a tooltip |
| [#358](https://github.com/linuxfoundation/lfx-mentorship/pull/358) (F2, logo) | ✅ Logo uploaded on the first try (1 run) |
| [#358](https://github.com/linuxfoundation/lfx-mentorship/pull/358) (F2, apply) | ⚠️ Submit now takes about 10 s, while the access wait times out. On 1 of 2 runs the overview still showed "Forbidden"; access appeared about 15 s after Submit |
| [#361](https://github.com/linuxfoundation/lfx-mentorship/pull/361)–[#365](https://github.com/linuxfoundation/lfx-mentorship/pull/365) (F10) | ✅ A brand-new user (`mlehotsky1`) has a name in the admin and mentor lists. The backend sends their emails, and the copies in `EMAIL_COPY_INBOX` are correct |
| F12 | ❌ Still reproduces: 5 of 5 times on idle DEV sessions. PROD RUM, last 30 days: 17 guard timeouts in 17 sessions, 9 of them `mentorshipEnabledGuard` |

| # | Re-test or new check (2026-10-23, DEV) | Result |
|---|---|---|
| 7.6 | Stored script or HTML in the program description and mentee introduction, sent past the editor | ✅ Never runs on the public site or in Self Serve. The backend stores and returns raw HTML, so both frontends must keep sanitizing it |
| 7.7 | HTML file renamed `.pdf` and uploaded as a task file | ✅ Accepted as plain text, but served as a `text/plain` download renamed `.txt`, with `nosniff` |
| 5.12 | An accepted mentee applies to another program | ✅ Refused: "You are already in a mentorship" |
| 5.13 | Double-click on Submit Application | ✅ One application created |
| 5.14 | More than 3 active applications | ❌ The 4th is accepted; legacy refused it ([F15](#findings)) |
| 3.9 | Close a term that has a pending application | ✅ The pending application becomes Declined |

Emails:

- PROD before the fix: all 8 mentee emails failed with `user … has no email address` (Datadog), confirming F10.
- DEV after the fix: SES delivered every mentee and admin email (13 of 13, no bounces). The direct copies to a plus address (`mlehotsky+1@`) never reached Gmail, while mail to the base address did. This looks like Google Workspace filtering of the DEV sender domain, not Mentorship; re-check on PROD, which uses a different sender.
- PROD SES: healthy, DKIM passing, 50,000 sends a day, 0.9% rolling bounce rate. The account is shared by every LFX v2 service, so a bounce spike from stale legacy addresses at launch would affect them all.

### Re-tests (2026-10-26, DEV)

PROD is still on `v0.1.10`; these fixes are on `main` only.

| Fix | DEV result |
|---|---|
| [#370](https://github.com/linuxfoundation/lfx-mentorship/pull/370) (F15) | ✅ A 4th pending application is refused (422), and the apply page says "You have reached the application limit" |
| [#371](https://github.com/linuxfoundation/lfx-mentorship/pull/371) (F16) | ✅ Legacy approval, program-mentees and mentor links redirect to Self Serve; signed in as an approver and admin, each opens the right page, and the approve or decline choice carries over |
| [lfx-self-serve#4261](https://github.com/linuxfoundation/lfx-self-serve/pull/4261) (F14) | ✅ A brand-new account (`mlehotsky2`) registered from an Apply link, landed on the application and submitted it |

| # | Gap check against legacy (2026-10-26, DEV) | Result |
|---|---|---|
| 5.15 | Reapply to a term after being declined there | ✅ Refused (409): "You already applied to this term… so you cannot apply again" |
| 3.10 | Re-open a closed term whose end date has not passed | ✅ Re-Open confirms, the term is open (also on the public site), and Close restores it |
| 3.11 | Re-open an ended term | ✅ Ended terms show no actions; the backend also refuses (`program_term_service.go:193`) |
| 6.8 | Re-open a declined application | — The backend allows `declined` → `pending`, but no page offers it |

Comparing the legacy jobspring features with this plan left these untested: accepting or declining a mentor invite from the email link (4.2), and the stipend feed for newly accepted mentees (Postgres → Fivetran → Snowflake → Crowdfunding `mentorship-sync`), which starts after release. 22 of the 23 email types were sent during testing; the missing one is `admin_mentor_declined` (a mentor declines an invite). By design, mentors no longer accept or decline mentee applications ([04](../rewrite/04-authorization-model.md)), which legacy allowed.

F12 reproduced twice more in this session's normal testing, as a mentee and as an admin.

## 13. Legacy parity checks (2026-10-23)

| # | Check | Result |
|---|---|---|
| 13.1 | Ended terms close on their own, as the legacy `update-program-terms-active` job did | ❌ Nothing closes them. On PROD, 123 ended terms are still open, and their programs show "In progress" ([F17](#findings)) |
| 13.2 | The admin "tasks submitted" email is sent once per application (legacy `adminNotified`) | ✅ DEV: sent once, when the 4th prerequisite was uploaded; replacing 2 files afterwards sent nothing |
| 13.3 | The backend enforces the application window while the term is still `open` | ✅ DEV: `POST /api/mentorship/mentee/apply` to two open terms past their deadline returns 422 "application window has closed" |
| 13.4 | Funding matches legacy (PROD) | ✅ Neither UI shows a per-program amount raised, and legacy `amountRaised` is 0 for 59 of 60 sampled programs. The Sponsors tab reads Crowdfunding and lists more sponsors than legacy did. Site totals: stipends paid $4.18M against legacy $4.02M (not investigated); 1,510 graduates (distinct mentees) against 1,621 legacy graduated applications |
| 13.5 | HR gets the mentee-accepted notice (PROD) | ✅ Sent to `EMAIL_COPY_INBOX` (`lfx-mentorship-sent@`) on 2026-10-22, as decided in [07-email-delivery.md](../rewrite/07-email-delivery.md) |
| 13.6 | Public "Contact support" sends users to the Self Serve chat (release plan) | ⚠️ Opens the Intercom messenger on DEV and PROD; [#366](https://github.com/linuxfoundation/lfx-mentorship/pull/366) (merged 2026-10-26) moves it to Self Serve |

## Findings

| # | Finding | Env | Severity | Ticket |
|---|---|---|---|---|
| F1 | Sponsors tab shows "Unable to load sponsors" (503) and Donate opens a Crowdfunding 404 when a program has no Crowdfunding initiative. Every program created in the new system is affected. | DEV, new PROD programs | Medium | [linuxfoundation/lfx-mentorship#355](https://github.com/linuxfoundation/lfx-mentorship/issues/355): fixed on DEV, PROD pending release |
| F2 | Right after a create, the new object is denied until its FGA tuples sync: the logo upload gets 403 (intermittent), and the mentee overview shows "Forbidden" for about 10 s after applying. | DEV, PROD | Medium | [linuxfoundation/lfx-mentorship#354](https://github.com/linuxfoundation/lfx-mentorship/issues/354): logo fixed; apply still affected |
| F3 | A pending program is readable by UUID on the public site. This is intentional for now: the review email links there. | DEV, PROD | Known | [linuxfoundation/lfx-mentorship#343](https://github.com/linuxfoundation/lfx-mentorship/issues/343) |
| F4 | Become a Mentor form: after withdrawing a picked program and picking it again, no request was sent on Submit. The picker does not reset after a pick. Seen once; re-test with a new account. | DEV | Low | — |
| F5 | Request Changes on a task carries no feedback, and the mentee sees the task back at To Do with no reason. | DEV | Low | — |
| F6 | A task the mentor approved still shows "Submitted" to the mentee. | DEV | Low | — |
| F7 | Enrollment step 3 with no prerequisite selected shows only the generic "form incomplete" banner; nothing points to the prerequisites. | DEV | Low | — |
| F8 | The `/programs` header hard-codes "42 foundations" ([`programs-header.config.ts`](../../frontend/app/components/modules/programs/config/programs-header.config.ts)). | DEV, PROD | Low | — |
| F9 | The landing page logs a Vue hydration mismatch. | DEV, PROD | Low | — |
| F10 | Mentees whose accounts are new to Mentorship have no name or email in Mentorship: blank in the admin and mentor lists, "Mentee" on the public site, "A mentee" in admin emails, and no mentee emails at all. Their LFX profiles have both. Not reproduced on DEV. | PROD | High | [linuxfoundation/lfx-mentorship#357](https://github.com/linuxfoundation/lfx-mentorship/issues/357), root cause [#360](https://github.com/linuxfoundation/lfx-mentorship/issues/360): fixed on DEV, PROD pending release and `user-backfill` |
| F12 | Mentorship pages forward to the dashboard after about 10 s on an idle session. The server-side render treats an expired access token as signed out without refreshing it, so LaunchDarkly never gets the user and the flag guard times out. Other flag-gated pages (for example EasyCLA in the Org lens) share the cause. | DEV, PROD | High (launch) | [linuxfoundation/lfx-self-serve#4179](https://github.com/linuxfoundation/lfx-self-serve/issues/4179) |
| F13 | The legacy paths `/mentee` and `/mentor` with no ID return 404 instead of redirecting. | PROD | Low | — |
| F15 | A mentee can have more than 3 active applications. Legacy refused a fourth, and the public site still promises the limit. | DEV | Medium | [linuxfoundation/lfx-mentorship#368](https://github.com/linuxfoundation/lfx-mentorship/issues/368): fixed on DEV, PROD pending release |
| F16 | Legacy email links for approvers, program admins and invited mentors land on the public site after cutover, instead of the Self Serve page where they can act. Filed by Michal; re-test before the 2026-10-27 cutover. | PROD | High (cutover) | [linuxfoundation/lfx-mentorship#367](https://github.com/linuxfoundation/lfx-mentorship/issues/367): fixed on DEV, PROD pending release |
| F14 | A first-time applicant who registers from an Apply link stays on the registration form after the profile is saved, with no message and no move on to the application. | DEV | High (launch) | [linuxfoundation/lfx-self-serve#4232](https://github.com/linuxfoundation/lfx-self-serve/issues/4232): fixed on DEV |
| F17 | Ended terms never close. Legacy closed them automatically once no applications were active. 123 ended PROD terms are still open, so their programs show "In progress", and they count toward the cap of 4 open terms per program. | PROD | Medium | [linuxfoundation/lfx-mentorship#369](https://github.com/linuxfoundation/lfx-mentorship/issues/369) |

**Info, no action proposed:**
- The landing page counts 5 graduated mentees on DEV, while the directory's Graduated tab lists 4.
- The public "Sign in to apply" dialog appears even when the user is already signed in to Self Serve.
- A graduate finds out they cannot apply only after filling in the apply form.
- Program filters and sort are not kept in the URL.
- An approver can approve a program they administer. This is by design: approval checks approver-team membership only.
- Declining a mentor request shows no toast.
- A program with graduates can't be hidden. This is by design, though the comment on `CountBlockingAppsForProgram` says "non-terminal".
- One PROD mentee's display name is their email address. It is data they entered; the API returns no email field.

## Follow-ups

- After [#343](https://github.com/linuxfoundation/lfx-mentorship/issues/343) ships, re-test 1.11: a pending program should return 404 publicly and show on the Self Serve review page.
- Re-test F4 with a new account, and run 3.7, 4.2 and 6.6.
