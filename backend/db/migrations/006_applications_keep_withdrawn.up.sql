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

COMMIT;
