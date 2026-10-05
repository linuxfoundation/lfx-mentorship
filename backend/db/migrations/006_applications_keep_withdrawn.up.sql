-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

BEGIN;

SET LOCAL search_path TO mentorship, public;

-- Reapplying used to delete the withdrawn application and insert a new one,
-- which let an applicant erase the reviewer note, evaluation, and tasks on it.
-- A withdrawn application is now kept as history beside the new one, so the
-- table-wide UNIQUE (program_term_id, user_id, role) narrows to the
-- applications still in play. Withdrawn is terminal, so a kept row can never
-- become a second live application.
ALTER TABLE applications
  DROP CONSTRAINT IF EXISTS applications_program_term_id_user_id_role_key;

CREATE UNIQUE INDEX IF NOT EXISTS uq_applications_active
  ON applications (program_term_id, user_id, role)
  WHERE status <> 'withdrawn';

-- Telling a reapplication from its withdrawn history orders on created_on, and
-- the application model reads both timestamps as non-null. Backfilled legacy
-- rows can lack them, so fill the gaps from the other timestamp and enforce it.
UPDATE applications
  SET created_on = COALESCE(created_on, updated_on, NOW()),
      updated_on = COALESCE(updated_on, created_on, NOW())
  WHERE created_on IS NULL OR updated_on IS NULL;

ALTER TABLE applications
  ALTER COLUMN created_on SET NOT NULL,
  ALTER COLUMN updated_on SET NOT NULL;

COMMIT;
