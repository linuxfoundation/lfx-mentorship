<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship Rewrite — 09: Release Plan

Status: Draft. Target release: **2026-10-26**, time TBD.
Related: [08 runbook](./08-prod-migration-runbook.md) holds the commands; this page holds the order and the owners.

## Approach

1. **Test on prod data now.** Import legacy prod data into the new prod and test there while legacy keeps serving users. Email reaches only `EMAIL_ALLOWED_RECIPIENTS`, and Self Serve shows Mentorship only to testers (LaunchDarkly `mentorship-enabled`).
2. **Release with a few hours of downtime.** Close legacy, wipe the test data, run a fresh full import, verify, then send everyone to the new site.

## Before release

- [ ] Runbook 08 updated: Phase 1 and the no-go step describe the test import and the `TRUNCATE` (Eng)
- [ ] Test import into prod, in-cluster, off-hours, platform team warned: about 330k FGA and 330k index messages (Eng)
- [ ] Import and outbox-drain time measured; it sets the downtime window (Eng)
- [ ] Legacy-link forwarding in the frontend, as in [lfx-crowdfunding#358](https://github.com/linuxfoundation/lfx-crowdfunding/pull/358): `/project/{id}[/…]` → `/programs/{id}`, `/mentee/{id}[,{projectId}]` → `/mentees/{id}`, `/mentor/{id}` → `/mentors/{id}`, other legacy pages → `/` (Eng)
- [ ] Legacy links tested on prod with the legacy path on the new host, including every program link in the [CNCF term docs](https://github.com/cncf/mentoring/tree/main/programs/lfx-mentorship) (Eng)
- [ ] CNCF `/lfx-url` bot accepts the new URL shape: PR to `cncf/mentoring` accepting both shapes (Eng, CNCF review)
- [ ] Legacy API consumers known before it is switched off: CNCF automation, and legacy Crowdfunding (`LFF`), which calls `/users/external/{lfid}` (Eng)
- [ ] Host redirect prepared: `mentorship.lfx.linuxfoundation.org` → `mentorship.linuxfoundation.org`, path kept (owner TBD: DNS is in Cloudflare)
- [ ] Email: `lfx-mentorship-sent@` group exists and `EMAIL_HR_INBOX` points at it; copy-every-email feature built (DevOps, Eng)
- [ ] Release tagged and pinned in [lfx-v2-argocd](https://github.com/linuxfoundation/lfx-v2-argocd) (Eng)
- [ ] Downtime announced: legacy notice text, CNCF and other program admins (Product)

## Release day

1. **Freeze legacy:** notice page on the legacy site, legacy API writes off.
2. **Wipe test data:** `TRUNCATE` every table in the `mentorship` schema, outboxes included. Leave `public.schema_migrations` alone.
3. **Copy, import, verify:** runbook Phases 2–5. Both verifiers must pass.
4. **Re-add program approvers** through `POST /admin/approver-team/members`; the import does not restore them.
5. **Go/no-go:** runbook Phase 6, step 1.
6. **Lock the legacy uploads bucket:** runbook Phase 6, step 2.
7. **Email to everyone:** remove `EMAIL_ALLOWED_RECIPIENTS` and wait for the rollout, runbook Phase 6, step 3.
8. **Self Serve to everyone:** turn `mentorship-enabled` on for all users.
9. **Redirect the legacy host**, then re-run the legacy-link test.

**Rollback:** clean until step 9 (unfreeze legacy, restore the allowlist and flag, lift the bucket block). After step 9, fix forward.

## After release

- [ ] Snowflake reads Postgres: Fivetran connector and dbt models ([lfx-self-serve#1532](https://github.com/linuxfoundation/lfx-self-serve/issues/1532), [#1537](https://github.com/linuxfoundation/lfx-self-serve/issues/1537)), within 2–3 days; check Crowdfunding's sync creates no duplicate initiatives
- [ ] Legacy decommission per [03](./03-migration-plan.md) Phase 5
- [ ] Follow up with CNCF on automating program creation
