-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

BEGIN;

SET LOCAL search_path TO mentorship, public;

ALTER TABLE programs
  ADD COLUMN IF NOT EXISTS project_uid TEXT;

CREATE INDEX IF NOT EXISTS idx_programs_project_uid
  ON programs(project_uid)
  WHERE project_uid IS NOT NULL;

CREATE TABLE IF NOT EXISTS mentorship_approver_team_members (
  user_id    UUID PRIMARY KEY REFERENCES users(id),
  created_on TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_on TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS fga_outbox (
  id                 BIGSERIAL PRIMARY KEY,
  marker_kind        TEXT NOT NULL,
  object_type        TEXT NOT NULL,
  object_uid         TEXT NOT NULL,
  relation           TEXT,
  username           TEXT,
  desired_operation  TEXT NOT NULL,
  generation         BIGINT NOT NULL DEFAULT 1,
  state              TEXT NOT NULL DEFAULT 'pending',
  claimed_generation BIGINT,
  claimed_at         TIMESTAMPTZ,
  attempts           INTEGER NOT NULL DEFAULT 0,
  next_attempt_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_error         TEXT,
  created_on         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_on         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT fga_outbox_marker_kind_check
    CHECK (marker_kind IN ('object', 'membership')),
  CONSTRAINT fga_outbox_operation_check
    CHECK (desired_operation IN ('sync', 'remove', 'update_access', 'delete_access')),
  CONSTRAINT fga_outbox_state_check
    CHECK (state IN ('pending', 'in_flight', 'dead_letter')),
  CONSTRAINT fga_outbox_membership_fields_check
    CHECK (
      marker_kind = 'object'
      OR (relation IS NOT NULL AND username IS NOT NULL)
    ),
  CONSTRAINT fga_outbox_object_operation_check
    CHECK (
      (marker_kind = 'object' AND desired_operation IN ('update_access', 'delete_access'))
      OR (marker_kind = 'membership' AND desired_operation IN ('sync', 'remove'))
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fga_outbox_object_marker
  ON fga_outbox(object_type, object_uid)
  WHERE marker_kind = 'object';

CREATE UNIQUE INDEX IF NOT EXISTS uq_fga_outbox_membership_marker
  ON fga_outbox(object_type, object_uid, relation, username)
  WHERE marker_kind = 'membership';

CREATE INDEX IF NOT EXISTS idx_fga_outbox_claimable
  ON fga_outbox(state, next_attempt_at, id);

CREATE INDEX IF NOT EXISTS idx_fga_outbox_object_serialization
  ON fga_outbox(object_type, object_uid, id);

-- Review fields are private application data and are exposed only through
-- reviewer-authorized routes.
ALTER TABLE applications ADD COLUMN IF NOT EXISTS evaluation TEXT;
ALTER TABLE applications ADD COLUMN IF NOT EXISTS reviewer_note TEXT;

-- Keep the parent columns nullable until the migration/backfill process has
-- repaired existing rows. The strict NOT NULL checks are applied separately
-- after that repair report is clean.
ALTER TABLE tasks
  DROP CONSTRAINT IF EXISTS tasks_application_id_fkey;

ALTER TABLE tasks
  ADD CONSTRAINT tasks_application_id_fkey
  FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE;

CREATE TABLE IF NOT EXISTS fga_membership_tombstones (
  object_type TEXT NOT NULL,
  object_uid  TEXT NOT NULL,
  relation    TEXT NOT NULL,
  username    TEXT NOT NULL,
  deleted_on  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_reconciled_on TIMESTAMPTZ,
  PRIMARY KEY (object_type, object_uid, relation, username)
);

CREATE INDEX IF NOT EXISTS idx_fga_membership_tombstones_reconcile
  ON fga_membership_tombstones(last_reconciled_on, deleted_on);

CREATE TABLE IF NOT EXISTS index_outbox (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  object_type     TEXT NOT NULL,
  object_uid      UUID NOT NULL,
  action          TEXT NOT NULL CHECK (action IN ('created', 'updated', 'deleted')),
  headers         JSONB NOT NULL,
  data            JSONB,
  indexing_config JSONB,
  state           TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'in_flight', 'sent', 'dead_letter')),
  generation      BIGINT NOT NULL DEFAULT 1,
  claimed_generation BIGINT,
  attempts        INTEGER NOT NULL DEFAULT 0,
  created_on      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  sent_on         TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_index_outbox_pending
  ON index_outbox(created_on) WHERE state = 'pending';

CREATE UNIQUE INDEX IF NOT EXISTS uq_index_outbox_object
  ON index_outbox(object_type, object_uid);

ALTER TABLE index_outbox
  ADD COLUMN claimed_at TIMESTAMPTZ;

ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_status_check;

ALTER TABLE tasks
  ADD CONSTRAINT tasks_status_check
  CHECK (status IN ('incomplete', 'in_progress', 'submitted', 'complete'));

DELETE FROM user_profiles AS duplicate
USING user_profiles AS keeper
WHERE duplicate.user_id = keeper.user_id
  AND duplicate.profile_type = keeper.profile_type
  AND duplicate.profile_type = 'mentee'
  AND duplicate.id <> keeper.id
  AND (COALESCE(duplicate.created_on, '-infinity'::timestamptz), duplicate.id)
      < (COALESCE(keeper.created_on, '-infinity'::timestamptz), keeper.id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_profiles_user_type
  ON user_profiles(user_id, profile_type)
  WHERE profile_type = 'mentee';

COMMIT;
