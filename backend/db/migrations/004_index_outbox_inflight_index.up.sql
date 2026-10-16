-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

BEGIN;

SET LOCAL search_path TO mentorship, public;

-- The index relay's claim selects pending records OR in-flight records whose
-- claim went stale, and its stale-claim sweep selects in-flight records alone.
-- Only the pending branch had an index, so both statements scanned the whole
-- table on every poll, once a second per pod. Sent records stay in the table,
-- so that scan grew with the data: 141 ms over 330k sent records in prod after
-- the import. With this index the claim combines the two partial indexes and
-- the sweep uses this one.
CREATE INDEX IF NOT EXISTS idx_index_outbox_inflight
  ON index_outbox (claimed_at)
  WHERE state = 'in_flight';

COMMIT;
