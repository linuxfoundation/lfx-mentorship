-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

BEGIN;

SET LOCAL search_path TO mentorship, public;

ALTER TABLE users
  ALTER COLUMN created_on DROP NOT NULL,
  ALTER COLUMN updated_on DROP NOT NULL;

COMMIT;
