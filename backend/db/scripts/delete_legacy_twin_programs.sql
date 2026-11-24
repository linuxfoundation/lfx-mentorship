-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

-- One-time delete of three legacy programs that duplicate a published program
-- (linuxfoundation/lfx-mentorship#468). Run once against the prod database:
--
--   psql "$PROD_DSN" -v ON_ERROR_STOP=1 -f backend/db/scripts/delete_legacy_twin_programs.sql
--
-- Each duplicate has the same name, LF project and creator as its published
-- twin and was never approved; the stale-pending cleanup
-- (close_stale_pending_programs.sql) rejected or hid it. Each shares a
-- Crowdfunding initiative with its twin, so Crowdfunding's nightly
-- mentorship-sync matches the duplicate and declines the funded page
-- (linuxfoundation/lfx-crowdfunding#502). Once these rows are gone they drop
-- out of ANALYTICS.GOLD_FACT.MENTORSHIP_PROGRAMS at the next prod dbt build.
--
-- Plain SQL because the API delete needs writer on each program, which nobody
-- holds for these. The script does what ProgramRepository.Delete does for a
-- program with no terms: the DELETE cascades program_members, program_skills
-- and program_funding_stats, and per program it queues the OpenFGA access
-- delete, the search-index delete and, when no other program uses it, the logo.
--
-- The transaction rolls back unless each program still exists, is rejected or
-- hidden, has no terms, and its published twin still exists, and unless
-- exactly 3 rows are deleted.

BEGIN;
SET LOCAL search_path TO mentorship;

CREATE TEMP TABLE legacy_twins (id uuid PRIMARY KEY, keep_id uuid NOT NULL) ON COMMIT DROP;
INSERT INTO legacy_twins (id, keep_id) VALUES
  ('220b63a7-4ee4-4290-995c-c6441e5c44bb', '1f3216bc-e266-415c-84c7-a9a728dff3ce'), -- Linux Kernel Mentorship
  ('c567fa35-dbe8-416f-9996-80cdeb9c1992', '13ffdfb6-cb76-4ff0-8ac1-c27977b8cb56'), -- MLModelScope (OMP)
  ('5998326f-2bc4-437a-b9fc-70174a03225b', 'f8c14eaa-8e20-4398-83ab-b8ac55d20e12'); -- Yocto Project

DO $$
DECLARE n int;
BEGIN
  PERFORM 1 FROM programs WHERE id IN (SELECT id FROM legacy_twins) FOR UPDATE;

  SELECT count(*) INTO n FROM programs p JOIN legacy_twins t ON t.id = p.id
  WHERE p.status IN ('rejected', 'hidden');
  IF n <> 3 THEN
    RAISE EXCEPTION 'expected 3 rejected or hidden programs, found %; nothing was changed', n;
  END IF;

  SELECT count(*) INTO n FROM program_terms WHERE program_id IN (SELECT id FROM legacy_twins);
  IF n <> 0 THEN
    RAISE EXCEPTION 'expected no terms on the programs, found %; nothing was changed', n;
  END IF;

  SELECT count(*) INTO n FROM programs p JOIN legacy_twins t ON t.keep_id = p.id
  WHERE p.status = 'published';
  IF n <> 3 THEN
    RAISE EXCEPTION 'expected 3 published twins, found %; nothing was changed', n;
  END IF;

  -- A duplicate can carry its twin's logo URL; deleting that object would
  -- break the twin's logo, so only queue a logo nobody else uses.
  INSERT INTO object_deletions (bucket, locator)
  SELECT DISTINCT 'logos', p.logo_url
  FROM programs p JOIN legacy_twins t ON t.id = p.id
  WHERE p.logo_url <> ''
    AND NOT EXISTS (
      SELECT 1 FROM programs o
      WHERE o.logo_url = p.logo_url AND o.id NOT IN (SELECT id FROM legacy_twins)
    );
  GET DIAGNOSTICS n = ROW_COUNT;
  RAISE NOTICE 'logos queued for deletion: %', n;

  DELETE FROM programs WHERE id IN (SELECT id FROM legacy_twins);
  GET DIAGNOSTICS n = ROW_COUNT;
  IF n <> 3 THEN
    RAISE EXCEPTION 'expected to delete 3 programs, deleted %; nothing was changed', n;
  END IF;

  INSERT INTO fga_outbox (marker_kind, object_type, object_uid, desired_operation)
  SELECT 'object', 'mentorship_program', id, 'delete_access' FROM legacy_twins
  ON CONFLICT (object_type, object_uid) WHERE marker_kind = 'object'
  DO UPDATE SET desired_operation = EXCLUDED.desired_operation,
                generation = fga_outbox.generation + 1,
                state = CASE WHEN fga_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END,
                claimed_generation = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_generation ELSE NULL END,
                claimed_at = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_at ELSE NULL END,
                attempts = 0, next_attempt_at = NOW(), last_error = NULL,
                updated_on = NOW();

  -- The relay adds its own authorization header, so empty headers suffice.
  INSERT INTO index_outbox (object_type, object_uid, action, headers)
  SELECT 'mentorship_program', id, 'deleted', '{}'::jsonb FROM legacy_twins
  ON CONFLICT (object_type, object_uid) DO UPDATE SET
    action = 'deleted',
    headers = EXCLUDED.headers,
    data = NULL,
    indexing_config = NULL,
    generation = index_outbox.generation + 1,
    state = CASE WHEN index_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END,
    claimed_generation = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_generation ELSE NULL END,
    claimed_at = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_at ELSE NULL END,
    attempts = 0,
    next_attempt_at = NOW(),
    sent_on = NULL;

  RAISE NOTICE 'deleted 3 programs; queued FGA and index deletes';
END $$;

COMMIT;
