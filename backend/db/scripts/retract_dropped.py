# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

"""Retract what an earlier import published but the final import did not bring back.

Truncating the mentorship tables before the final import leaves OpenFGA tuples and
search documents behind for rows that do not come back (test data, anything deleted
in legacy since). This queues their removal through the normal outbox relays.

Usage (runbook Phase 4):
    python retract_dropped.py snapshot /work/published.json   # before the truncate
    python retract_dropped.py retract /work/published.json    # after the final import
"""

import json
import logging
import sys

import psycopg2

import migrate_dynamo_to_postgres as m

log = logging.getLogger("retract_dropped")

# FGA object type -> table whose ids were published under it.
OBJECTS = {"mentorship_program": "programs", "mentorship_application": "applications", "mentorship_task": "tasks"}

APPROVERS_SQL = """
    SELECT users.lfid FROM mentorship_approver_team_members
    JOIN users ON users.id = mentorship_approver_team_members.user_id
    WHERE NULLIF(users.lfid, '') IS NOT NULL"""

_RETRACT_OBJECTS = """
    WITH dropped AS (
        SELECT uid FROM unnest(%(ids)s::uuid[]) AS uid
        WHERE NOT EXISTS (SELECT 1 FROM {table} WHERE id = uid)
    ), fga AS (
        INSERT INTO fga_outbox (marker_kind, object_type, object_uid, desired_operation)
        SELECT 'object', %(type)s, uid::text, 'delete_access' FROM dropped
        ON CONFLICT (object_type, object_uid) WHERE marker_kind = 'object'
        DO UPDATE SET desired_operation = EXCLUDED.desired_operation,
                      generation = fga_outbox.generation + 1,
                      state = CASE WHEN fga_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END,
                      claimed_generation = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_generation ELSE NULL END,
                      claimed_at = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_at ELSE NULL END,
                      attempts = 0, next_attempt_at = NOW(), last_error = NULL, updated_on = NOW()
    )
    INSERT INTO index_outbox (object_type, object_uid, action, headers)
    SELECT %(type)s, uid, 'deleted', '{{}}'::jsonb FROM dropped
    ON CONFLICT (object_type, object_uid) DO UPDATE SET
        action = 'deleted', headers = EXCLUDED.headers, data = NULL, indexing_config = NULL,
        generation = index_outbox.generation + 1,
        state = CASE WHEN index_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END,
        claimed_generation = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_generation ELSE NULL END,
        claimed_at = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_at ELSE NULL END,
        attempts = 0, next_attempt_at = NOW(), sent_on = NULL
    RETURNING object_uid"""

_RETRACT_APPROVERS = """
    INSERT INTO fga_outbox (marker_kind, object_type, object_uid, relation, username, desired_operation)
    SELECT 'membership', 'mentorship_approver_team', 'global', 'member', lfid, 'remove'
    FROM unnest(%s::text[]) AS lfid
    WHERE lfid NOT IN (""" + APPROVERS_SQL + """)
    ON CONFLICT (object_type, object_uid, relation, username) WHERE marker_kind = 'membership'
    DO UPDATE SET desired_operation = 'remove',
                  generation = fga_outbox.generation + 1,
                  state = CASE WHEN fga_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END,
                  claimed_generation = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_generation ELSE NULL END,
                  claimed_at = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_at ELSE NULL END,
                  attempts = 0, next_attempt_at = NOW(), last_error = NULL, updated_on = NOW()
    RETURNING username"""


def snapshot(cur) -> dict:
    published = {}
    for object_type, table in OBJECTS.items():
        cur.execute(f"SELECT id::text FROM {table}")
        published[object_type] = [row[0] for row in cur.fetchall()]
    cur.execute(APPROVERS_SQL)
    published["approvers"] = [row[0] for row in cur.fetchall()]
    return published


def retract(cur, published: dict) -> dict:
    """Queues removals for everything in the snapshot that is gone now; returns the counts."""
    counts = {}
    for object_type, table in OBJECTS.items():
        cur.execute(_RETRACT_OBJECTS.format(table=table), {"ids": published[object_type], "type": object_type})
        counts[object_type] = len(cur.fetchall())
    cur.execute(_RETRACT_APPROVERS, (published["approvers"],))
    counts["approvers"] = len(cur.fetchall())
    return counts


def main() -> None:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    if len(sys.argv) != 3 or sys.argv[1] not in ("snapshot", "retract"):
        sys.exit(__doc__)
    command, path = sys.argv[1:]
    conn = psycopg2.connect(m.PG_DSN)
    try:
        with conn, conn.cursor() as cur:
            if command == "snapshot":
                published = snapshot(cur)
                with open(path, "w", encoding="utf-8") as f:
                    json.dump(published, f)
                log.info("Saved %s to %s", {k: len(v) for k, v in published.items()}, path)
            else:
                with open(path, encoding="utf-8") as f:
                    counts = retract(cur, json.load(f))
                log.info("Queued removals: %s", counts)
    finally:
        conn.close()


if __name__ == "__main__":
    main()
