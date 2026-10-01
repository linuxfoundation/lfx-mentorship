-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

BEGIN;

SET LOCAL search_path TO mentorship, public;

-- A granted program membership is stored as 'approved', the legacy term, instead of 'active'.
ALTER TABLE program_members DROP CONSTRAINT IF EXISTS program_members_status_check;

-- A rename is not an edit; keep each row's updated_on.
ALTER TABLE program_members DISABLE TRIGGER set_updated_on;
UPDATE program_members SET status = 'approved' WHERE status = 'active';
ALTER TABLE program_members ENABLE TRIGGER set_updated_on;

ALTER TABLE program_members
  ADD CONSTRAINT program_members_status_check
  CHECK (status IS NULL OR status IN ('invited', 'requested', 'pending', 'approved', 'declined', 'withdrawn'));

COMMIT;
