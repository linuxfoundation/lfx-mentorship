-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

BEGIN;

SET LOCAL search_path TO mentorship, public;

-- The relay's claim ORs pending rows with stale in_flight ones; without an
-- index for the in_flight branch Postgres seq-scans the whole table, sent rows
-- included. With both partial indexes it can BitmapOr them instead.
CREATE INDEX IF NOT EXISTS idx_index_outbox_in_flight
  ON index_outbox(claimed_at) WHERE state = 'in_flight';

-- The FGA claim's NOT EXISTS looks up in-flight markers by object; without
-- this it walks idx_fga_outbox_object_serialization across every pending row.
CREATE INDEX IF NOT EXISTS idx_fga_outbox_in_flight_object
  ON fga_outbox(object_type, object_uid) WHERE state = 'in_flight';

COMMIT;
