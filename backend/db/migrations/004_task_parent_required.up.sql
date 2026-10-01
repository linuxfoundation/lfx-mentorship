-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

BEGIN;

SET LOCAL search_path TO mentorship, public;

-- Tasks with no application have no authorization parent; hold them for operator repair.
CREATE TABLE IF NOT EXISTS quarantined_tasks (
  LIKE tasks INCLUDING DEFAULTS INCLUDING CONSTRAINTS,
  quarantine_reason TEXT        NOT NULL,
  quarantined_on    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (id)
);

INSERT INTO quarantined_tasks
SELECT tasks.*, 'missing application_id', NOW()
FROM tasks
WHERE application_id IS NULL
ON CONFLICT (id) DO NOTHING;

DELETE FROM tasks WHERE application_id IS NULL;

ALTER TABLE tasks
  ALTER COLUMN application_id SET NOT NULL;

COMMIT;
