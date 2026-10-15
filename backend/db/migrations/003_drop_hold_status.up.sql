-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

BEGIN;

SET LOCAL search_path TO mentorship, public;

-- "hold" is not an application status (02-target-architecture.md). A mentee
-- row on hold is a legacy paused accepted mentorship, so it becomes accepted;
-- the import maps it the same way. A mentor row reached hold only from pending
-- in the rewrite, so it goes back to pending: an admin accepting it then grants
-- the approved mentor membership as any acceptance does. The constraint is
-- dropped before its rows are rewritten.
--
-- The migration runs while the old pods still serve writes, so it locks the
-- tables first: a decision committed between the snapshot and the rewrite would
-- otherwise be overwritten, and a row moved to hold meanwhile would fail the
-- new constraint. ALTER TABLE needs ACCESS EXCLUSIVE on applications anyway, and
-- taking it up front avoids upgrading the lock later.
LOCK TABLE applications IN ACCESS EXCLUSIVE MODE;
LOCK TABLE tasks, quarantined_tasks IN SHARE ROW EXCLUSIVE MODE;

CREATE TEMP TABLE held_applications ON COMMIT DROP AS
  SELECT id, CASE WHEN role = 'mentor' THEN 'pending' ELSE 'accepted' END AS status
  FROM applications
  WHERE status = 'hold';

ALTER TABLE applications DROP CONSTRAINT IF EXISTS applications_status_check;

UPDATE applications a SET status = h.status
FROM held_applications h
WHERE a.id = h.id;

-- Tasks carry a denormalised copy of their application's status, and so do
-- quarantined tasks, which a repair can move back into tasks. An import copied
-- that copy independently, so a task can hold "hold" under an application that
-- does not; every such task takes its application's status, and the ones
-- rewritten are kept for the search refresh. A quarantined task without an
-- application is a legacy mentee task.
CREATE TEMP TABLE rewritten_tasks (id UUID PRIMARY KEY) ON COMMIT DROP;

WITH rewritten AS (
  UPDATE tasks t SET application_status = a.status
  FROM applications a
  WHERE a.id = t.application_id AND t.application_status = 'hold'
  RETURNING t.id
)
INSERT INTO rewritten_tasks SELECT id FROM rewritten;

UPDATE quarantined_tasks q SET application_status = COALESCE(
  (SELECT a.status FROM applications a WHERE a.id = q.application_id), 'accepted')
WHERE q.application_status = 'hold';

ALTER TABLE applications ADD CONSTRAINT applications_status_check
  CHECK (status IN ('pending', 'accepted', 'declined', 'withdrawn', 'graduated'));

-- Refresh the search documents the rewrite changed: the applications, the
-- rewritten tasks, and the applications' programs, whose accepted-mentee counts
-- move. The documents match those the API writes (program_index.go,
-- application_task_index.go; docs/indexer-contract.md).
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
    'sort_name', a.user_id::text,
    'name_and_aliases', jsonb_build_array(a.user_id::text, a.role),
    'public', false,
    'tags', jsonb_build_array('role:' || a.role, 'status:' || a.status),
    'parent_refs', jsonb_build_array('mentorship_program:' || pt.program_id::text)
  )
FROM applications a
JOIN held_applications h ON h.id = a.id
JOIN program_terms pt ON pt.id = a.program_term_id
JOIN users u ON u.id = a.user_id
WHERE NULLIF(u.lfid, '') IS NOT NULL
ON CONFLICT (object_type, object_uid) DO UPDATE SET
  action = EXCLUDED.action,
  headers = EXCLUDED.headers,
  data = EXCLUDED.data,
  indexing_config = EXCLUDED.indexing_config,
  generation = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.generation ELSE index_outbox.generation + 1 END,
  state = CASE WHEN index_outbox.state IN ('in_flight', 'dead_letter') THEN index_outbox.state ELSE 'pending' END,
  claimed_generation = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_generation ELSE NULL END,
  claimed_at = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_at ELSE NULL END,
  attempts = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.attempts ELSE 0 END,
  next_attempt_at = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.next_attempt_at ELSE NOW() END,
  sent_on = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.sent_on ELSE NULL END;

INSERT INTO index_outbox (object_type, object_uid, action, headers, data, indexing_config)
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
    'prerequisite', t.category = 'prerequisite',
    'status', t.status,
    'application_status', t.application_status,
    'program_term_status', t.program_term_status,
    'custom', t.custom,
    'submit_file', t.submit_file,
    'has_file', t.file IS NOT NULL,
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
    'sort_name', COALESCE(t.name, ''),
    'name_and_aliases', jsonb_build_array(COALESCE(t.name, ''), COALESCE(t.category, '')),
    'public', false,
    'tags', jsonb_build_array(
      'status:' || t.status,
      'category:' || COALESCE(t.category, ''),
      'assignee_id:' || t.assignee_id::text
    ),
    'parent_refs', jsonb_build_array('mentorship_application:' || t.application_id::text)
  )
