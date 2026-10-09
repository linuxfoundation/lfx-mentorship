# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

"""Tests for verify_migration: a clean import passes, and drift is reported.

Needs a migrated database in MIGRATION_TEST_PG_DSN; skipped without one.
"""

import os
import random

import psycopg2
import pytest

import migrate_dynamo_to_postgres as m
import verify_migration as v

CNCF = "d9431bb9-b0a8-47ff-a3bc-30a2a989572c"
PROGRAM = "11111111-1111-4111-8111-111111111111"
TERM = "22222222-2222-4222-8222-222222222222"
ADMIN = "33333333-3333-4333-8333-333333333333"
MENTEE = "44444444-4444-4444-8444-444444444444"
APPLICATION = "55555555-5555-4555-8555-555555555555"
TASK = "66666666-6666-4666-8666-666666666666"
ORPHAN_TASK = "77777777-7777-4777-8777-777777777777"

SOURCES = {
    "users": [
        {"id": ADMIN, "email": "shared@example.org", "lfid": "ada", "name": "Ada", "createdAt": "2024-01-02T03:04:05Z"},
        {"id": MENTEE, "email": "shared@example.org", "lfid": "grace", "givenName": "Grace"},
    ],
    "user-profiles": [
        {"id": "88888888-8888-4888-8888-888888888888", "userId": ADMIN, "type": "mentor", "firstName": "Ada", "introduction": "Hi"},
    ],
    "projects": [
        {"projectId": PROGRAM, "lfProjectUid": CNCF, "name": "Program", "status": "published", "repoLink": "https://github.com/x", "createdOn": "2024-02-01T00:00:00Z"},
    ],
    "program-terms": [
        {"id": TERM, "projectId": PROGRAM, "name": "Spring", "Active": "open", "startDateTime": "1767225600", "endDateTime": "1777593600",
         "applicationStartDate": "1764547200", "applicationEndDate": "1767139200"},
    ],
    "project-members": [
        {"id": "99999999-9999-4999-8999-999999999999", "projectId": PROGRAM, "userId": ADMIN, "memberType": "maintainer", "status": "accepted", "email": "ada@example.org"},
    ],
    "program-term-mentees": [
        {"id": APPLICATION, "programTermId": TERM, "userId": MENTEE, "status": "approved", "tasksSubmitted": True, "startDateTime": "1767225600"},
    ],
    "tasks": [
        {"id": TASK, "programTermId": TERM, "assigneeId": MENTEE, "ownerId": ADMIN, "name": "Intro", "status": "submitted", "category": "prerequisite"},
        {"id": ORPHAN_TASK, "programTermId": TERM, "assigneeId": ADMIN, "name": "No application"},
    ],
}

DSN = os.environ.get("MIGRATION_TEST_PG_DSN")


class _Files:
    def rewrite(self, *args):
        return None


@pytest.fixture
def imported():
    """Imports SOURCES into an emptied database inside a transaction that is rolled back."""
    if not DSN:
        pytest.skip("MIGRATION_TEST_PG_DSN is not set")
    conn = psycopg2.connect(DSN, options="-c search_path=mentorship,public")
    try:
        with conn.cursor() as cur:
            cur.execute("""TRUNCATE users, user_profiles, programs, program_terms, program_members, applications,
                tasks, quarantined_tasks, fga_outbox, index_outbox RESTART IDENTITY CASCADE""")
            files = _Files()
            users = m.migrate_users(cur, SOURCES["users"], files)
            m.migrate_user_profiles(cur, SOURCES["user-profiles"], users, files)
            programs = m.migrate_programs(cur, SOURCES["projects"], users, files, {CNCF: (CNCF, "cncf", "CNCF")})
            terms = m.migrate_program_terms(cur, SOURCES["program-terms"], programs)
            m.migrate_program_members(cur, SOURCES["project-members"], programs, users)
            applications = m.migrate_mentees(cur, SOURCES["program-term-mentees"], terms, users)
            m.migrate_tasks(cur, SOURCES["tasks"], applications, terms, users, files)
            yield cur
    finally:
        conn.rollback()
        conn.close()


def run(cur) -> list:
    return v.verify(cur, SOURCES, 50, random.Random(1)).problems


def test_clean_import_passes(imported):
    assert run(imported) == []


def test_shifted_field_is_reported(imported):
    imported.execute("UPDATE programs SET description = repo_link, repo_link = NULL WHERE id = %s", (PROGRAM,))
    problems = run(imported)
    assert any("programs" in p and "description" in p for p in problems)
    assert any("programs" in p and "repo_link" in p for p in problems)


def test_missing_row_is_reported(imported):
    imported.execute("DELETE FROM program_members WHERE program_id = %s", (PROGRAM,))
    assert any(p.startswith("program_members") and "no imported row" in p for p in run(imported))


def test_quarantined_task_is_expected_in_quarantine(imported):
    imported.execute("DELETE FROM quarantined_tasks WHERE id = %s", (ORPHAN_TASK,))
    assert run(imported) == [f"quarantined_tasks {ORPHAN_TASK}: no imported row"]


def test_broken_invariant_is_reported(imported):
    imported.execute("UPDATE applications SET program_term_status = 'closed' WHERE id = %s", (APPLICATION,))
    assert "integrity: 1 application term status differs from its open or closed term" in run(imported)


def test_duplicate_email_nulled_by_the_importer_is_not_a_mismatch(imported):
    imported.execute("SELECT email FROM users WHERE id = %s", (MENTEE,))
    assert imported.fetchone()[0] is None
    assert run(imported) == []
