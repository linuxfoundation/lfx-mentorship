<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship Rewrite — 08: Production Data Migration Runbook

Step-by-step procedure for moving legacy jobspring data (DynamoDB + S3) into the Mentorship Postgres database in production. It implements Phase 4 of [03](./03-migration-plan.md) with the scripts in `backend/db/scripts/`.

## Where things are

| | |
|---|---|
| Target namespace | `mentorship-backend` in the LFX v2 prod cluster |
| ServiceAccount | `lfx-mentorship-backend` (IRSA `arn:aws:iam::372256339901:role/lfx-mentorship-backend`) |
| DB credentials | secret `lfx-mentorship-backend-secrets`, keys `host`, `port`, `username`, `password`, `dbname`; `sslmode=require`; tables in schema `mentorship` |
| New buckets (us-west-2) | `lfx-v2-mentorship-logos-public-prod`, `lfx-v2-mentorship-attachments-prod` |
| Logos CDN | `https://mentorship-logos-public.downloads.lfx.community` |
| NATS | `nats://lfx-platform-nats.lfx.svc.cluster.local:4222` |
| Legacy source | account `716487311010`, `us-east-1`: tables `jobspring-prod-*`, bucket `jobspring-prod-uploads` |
| Legacy hostname | `mentorship.lfx.linuxfoundation.org` (Cloudflare) |
| New hostname | `mentorship.linuxfoundation.org`, already served by the prod frontend and already resolving to the v2 cluster |

## Phase 0: preflight

Every check must pass before anything runs.

