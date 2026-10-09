# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

import retract_dropped as r
from test_verify_migration import ADMIN, PROGRAM, TASK, db, imported  # noqa: F401  (fixtures)


def outbox(cur):
    cur.execute("SELECT object_type, object_uid, desired_operation FROM fga_outbox WHERE desired_operation IN ('delete_access', 'remove')")
    fga = set(cur.fetchall())
    cur.execute("SELECT object_type, object_uid::text FROM index_outbox WHERE action = 'deleted'")
    return fga, set(cur.fetchall())


def test_rows_that_came_back_are_left_alone(imported):
    published = r.snapshot(imported)
    assert r.retract(imported, published) == {t: 0 for t in [*r.OBJECTS, "approvers"]}
    assert outbox(imported) == (set(), set())


def test_dropped_rows_and_approvers_are_retracted(imported):
    imported.execute("INSERT INTO mentorship_approver_team_members (user_id) VALUES (%s)", (ADMIN,))
    published = r.snapshot(imported)
    assert published["approvers"] == ["ada"]
    imported.execute("DELETE FROM mentorship_approver_team_members")
    imported.execute("DELETE FROM tasks WHERE id = %s", (TASK,))
    imported.execute("DELETE FROM fga_outbox; DELETE FROM index_outbox")

    counts = r.retract(imported, published)

    assert counts == {"mentorship_program": 0, "mentorship_application": 0, "mentorship_task": 1, "approvers": 1}
    fga, index = outbox(imported)
    assert fga == {("mentorship_task", TASK, "delete_access"), ("mentorship_approver_team", "global", "remove")}
    assert index == {("mentorship_task", TASK)}
    assert PROGRAM not in {uid for _, uid in index}
