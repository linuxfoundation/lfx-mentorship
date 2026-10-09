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

## Phase 0: preflight

Every check must pass before anything runs.

1. **The legacy-session fix is merged** ([lfx-mentorship#286](https://github.com/linuxfoundation/lfx-mentorship/pull/286)). Without it, the scripts in a pod scan DynamoDB in `us-west-2`, and the copy fails across accounts. [#285](https://github.com/linuxfoundation/lfx-mentorship/pull/285) (resolving missing member `userId`s) is already merged.
2. **The prod backend is running the current image, with the schema migrated:**
   ```bash
   kubectl -n mentorship-backend get deploy,pods
   kubectl -n mentorship-backend logs job/$(kubectl -n mentorship-backend get jobs -o name | grep migrate | tail -1 | cut -d/ -f2) | tail -1
   # expect: migrations applied: schema version N (dirty=false)
   ```
3. **The secret has the keys the relays need.** This prints key names only, never values:
   ```bash
   kubectl -n mentorship-backend get secret lfx-mentorship-backend-secrets -o json | jq '.data | keys'
   # needs host, port, username, password, dbname, and INDEXER_SERVICE_TOKEN (without it the search-index relay idles)
   ```
4. **The prod database is empty, or you know what's in it.** The importer updates rows that exist but never deletes rows removed from legacy since an earlier import. Don't import into prod twice weeks apart (see Phase 1).
   ```sql
   SELECT (SELECT count(*) FROM mentorship.programs) programs, (SELECT count(*) FROM mentorship.users) users;
   ```
5. **Legacy credentials.** The operator can sign in to the legacy-account read-only role (`716487311010`). Use the longest session duration it allows. The file copy can take hours; if the credentials expire it stops, and is re-run.
6. **Decisions agreed:**
   - About 350 member rows stay unlinked, mostly mentor invites nobody accepted. They are logged as `UNRESOLVED_MEMBER_USER`.
   - About 1,760 pending mentor rows are not imported (`UNMAPPED_MENTOR_MEMBER`, per [03](./03-migration-plan.md) §ii).
   - About 32,000 tasks have no matching application and go to the `quarantined_tasks` table (see [Gaps](#gaps)).

## Phase 1: rehearsal (days before cutover)

Copy the files only. Copied files are reused by later runs, so the slow bulk copy happens outside the freeze.

Don't run the import against prod in the rehearsal:

- the import never deletes rows, so anything removed from legacy before cutover would stay;
- it pushes every program into prod permissions (OpenFGA) and search.

Do Phase 2 (runner pod) and Phase 3 (copy), then delete the pod. The import has already been tested end to end on prod data against a local Postgres; the results are under Phase 5.

## Phase 2: start the runner pod

The backend image has no Python, so use a throwaway pod with the backend's ServiceAccount. Its IAM role can write to the new buckets.

What runs next to the credentials is fixed in advance, and identical for the rehearsal and the cutover:

- the base image is pinned by digest;
- the scripts come from one recorded commit of `main`;
- the dependencies are the hash-locked wheels in `backend/db/scripts/requirements.txt`.

The dependencies install in an init container that never receives the legacy credentials, and as wheels only, so no package build code runs. The pod's IRSA token is injected into every container, including that one.

### 1. Put the legacy credentials in a temporary secret

On the operator's laptop, after signing in to the legacy read-only role:

```bash
aws configure export-credentials --profile <legacy-readonly-profile> --format env-no-export \
  | sed 's/^AWS_/LEGACY_AWS_/' > /tmp/legacy.env
kubectl -n mentorship-backend create secret generic mentorship-etl-legacy --from-env-file=/tmp/legacy.env
rm /tmp/legacy.env
```

They are named `LEGACY_AWS_*` on purpose. Plain `AWS_*` keys would replace the pod's own identity, and the writes to the new buckets would fail. They are also required: without them the scripts fall back to the default credential chain, which in the pod is the v2 role, and log a warning saying so.

### 2. Load the reviewed scripts

From a clean checkout of `main` that includes #286. Record the commit, and use the same one for the rehearsal and the cutover:

```bash
git rev-parse HEAD   # note this in the cutover log
kubectl -n mentorship-backend create configmap mentorship-etl-scripts \
  --from-file=backend/db/scripts/migrate_dynamo_to_postgres.py \
  --from-file=backend/db/scripts/copy_legacy_objects.py \
  --from-file=backend/db/scripts/legacy_objects.py \
  --from-file=backend/db/scripts/requirements.txt
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
EOF
```

**Stop if any line is wrong, or if the output includes the warning that legacy reads use the default credential chain.**

## Phase 3: copy the legacy files

```bash
python copy_legacy_objects.py 2>&1 | tee /work/copy.log
```

- It ends with per-column counts (`copied`, `quarantined`, `missing`, `foreign`) and `Wrote N manifest entries`.
- Every `QUARANTINED` or `MISSING` line is logged; keep the list for review.
- If the legacy credentials expire partway: refresh the secret (Phase 2, step 1), recreate the pod, and re-run. Files already copied are skipped.

## Phase 4: cutover run (inside the freeze)

1. **Freeze writes on legacy prod** (the content freeze in [03](./03-migration-plan.md) Phase 4). From here on, legacy is read-only.
2. **Re-run the copy** (Phase 3). It copies only files added since the rehearsal, and it writes the manifest the import needs.
3. **Import:**
   ```bash
   python migrate_dynamo_to_postgres.py 2>&1 | tee /work/import.log
   ```
   The import is all or nothing: if any part fails, nothing is saved. Fix the cause and re-run.
4. **If needed, link specific members by hand.** Write a CSV with the header `member_id,user_id` to `/work/overrides.csv`, then re-run step 3 with `MEMBER_USER_OVERRIDES=/work/overrides.csv`. Overrides only apply where automatic matching failed.

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
4. **Spot checks in the UI:**
   - a program admin can open and edit their own programs;
   - a published program's logo loads from the CDN;
   - a migrated task submission downloads;
   - a quarantined file shows "no file".

## Phase 6: go/no-go and go-live

The API has no read-only mode (see [Gaps](#gaps)), so the decision is made **before** the new stack gets public traffic. Once DNS points at it, users write to Postgres only, and rollback is no longer clean ([03](./03-migration-plan.md) Phase 4 and "Rollback").

1. **Go/no-go**, on the Phase 5 results.
   - **No-go:** unfreeze legacy and stop. Nothing has been exposed, and legacy still holds every write.
2. **Lock the legacy uploads bucket:** turn on S3 Block Public Access for `jobspring-prod-uploads` (legacy account). This can be reversed and nothing is deleted.
3. **Switch DNS:** point `mentorship.linuxfoundation.org` at the new frontend. From this moment writes are live on the new stack.
4. **Go-live:** in [lfx-v2-argocd](https://github.com/linuxfoundation/lfx-v2-argocd), remove `EMAIL_ALLOWED_RECIPIENTS` from `values/prod/lfx-mentorship-backend.yaml` so email reaches real users.
5. **Rollback:**
   - **Before step 3** it is clean: lift the bucket block if it was applied, and unfreeze legacy.
   - **After step 3** it means reverting DNS and losing any writes made on the new stack since, because no reverse sync exists. Past that point, fix forward.

## Phase 7: clean up

```bash
kubectl -n mentorship-backend delete pod mentorship-etl
kubectl -n mentorship-backend delete secret mentorship-etl-legacy
kubectl -n mentorship-backend delete configmap mentorship-etl-scripts
```

The manifest and logs contain user emails. Don't copy them out of the pod unless needed, and delete any copies afterwards.

## Gaps

Not built yet; decide before cutover.

1. **No read-only mode in the API.** [03](./03-migration-plan.md) Phase 4 assumes "writes rejected at the API" during verification, but the backend has nothing that does this. Before DNS points at the new stack nobody can reach it anyway; after the switch, writes are live immediately.
2. **No sweep for unused copied files** ([03](./03-migration-plan.md) §S3 objects, step 5a). Files copied in the rehearsal for rows later deleted or replaced stay in the new buckets. This is cleanup work, not a blocker.
3. **No invitation-token backfill** ([03](./03-migration-plan.md), "Invitation tokens"). Invite links sent from legacy before the freeze won't work on the new stack. The documented fallback is to stop sending invites when the freeze starts.
4. **About 32,000 quarantined tasks** (about 11%) have no matching application. They are kept in `quarantined_tasks`, not lost, but someone should decide whether that is acceptable for go-live.
