<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship Rewrite — 07: Email Delivery

Status: Approved — reviewed 2026-09-22; amended 2026-09-29 (see Decision log)
Related: [02-target-architecture.md](./02-target-architecture.md) (Integrations table, records the same rail), [01-current-system.md](./01-current-system.md) (legacy Mandrill setup), 08 mentor invitations (draft, [lfx-mentorship#177](https://github.com/linuxfoundation/lfx-mentorship/pull/177) — changes the invitation rows below)
Decision ticket: [linuxfoundation/lfx-self-serve#2188](https://github.com/linuxfoundation/lfx-self-serve/issues/2188)

The rewrite needs transactional email — mentor invitations, application decisions, task notifications. Legacy ([jobspring](https://github.com/linuxfoundation/jobspring)) sends via **Mandrill**, synchronously inside HTTP handlers, with about 47 templates authored in Mailchimp's editor and hand-synced by `curl`. This doc records what replaces it, and which of those templates the rewrite owes for feature parity.

## Decisions

| # | Question | Decision |
| --- | --- | --- |
| 1 | Delivery rail | **[lfx-v2-email-service](https://github.com/linuxfoundation/lfx-v2-email-service)** — NATS request/reply on `lfx.email-service.send_email`, via its Go client `pkg/api`. Not SendGrid, not a direct SES/Mandrill client. |
| 2 | Template ownership | **This repo.** Go `html/template` renders an HTML and a plain-text body per notification; no externally hosted templates. |
| 3 | Legacy template scope | **The [parity checklist](#parity-checklist) is the target**: every legacy template with a live send in jobspring whose trigger exists in the rewrite — 24 of 46. Excluded: employer-portal templates, the `SpecialRecipients` CC hack, and templates legacy no longer sends. |
| 4 | Keep Mandrill? | **No.** Mandrill is legacy-only. Mentorship holds no provider credentials of any kind. |

## Why this rail

- **No service sends its own email.** email-service owns the SMTP credentials, the sender and recipient allowlists, and bounce tracking. It runs in dev, staging, and prod, and [invite](https://github.com/linuxfoundation/lfx-v2-invite-service), [project](https://github.com/linuxfoundation/lfx-v2-project-service), [committee](https://github.com/linuxfoundation/lfx-v2-committee-service), [formation](https://github.com/linuxfoundation/lfx-v2-formation-service), and [member](https://github.com/linuxfoundation/lfx-v2-member-service) services already use it, so there is a pattern to copy.
- **SendGrid (COPS-433) is newsletter-only.** It is a provider switch inside newsletter-service, chosen for bulk-mail needs such as click tracking and per-publication sender reputation. There is no shared SendGrid rail to join.
- **In-repo templates are forced by the contract.** The relay accepts only pre-rendered mail. That also fixes legacy's worst trait: templates that were unversioned, unreviewed, and drifted from the code filling them.

The full research is in the comments on [linuxfoundation/lfx-self-serve#2188](https://github.com/linuxfoundation/lfx-self-serve/issues/2188).

## Contract

`pkg/api.SendEmailRequest` — `to`, `subject`, `html`, `text` all required; optional `from` (domain-allowlisted), `from_display_name` (a free display-name string — **no domain check**), `reply_to` (domain-allowlisted), `group_id`. Success replies `SendEmailResponse{email_id, group_id}`; failure replies `SendEmailErrorResponse{error}`.

**Sender identity.** Mentorship does not set `from`, so mail goes from the platform default — `noreply@lfx.linuxfoundation.org` in prod, already allowlisted. It sets only `from_display_name`, to `LFX Mentorship` (the relay default is `LFX Self Serve`). Confirm the sending domain with Cloud Ops before the first non-dev send.

| Limit | Consequence for Mentorship |
| --- | --- |
| **No send retry** | A refused send is lost unless the caller re-publishes. Mentorship does not in v1. |
| **Accepted is not delivered** | A success reply means SES accepted the message. Bounces arrive later on `lfx.email-service.email_failed` ([email-service#29](https://github.com/linuxfoundation/lfx-v2-email-service/pull/29)), a core-NATS publish. See [Bounces](#bounces). |
| No attachments, CC, or BCC | Nothing on the checklist needs them. One message per recipient. |
| Non-prod recipient allowlist | Dev and staging deliver only to `linuxfoundation.org`. A blocked recipient gets a normal success reply with empty `email_id` and `group_id`, not an error — treat it as sent, and test with `linuxfoundation.org` addresses, including the staff inboxes. |
| Headers | The relay strips CR/LF and encodes the subject, so no header sanitiser is needed here. |

## Parity checklist

02 promises "same email notifications" as legacy. This table is what that means. Source: every `Send*` method under jobspring [`backend/email/`](https://github.com/linuxfoundation/jobspring/tree/main/backend/email) and its call sites on `main` as of 2026-09-29. A template counts as **live** when at least one call site is not commented out. Legacy copy is a content starting point, not a contract.

Status column: **hook** — a `Notifier` method exists but delivers nothing yet; **none** — the rewrite transition exists with no `Notifier` call, unless the row says the transition is missing; **cron** — needs a scheduled job; **done** — template and send shipped. Update the column as rows ship.

| Legacy template | To | Rewrite trigger | Status |
| --- | --- | --- | --- |
| **Programs** | | | |
| `admin-mentorship-submission-confirmation` | Program Admin | program `draft → submitted` | none |
| `communitybridge-review-mentorship-submission` | LF staff review inbox | program `draft → submitted` and `rejected → submitted` — the notification-only replacement for the signed approval link ([02](./02-target-architecture.md)) | none |
| `admin-mentorship-submission-approved` | Program Admin | program `submitted → published` | none |
| `admin-mentorship-submission-rejected` | Program Admin | program `submitted → rejected` | none |
| `admin-project-edited-notification` | Program Admin | Program Admin edits a program that is not `rejected` | none |
| **Mentors** | | | |
| `mentor-project-invite` | invited mentor | mentor added to a program (`invited`) | hook: `NotifyMentorInvited` — signature changes with 08 |
| `admin-mentor-accepted` | active Program Admins | mentor accepts the invite (`invited → active`) | none — 08 adds `NotifyMentorAccepted` |
| `admin-mentor-declined` | active Program Admins | mentor declines the invite (`invited → declined`) | hook: `NotifyMentorDeclined` from `DeclineInvite` — needs its own method, see below |
| `mentor-admin-declined` | mentor | Program Admin declines a mentor (member `→ declined`) | hook: `NotifyMentorDeclined` from `Update` — called before the write today; move it after `repo.Update` so a failed write sends nothing |
| `admin-new-mentor-request` | active Program Admins | mentor applies to a program (application with role `mentor`) | none |
| `admin-mentor-withdrew-request` | active Program Admins | mentor application `→ withdrawn` | none |
| `admin-mentor-removed-project` | active Program Admins | active mentor leaves (member `active → withdrawn`) | none — transition missing: only a Program Admin can withdraw a member today, so mentors need a way to leave |
| **Mentee applications** | | | |
| `mentee-application-received` | mentee | application created with role `mentee`; lists the prerequisite tasks | none |
| `admin-review-mentee-application` | active Program Admins | last prerequisite task submitted | hook: `NotifyAdminTasksSubmitted` — fires today only when every prerequisite is `complete`; the count must also include `submitted`, and the hook must fire only when `tasks_submitted` first turns true, or every later review re-sends it |
| `mentee-mentorship-accepted` | mentee | application `→ accepted` | hook: `NotifyMenteeAccepted` |
| `hr-mentee-accepted` | LF staff HR inbox and the Program Admin | application `→ accepted`; carries attendance type and term dates | none — same event as the row above; the `attendanceType` argument exists for it |
| `mentee-application-declined` | mentee | application `→ declined` | none |
| `mentee-application-withdrawn` | mentee | application `→ withdrawn` | none |
| `admin-mentee-application-withdrawn` | active Program Admins | application `→ withdrawn` | none |
| `mentee-mentee-graduated-v2` | mentee | application `accepted → graduated` | none |
| **Tasks** | | | |
| `mentee-new-task-assigned-v2` | mentee | task created for an application by a reviewer | none |
| **Profiles** | | | |
| `mentee-new-profile-submitted` | mentee | first mentee profile saved | none |
| `mentor-mentorship-profile-submitted` | mentor | first mentor profile saved | none |
| **Scheduled** | | | |
| `mentee-incomplete-prerequisite-reminder` | mentee | pending application with incomplete prerequisite tasks, before the term's application deadline | cron — the rewrite has no such CronJob yet |

Three rows are confirmation-only mail (`admin-project-edited-notification` and the two profile confirmations). They are in scope by the rule above; product may cut them without loss of function.

**Not ported.**

- **Employer portal (6)**, dropped with the feature ([02](./02-target-architecture.md)): `organization-employer-profile-confirmation`, `communitybridge-review-organization-profile`, `organization-employer-profile-approved`, `organization-employer-profile-rejected`, `mentor-employer-recommendation`, `admin-new-mentor-invited-notice`. The HR acceptance notice is *not* in this group — it is an LF-staff notification and stays.
- **Already dead in legacy (16)** — every call site commented out, so users stopped receiving them long ago: `admin-mentee-accepted`, `admin-mentee-application-accepted`, `admin-mentee-application-received`, `admin-mentee-graduated`, `admin-mentorship-project-invite`, `admin-new-task-assigned-v2`, `admin-task-status-changed-v2`, `mentee-mentee-graduated` (superseded by `-v2`), `mentee-task-status-changed-v2`, `mentor-mentee-accepted`, `mentor-mentee-application-received`, `mentor-mentee-application-withdrawn`, `mentor-mentee-graduated`, `mentor-mentorship-application-submitted`, `mentor-new-task-assigned-v2`, `mentor-task-status-change-v2`. Notably, **no task-status-change email is live** in legacy, and mentors receive no application or task mail at all.
- **`SpecialRecipients`** — the prod-only CC of two contractor addresses on acceptance mail. A configurable staff inbox covers the need.

## Implementation shape

[`domain.Notifier`](../../backend/internal/domain/notifier.go) is the swap point. Services already call it for the four **hook** rows.

- **Transport.** Reuse the NATS connection the backend already opens for `FGA_NATS_URL` ([`server.go`](../../backend/cmd/mentorship-api/server.go)). No new client, URL, or flag. Without the URL, the existing [`LogNotifier`](../../backend/internal/infrastructure/notifier.go) stays wired.
- **Recipients.** `Notifier` methods take the domain models the service already holds. The email implementation looks up addresses — the user, the program's active Program Admins, the staff inboxes from config — then renders and sends. A user with no email is logged and skipped. 08 uses the same shape.
- **Interface changes.** Split `NotifyMentorDeclined` in two: a Program Admin declining a mentor mails the mentor; a mentor declining an invite mails the Program Admins. `NotifyAdminTasksSubmitted` sends `admin-review-mentee-application`. `NotifyMenteeAccepted` sends `mentee-mentorship-accepted` and `hr-mentee-accepted`. Every other trigger adds one method when its feature ships, and that method sends all of the trigger's rows.
- **Templates.** HTML and text per row, embedded with `//go:embed`, a shared layout, and a subject per template. Copy the adapter and loader from invite-service ([`email_sender.go`](https://github.com/linuxfoundation/lfx-v2-invite-service/blob/main/internal/infrastructure/nats/email_sender.go), [`templates.go`](https://github.com/linuxfoundation/lfx-v2-invite-service/blob/main/internal/infrastructure/smtp/templates.go)).
- **Sends.** Run each send in a detached goroutine with a short timeout (context from `context.WithoutCancel`), log failures, and never fail the business operation. member-service makes the same best-effort send inline in its HTTP path; Mentorship detaches because one event can mail several recipients (the mentee, the HR inbox, every active Program Admin), and inline sends would add a round trip each to the response. `Server.Shutdown` waits for in-flight sends, within its existing timeout, before it closes NATS and the database pool, so a rollout does not drop them. No outbox or retry in v1.
- **Config.** New required values, guarded in `templates/validate.yaml` when the NATS URL is set: the public-site and Self Serve base URLs for links (management pages live in Self Serve, per [02](./02-target-architecture.md)), the LF staff review inbox, and the LF staff HR inbox. The HR notice goes out once per acceptance, without legacy's once-per-program dedupe.
- **Not in v1:** storing `email_id`, subscribing to `email_failed`, `reply_to`, a custom `from`, CC.

### Bounces

Reacting to a bounce is the sender's job, not the relay's. For v1 we accept the gap: every notification has a UI path that shows the same state. The one risk is a bounced mentor invitation, which leaves the mentor with no signal. If that matters, store the returned `email_id` on the invitation, subscribe to `email_failed`, and show delivery state to the Program Admin. No other consumer does this yet.

## Out of scope

- **HMAC-signed approval links.** Program approval happens in Self Serve, so its email is a notification only.
- **Bulk and marketing mail**, and with it unsubscribe handling.

## Decision log

| Date | Decision |
| --- | --- |
| 2026-09-22 | Approved on [lfx-mentorship#166](https://github.com/linuxfoundation/lfx-mentorship/pull/166). The email-service author and the platform lead confirmed decision 1 — "the point is to have a central service for this purpose" — and that COPS-433 does not move new consumers to SendGrid. API gaps go to the email-service team, not local workarounds. Retry stays the caller's job until the service adds it. Mentorship uses the shared `lfx.linuxfoundation.org` sending domain, pending Cloud Ops. |
| 2026-09-28 | email-service publishes bounce and delivery events ([email-service#29](https://github.com/linuxfoundation/lfx-v2-email-service/pull/29)), closing the push-on-bounce ask from the review. |
| 2026-09-29 | Decision 3 replaced by the parity checklist. Earlier pairings of `NotifyAdminTasksSubmitted`, `NotifyMenteeAccepted`, and `NotifyMentorDeclined` with `admin-new-task-assigned-v2`, `admin-mentee-accepted`, and `mentor-admin-declined` alone withdrawn. Reuse the NATS connection, resolve recipients in the notifier, detach sends with no outbox and drain them on shutdown, keep the HR notice, defer bounce handling. |
