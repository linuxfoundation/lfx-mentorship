<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship Rewrite — 07: Email Delivery

Status: Approved — reviewed 2026-09-22 (see Review outcome)
Related: [02-target-architecture.md](./02-target-architecture.md) (Integrations table, records the same rail), [01-current-system.md](./01-current-system.md) (legacy Mandrill setup)
Decision ticket: [linuxfoundation/lfx-self-serve#2188](https://github.com/linuxfoundation/lfx-self-serve/issues/2188)

The rewrite needs transactional email — mentor invitations, application decisions, task notifications. Legacy ([jobspring](https://github.com/linuxfoundation/jobspring)) sends via **Mandrill**, synchronously inside HTTP handlers, with 47 templates authored in Mailchimp's editor and hand-synced by `curl`. This doc records what replaces it.

## Decisions

| # | Question | Decision |
| --- | --- | --- |
| 1 | Delivery rail | **[lfx-v2-email-service](https://github.com/linuxfoundation/lfx-v2-email-service)** — NATS request/reply on `lfx.email-service.send_email`, via its Go client `pkg/api`. Not SendGrid, not a direct SES/Mandrill client. |
| 2 | Template ownership | **This repo.** Go `html/template` renders an HTML and a plain-text body per notification; no externally hosted templates. |
| 3 | Legacy template scope | The 47 legacy templates are a parity **checklist, not a porting target**. Implement the 4 the `Notifier` interface already declares; each later feature adds its own template alongside its logic. |
| 4 | Keep Mandrill? | **No.** Mandrill is legacy-only. Mentorship holds no provider credentials of any kind. |

## Why this rail

**The platform convention is that no service sends its own email.** email-service is a thin, stateless relay: it owns the SMTP credentials, the from-domain allowlist, the non-prod recipient guardrail, and the SES→SNS→SQS engagement pipeline. It is deployed in dev, staging, and prod ([lfx-v2-argocd](https://github.com/linuxfoundation/lfx-v2-argocd) `values/{dev,staging,prod}/lfx-v2-email-service.yaml`), and five services already import `pkg/api`: [invite-service](https://github.com/linuxfoundation/lfx-v2-invite-service) (the closest analogue to mentorship's invitation mail), [project](https://github.com/linuxfoundation/lfx-v2-project-service), [committee](https://github.com/linuxfoundation/lfx-v2-committee-service), [formation](https://github.com/linuxfoundation/lfx-v2-formation-service), and [member-service](https://github.com/linuxfoundation/lfx-v2-member-service). Four of them put the adapter in the same place — `internal/infrastructure/nats/email_sender.go` — so there is a pattern to copy rather than invent.

**The SendGrid cutover (COPS-433) is newsletter-specific, not a platform direction.** `EMAIL_PROVIDER` is a switch internal to newsletter-service, flipped to `sendgrid` in dev and prod only — staging still runs the default, and the documented rollback is "set provider back to email-service". Its NATS dispatcher still imports `lfx-v2-email-service/pkg/api`. The motives are bulk-newsletter concerns (branded click tracking, a signed event webhook, per-publication subusers for sender-reputation isolation, per-project From domains) that do not apply to low-volume transactional mail. There is no shared platform SendGrid rail — newsletter brokers SendGrid directly with its own API key, webhook, and domain authentication, which a mentorship integration would have to rebuild from scratch.

**Owning templates in-repo is forced by the contract and is also the right call.** The relay is pre-rendered only, by explicit design. That happens to fix the legacy arrangement's worst trait: template content that was unversioned, unreviewable, and free to drift from the code filling it. In-repo templates go through PR review and are unit-testable.

## Contract

`pkg/api.SendEmailRequest` — `to`, `subject`, `html`, `text` all required; optional `from` (domain-allowlisted), `from_display_name` (a free display-name string — **no domain check**), `reply_to` (domain-allowlisted), `group_id`. Success replies `SendEmailResponse{email_id, group_id}`; failure replies `SendEmailErrorResponse{error}`.

**Sender identity.** Mentorship sends from the platform default, `noreply@lfx.linuxfoundation.org` — it does not set `from`. That is the prod `smtpFrom` and is already in `smtpAllowedFromDomains`, so this aligns with the rest of the platform and needs no allowlist change. Only `from_display_name` is set, to distinguish Mentorship mail in the inbox. Confirm the sending domain with Cloud Ops before the first non-dev send.

Limits to design around:

| Limit | Consequence for Mentorship |
| --- | --- |
| **No send retry** in the relay | A send the relay refuses is lost unless Mentorship re-publishes. Acceptable for these notifications; revisit only with evidence. |
| **Acceptance is not delivery** | A `SendEmailResponse` means SES accepted the message, not that it arrived. Transient and hard bounces surface later. |
| No attachments, no CC/BCC | None of the 4 notifications need them. The legacy `SpecialRecipients` hack (CC'ing contractor addresses on prod admin mail) is not carried forward. |
| Non-prod recipient allowlist | Dev delivers only to `linuxfoundation.org` (`smtpAllowedRecipientDomains`) — test accounts must use that domain. |
| No templating | This repo renders both bodies. |

## Implementation shape

[`domain.Notifier`](../../backend/internal/domain/notifier.go) is already the swap point, and services already call it at the right points — only delivery is missing. Its 4 methods map onto the nearest legacy templates — the pairing is a content starting point, not a promise to reproduce legacy copy:

| `Notifier` method | Legacy template |
| --- | --- |
| `NotifyMentorInvited` | `mentor-project-invite` |
| `NotifyMentorDeclined` | `mentor-admin-declined` (to the mentor; legacy also had an admin-facing `admin-mentor-declined`) |
| `NotifyAdminTasksSubmitted` | `admin-new-task-assigned-v2` |
| `NotifyMenteeAccepted` | `admin-mentee-accepted` |

Work required: add a NATS client to the backend (it has **no NATS dependency today** — this is the one non-trivial piece), add an email-sending `Notifier` implementation beside the existing [`LogNotifier`](../../backend/internal/infrastructure/notifier.go), add templates, and add the NATS URL to the chart plus a `templates/validate.yaml` guard.

**Sends stay fire-and-forget.** The `Notifier` methods return no error by design; the implementation logs and continues on failure. A failed send must never fail the business operation — the opposite of legacy, where a Mandrill error could fail the parent HTTP request.

### Post-acceptance failures are ours to handle

A successful reply means SES *accepted* the message. It can still bounce afterwards — transiently, or hard. email-service does record this: its SES→SNS→SQS pipeline marks the recipient record `Failed` on `BOUNCE` and `COMPLAINT`. But **nothing pushes that back to the caller** — it is only readable by polling `get_email_status` (per `email_id`/`group_id`) or `get_email_engagement_analytics`. The relay's job ends at acceptance; reacting to a bounce is the sending service's responsibility.

For the initial 4 notifications we **accept this gap knowingly** rather than build a poller: all 4 are internally triggered, low volume, and each has a UI path that shows current state, so a lost mail is recoverable by the user or an admin. The one to watch is `NotifyMentorInvited` — an invitation that silently bounces leaves a mentor waiting with no signal. If that proves to matter in practice, the smallest fix is to store the returned `email_id` against the invitation and surface delivery state to the program admin, not to add a background retry loop.

Worth raising with the platform team if it recurs across services: a push notification on bounce (a NATS subject the sender can subscribe to) would be more useful to every consumer than each service polling.

## Out of scope

Dropped with their features, not ported: the 6 employer/HR templates (employer portal is [explicitly out of scope](./02-target-architecture.md)), and the HMAC-signed email approval links — program submission becomes an authenticated approval in Self Serve, so its email is a notification only. Bulk and marketing mail is not in scope at all; unsubscribe handling therefore is not either.

## Review outcome

Decision 1 is **confirmed**. Reviewed 2026-09-22 by the email-service author and the platform lead on [lfx-mentorship#166](https://github.com/linuxfoundation/lfx-mentorship/pull/166): email-service is the intended rail for transactional mail — "the point is to have a central service for this purpose" — and COPS-433 does not signal a move of new consumers to SendGrid. Gaps in the email-service API are to be raised with that team rather than worked around locally; retry may be added service-side later, and until then a failed send is re-published by the caller.

Two follow-ups this review produced, both folded in above: post-acceptance bounces are the sending service's responsibility, and Mentorship uses the shared `lfx.linuxfoundation.org` sending domain for platform consistency (to be confirmed with Cloud Ops before the first non-dev send).
