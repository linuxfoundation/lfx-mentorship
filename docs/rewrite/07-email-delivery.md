<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship Rewrite — 07: Email Delivery

Status: Approved — reviewed 2026-09-22; amended 2026-09-29 with the parity checklist and corrections (see Decision log)
Related: [02-target-architecture.md](./02-target-architecture.md) (Integrations table, records the same rail), [01-current-system.md](./01-current-system.md) (legacy Mandrill setup), 08 mentor invitations (draft, [lfx-mentorship#177](https://github.com/linuxfoundation/lfx-mentorship/pull/177) — changes the invitation rows below)
Decision ticket: [linuxfoundation/lfx-self-serve#2188](https://github.com/linuxfoundation/lfx-self-serve/issues/2188)

The rewrite needs transactional email — mentor invitations, application decisions, task notifications. Legacy ([jobspring](https://github.com/linuxfoundation/jobspring)) sends via **Mandrill**, synchronously inside HTTP handlers, with about 47 templates authored in Mailchimp's editor and hand-synced by `curl`. This doc records what replaces it, and which of those templates the rewrite owes for feature parity.

## Decisions

| # | Question | Decision |
| --- | --- | --- |
| 1 | Delivery rail | **[lfx-v2-email-service](https://github.com/linuxfoundation/lfx-v2-email-service)** — NATS request/reply on `lfx.email-service.send_email`, via its Go client `pkg/api`. Not SendGrid, not a direct SES/Mandrill client. |
| 2 | Template ownership | **This repo.** Go `html/template` renders an HTML and a plain-text body per notification; no externally hosted templates. |
| 3 | Legacy template scope | **The [parity checklist](#parity-checklist) below is the target**: every legacy template that still has a live send in jobspring and whose trigger exists in the rewrite — 24 of the 46 distinct slugs. Excluded: the 6 employer-portal templates, the `SpecialRecipients` CC hack, and the 16 templates whose sends legacy has already commented out. The 4 `Notifier` methods that exist today are the first slice, not the target. |
| 4 | Keep Mandrill? | **No.** Mandrill is legacy-only. Mentorship holds no provider credentials of any kind. |

## Why this rail

**The platform convention is that no service sends its own email.** email-service is a thin, stateless relay: it owns the SMTP credentials, the from-domain allowlist, the non-prod recipient guardrail, and the SES→SNS→SQS engagement pipeline. It is deployed in dev, staging, and prod ([lfx-v2-argocd](https://github.com/linuxfoundation/lfx-v2-argocd) `values/{dev,staging,prod}/lfx-v2-email-service.yaml`), and five services already import `pkg/api`: [invite-service](https://github.com/linuxfoundation/lfx-v2-invite-service) (the closest analogue to mentorship's invitation mail), [project](https://github.com/linuxfoundation/lfx-v2-project-service), [committee](https://github.com/linuxfoundation/lfx-v2-committee-service), [formation](https://github.com/linuxfoundation/lfx-v2-formation-service), and [member-service](https://github.com/linuxfoundation/lfx-v2-member-service). Four of them put the adapter in the same place — `internal/infrastructure/nats/email_sender.go` — so there is a pattern to copy rather than invent.

**The SendGrid cutover (COPS-433) is newsletter-specific, not a platform direction.** `EMAIL_PROVIDER` is a switch internal to newsletter-service, flipped to `sendgrid` in dev and prod only — staging still runs the default, and the documented rollback is "set provider back to email-service". Its NATS dispatcher still imports `lfx-v2-email-service/pkg/api`. The motives are bulk-newsletter concerns (branded click tracking, a signed event webhook, per-publication subusers for sender-reputation isolation, per-project From domains) that do not apply to low-volume transactional mail. There is no shared platform SendGrid rail — newsletter brokers SendGrid directly with its own API key, webhook, and domain authentication, which a mentorship integration would have to rebuild from scratch.

**Owning templates in-repo is forced by the contract and is also the right call.** The relay is pre-rendered only, by explicit design. That happens to fix the legacy arrangement's worst trait: template content that was unversioned, unreviewable, and free to drift from the code filling it. In-repo templates go through PR review and are unit-testable.

## Contract

`pkg/api.SendEmailRequest` — `to`, `subject`, `html`, `text` all required; optional `from` (domain-allowlisted), `from_display_name` (a free display-name string — **no domain check**), `reply_to` (domain-allowlisted), `group_id`. Success replies `SendEmailResponse{email_id, group_id}`; failure replies `SendEmailErrorResponse{error}`.

Since email-service [PR 29](https://github.com/linuxfoundation/lfx-v2-email-service/pull/29) (merged 2026-09-28, [linuxfoundation/lfx-self-serve#2831](https://github.com/linuxfoundation/lfx-self-serve/issues/2831)) the relay also **publishes** delivery outcomes: `lfx.email-service.email_failed` (`EmailFailedEvent{email_id, group_id, reason, failed_at}`, reason `bounce` or `complaint`) and `lfx.email-service.email_delivered`, plus open and click events. Mentorship does not subscribe in v1 — see [Post-acceptance failures](#post-acceptance-failures-are-ours-to-handle).

**Sender identity.** Mentorship sends from the platform default, `noreply@lfx.linuxfoundation.org` — it does not set `from`. That is the prod `smtpFrom` and is already in `smtpAllowedFromDomains`, so this aligns with the rest of the platform and needs no allowlist change. Only `from_display_name` is set, to distinguish Mentorship mail in the inbox. Confirm the sending domain with Cloud Ops before the first non-dev send.

Limits to design around:

| Limit | Consequence for Mentorship |
| --- | --- |
| **No send retry** in the relay | A send the relay refuses is lost unless Mentorship re-publishes. Acceptable for these notifications; revisit only with evidence. |
| **Acceptance is not delivery** | A `SendEmailResponse` means SES accepted the message, not that it arrived. Transient and hard bounces surface later. |
| No attachments, no CC/BCC | Nothing on the checklist needs them. Where legacy fanned one message out to several recipients (a Program Admin plus an LF staff inbox), the rewrite sends one message per recipient. The legacy `SpecialRecipients` hack (CC'ing contractor addresses on prod admin mail) is not carried forward. |
| Non-prod recipient allowlist | Dev delivers only to `linuxfoundation.org` (`smtpAllowedRecipientDomains`) — test accounts and the staff inboxes must use that domain. |
| No templating | This repo renders both bodies. |

## Parity checklist

02 promises "same email notifications" as legacy. This table is what that means. Source: every `Send*` method under jobspring [`backend/email/`](https://github.com/linuxfoundation/jobspring/tree/main/backend/email) and its call sites on `main` as of 2026-09-29. A template counts as **live** when at least one call site is not commented out. Legacy copy is a content starting point, not a contract.

Status column: **hook** — a `Notifier` method exists but delivers nothing yet; **none** — the rewrite transition exists with no `Notifier` call; **cron** — needs a scheduled job; **done** — template and send shipped. Update the column as rows ship.

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
| `mentor-admin-declined` | mentor | Program Admin declines a mentor (member `→ declined`) | hook: `NotifyMentorDeclined` from `Update` |
| `admin-new-mentor-request` | active Program Admins | mentor applies to a program (application with role `mentor`) | none |
| `admin-mentor-withdrew-request` | active Program Admins | mentor application `→ withdrawn` | none |
| `admin-mentor-removed-project` | active Program Admins | active mentor leaves (member `active → withdrawn`) | none |
| **Mentee applications** | | | |
| `mentee-application-received` | mentee | application created with role `mentee`; lists the prerequisite tasks | none |
| `admin-review-mentee-application` | active Program Admins | last prerequisite task submitted | hook: `NotifyAdminTasksSubmitted` |
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

[`domain.Notifier`](../../backend/internal/domain/notifier.go) is the swap point; services already call it at the right points for the four **hook** rows. What follows are the decisions the checklist forces.

**Transport already exists.** The backend opens a NATS connection when `FGA_NATS_URL` is set ([`server.go`](../../backend/cmd/mentorship-api/server.go)) and runs the FGA and indexer outbox relays on it. The email sender reuses that connection — no second URL, no new client, no new `validate.yaml` guard for it. When the URL is empty (local dev without NATS) the existing [`LogNotifier`](../../backend/internal/infrastructure/notifier.go) stays wired, so no separate email on/off flag is needed either.

**Recipients are resolved in the notifier, not in services.** `Notifier` methods take the domain models the calling service already holds (the application, the member, the program) rather than bare IDs. The email implementation resolves recipients — a user's address via `domain.UserRepository`, a program's active Program Admins via the member repository, the LF staff inboxes from config — then renders and sends. Services stay free of email concerns, and the "all active Program Admins" fan-out that a third of the checklist needs lives in one place. `users.email` is nullable: a recipient with no address is logged and skipped, never an error. This is the shape 08 already adopts with `NotifyMentorInvited(ctx, invitation)`.

**Interface changes the checklist implies.** `NotifyMentorDeclined` is called from two places with opposite recipients — a Program Admin declining a mentor mails the mentor; a mentor declining an invite mails the admins — and splits into two methods. `NotifyAdminTasksSubmitted` renders `admin-review-mentee-application`; `NotifyMenteeAccepted` renders `mentee-mentorship-accepted` to the mentee and `hr-mentee-accepted` to the staff HR inbox and the Program Admin. The earlier pairing of these three with `admin-new-task-assigned-v2`, `admin-mentee-accepted`, and `mentor-admin-declined` alone pointed at templates legacy no longer sends, or at the wrong recipient, and is withdrawn. Every other row adds one method when its feature ships.

**Templates.** One HTML and one text template per row, embedded with `//go:embed` and parsed once at start-up (`html/template` and `text/template`), a shared layout, a subject per template, and header values passed through a single-line sanitiser. Copy the adapter and the template loader from invite-service ([`email_sender.go`](https://github.com/linuxfoundation/lfx-v2-invite-service/blob/main/internal/infrastructure/nats/email_sender.go), [`templates.go`](https://github.com/linuxfoundation/lfx-v2-invite-service/blob/main/internal/infrastructure/smtp/templates.go)) rather than designing a new one.

**Sends stay fire-and-forget, and off the request path.** `Notifier` methods return no error by design. The implementation runs each send in a detached goroutine with its own short timeout (the request context ends when the response is written), logs and drops on failure, and never fails the business operation — the opposite of legacy, where a Mandrill error could fail the parent HTTP request. No email outbox, relay, or retry loop in v1: the two relays that exist serve state that must converge (FGA tuples, the search index); a lost notification is recoverable through the UI.

**New config**, required whenever the NATS URL is set and guarded in `templates/validate.yaml`: the public frontend base URL for deep links, the LF staff review inbox (program submissions), and the LF staff HR inbox (mentee acceptances). The from display name is a constant, `LFX Mentorship`. The HR notice goes out once per acceptance; legacy deduplicated it to once per program with a sent-mail table, which is not worth the state.

**Not in v1:** persisting `email_id`, subscribing to `email_failed`, `reply_to`, a custom `from`, CC of any kind.

### Post-acceptance failures are ours to handle

A successful reply means SES *accepted* the message. It can still bounce afterwards — transiently, or hard. email-service records this through its SES→SNS→SQS pipeline and, since [PR 29](https://github.com/linuxfoundation/lfx-v2-email-service/pull/29), **publishes** it: `email_failed` on bounce or complaint, `email_delivered` on delivery. The relay's job ends at acceptance; reacting to a bounce is the sending service's responsibility, and a sender that wants to know subscribes.

For v1 we **accept this gap knowingly** rather than subscribe: every row is internally triggered, low volume, and has a UI path that shows current state, so a lost mail is recoverable by the user or a Program Admin. The one to watch is the mentor invitation — an invitation that silently bounces leaves a mentor waiting with no signal. If that proves to matter in practice, the smallest fix is to store the returned `email_id` against the invitation, subscribe to `email_failed`, and surface delivery state to the Program Admin — not to add a background retry loop.

## Out of scope

Dropped with their features, not ported: the 6 employer-portal templates (employer portal is [explicitly out of scope](./02-target-architecture.md)), and the HMAC-signed email approval links — program submission becomes an authenticated approval in Self Serve, so its email is a notification only. Bulk and marketing mail is not in scope at all; unsubscribe handling therefore is not either.

## Review outcome

Decision 1 is **confirmed**. Reviewed 2026-09-22 by the email-service author and the platform lead on [lfx-mentorship#166](https://github.com/linuxfoundation/lfx-mentorship/pull/166): email-service is the intended rail for transactional mail — "the point is to have a central service for this purpose" — and COPS-433 does not signal a move of new consumers to SendGrid. Gaps in the email-service API are to be raised with that team rather than worked around locally; retry may be added service-side later, and until then a failed send is re-published by the caller.

Two follow-ups this review produced, both folded in above: post-acceptance bounces are the sending service's responsibility, and Mentorship uses the shared `lfx.linuxfoundation.org` sending domain for platform consistency (to be confirmed with Cloud Ops before the first non-dev send).

## Decision log

| Date | Decision |
| --- | --- |
| 2026-09-22 | Rail confirmed as email-service over NATS; templates in-repo; no Mandrill. Push-on-bounce raised with the email-service team ([lfx-mentorship#166](https://github.com/linuxfoundation/lfx-mentorship/pull/166)). |
| 2026-09-28 | email-service ships `email_failed` / `email_delivered` push subjects ([PR 29](https://github.com/linuxfoundation/lfx-v2-email-service/pull/29)); the 2026-09-22 ask is closed. |
| 2026-09-29 | Parity scope fixed as the checklist rule: live legacy send + trigger exists in the rewrite, minus employer, `SpecialRecipients`, and dead templates (24 rows). Three template pairings corrected. Email reuses the existing NATS connection; recipients resolved in the notifier from domain models; sends detached from the request, no outbox; HR acceptance notice kept as an LF-staff mail, once per acceptance; bounce subscription deferred. |
