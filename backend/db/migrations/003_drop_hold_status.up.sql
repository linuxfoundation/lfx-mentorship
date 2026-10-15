-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

BEGIN;

SET LOCAL search_path TO mentorship, public;

-- "hold" is not an application status (02-target-architecture.md). In legacy it
-- marks a paused accepted mentorship, so a held application is an accepted one;
-- the import maps it the same way. The constraint is dropped before its rows are
-- rewritten, and the tasks' denormalised copy of the status follows.
ALTER TABLE applications DROP CONSTRAINT IF EXISTS applications_status_check;

UPDATE applications SET status = 'accepted' WHERE status = 'hold';
UPDATE tasks SET application_status = 'accepted' WHERE application_status = 'hold';

ALTER TABLE applications ADD CONSTRAINT applications_status_check
  CHECK (status IN ('pending', 'accepted', 'declined', 'withdrawn', 'graduated'));

COMMIT;
