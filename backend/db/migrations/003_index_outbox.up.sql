-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

BEGIN;

SET LOCAL search_path TO mentorship, public;

CREATE TABLE index_outbox (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    object_type     TEXT NOT NULL,
    object_uid      UUID NOT NULL,
    action          TEXT NOT NULL CHECK (action IN ('created', 'updated', 'deleted')),
    headers         JSONB NOT NULL,
    data            JSONB,
    indexing_config JSONB,
    state           TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'in_flight', 'sent', 'dead_letter')),
    attempts        INTEGER NOT NULL DEFAULT 0,
    created_on      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_on         TIMESTAMPTZ
);

CREATE INDEX idx_index_outbox_pending ON index_outbox (created_on) WHERE state = 'pending';

COMMIT;