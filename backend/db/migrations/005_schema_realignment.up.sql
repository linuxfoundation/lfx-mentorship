-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

-- 002 and 003 were edited after dev had already applied them, and dev also
-- ran a since-removed 003_index_outbox, so its schema never reached what the
-- code expects. This re-asserts every element those edits introduced. Each
-- statement is idempotent: a database that ran the final 002 and 003 is left
-- unchanged.

BEGIN;

SET LOCAL search_path TO mentorship, public;

-- programs.project_uid became lf_project_uid in 003. Dev already carries
-- lf_project_uid, so rename only when the old column is still the only one.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.columns
             WHERE table_schema = 'mentorship' AND table_name = 'programs'
               AND column_name = 'project_uid') THEN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = 'mentorship' AND table_name = 'programs'
                 AND column_name = 'lf_project_uid') THEN
      UPDATE programs SET lf_project_uid = project_uid WHERE lf_project_uid IS NULL;
      ALTER TABLE programs DROP COLUMN project_uid;
    ELSE
      ALTER TABLE programs RENAME COLUMN project_uid TO lf_project_uid;
    END IF;
  END IF;
END $$;

ALTER TABLE programs
  ADD COLUMN IF NOT EXISTS lf_project_uid TEXT,
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

ALTER TABLE tasks
  DROP CONSTRAINT IF EXISTS tasks_due_date_check;

ALTER TABLE tasks
  ADD CONSTRAINT tasks_due_date_check
    CHECK (due_date IS NULL OR due_date ~ '^[0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$');

-- 004 cloned tasks before 003 ran on dev, so the clone still has a DATE column
-- and none of the check it would have copied.
ALTER TABLE quarantined_tasks
  ALTER COLUMN due_date TYPE TEXT USING due_date::text;

ALTER TABLE quarantined_tasks
  DROP CONSTRAINT IF EXISTS tasks_due_date_check;

ALTER TABLE quarantined_tasks
  ADD CONSTRAINT tasks_due_date_check
    CHECK (due_date IS NULL OR due_date ~ '^[0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$');

ALTER TABLE index_outbox
  ADD COLUMN IF NOT EXISTS generation BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS claimed_generation BIGINT,
  ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS last_error TEXT;

-- The outbox holds one row per object; older rows for the same object are
-- superseded by the newest.
DELETE FROM index_outbox AS stale
USING index_outbox AS latest
WHERE stale.object_type = latest.object_type
  AND stale.object_uid = latest.object_uid
  AND stale.id <> latest.id
  AND (stale.created_on, stale.id) < (latest.created_on, latest.id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_index_outbox_object
  ON index_outbox(object_type, object_uid);

DROP INDEX IF EXISTS idx_index_outbox_pending;

CREATE INDEX IF NOT EXISTS idx_index_outbox_pending
  ON index_outbox(next_attempt_at, created_on) WHERE state = 'pending';

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