1. **The legacy-session fix is merged** ([lfx-mentorship#286](https://github.com/linuxfoundation/lfx-mentorship/pull/286)). Without it, the scripts in a pod scan DynamoDB in `us-west-2`, and the copy fails across accounts. [#285](https://github.com/linuxfoundation/lfx-mentorship/pull/285) (resolving missing member `userId`s) is already merged.
2. **The prod backend is running the current image:**
   ```bash
   kubectl -n mentorship-backend get deploy,pods
   ```
   Don't look for the migrate Job's logs: Helm deletes the Job once it succeeds. The schema version is checked from the database in the Phase 2 setup check.
3. **The secret has the keys the relays need.** This prints key names only, never values:
   ```bash
   kubectl -n mentorship-backend get secret lfx-mentorship-backend-secrets -o json | jq '.data | keys'
   # needs host, port, username, password, dbname, and INDEXER_SERVICE_TOKEN (without it the search-index relay idles)
   ```
4. **You know what's in the prod database.** The importer updates rows that exist but never deletes rows removed from legacy since an earlier import, so the final run starts from emptied tables (Phase 4, step 5). The row counts are printed by the Phase 2 setup check.
5. **Legacy credentials.** The operator can sign in to the legacy-account read-only role (`716487311010`). Use the longest session duration it allows. The file copy can take hours; if the credentials expire it stops, and is re-run.
6. **Decisions agreed:**
   - About 350 member rows stay unlinked, mostly mentor invites nobody accepted. They are logged as `UNRESOLVED_MEMBER_USER`.
   - About 1,760 pending mentor rows are not imported (`UNMAPPED_MENTOR_MEMBER`, per [03](./03-migration-plan.md) §ii).
   - About 32,000 tasks have no matching application and go to the `quarantined_tasks` table (see [Gaps](#gaps)).
   - **Whether the new site stays closed until go/no-go.** `mentorship.linuxfoundation.org` is already public, so the imported data is visible there as soon as the import finishes, and users can write to it before the decision. See [Phase 4](#phase-4-cutover-run-inside-the-freeze) step 1.

## Phase 1: test import into prod (days before cutover)

A full run in-cluster, with NATS, against prod. It does the slow bulk file copy outside the freeze, and gives real data to test the new site and the legacy links on. Everything it writes to the database is dropped before the final run (Phase 4, step 5); the copied files stay, since the copy skips objects that already exist.

The import queues about 330,000 permission (OpenFGA) and 330,000 search-index updates through fga-sync. Run it off-hours, and give the platform team a heads-up first.

1. Do Phase 2 (runner pod), Phase 3 (copy), Phase 4 step 6 (import) and Phase 5 (verify).
2. **Record the timings:** the copy, the import (from `import.log`; the smoke import took about 16 minutes, 14 of them scanning DynamoDB) and how long both outboxes take to drain (Phase 5, step 3). The final run is reserved from these.
3. **Count the duplicate users.** The import keeps a shared email or LFID on one user and clears it on the others, and a user left without one cannot use Mentorship. This lists how many there are, and how many of them hold memberships, applications or tasks; a handful are fixed by hand:
   ```bash
   python - <<'EOF'
   import collections, psycopg2, migrate_dynamo_to_postgres as m
   users = m.scan_table(m.legacy_session().client("dynamodb"), f"{m.TABLE_PREFIX}-users")
   cur = psycopg2.connect("").cursor()
   for column, field in (("email", "email"), ("lfid", "lfid")):
       holders = collections.defaultdict(list)
       for u in users:
           if (value := (u.get(field) or "").strip()) and (uid := m._as_uuid(u.get("id"))):
               holders[value].append(uid)
       shared = [uid for ids in holders.values() if len(ids) > 1 for uid in ids]
       cur.execute(f"""SELECT count(*),
           count(*) FILTER (WHERE EXISTS (SELECT 1 FROM program_members WHERE user_id = u.id)
                             OR EXISTS (SELECT 1 FROM applications WHERE user_id = u.id)
                             OR EXISTS (SELECT 1 FROM tasks WHERE assignee_id = u.id))
           FROM users u WHERE u.id = ANY(%s::uuid[]) AND u.{column} IS NULL""", (shared,))
       cleared, with_data = cur.fetchone()
       print(f"{column}: {sum(len(i) > 1 for i in holders.values())} shared values; cleared on {cleared} users, {with_data} of them with data")
   EOF
   ```
4. **Test the legacy links** on the new site with real ids, once the frontend release with the path mapping (Phase 6, step 4) is out: for example `https://mentorship.linuxfoundation.org/project/{id}` from a CNCF page must land on that program.
5. **Note the approvers.** Whoever is added to the approver team while testing (`POST /mentorship/v1/admin/approver-team/members`) must be added again after the final import (Phase 4, step 8). Keep the list: `GET /mentorship/v1/admin/approver-team/members`.
6. Delete the runner pod (Phase 7).

## Phase 2: start the runner pod

The backend image has no Python, so use a throwaway pod with the backend's ServiceAccount. Its IAM role can write to the new buckets.

What runs next to the credentials is fixed in advance, and identical for the test import and the cutover:

- the base image is pinned by digest;
- the scripts come from one recorded commit of `main`;
- the dependencies are the hash-locked wheels in `backend/db/scripts/requirements.txt`.

The dependencies install in an init container that never receives the legacy credentials, and as wheels only, so no package build code runs. The pod's IRSA token is injected into every container, including that one.

### 1. Put the legacy credentials in a temporary secret

On the operator's laptop, after signing in to the legacy read-only role:

```bash
aws configure export-credentials --profile <legacy-readonly-profile> --format env-no-export \
  | sed 's/^AWS_/LEGACY_AWS_/' > /tmp/legacy.env
kubectl -n mentorship-backend create secret generic mentorship-etl-legacy --from-env-file=/tmp/legacy.env \
  --dry-run=client -o yaml | kubectl apply -f -
rm /tmp/legacy.env
```

The same command refreshes expired credentials. A running pod keeps the values it started with, so recreate the pod afterwards (step 4).

They are named `LEGACY_AWS_*` on purpose. Plain `AWS_*` keys would replace the pod's own identity, and the writes to the new buckets would fail. They are also required: without them the scripts fall back to the default credential chain, which in the pod is the v2 role, and log a warning saying so.

### 2. Load the reviewed scripts

From a clean checkout of `main` that includes #286. Record the commit, and use the same one for the test import and the cutover:

```bash
git rev-parse HEAD   # note this in the cutover log
kubectl -n mentorship-backend create configmap mentorship-etl-scripts \
  --from-file=backend/db/scripts/migrate_dynamo_to_postgres.py \
  --from-file=backend/db/scripts/copy_legacy_objects.py \
  --from-file=backend/db/scripts/legacy_objects.py \
  --from-file=backend/db/scripts/verify_migration.py \
  --from-file=backend/db/scripts/verify_objects.py \
  --from-file=backend/db/scripts/retract_dropped.py \
  --from-file=backend/db/scripts/requirements.txt \
  --dry-run=client -o yaml | kubectl apply -f -
```

### 3. Create the pod

```yaml
# mentorship-etl.yaml
apiVersion: v1
kind: Pod
metadata:
  name: mentorship-etl
  namespace: mentorship-backend
spec:
  serviceAccountName: lfx-mentorship-backend
  automountServiceAccountToken: true
  restartPolicy: Never
  securityContext: {runAsNonRoot: true, runAsUser: 65532, runAsGroup: 65532, fsGroup: 65532, seccompProfile: {type: RuntimeDefault}}
  initContainers:
    # No legacy credentials here: installs only the hash-locked wheels.
    - name: deps
      image: python:3.12-slim@sha256:05cda9777409a9c3ffddd94a4c476b79f0769a0b4857f0c7ed9226b6800b0d6f
      command: ["sh", "-c", "python -m venv /work/venv && /work/venv/bin/pip install --no-cache-dir --no-deps --require-hashes --only-binary=:all: -r /scripts/requirements.txt"]
      env: [{name: HOME, value: /work}]
      securityContext: {allowPrivilegeEscalation: false, capabilities: {drop: [ALL]}}
      volumeMounts: [{name: work, mountPath: /work}, {name: scripts, mountPath: /scripts, readOnly: true}]
  containers:
    - name: etl
      image: python:3.12-slim@sha256:05cda9777409a9c3ffddd94a4c476b79f0769a0b4857f0c7ed9226b6800b0d6f
      command: ["sleep", "infinity"]
      workingDir: /scripts
      securityContext: {allowPrivilegeEscalation: false, capabilities: {drop: [ALL]}}
      resources: {requests: {cpu: "1", memory: 2Gi}, limits: {memory: 6Gi}}
      envFrom:
        - secretRef: {name: mentorship-etl-legacy}
      env:
        - {name: HOME, value: /work}
        - {name: PATH, value: "/work/venv/bin:/usr/local/bin:/usr/bin:/bin"}
        - {name: PYTHONDONTWRITEBYTECODE, value: "1"}
        - {name: PG_DSN, value: ""}          # empty on purpose: connection comes from the PG* vars below
        - {name: PGHOST, valueFrom: {secretKeyRef: {name: lfx-mentorship-backend-secrets, key: host}}}
        - {name: PGPORT, valueFrom: {secretKeyRef: {name: lfx-mentorship-backend-secrets, key: port}}}
        - {name: PGUSER, valueFrom: {secretKeyRef: {name: lfx-mentorship-backend-secrets, key: username}}}
        - {name: PGPASSWORD, valueFrom: {secretKeyRef: {name: lfx-mentorship-backend-secrets, key: password}}}
        - {name: PGDATABASE, valueFrom: {secretKeyRef: {name: lfx-mentorship-backend-secrets, key: dbname}}}
        - {name: PGSSLMODE, value: require}
        - {name: PGOPTIONS, value: "-c search_path=mentorship,public"}
        - {name: DYNAMODB_TABLE_PREFIX, value: jobspring-prod}
        - {name: LOGOS_S3_BUCKET, value: lfx-v2-mentorship-logos-public-prod}
        - {name: ATTACHMENTS_S3_BUCKET, value: lfx-v2-mentorship-attachments-prod}
        - {name: LOGOS_CDN_URL_PREFIX, value: https://mentorship-logos-public.downloads.lfx.community}
        - {name: NATS_URL, value: nats://lfx-platform-nats.lfx.svc.cluster.local:4222}
        - {name: COPY_MANIFEST, value: /work/legacy-object-manifest.json}
      volumeMounts: [{name: work, mountPath: /work}, {name: scripts, mountPath: /scripts, readOnly: true}]
  volumes:
    - {name: work, emptyDir: {}}
    - {name: scripts, configMap: {name: mentorship-etl-scripts}}
```

`PG_DSN` must be present and empty: if it is missing, the importer connects to localhost instead.

### 4. Check the setup

```bash
kubectl -n mentorship-backend delete pod mentorship-etl --ignore-not-found --wait
kubectl apply -f mentorship-etl.yaml
kubectl -n mentorship-backend wait --for=condition=Ready pod/mentorship-etl --timeout=300s
kubectl -n mentorship-backend exec -it mentorship-etl -- bash
```

Inside the pod (working directory `/scripts`):

```bash
python - <<'EOF'
import boto3, psycopg2, migrate_dynamo_to_postgres as m
s = m.legacy_session()
print("legacy:", s.client("sts").get_caller_identity()["Account"], s.region_name)            # 716487311010 us-east-1
print("v2:    ", boto3.client("sts").get_caller_identity()["Arn"])                            # .../lfx-mentorship-backend/...
print("table: ", s.client("dynamodb").describe_table(TableName="jobspring-prod-project-members")["Table"]["TableStatus"])
c = psycopg2.connect(""); cur = c.cursor(); cur.execute("show search_path"); print("pg:    ", cur.fetchone())
cur.execute("SELECT version, dirty FROM public.schema_migrations"); print("schema:", cur.fetchone())   # (1, False)
cur.execute("SELECT (SELECT count(*) FROM programs), (SELECT count(*) FROM users)"); print("rows:  ", cur.fetchone())
EOF
```

**Stop if any line is wrong, or if the output includes the warning that legacy reads use the default credential chain.** `schema` must be the highest version under `backend/db/migrations/` with `dirty` false. `rows` is `(0, 0)` unless you know why it isn't (Phase 0, check 4).

## Phase 3: copy the legacy files

```bash
python copy_legacy_objects.py 2>&1 | tee /work/copy.log
```

- It ends with per-column counts (`copied`, `quarantined`, `missing`, `foreign`) and `Wrote N manifest entries`.
- Every `QUARANTINED` or `MISSING` line is logged; keep the list for review.
- If the legacy credentials expire partway: refresh the secret (Phase 2, step 1), recreate the pod and re-run the setup check (Phase 2, step 4), then re-run the copy. The pod reads the secret only when it starts, and `/work` is lost with it, so expect the copy to start over; files already copied are skipped.

## Phase 4: cutover run (inside the freeze)

1. **If agreed in Phase 0, close the new site.** Merge an [lfx-v2-argocd](https://github.com/linuxfoundation/lfx-v2-argocd) change to `values/prod/lfx-mentorship-frontend.yaml` that turns off its ingress, and confirm `https://mentorship.linuxfoundation.org` no longer serves the app. Scaling with `kubectl` doesn't work: ArgoCD self-heal reverts it. This hides the site only; the backend API keeps its own route.
2. **Freeze writes on legacy prod** (the content freeze in [03](./03-migration-plan.md) Phase 4). From here on, legacy is read-only.
3. **Rebuild the runner.** The test-import pod is gone and its credentials have expired. Using the commit recorded in the test import: refresh the secret (Phase 2, step 1), check out that commit and re-apply the ConfigMap (step 2), recreate the pod and run the setup check (steps 3 and 4).
4. **Re-run the copy** (Phase 3). It copies only files added since the test import, and it writes the manifest the import needs.
5. **Empty the tables.** This drops the test import and anything written on the new site since. First record what was published, so whatever doesn't come back can be retracted in step 9 (run steps 5 to 9 in the same pod: `/work` is lost with it):
   ```bash
   python retract_dropped.py snapshot /work/published.json
   ```
   Then truncate every table in the `mentorship` schema, the outboxes included, leaving `public.schema_migrations` alone:
   ```bash
   python - <<'EOF'
   import psycopg2
   c = psycopg2.connect(""); cur = c.cursor()
   cur.execute("SELECT string_agg(format('%I.%I', schemaname, tablename), ', ') FROM pg_tables WHERE schemaname = 'mentorship'")
   tables = cur.fetchone()[0]; print("truncating:", tables)
   cur.execute(f"TRUNCATE {tables} RESTART IDENTITY CASCADE"); c.commit()
   cur.execute("SELECT version, dirty FROM public.schema_migrations"); print("schema:", cur.fetchone())
   EOF
   ```
6. **Import:**
   ```bash
   python migrate_dynamo_to_postgres.py 2>&1 | tee /work/import.log
   ```
   The import is all or nothing: if any part fails, nothing is saved. Fix the cause and re-run.
7. **If needed, link specific members by hand.** Write a CSV with the header `member_id,user_id` to `/work/overrides.csv`, then re-run step 6 with `MEMBER_USER_OVERRIDES=/work/overrides.csv`. Overrides only apply where automatic matching failed.
8. **Re-add the approvers.** Step 5 also emptied `mentorship_approver_team_members`, which only the admin API fills and the import never recreates. Add the same people as during the test import (Phase 1, step 5), each with `POST /mentorship/v1/admin/approver-team/members` and `{"user_id": "..."}`. Until then nobody can approve new programs.
9. **Retract what didn't come back.** The truncate does not reach OpenFGA or the search index, so a program, application or task from step 5's snapshot that the import didn't bring back (test data, anything deleted in legacy since) would stay searchable to whoever its old tuples authorize, and an approver not re-added would keep approving. This queues `delete_access` and an index `deleted` for each of those rows, and a membership removal for each approver missing now:
   ```bash
   python retract_dropped.py retract /work/published.json
   ```
   The relays send them on; Phase 5, step 3 checks that the outboxes drain.

## Phase 5: verify (before go/no-go)

1. **`import.log`** ends with `Migration complete.`. Check these lines:
   - `Member rows without userId: 1876 (… email_unique=1523, email_and_program_lfid=1 …)` means about 1,524 rows were linked;
   - file-rewrite counts: `missing` should be roughly zero for each column;
   - the `UNRESOLVED_MEMBER_USER`, `UNMAPPED_PROGRAM_PROJECT_MAPPING` and `UNMAPPED_MENTOR_MEMBER` reports.
2. **Row counts.** Reference numbers from a smoke import of prod data on 2026-10-09; small growth since then is expected:

   | table | smoke |
   |---|---|
   | users | 91,791 |
   | user_profiles | 39,393 |
   | programs | 1,553 |
   | program_terms | 1,687 |
   | program_members | 4,372 (program_admin 1,503, mentor 2,869) |
   | applications | 76,003 |
   | tasks / quarantined_tasks | 252,899 / 32,352 |

   ```sql
   SET search_path = mentorship;
   SELECT member_type, status, count(*) FROM program_members GROUP BY 1,2 ORDER BY 1,2;
   -- Most programs should be mapped in-cluster (the smoke run had no NATS, so all were unmapped there).
   SELECT count(*) FILTER (WHERE lf_project_uid IS NULL) unmapped, count(*) FROM programs;
   ```
3. **Permissions and search updates are sent out.** The running API sends them on to OpenFGA and the indexer:
   ```sql
   SELECT 'fga', state, count(*) FROM fga_outbox GROUP BY 2
   UNION ALL SELECT 'index', state, count(*) FROM index_outbox GROUP BY 2;
   ```
   - FGA rows should drain to nothing, with no `dead_letter`.
   - Index rows should become `sent`, with no `dead_letter`.
   - Dead letters are repaired with `backend/cmd/outbox-repair`.
4. **Field-level reconciliation and integrity.** Row counts can hide a shifted or mis-transformed field ([03](./03-migration-plan.md) Phase 4 requires field-level checks). Run, in the runner pod with the same environment as the import:
   ```bash
   VERIFY_SAMPLE_SIZE=500 python verify_migration.py 2>&1 | tee /work/verify.log
   ```
   - It samples up to 500 items from each legacy table and compares each one's fields with the imported row: identities, names, emails, LFIDs, statuses, parent links, dates and flags. It follows the importer's skip, deduplication and quarantine rules, so a row the import should have skipped counts as an error if it is present.
   - It then runs integrity queries the schema cannot enforce: denormalised term and status copies that disagree with their parents, a task that is both live and quarantined, and more than one live application per term and user. Parent links that Postgres enforces (task to application, application to term) are rechecked too.
   - It must end with `Verification passed: no mismatches`. Any `MISMATCH` line is a no-go until it is explained. The log prints `VERIFY_SEED`; rerun with it to reproduce the same sample.
5. **Object reconciliation** ([03](./03-migration-plan.md) §S3 objects, step 5 (b) and (c)). The copy's manifest says what was copied, not that it is still there and intact. Run, in the same pod, before the legacy bucket is locked:
   ```bash
   python verify_objects.py 2>&1 | tee /work/verify-objects.log
   ```
   - Every file column that is not `NULL` must resolve in its bucket: task files by key, logos and avatars by their CDN URL. Foreign avatar and logo URLs are skipped.
   - Each of those objects must have the same ETag as its legacy source. Quarantined objects are compared by size, since a large one is uploaded in parts and has a different kind of ETag.
   - It must end with `Object verification passed: no mismatches`. Any `MISMATCH` line is a no-go until it is explained.
6. **Spot checks in the UI:**
   - a program admin can open and edit their own programs;
   - a published program's logo loads from the CDN;
   - a migrated task submission downloads;
   - a quarantined file shows "no file".

## Phase 6: go/no-go and go-live

The API has no read-only mode (see [Gaps](#gaps)), so the decision is made **before** legacy users are sent to the new stack. Once the legacy hostname redirects, users write to Postgres only, and rollback is no longer clean ([03](./03-migration-plan.md) Phase 4 and "Rollback").

1. **Go/no-go**, on the Phase 5 results. **Go** requires both `verify_migration.py` and `verify_objects.py` to pass (Phase 5, steps 4 and 5), and the approvers to be back (Phase 4, step 8) and the dropped rows retracted (step 9).
   - **No-go:** unfreeze legacy and stop. Legacy still holds every write. If the new site was left open, the imported data was visible on it, and any writes made there are lost: the next attempt starts again at Phase 4, which empties the tables.
2. **Lock the legacy uploads bucket:** turn on S3 Block Public Access for `jobspring-prod-uploads` (legacy account). This can be reversed and nothing is deleted.
3. **Let email reach real users**, before anyone is sent to the new site. Until this step, every notification to an address not on `EMAIL_ALLOWED_RECIPIENTS` is dropped, not queued, so a real write made before it rolls out would lose its email for good.
   - In [lfx-v2-argocd](https://github.com/linuxfoundation/lfx-v2-argocd), remove `EMAIL_ALLOWED_RECIPIENTS` from `values/prod/lfx-mentorship-backend.yaml` and merge.
   - Wait for the rollout: `kubectl -n mentorship-backend rollout status deploy/lfx-mentorship-backend`.
   - Confirm no running pod logs `email restricted to EMAIL_ALLOWED_RECIPIENTS` at startup.
4. **Send legacy users to the new site.** If it was closed in Phase 4, re-enable the frontend ingress first. Then, in Cloudflare, set a permanent redirect from `mentorship.lfx.linuxfoundation.org` to `https://mentorship.linuxfoundation.org`, keeping the path and query ([03](./03-migration-plan.md) OQ-3). IT owns the Cloudflare change; line it up with them ahead of release day. The new hostname already points at the v2 cluster; it needs no DNS change. From this moment writes are live on the new stack.

   Cloudflare only swaps the host. The frontend maps the legacy paths (`frontend/server/middleware/legacy-redirects.ts`, tested on prod in Phase 1, step 4), with a 301 on `GET` and `HEAD`:
   - `/project/{id}[/...]` → `/programs/{id}`, query kept;
   - `/mentee/{id}[,{projectId}]` → `/mentees/{id}`, query kept;
   - `/mentor/{id}` → `/mentors/{id}`, query kept;
   - `/project/applied`, `/mentee/applications`, `/mentee/tasks`, `/participate/**`, `/profile/**`, `/email/**` → `/`, query dropped.

   Ids match on both sides: program UUIDs are kept, and both sites key mentees and mentors by user id. Project sites and docs (CNCF alone has dozens) link to `/project/{id}`, so this mapping must be live before the redirect.
5. **Rollback:**
   - **Before step 4** it is clean: restore the allowlist if step 3 ran, lift the bucket block if it was applied, and unfreeze legacy.
   - **After step 4** it means removing the redirect and losing any writes made on the new stack since, because no reverse sync exists. Past that point, fix forward.

## Phase 7: clean up

```bash
kubectl -n mentorship-backend delete pod mentorship-etl
kubectl -n mentorship-backend delete secret mentorship-etl-legacy
kubectl -n mentorship-backend delete configmap mentorship-etl-scripts
```

The manifest and logs contain user emails. Don't copy them out of the pod unless needed, and delete any copies afterwards.

## Gaps

Not built yet; decide before cutover.

1. **No read-only mode in the API.** [03](./03-migration-plan.md) Phase 4 assumes "writes rejected at the API" during verification, but the backend has nothing that does this. `mentorship.linuxfoundation.org` is already public, so the only way to keep users out during verification is to close the frontend (Phase 4, step 1), and even then the API stays reachable. After the legacy redirect, writes are live immediately.
2. **No sweep for unused copied files** ([03](./03-migration-plan.md) §S3 objects, step 5a). Files copied in the test import for rows later deleted or replaced stay in the new buckets. This is cleanup work, not a blocker.
3. **No invitation-token backfill** ([03](./03-migration-plan.md), "Invitation tokens"). Invite links sent from legacy before the freeze won't work on the new stack. The documented fallback is to stop sending invites when the freeze starts.
4. **About 32,000 quarantined tasks** (about 11%) have no matching application. They are kept in `quarantined_tasks`, not lost, but someone should decide whether that is acceptable for go-live.
