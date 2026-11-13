-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

-- One-time repair of users stored under an identity provider subject
-- (users.lfid = 'auth0|<username>') instead of their LF username, tracked in
-- linuxfoundation/lfx-mentorship#443. It is safe to re-run: rows created before
-- the guard (linuxfoundation/lfx-mentorship#446) is deployed are repaired by
-- the next run.
--
-- Dry run, the default. It prints each step's row count and rolls back:
--
--   psql "$PROD_DSN" -v ON_ERROR_STOP=1 -f backend/db/scripts/repair_auth0_prefixed_users.sql
--
-- Apply:
--
--   psql "$PROD_DSN" -v ON_ERROR_STOP=1 -v apply=1 -f backend/db/scripts/repair_auth0_prefixed_users.sql
--
-- In one transaction:
--   * A row with no twin under the real username is renamed.
--   * A row with a twin is merged into it: its applications, tasks, program
--     memberships and profiles move to the twin and the row is deleted. The
--     twin keeps its own email, name and profiles.
--   * Where both rows hold an application for the same term, the twin's is the
--     newer one and the one the mentee sees, so it is kept as it is, decided
--     or not. The older one, still pending, becomes withdrawn and stays as
--     history.
--   * Every application, task and program whose FGA tuples name an affected
--     user gets an outbox marker, so the relay republishes its access from the
--     repaired rows. update_access is a full sync, so fga-sync drops the
--     user:auth0|<username> tuples. Moved applications and tasks also get a
--     fresh search document.
--
-- Plain SQL on purpose: withdrawing through the API would email the mentees.
-- Data the rules above do not cover raises an exception and nothing is changed.

BEGIN;
SET LOCAL search_path TO mentorship;

-- The API keeps serving while this runs, so the tables are locked first, as in
-- migration 003: a decision or a new application committed between the
-- snapshots below and the changes would otherwise be overwritten, or moved
-- without its marker. Writes and row locks wait until the end; reads continue.
-- The timeout keeps this from queueing the API behind a long transaction.
SET LOCAL lock_timeout = '5s';
LOCK TABLE applications, tasks, program_members, user_profiles, users IN EXCLUSIVE MODE;

\echo 'affected users:'
CREATE TEMP TABLE repair_users ON COMMIT DROP AS
SELECT old.id AS old_id, substr(old.lfid, 7) AS lfid, twin.id AS twin_id
FROM users old
LEFT JOIN users twin ON twin.lfid = substr(old.lfid, 7)
WHERE old.lfid LIKE 'auth0|%';

-- merged marks what changes owner; the rest only needs its FGA tuple rewritten.
\echo 'applications on them:'
CREATE TEMP TABLE repair_applications ON COMMIT DROP AS
SELECT a.id, r.twin_id IS NOT NULL AS merged
FROM applications a
JOIN repair_users r ON r.old_id = a.user_id;

\echo 'tasks assigned to them:'
CREATE TEMP TABLE repair_tasks ON COMMIT DROP AS
SELECT t.id, r.twin_id IS NOT NULL AS merged
FROM tasks t
JOIN repair_users r ON r.old_id = t.assignee_id;

-- Only an approved membership has an FGA tuple.
\echo 'programs with an approved member among them:'
CREATE TEMP TABLE repair_programs ON COMMIT DROP AS
SELECT DISTINCT m.program_id AS id
FROM program_members m
JOIN repair_users r ON r.old_id = m.user_id
WHERE m.status = 'approved';

\echo 'applications the twin also holds for the same term:'
CREATE TEMP TABLE repair_duplicates ON COMMIT DROP AS
SELECT o.id, o.status, o.created_on < n.created_on AS older
FROM applications o
JOIN repair_users r ON r.old_id = o.user_id
JOIN applications n ON n.user_id = r.twin_id AND n.program_term_id = o.program_term_id AND n.role = o.role
WHERE o.status <> 'withdrawn' AND n.status <> 'withdrawn';

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM repair_users WHERE lfid = '') THEN
    RAISE EXCEPTION 'a row has no username after the auth0| prefix; nothing was changed';
  END IF;
  IF EXISTS (SELECT 1 FROM repair_duplicates WHERE status <> 'pending' OR NOT older) THEN
    RAISE EXCEPTION 'a duplicate application is not pending, or is newer than the twin''s; decide it by hand; nothing was changed';
  END IF;
  IF EXISTS (SELECT 1 FROM mentorship_approver_team_members m JOIN repair_users r ON r.old_id = m.user_id) THEN
    RAISE EXCEPTION 'an approver team member is affected, which this script does not cover; nothing was changed';
  END IF;
  IF EXISTS (SELECT 1 FROM programs WHERE lfid LIKE 'auth0|%') OR EXISTS (SELECT 1 FROM tasks WHERE created_by LIKE 'auth0|%') THEN
    RAISE EXCEPTION 'programs.lfid or tasks.created_by holds an auth0| value, which this script does not cover; nothing was changed';
  END IF;
