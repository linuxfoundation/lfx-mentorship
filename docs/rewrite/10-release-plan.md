<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship Rewrite — 10: Release Plan

Status: Draft. Target release: **Tuesday 2026-10-27**, to be confirmed with the time at the release-readiness review on **Friday 2026-10-23**.
Related: [08 runbook](./08-prod-migration-runbook.md) holds the commands; this page holds the order and the owners.

## Approach

1. **Test on prod data now.** Import legacy prod data into the new prod and test there while legacy keeps serving users. Email reaches only `EMAIL_ALLOWED_RECIPIENTS`, and Self Serve shows Mentorship only to testers (LaunchDarkly `mentorship-enabled`).
2. **Release with a few hours of downtime.** Close legacy, wipe the test data, run a fresh full import, verify, then send everyone to the new site. The import itself took about 16 minutes on the smoke run; the window is the file copy, verification and the FGA drain for applications and programs on top: about 2 to 2.5 hours. Task permissions keep draining for about 3.5 hours after go-live; existing tasks already have theirs.

## Before release

- [x] Runbook 08 updated: Phase 1 and the no-go step describe the test import and the `TRUNCATE` ([#290](https://github.com/linuxfoundation/lfx-mentorship/pull/290), Eng)
- [x] Test import into prod, in-cluster, off-hours, platform team warned: about 330k FGA and 330k index messages (Eng)
- [x] Import and outbox-drain time measured: the import takes minutes; the FGA outbox drains in about 4.6 hours at the throttled 20 messages/s (330k markers), by object type: applications (about 65 min), then programs (about 1 min), then tasks (about 3.5 h). OpenFGA already holds the test import's tuples under the same ids, so unchanged objects lose nothing while they wait; only objects changed since the test import do. Go-live waits for applications and programs, not tasks (Eng)
- [ ] Duplicate legacy identities resolved: 58 emails and 14 LFIDs are each shared by two legacy users; the importer keeps each on one user and clears it on the other, and a cleared LFID means no tuples, and none for a program where that user is an active admin or mentor. Lewis checks how many of the 72 hold data and has `user-fixes.sql` ready on Monday; the importer applies it through `USER_FIXES` (Eng)
- [ ] Data exceptions reviewed at Friday's readiness review: programs with no LF project or no admin, and pending mentor rows (Lewis brings the counts). The ~32k quarantined tasks stay in `quarantined_tasks` for go-live and are triaged after (Product, Eng)
- [x] Legacy-link forwarding in the frontend, live in prod, as in [lfx-crowdfunding#358](https://github.com/linuxfoundation/lfx-crowdfunding/pull/358): `/project/{id}[/…]` → `/programs/{id}`, `/mentee/{id}[,{projectId}]` → `/mentees/{id}`, `/mentor/{id}` → `/mentors/{id}`, other legacy pages → `/` ([#290](https://github.com/linuxfoundation/lfx-mentorship/pull/290), Eng)
- [x] Legacy links tested on prod with the legacy path on the new host (2026-10-21): all 197 program links in the [CNCF 2026 term docs](https://github.com/cncf/mentoring/tree/main/programs/lfx-mentorship) redirect to `/programs/{id}`; 192 open, and the other 5 are past-term programs hidden in legacy, which the new site does not show (Eng)
- [ ] CNCF `/lfx-url` bot accepts the new URL shape: [cncf/mentoring#2142](https://github.com/cncf/mentoring/pull/2142) accepts both shapes; Nate merges it before go-live, since CNCF is already posting next term's programs (CNCF)
- [x] Legacy API consumers found and handled: CNCF automation does not call it; legacy Crowdfunding's only caller was a fallback in `crowdfunding-cli`, and legacy Crowdfunding has been in maintenance since June (Eng)
- [x] Maintenance switch ready and tried on dev, both in prod with the flags off (frontend `v1.5.5`; jobspring `v1.5.28`, 2026-10-23; `v1.5.27` predates the switch), as Crowdfunding did ([lfx-crowdfunding-upgrade#48](https://github.com/linuxfoundation/lfx-crowdfunding-upgrade/pull/48)): a `maintenanceMode` flag in `lfx-mentorship-upgrade` that shows a "being upgraded" page ([#200](https://github.com/linuxfoundation/lfx-mentorship-upgrade/pull/200)), and the same flag in `jobspring` that answers 503 on the API (except `/health`) and pauses the cron jobs and the SQS consumer ([#2281](https://github.com/linuxfoundation/jobspring/pull/2281)) (Eng)
- [ ] Host redirect `mentorship.lfx.linuxfoundation.org` → `mentorship.linuxfoundation.org`, path and query kept ([requested](https://linuxfoundation.slack.com/archives/C02RMNFE9FF/p1792698887269559)). [lfx-v2-argocd#1906](https://github.com/linuxfoundation/lfx-v2-argocd/pull/1906) adds the Traefik redirect and merges before; [lfx-v2-opentofu#274](https://github.com/linuxfoundation/lfx-v2-opentofu/pull/274) points the DNS record at the cluster and is the redirect itself, merged in the window (DevOps)
- [x] Support entry point on the public site: a "Contact support" link that opens a popup sending the user to the chat in Self Serve (`app.lfx.dev/mentorship/mentee`); the Mentorship site itself does not load Intercom. Agreed with Support ([Heather, 2026-10-23](https://linuxfoundation.slack.com/archives/C07192MR02X/p1792699963761369)); ticket [lfx-self-serve#1533](https://github.com/linuxfoundation/lfx-self-serve/issues/1533) ; merged in [#366](https://github.com/linuxfoundation/lfx-mentorship/pull/366), ships with the release (Eng)
- [ ] fga-sync owners and DevOps told the date and start time of the drain: about 4.6 hours at 20 messages/s, half the shared consumer's capacity, which is the condition they [agreed to](https://linuxfoundation.slack.com/archives/C02RMNFE9FF/p1792133200158979) (Eng)
- [x] Email: every email is copied to one staff inbox per environment, `EMAIL_COPY_INBOX`, which is `lfx-mentorship-sent@` in prod ([#335](https://github.com/linuxfoundation/lfx-mentorship/pull/335), [lfx-v2-argocd#1860](https://github.com/linuxfoundation/lfx-v2-argocd/pull/1860), DevOps, Eng)
- [x] Approvers listed in prod values ([lfx-v2-argocd#1874](https://github.com/linuxfoundation/lfx-v2-argocd/pull/1874), merged)
- [ ] User docs on the public site at `mentorship.linuxfoundation.org/docs`, built like [Crowdfunding's](https://crowdfunding.linuxfoundation.org/docs) from markdown in this repo, and Support asked to point Fin AI at them once live ([lfx-self-serve#1534](https://github.com/linuxfoundation/lfx-self-serve/issues/1534)) (Eng, Product)
- [x] Program admins told in [#mentorship_admins](https://linuxfoundation.slack.com/archives/CT3NP3UDU/p1792695758113669) (2026-10-22), without the new address; it is shared once live (Product)

## Release day

Times are from the freeze (T); commands are in runbook 08. Go-live waits for the application and program permissions to drain, not the tasks: see "Import and outbox-drain time measured" above.

- [ ] **T+0 Freeze legacy** (Eng): announce downtime on Slack and the banner, deploy `lfx-mentorship-upgrade` and `jobspring` with `maintenanceMode` on (`jobspring` also `sqsConsumerEnabled: false`), wait 15 min for running cron jobs
- [ ] **T+0 Final prod release** (Eng), alongside the freeze: tag `main`, bump the backend and frontend in [lfx-v2-argocd](https://github.com/linuxfoundation/lfx-v2-argocd), sync. Done before the `TRUNCATE`, so the import runs on the final schema. Then check `/docs` and the legacy paths from [#367](https://github.com/linuxfoundation/lfx-mentorship/issues/367) on the new host
- [ ] **T+15m Migrate** (Eng), runbook Phases 2–4:
  - [ ] Rebuild the runner pod: SSO login, legacy secret, scripts ConfigMap, pod (10 min)
  - [ ] Copy the S3 delta (20 min)
  - [ ] `TRUNCATE` the `mentorship` schema
  - [ ] Import without `--test-run`, with `USER_FIXES` if needed (16 min); the FGA drain starts at commit
- [ ] **T+1h Approvers** (Eng): non-legacy approvers sign in to Self Serve, re-run the sync, check `approver-admin list`
- [ ] **T+1h Verify** (Eng): both verifiers pass, UI spot checks (Phase 5)
- [ ] **Go/no-go** (Eng, Product): Phase 6, step 1
- [ ] **T+1h → T+2h Drain applications and programs** (Eng): no pending `mentorship_application` or `mentorship_program` rows in `fga_outbox`, no `dead_letter`; watch the fga-sync backlog, backend errors, RDS CPU. Don't raise the rate: fga-sync's owners agreed on condition it stays throttled
- [ ] **T+2h Email to everyone** (Eng): remove `EMAIL_ALLOWED_RECIPIENTS`, wait for the rollout
- [ ] **Self Serve to everyone** (Eng): `mentorship-enabled` on for all users
- [ ] **Redirect the legacy host** (DevOps): merge the DNS PR. Its certificate is issued only once DNS points at the cluster, so re-test legacy links after it is ready, a few minutes later. Then announce "we're live", tell Support the docs are live for Fin AI, send CNCF the one-line PR
- [ ] **T+2h → T+5.5h Tasks drain** (Eng): the remaining task markers finish in the background; tasks created since the test import wait up to that long for their permissions, existing tasks already have theirs. Keep watching the fga-sync backlog and `dead_letter`

**Rollback:** clean until "Self Serve to everyone". After that, fix forward.

## After release

- [ ] First week: watch `service:lfx-mentorship-backend status:error` in Datadog Logs by hand. No monitor exists yet, and dev and prod log under the same service with no `env` tag, so a monitor would alert on dev too; ask DevOps for the tags and a prod error monitor (Eng, DevOps)
- [ ] Keep legacy in maintenance mode until decommission. It also keeps the legacy reminder cron off, so mentees don't get two reminders now that the new reminder CronJob runs in prod ([lfx-v2-argocd#1846](https://github.com/linuxfoundation/lfx-v2-argocd/pull/1846))
- [ ] Revert the FGA relay throttle once the import's FGA outbox has drained: in `values/prod/lfx-mentorship-backend.yaml`, remove `FGA_RELAY_BATCH_SIZE: "10"` (default 50) and `autoscaling.maxReplicas: 2` (default 10), set in [lfx-v2-argocd#1824](https://github.com/linuxfoundation/lfx-v2-argocd/pull/1824). It keeps the import's ~330k markers at 20 messages/s so the shared fga-sync consumer doesn't back up, but it also caps the API at 2 pods, which live traffic may need (Eng)
- [ ] Any time after release: lock the legacy uploads bucket, S3 Block Public Access on `jobspring-prod-uploads` in the legacy account (runbook Phase 6, step 2). Nothing writes to it once legacy is in maintenance mode, and it has been public for years, so it is not urgent (DevOps)
- [ ] Day after: Fivetran connector from the Mentorship Postgres database to Snowflake ([lfx-self-serve#1532](https://github.com/linuxfoundation/lfx-self-serve/issues/1532), DevOps), then the dbt models read it ([lfx-self-serve#1537](https://github.com/linuxfoundation/lfx-self-serve/issues/1537), Eng); check Crowdfunding's sync creates no duplicate initiatives
- [ ] Links we control point at the new host: LFX header, Crowdfunding footer and tools menu; they redirect until then
- [ ] About two weeks after release: remove the `mentorship-enabled` LaunchDarkly flag and the `mentorshipEnabledGuard` that reads it in lfx-self-serve. Once Mentorship is on for everyone the gate is no longer needed, and its readiness timeout sends users from Mentorship pages to the dashboard when LaunchDarkly starts without their identity ([lfx-self-serve#4179](https://github.com/linuxfoundation/lfx-self-serve/issues/4179)) (Eng)
- [ ] Remove the legacy Mentorship docs from [lfx-product-documentation](https://github.com/linuxfoundation/lfx-product-documentation/tree/main/mentorship) once Fin AI uses the new ones, as Crowdfunding did in [lfx-product-documentation#114](https://github.com/linuxfoundation/lfx-product-documentation/pull/114) (Eng)
- [ ] Legacy decommission per [03](./03-migration-plan.md) Phase 5
- [ ] Follow up with CNCF on automating program creation
- [ ] Automated E2E tests: a cross-service Playwright journey suite on dev and a sign-in-free prod smoke test in this repo, based on the [E2E test plan](https://github.com/linuxfoundation/lfx-mentorship/pull/356). Sameh's feature specs stay in `lfx-self-serve`; CI secrets need DevOps ([thread](https://linuxfoundation.slack.com/archives/C0BK7NQ3Q6Q/p1792783931212209)) (Eng)
- [ ] Close ended terms: nothing closes them since legacy's auto-close job, and 123 ended terms are still open in prod ([#369](https://github.com/linuxfoundation/lfx-mentorship/issues/369)). Mentorship admins first decide what happens to the graduated applications in those terms (Product, Eng)
