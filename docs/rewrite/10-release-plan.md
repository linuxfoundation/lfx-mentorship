<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship Rewrite — 10: Release Plan

Status: Draft. Target release: **Tuesday 2026-10-27**, to be confirmed with the time at the release-readiness review on **Friday 2026-10-23**.
Related: [08 runbook](./08-prod-migration-runbook.md) holds the commands; this page holds the order and the owners.

## Approach

1. **Test on prod data now.** Import legacy prod data into the new prod and test there while legacy keeps serving users. Email reaches only `EMAIL_ALLOWED_RECIPIENTS`, and Self Serve shows Mentorship only to testers (LaunchDarkly `mentorship-enabled`).
2. **Release with a few hours of downtime.** Close legacy, wipe the test data, run a fresh full import, verify, then send everyone to the new site. The import itself took about 16 minutes on the smoke run; the window is the file copy, verification and the FGA outbox drain on top: about 6 hours in all, most of it the drain (release day, step 6).

## Before release

- [x] Runbook 08 updated: Phase 1 and the no-go step describe the test import and the `TRUNCATE` ([#290](https://github.com/linuxfoundation/lfx-mentorship/pull/290), Eng)
- [x] Test import into prod, in-cluster, off-hours, platform team warned: about 330k FGA and 330k index messages (Eng)
- [x] Import and outbox-drain time measured: the import takes minutes; the FGA outbox drains in about 4.6 hours at the throttled 20 messages/s (330k markers). OpenFGA already holds the test import's tuples under the same ids, so unchanged objects lose nothing while they wait. But the relay works through the queue by object type, not by time: applications first (76k, about an hour), then programs (1.5k, a minute), then tasks (253k, about 3.5 hours). Anything written after go-live takes its type's place in that order, so a new applicant's tasks would wait behind the import's tasks. The drain is therefore part of the window (release day, step 6). The index outbox is first-in, first-out at the same rate, which only delays global search (Eng)
- [ ] Duplicate legacy identities resolved: the importer keeps a shared email or LFID on one user and clears it on the others, which then cannot use Mentorship; get the counts from the test import, then merge at import or fix by hand (Eng)
- [ ] Data exceptions from the test import reviewed: programs with no LF project or no admin, pending mentor rows, quarantined tasks (Product, Eng)
- [x] Legacy-link forwarding in the frontend, live in prod, as in [lfx-crowdfunding#358](https://github.com/linuxfoundation/lfx-crowdfunding/pull/358): `/project/{id}[/…]` → `/programs/{id}`, `/mentee/{id}[,{projectId}]` → `/mentees/{id}`, `/mentor/{id}` → `/mentors/{id}`, other legacy pages → `/` ([#290](https://github.com/linuxfoundation/lfx-mentorship/pull/290), Eng)
- [x] Legacy links tested on prod with the legacy path on the new host (2026-10-21): all 197 program links in the [CNCF 2026 term docs](https://github.com/cncf/mentoring/tree/main/programs/lfx-mentorship) redirect to `/programs/{id}`; 192 open, and the other 5 are past-term programs hidden in legacy, which the new site does not show (Eng)
- [ ] CNCF `/lfx-url` bot accepts the new URL shape: [cncf/mentoring#2142](https://github.com/cncf/mentoring/pull/2142) accepts both shapes; Nate merges it before go-live, since CNCF is already posting next term's programs (CNCF)
- [ ] Legacy API consumers found and handled before it is switched off: CNCF automation does not call it (checked); still open is legacy Crowdfunding (`LFF`), which calls `/users/external/{lfid}` (Eng)
- [x] Maintenance switch ready and tried on dev, both in prod with the flags off (frontend `v1.5.5`; jobspring `v1.5.28`, 2026-10-23; `v1.5.27` predates the switch), as Crowdfunding did ([lfx-crowdfunding-upgrade#48](https://github.com/linuxfoundation/lfx-crowdfunding-upgrade/pull/48)): a `maintenanceMode` flag in `lfx-mentorship-upgrade` that shows a "being upgraded" page ([#200](https://github.com/linuxfoundation/lfx-mentorship-upgrade/pull/200)), and the same flag in `jobspring` that answers 503 on the API (except `/health`) and pauses the cron jobs and the SQS consumer ([#2281](https://github.com/linuxfoundation/jobspring/pull/2281)) (Eng)
- [ ] Host redirect `mentorship.lfx.linuxfoundation.org` → `mentorship.linuxfoundation.org`, path kept, with a contact on the day ([requested](https://linuxfoundation.slack.com/archives/C02RMNFE9FF/p1792698887269559); remind them with the exact date after Friday's review) (DevOps, Cloudflare)
- [ ] Support entry point on the public site: a "Contact support" link that opens a popup sending the user to the chat in Self Serve (`app.lfx.dev/mentorship/mentee`); the Mentorship site itself does not load Intercom. Agreed with Support ([Heather, 2026-10-23](https://linuxfoundation.slack.com/archives/C07192MR02X/p1792699963761369)); ticket [lfx-self-serve#1533](https://github.com/linuxfoundation/lfx-self-serve/issues/1533) (Eng)
- [ ] fga-sync owners and DevOps told the date and start time of the drain: about 4.6 hours at 20 messages/s, half the shared consumer's capacity, which is the condition they [agreed to](https://linuxfoundation.slack.com/archives/C02RMNFE9FF/p1792133200158979) (Eng)
- [x] Email: every email is copied to one staff inbox per environment, `EMAIL_COPY_INBOX`, which is `lfx-mentorship-sent@` in prod ([#335](https://github.com/linuxfoundation/lfx-mentorship/pull/335), [lfx-v2-argocd#1860](https://github.com/linuxfoundation/lfx-v2-argocd/pull/1860), DevOps, Eng)
- [x] Approvers listed in prod values ([lfx-v2-argocd#1874](https://github.com/linuxfoundation/lfx-v2-argocd/pull/1874), merged)
- [ ] Release tagged and pinned in [lfx-v2-argocd](https://github.com/linuxfoundation/lfx-v2-argocd) (Eng)
- [x] Program admins told in [#mentorship_admins](https://linuxfoundation.slack.com/archives/CT3NP3UDU/p1792695758113669) (2026-10-22), without the new address; it is shared once live (Product)

## Release day

1. **Freeze legacy:** announce "downtime starts" on Slack and the banner, then deploy `lfx-mentorship-upgrade` and `jobspring` with the maintenance flag on (`jobspring`: also `sqsConsumerEnabled: false`). Wait 15 minutes, the cron timeout, so API requests and cron jobs already running finish before the import reads DynamoDB.
2. **Wipe, copy, import:** runbook Phases 2–4. The wipe is in Phase 4: `TRUNCATE` every table in the `mentorship` schema, outboxes included, leaving `public.schema_migrations` alone.
3. **Restore program approvers:** the wipe removes them. Approvers who are not legacy users sign in to Self Serve again, then re-run the approver sync: the one-off Job in `backend/docs/api.md`, or a manual ArgoCD sync of `lfx-mentorship-backend` with DevOps on standby (runbook Phase 4, step 8).
4. **Verify:** runbook Phase 5. Both verifiers must pass. Don't wait for the outboxes here.
5. **Go/no-go:** runbook Phase 6, step 1.
6. **Drain:** wait until the FGA outbox is empty, about 4.6 hours, with no `dead_letter`; the approvers' team tuples go out with it, after the applications. The relay works by object type (applications, programs, tasks), so anything written after go-live would otherwise wait behind the import's 253k tasks: a new applicant would be denied on their task uploads for up to 3.5 hours. If the window has to be shorter, go live once the applications and programs are through, about an hour in, and accept that task lag. Raising the relay rate doesn't help: fga-sync handles about 40 messages/s for the whole platform, one at a time, and its owners agreed to the import on condition it stays throttled.
7. **Email to everyone:** remove `EMAIL_ALLOWED_RECIPIENTS` and wait for the rollout, runbook Phase 6, step 3.
8. **Self Serve to everyone:** turn `mentorship-enabled` on for all users.
9. **Redirect the legacy host** (DevOps), re-run the legacy-link test, then announce "we're live" with the new URL. Then send CNCF the one-line PR pointing the verification link in `lfx-proposal-validate.yml` at the new host.

**Rollback:** clean until step 8, the first step that lets everyone write (redeploy both legacy repos with the maintenance flag off, restore the allowlist). From step 8 on, fix forward.

## After release

- [ ] First week: watch `service:lfx-mentorship-backend status:error` in Datadog Logs by hand. No monitor exists yet, and dev and prod log under the same service with no `env` tag, so a monitor would alert on dev too; ask DevOps for the tags and a prod error monitor (Eng, DevOps)
- [ ] Keep legacy in maintenance mode until decommission. It also keeps the legacy reminder cron off, so mentees don't get two reminders now that the new reminder CronJob runs in prod ([lfx-v2-argocd#1846](https://github.com/linuxfoundation/lfx-v2-argocd/pull/1846))
- [ ] Revert the FGA relay throttle once the import's FGA outbox has drained: in `values/prod/lfx-mentorship-backend.yaml`, remove `FGA_RELAY_BATCH_SIZE: "10"` (default 50) and `autoscaling.maxReplicas: 2` (default 10), set in [lfx-v2-argocd#1824](https://github.com/linuxfoundation/lfx-v2-argocd/pull/1824). It keeps the import's ~330k markers at 20 messages/s so the shared fga-sync consumer doesn't back up, but it also caps the API at 2 pods, which live traffic may need (Eng)
- [ ] Any time after release: lock the legacy uploads bucket, S3 Block Public Access on `jobspring-prod-uploads` in the legacy account (runbook Phase 6, step 2). Nothing writes to it once legacy is in maintenance mode, and it has been public for years, so it is not urgent (DevOps)
- [ ] Day after: Fivetran connector from the Mentorship Postgres database to Snowflake ([lfx-self-serve#1532](https://github.com/linuxfoundation/lfx-self-serve/issues/1532), DevOps), then the dbt models read it ([lfx-self-serve#1537](https://github.com/linuxfoundation/lfx-self-serve/issues/1537), Eng); check Crowdfunding's sync creates no duplicate initiatives
- [ ] Links we control point at the new host: LFX header, Crowdfunding footer and tools menu; they redirect until then
- [ ] About two weeks after release: remove the `mentorship-enabled` LaunchDarkly flag and the `mentorshipEnabledGuard` that reads it in lfx-self-serve. Once Mentorship is on for everyone the gate is no longer needed, and its readiness timeout sends users from Mentorship pages to the dashboard when LaunchDarkly starts without their identity ([lfx-self-serve#4179](https://github.com/linuxfoundation/lfx-self-serve/issues/4179)) (Eng)
- [ ] Legacy decommission per [03](./03-migration-plan.md) Phase 5
- [ ] Follow up with CNCF on automating program creation