END $$;

-- The older duplicate becomes history, as a withdrawal through the API leaves it.
\echo 'duplicate applications withdrawn:'
UPDATE applications SET status = 'withdrawn' WHERE id IN (SELECT id FROM repair_duplicates);
\echo 'their tasks:'
UPDATE tasks SET application_status = 'withdrawn' WHERE application_id IN (SELECT id FROM repair_duplicates);

-- Merge: everything on a row with a twin moves to the twin.
\echo 'applications moved to the twin:'
UPDATE applications a SET user_id = r.twin_id
FROM repair_users r WHERE a.user_id = r.old_id AND r.twin_id IS NOT NULL;
\echo 'tasks moved to the twin (assignee, then owner):'
UPDATE tasks t SET assignee_id = r.twin_id
FROM repair_users r WHERE t.assignee_id = r.old_id AND r.twin_id IS NOT NULL;
UPDATE tasks t SET owner_id = r.twin_id
FROM repair_users r WHERE t.owner_id = r.old_id AND r.twin_id IS NOT NULL;
\echo 'program memberships moved to the twin:'
UPDATE program_members m SET user_id = r.twin_id
FROM repair_users r WHERE m.user_id = r.old_id AND r.twin_id IS NOT NULL;
-- A profile moves only where the twin has none of that type. Otherwise the
-- twin's, which the user sees and edits, is kept and the other is removed.
\echo 'profiles moved to the twin:'
UPDATE user_profiles p SET user_id = r.twin_id
FROM repair_users r
WHERE p.user_id = r.old_id AND r.twin_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM user_profiles q WHERE q.user_id = r.twin_id AND q.profile_type = p.profile_type);
\echo 'profiles removed, the twin has its own:'
DELETE FROM user_profiles p
USING repair_users r WHERE p.user_id = r.old_id AND r.twin_id IS NOT NULL;
\echo 'users merged into their twin:'
DELETE FROM users u
USING repair_users r WHERE u.id = r.old_id AND r.twin_id IS NOT NULL;

\echo 'users renamed:'
UPDATE users u SET lfid = r.lfid
FROM repair_users r WHERE u.id = r.old_id AND r.twin_id IS NULL;

-- The same upsert the API uses to enqueue an object marker.
\echo 'FGA markers enqueued:'
INSERT INTO fga_outbox (marker_kind, object_type, object_uid, desired_operation)
SELECT 'object', 'mentorship_application', id::text, 'update_access' FROM repair_applications
UNION ALL
SELECT 'object', 'mentorship_task', id::text, 'update_access' FROM repair_tasks
UNION ALL
SELECT 'object', 'mentorship_program', id::text, 'update_access' FROM repair_programs
ON CONFLICT (object_type, object_uid) WHERE marker_kind = 'object'
DO UPDATE SET
  desired_operation = EXCLUDED.desired_operation,
  generation = fga_outbox.generation + 1,
  state = CASE WHEN fga_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END,
  claimed_generation = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_generation ELSE NULL END,
  claimed_at = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_at ELSE NULL END,
  attempts = 0,
  next_attempt_at = NOW(),
  last_error = NULL,
  updated_on = NOW();

