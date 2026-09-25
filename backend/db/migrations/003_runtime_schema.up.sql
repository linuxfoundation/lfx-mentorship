-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

BEGIN;

SET LOCAL search_path TO mentorship, public;

ALTER TABLE programs
  RENAME COLUMN project_uid TO lf_project_uid;

ALTER TABLE programs
  ADD COLUMN IF NOT EXISTS lf_project_slug TEXT,
  ADD COLUMN IF NOT EXISTS lf_project_name TEXT,
  ADD COLUMN IF NOT EXISTS lf_project_logo_url TEXT;

DROP INDEX IF EXISTS idx_programs_project_uid;

CREATE INDEX IF NOT EXISTS idx_programs_lf_project_uid
  ON programs(lf_project_uid)
  WHERE lf_project_uid IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_programs_lf_project_slug
  ON programs(lf_project_slug)
  WHERE lf_project_slug IS NOT NULL;

ALTER TABLE tasks
  ALTER COLUMN due_date TYPE TEXT USING due_date::text;

ALTER TABLE index_outbox
  ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS last_error TEXT;

DROP INDEX IF EXISTS idx_index_outbox_pending;

CREATE INDEX IF NOT EXISTS idx_index_outbox_pending
  ON index_outbox(next_attempt_at, created_on) WHERE state = 'pending';

-- Rows enqueued before sanitization may hold a client-supplied actor header.
UPDATE index_outbox
SET headers = headers - 'x-on-behalf-of'
WHERE headers ? 'x-on-behalf-of';

DROP INDEX IF EXISTS uq_user_profiles_user_type;

DELETE FROM user_profiles AS duplicate
USING user_profiles AS keeper
WHERE duplicate.user_id = keeper.user_id
  AND duplicate.profile_type = 'mentee'
  AND keeper.profile_type = 'mentee'
  AND duplicate.id <> keeper.id
  AND (COALESCE(duplicate.created_on, '-infinity'::timestamptz), duplicate.id)
      < (COALESCE(keeper.created_on, '-infinity'::timestamptz), keeper.id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_profiles_user_type
  ON user_profiles(user_id, profile_type)
  WHERE profile_type = 'mentee';

DROP TABLE IF EXISTS fga_membership_tombstones;

COMMIT;
