-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

BEGIN;

SET LOCAL search_path TO mentorship, public;

-- Restores the columns with their original types and defaults. The dropped
-- values are not restored: the copies are refilled from the tables that own
-- the data, and the legacy-only values (admin_notified, amount_raised,
-- mentee_needs, active_users, number_of_projects) come back as defaults.

ALTER TABLE applications
  ADD COLUMN IF NOT EXISTS program_term_status VARCHAR(20),
  ADD COLUMN IF NOT EXISTS start_date_time     TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS end_date_time       TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS admin_notified      BOOLEAN DEFAULT false;

UPDATE applications a
SET program_term_status = t.status,
    start_date_time     = t.start_date_time,
    end_date_time       = t.end_date_time
FROM program_terms t
WHERE t.id = a.program_term_id;

ALTER TABLE tasks
  ADD COLUMN IF NOT EXISTS application_status  VARCHAR(20),
  ADD COLUMN IF NOT EXISTS program_term_status VARCHAR(20);

UPDATE tasks k
SET application_status  = a.status,
    program_term_status = t.status
FROM applications a
JOIN program_terms t ON t.id = a.program_term_id
WHERE a.id = k.application_id;

-- A quarantined task has no application to refill from.
ALTER TABLE quarantined_tasks
  ADD COLUMN IF NOT EXISTS application_status  VARCHAR(20),
  ADD COLUMN IF NOT EXISTS program_term_status VARCHAR(20);

ALTER TABLE programs
  ADD COLUMN IF NOT EXISTS program_term_status VARCHAR(20),
  ADD COLUMN IF NOT EXISTS amount_raised       NUMERIC(20,2) DEFAULT 0,
  ADD COLUMN IF NOT EXISTS mentee_needs        JSONB;

ALTER TABLE program_terms ADD COLUMN IF NOT EXISTS active_users INTEGER DEFAULT 0;

ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS number_of_projects INTEGER DEFAULT 0;

COMMIT;