-- A moved application's document carries user_id, a moved task's assignee_id.
-- The documents match those the API writes (application_task_index.go).
\echo 'search documents enqueued:'
INSERT INTO index_outbox (object_type, object_uid, action, headers, data, indexing_config)
SELECT
  'mentorship_application',
  a.id,
  'updated',
  '{}'::jsonb,
  jsonb_strip_nulls(jsonb_build_object(
    'id', a.id,
    'program_term_id', a.program_term_id,
    'user_id', a.user_id,
    'role', a.role,
    'status', a.status,
    'program_term_status', a.program_term_status,
    'start_date_time', a.start_date_time,
    'end_date_time', a.end_date_time,
    'attendance_type', a.attendance_type,
    'tasks_submitted', a.tasks_submitted,
    'created_on', a.created_on,
    'updated_on', a.updated_on
  )),
  jsonb_build_object(
    'object_id', a.id,
    'access_check_object', 'mentorship_application:' || a.id::text,
    'access_check_relation', 'auditor',
    'history_check_object', 'mentorship_program:' || pt.program_id::text,
    'history_check_relation', 'auditor',
    'parent_refs', jsonb_build_array('mentorship_program:' || pt.program_id::text),
    'sort_name', a.user_id::text,
    'name_and_aliases', jsonb_build_array(a.user_id::text, a.role),
    'public', false,
    'tags', jsonb_build_array('role:' || a.role, 'status:' || a.status)
  )
FROM applications a
JOIN repair_applications r ON r.id = a.id AND r.merged
JOIN program_terms pt ON pt.id = a.program_term_id
UNION ALL
SELECT
  'mentorship_task',
  t.id,
  'updated',
  '{}'::jsonb,
  jsonb_strip_nulls(jsonb_build_object(
    'id', t.id,
    'application_id', t.application_id,
    'assignee_id', t.assignee_id,
    'name', t.name,
    'description', t.description,
    'category', t.category,
    'prerequisite', COALESCE(t.category = 'prerequisite', false),
    'status', t.status,
    'application_status', t.application_status,
    'program_term_status', t.program_term_status,
    'custom', t.custom,
    'submit_file', t.submit_file,
    'has_file', COALESCE(t.file, '') <> '',
    'due_date', t.due_date,
    'created_on', t.created_on,
    'updated_on', t.updated_on
  )),
  jsonb_build_object(
    'object_id', t.id,
    'access_check_object', 'mentorship_task:' || t.id::text,
    'access_check_relation', 'auditor',
    'history_check_object', 'mentorship_application:' || t.application_id::text,
    'history_check_relation', 'auditor',
    'parent_refs', jsonb_build_array('mentorship_application:' || t.application_id::text),
    'sort_name', COALESCE(t.name, ''),
    'name_and_aliases', jsonb_build_array(COALESCE(t.name, ''), COALESCE(t.category, '')),
    'public', false,
    'tags', jsonb_build_array(
      'status:' || t.status,
      'category:' || COALESCE(t.category, ''),
      'assignee_id:' || t.assignee_id::text
    )
  )
FROM tasks t
WHERE t.id IN (SELECT id FROM repair_tasks WHERE merged)
   OR t.application_id IN (SELECT id FROM repair_duplicates)
ON CONFLICT (object_type, object_uid) DO UPDATE SET
  action = EXCLUDED.action,
  headers = EXCLUDED.headers,
  data = EXCLUDED.data,
  indexing_config = EXCLUDED.indexing_config,
  generation = index_outbox.generation + 1,
  state = CASE WHEN index_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END,
  claimed_generation = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_generation ELSE NULL END,
  claimed_at = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_at ELSE NULL END,
  attempts = 0,
  next_attempt_at = NOW(),
  sent_on = NULL;

-- Only a true value commits: -v apply=0 is a dry run like no apply at all.
\if :{?apply}
\else
  \set apply false
\endif
\if :apply
  COMMIT;
  \echo 'Applied.'
\else
  ROLLBACK;
  \echo 'Dry run: rolled back. Pass -v apply=1 to commit.'
\endif
