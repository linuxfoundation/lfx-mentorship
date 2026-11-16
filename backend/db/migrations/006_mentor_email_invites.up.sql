-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

BEGIN;

SET LOCAL search_path TO mentorship, public;

-- A Program Admin can invite a mentor by email before that person has an LF
-- account. Such an invitation is a mentor row with no user: it holds only the
-- address, and accepting it after signing in sets user_id to the caller.
-- Until then the row may only be invited or declined, so it never needs the
-- FGA tuple or index entry an approved member gets.
ALTER TABLE program_members ALTER COLUMN user_id DROP NOT NULL;

ALTER TABLE program_members ADD CONSTRAINT program_members_email_invite_check
  CHECK (user_id IS NOT NULL OR (
    member_type = 'mentor'
    AND status IN ('invited', 'declined')
    AND email IS NOT NULL AND email <> ''
  ));

-- One open email invitation per address and program. The (program_id, user_id,
-- member_type) key does not cover these rows, since NULLs never collide.
CREATE UNIQUE INDEX IF NOT EXISTS uq_program_members_email_invite
  ON program_members (program_id, lower(email), member_type)
  WHERE user_id IS NULL;

COMMIT;
