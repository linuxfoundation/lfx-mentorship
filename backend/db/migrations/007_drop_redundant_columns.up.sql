-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

-- These columns came over from the DynamoDB record shapes. Each one copies
-- data another table owns, or holds a legacy value nothing reads, and their
-- values had drifted from the source of truth (#403). The backend stopped
-- reading and writing them one release earlier, so the pods still running
-- while this is applied do not name them. None has an index, constraint,
-- trigger or view depending on it.
--
-- Each table is changed in its own transaction, so the migration never holds
-- one table's lock while it waits for another's: a request that reads two of
-- these tables would otherwise deadlock with it, and Postgres rolls back the
-- migration. The drops are independent and repeatable, so a run that stops
-- part-way is safe to run again.

-- Copies of the term's status and dates, and a legacy notification flag.
-- program_terms holds the status and dates.
BEGIN;
SET LOCAL search_path TO mentorship, public;
ALTER TABLE applications
  DROP COLUMN IF EXISTS program_term_status,
  DROP COLUMN IF EXISTS start_date_time,
  DROP COLUMN IF EXISTS end_date_time,
  DROP COLUMN IF EXISTS admin_notified;
COMMIT;

-- Copies of applications.status and program_terms.status.
BEGIN;
SET LOCAL search_path TO mentorship, public;
ALTER TABLE tasks
  DROP COLUMN IF EXISTS application_status,
  DROP COLUMN IF EXISTS program_term_status;
COMMIT;

-- quarantined_tasks was created LIKE tasks and the importer writes both with
-- one column list, so it loses the same columns.
BEGIN;
SET LOCAL search_path TO mentorship, public;
ALTER TABLE quarantined_tasks
  DROP COLUMN IF EXISTS application_status,
  DROP COLUMN IF EXISTS program_term_status;
COMMIT;

-- program_term_status is derivable from the program's terms, amount_raised is
-- superseded by program_funding_stats, and mentee_needs is a legacy blob.
BEGIN;
SET LOCAL search_path TO mentorship, public;
ALTER TABLE programs
  DROP COLUMN IF EXISTS program_term_status,
  DROP COLUMN IF EXISTS amount_raised,
  DROP COLUMN IF EXISTS mentee_needs;
COMMIT;

-- Stored counters nothing maintains.
BEGIN;
SET LOCAL search_path TO mentorship, public;
ALTER TABLE program_terms DROP COLUMN IF EXISTS active_users;
COMMIT;

BEGIN;
SET LOCAL search_path TO mentorship, public;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS number_of_projects;
COMMIT;
