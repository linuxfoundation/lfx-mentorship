-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

BEGIN;

SET LOCAL search_path TO mentorship, public;

-- users.created_on and updated_on were the only timestamps in the schema that
-- allowed NULL, and the legacy importer wrote NULL when a DynamoDB record had
-- no timestamp. The API reads them into non-nullable Go fields, so such a row
-- failed every user query: GET /me returned 500 and the user was locked out
-- (issue #364). Repair the rows the test import left, then forbid a recurrence;
-- the importer now defaults the timestamps itself. The set_updated_on trigger
-- stamps updated_on with the time of this repair on every row it touches.
UPDATE users
   SET created_on = COALESCE(created_on, updated_on, NOW())
 WHERE created_on IS NULL OR updated_on IS NULL;

ALTER TABLE users
  ALTER COLUMN created_on SET NOT NULL,
  ALTER COLUMN updated_on SET NOT NULL;

COMMIT;
