-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

BEGIN;

SET LOCAL search_path TO mentorship, public;

-- An accepted program member is "approved", not "active", and programs no
-- longer have a "submitted" state: "pending" is the single review state. Each
-- constraint is dropped before its rows are rewritten, since the old one
-- rejects "approved".
ALTER TABLE program_members DROP CONSTRAINT IF EXISTS program_members_status_check;

UPDATE program_members SET status = 'approved' WHERE status = 'active';

ALTER TABLE program_members ADD CONSTRAINT program_members_status_check
  CHECK (status IS NULL OR status IN ('invited', 'requested', 'pending', 'approved', 'declined', 'withdrawn'));

ALTER TABLE programs DROP CONSTRAINT IF EXISTS programs_status_check;

UPDATE programs SET status = 'pending' WHERE status = 'submitted';

ALTER TABLE programs ADD CONSTRAINT programs_status_check
  CHECK (status IN ('pending', 'published', 'rejected', 'archived', 'hidden'));

COMMIT;