FROM tasks t
JOIN rewritten_tasks r ON r.id = t.id
JOIN users u ON u.id = t.assignee_id
WHERE NULLIF(u.lfid, '') IS NOT NULL
ON CONFLICT (object_type, object_uid) DO UPDATE SET
  action = EXCLUDED.action,
  headers = EXCLUDED.headers,
  data = EXCLUDED.data,
  indexing_config = EXCLUDED.indexing_config,
  generation = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.generation ELSE index_outbox.generation + 1 END,
  state = CASE WHEN index_outbox.state IN ('in_flight', 'dead_letter') THEN index_outbox.state ELSE 'pending' END,
  claimed_generation = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_generation ELSE NULL END,
  claimed_at = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_at ELSE NULL END,
  attempts = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.attempts ELSE 0 END,
  next_attempt_at = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.next_attempt_at ELSE NOW() END,
  sent_on = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.sent_on ELSE NULL END;

INSERT INTO index_outbox (object_type, object_uid, action, headers, data, indexing_config)
SELECT
  'mentorship_program',
  programs.id,
  'updated',
  '{}'::jsonb,
  jsonb_strip_nulls(jsonb_build_object(
    'id', programs.id,
    'project_uid', programs.lf_project_uid,
    'project_slug', programs.lf_project_slug,
    'project_name', programs.lf_project_name,
    'project_logo_url', programs.lf_project_logo_url,
    'name', programs.name,
    'slug', programs.slug,
    'status', programs.status,
    'logo_url', programs.logo_url,
    'skills', COALESCE((SELECT jsonb_agg(ps.skill ORDER BY ps.skill) FROM program_skills ps WHERE ps.program_id = programs.id), '[]'::jsonb),
    'stats', jsonb_build_object(
      'mentors', (SELECT COUNT(*) FROM program_members pm WHERE pm.program_id = programs.id AND pm.member_type = 'mentor' AND pm.status = 'approved'),
      'mentees', (SELECT COUNT(*) FROM applications a JOIN program_terms pt ON pt.id = a.program_term_id WHERE pt.program_id = programs.id AND a.role = 'mentee' AND a.status = 'accepted'),
      'graduated', (SELECT COUNT(*) FROM applications a JOIN program_terms pt ON pt.id = a.program_term_id WHERE pt.program_id = programs.id AND a.role = 'mentee' AND a.status = 'graduated')
    ),
    'created_on', programs.created_on,
    'updated_on', programs.updated_on
  )),
  jsonb_build_object(
    'object_id', programs.id,
    'access_check_object', 'mentorship_program:' || programs.id::text,
    'access_check_relation', 'viewer',
    'history_check_object', 'mentorship_program:' || programs.id::text,
    'history_check_relation', 'auditor',
    'sort_name', programs.name,
    'name_and_aliases', jsonb_build_array(programs.name, programs.slug),
    'public', programs.status = 'published',
    'tags', CASE
      WHEN programs.lf_project_uid IS NULL THEN jsonb_build_array('status:' || programs.status)
      ELSE jsonb_build_array('status:' || programs.status, 'project_uid:' || programs.lf_project_uid)
    END
  ) || CASE
    WHEN programs.lf_project_uid IS NULL THEN '{}'::jsonb
    ELSE jsonb_build_object('parent_refs', jsonb_build_array('project:' || programs.lf_project_uid))
  END
FROM programs
WHERE programs.id IN (
  SELECT pt.program_id FROM held_applications h
  JOIN applications a ON a.id = h.id
  JOIN program_terms pt ON pt.id = a.program_term_id
)
ON CONFLICT (object_type, object_uid) DO UPDATE SET
  action = EXCLUDED.action,
  headers = EXCLUDED.headers,
  data = EXCLUDED.data,
  indexing_config = EXCLUDED.indexing_config,
  generation = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.generation ELSE index_outbox.generation + 1 END,
  state = CASE WHEN index_outbox.state IN ('in_flight', 'dead_letter') THEN index_outbox.state ELSE 'pending' END,
  claimed_generation = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_generation ELSE NULL END,
  claimed_at = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_at ELSE NULL END,
  attempts = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.attempts ELSE 0 END,
  next_attempt_at = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.next_attempt_at ELSE NOW() END,
  sent_on = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.sent_on ELSE NULL END;

COMMIT;
