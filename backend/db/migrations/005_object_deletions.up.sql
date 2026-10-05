-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

BEGIN;

SET LOCAL search_path TO mentorship, public;

-- Transactional queue of stored files to delete. Every path that drops a file
-- locator writes the locator here in the same transaction; a relay deletes the
-- object once the entry is due and no file column still holds the locator.
CREATE TABLE IF NOT EXISTS object_deletions (
  id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  bucket          TEXT        NOT NULL CHECK (bucket IN ('logos', 'attachments')),
  -- The value the file column held: a CDN URL for logos, an object key for attachments.
  locator         TEXT        NOT NULL CHECK (locator <> ''),
  state           TEXT        NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'in_flight', 'done', 'dead_letter')),
  attempts        INTEGER     NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_error      TEXT,
  claimed_at      TIMESTAMPTZ,
  created_on      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  completed_on    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_object_deletions_pending
  ON object_deletions(next_attempt_at) WHERE state = 'pending';

CREATE INDEX IF NOT EXISTS idx_object_deletions_in_flight
  ON object_deletions(claimed_at) WHERE state = 'in_flight';

-- Resumes are not migrated (docs/rewrite/02 §file classes); drop links an earlier ETL run carried over.
UPDATE user_profiles SET profile_links = profile_links - 'resumeLink'
WHERE profile_links ? 'resumeLink';

COMMIT;
