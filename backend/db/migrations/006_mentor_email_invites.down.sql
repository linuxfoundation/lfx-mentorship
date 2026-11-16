-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

BEGIN;

SET LOCAL search_path TO mentorship, public;

-- Email invitations have no user to keep, so they are removed before user_id
-- becomes required again.
DELETE FROM program_members WHERE user_id IS NULL;

DROP INDEX IF EXISTS uq_program_members_email_invite;

ALTER TABLE program_members DROP CONSTRAINT IF EXISTS program_members_email_invite_check;

ALTER TABLE program_members ALTER COLUMN user_id SET NOT NULL;

COMMIT;
