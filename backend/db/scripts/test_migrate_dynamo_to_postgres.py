# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

"""Tests for the importer's project-parent resolution and program upsert.

Run from backend/db/scripts: python -m pytest. The upsert tests need a migrated
database in MIGRATION_TEST_PG_DSN and are skipped without one.
"""

import os
import sys
import types

import psycopg2
import pytest

import migrate_dynamo_to_postgres as m

PROGRAM = "11111111-1111-4111-8111-111111111111"
CNCF = "d9431bb9-b0a8-47ff-a3bc-30a2a989572c"
TPG = "2d296cca-1856-45dd-a3b0-2b37c9eabbfd"
SFID = "a0941000002wBz4AAE"


class _Msg:
    def __init__(self, data: str):
        self.data = data.encode()


@pytest.fixture
def nats(monkeypatch):
    """Install a fake nats module whose replies come from reply(subject, body)."""

    def install(reply):
        class Conn:
            async def request(self, subject, body, timeout=5):
                return _Msg(reply(subject, body.decode()))

            async def close(self):
                pass

        async def connect(url):
            return Conn()

        monkeypatch.setitem(sys.modules, "nats", types.SimpleNamespace(connect=connect))
        monkeypatch.setattr(m, "NATS_URL", "nats://fake")

    return install


def replies(mapping=CNCF, slug="cncf", name="CNCF"):
    table = {"lfx.lookup_v1_mapping": mapping, "lfx.projects-api.get_slug": slug, "lfx.projects-api.get_name": name}
    return lambda subject, body: table[subject]


def resolve_sfid():
    return m.resolve_lf_projects([{"projectId": PROGRAM, "lfProjectId": SFID}])[SFID]


def test_sfid_resolves_to_verified_project(nats):
    nats(replies())
    assert resolve_sfid() == (CNCF, "cncf", "CNCF")


def test_empty_mapping_reply_is_unmapped(nats):
    nats(replies(mapping=""))
    assert resolve_sfid() is None


@pytest.mark.parametrize(
    "kwargs",
    [
        {"mapping": "error: kv unavailable"},
        {"slug": ""},
        {"name": ""},
        {"slug": '{"error":"internal","message":"boom"}'},
        {"name": '{"error":"internal","message":"boom"}'},
    ],
    ids=["mapping-error", "empty-slug", "empty-name", "slug-internal", "name-internal"],
)
def test_lookup_failures_abort_the_import(nats, kwargs):
    nats(replies(**kwargs))
    with pytest.raises(RuntimeError, match=r"lfx\."):
        resolve_sfid()


@pytest.mark.parametrize("kwargs", [{"slug": '{"error":"not_found"}'}, {"name": '{"error":"not_found"}'}], ids=["slug", "name"])
def test_not_found_project_is_unmapped(nats, kwargs):
    nats(replies(**kwargs))
    assert resolve_sfid() is None


def test_value_that_is_not_an_error_body_is_kept(nats):
    nats(replies(name="{not json} project"))
    assert resolve_sfid() == (CNCF, "cncf", "{not json} project")


def test_without_nats_url_nothing_resolves(monkeypatch):
    monkeypatch.setattr(m, "NATS_URL", "")
    assert m.resolve_lf_projects([{"projectId": PROGRAM, "lfProjectId": SFID}]) == {}


class _Files:
    def rewrite(self, *args):
        return None


@pytest.mark.parametrize(
    "legacy, expected",
    [("draft", "pending"), ("pending", "pending"), ("DRAFT", "pending"), (None, "pending"), ("", "pending"), ("unknown", "pending"), ("published", "published")],
)
def test_program_status_maps_legacy_values(legacy, expected):
    assert m._normalize_program_status(legacy) == expected


def program_rows(monkeypatch, projects, resolved):
    captured = []
    monkeypatch.setattr(m.psycopg2.extras, "execute_batch", lambda cur, sql, rows, page_size=0: captured.append(rows) if "INSERT INTO programs" in sql else None)
    m.migrate_programs(None, projects, set(), _Files(), resolved)
    return {row[0]: row for row in captured[0]}


def test_selection_skips_rejected_and_self_referencing_candidates(monkeypatch):
    fabricated = "deadbeef-0000-5000-8000-000000000000"
    projects = [
        {"projectId": PROGRAM, "name": "A", "lfProjectId": SFID, "lfProjectLogo": "https://logo/a.svg"},
        {"projectId": TPG.replace("2d", "3e", 1), "name": "B", "lfProjectId": "unmapped", "lfProjectUid": fabricated},
        {"projectId": CNCF.replace("d9", "e9", 1), "name": "C", "projectUid": CNCF.replace("d9", "e9", 1), "lfProjectUid": TPG, "lfProjectLogo": "https://logo/c.svg"},
    ]
    resolved = {SFID: (CNCF, "cncf", "CNCF"), "unmapped": None, fabricated: None, TPG: (TPG, "test-project-group", "TPG")}
    rows = program_rows(monkeypatch, projects, resolved)

    assert rows[PROGRAM][1:5] == (CNCF, "cncf", "CNCF", "https://logo/a.svg")
    assert rows[projects[1]["projectId"]][1:5] == (None, None, None, None)
    # lfProjectLogo describes the lfProjectId project, so it is not applied to another parent.
    assert rows[projects[2]["projectId"]][1:5] == (TPG, "test-project-group", "TPG", None)


USER = "22222222-2222-4222-8222-222222222222"
OLD_MENTEE = "33333333-3333-4333-8333-333333333333"
NEW_MENTEE = "44444444-4444-4444-8444-444444444444"
UNDATED_MENTEE = "55555555-5555-4555-8555-555555555555"
MENTOR = "66666666-6666-4666-8666-666666666666"
DUPLICATE_PROFILES = [
    {"id": NEW_MENTEE, "userId": USER, "type": "mentee", "createdAt": "2024-02-01T00:00:00Z"},
    {"id": OLD_MENTEE, "userId": USER, "type": "mentee", "createdAt": "2023-01-01T00:00:00Z"},
    {"id": UNDATED_MENTEE, "userId": USER, "type": "mentee"},
    {"id": MENTOR, "userId": USER, "type": "mentor", "createdAt": "2022-01-01T00:00:00Z"},
]


def test_user_keeps_only_newest_mentee_profile(monkeypatch):
    captured = []
    monkeypatch.setattr(m.psycopg2.extras, "execute_batch", lambda cur, sql, rows, page_size=0: captured.append(rows))
    cur = types.SimpleNamespace(execute=lambda sql, args: None, rowcount=0)
    m.migrate_user_profiles(cur, DUPLICATE_PROFILES, {USER}, _Files())
    assert sorted((row[0], row[2]) for row in captured[0]) == [(NEW_MENTEE, "mentee"), (MENTOR, "mentor")]


def test_created_at_tie_keeps_highest_id(monkeypatch):
    captured = []
    monkeypatch.setattr(m.psycopg2.extras, "execute_batch", lambda cur, sql, rows, page_size=0: captured.append(rows))
    cur = types.SimpleNamespace(execute=lambda sql, args: None, rowcount=0)
    tied = [{"id": pid, "userId": USER, "type": "mentee", "createdAt": "2024-02-01T00:00:00Z"} for pid in (NEW_MENTEE, OLD_MENTEE)]
    m.migrate_user_profiles(cur, tied, {USER}, _Files())
    assert [row[0] for row in captured[0]] == [NEW_MENTEE]


DSN = os.environ.get("MIGRATION_TEST_PG_DSN")


@pytest.fixture
def cursor():
    if not DSN:
        pytest.skip("MIGRATION_TEST_PG_DSN is not set")
    conn = psycopg2.connect(DSN, options="-c search_path=mentorship,public")
    try:
        with conn.cursor() as cur:
            yield cur
    finally:
        conn.rollback()
        conn.close()


def upsert(cur, resolved, **source):
    m.migrate_programs(cur, [{"projectId": PROGRAM, "name": "P", **source}], set(), _Files(), resolved)
    cur.execute("SELECT lf_project_uid, lf_project_slug, lf_project_name, lf_project_logo_url FROM programs WHERE id = %s", (PROGRAM,))
    return cur.fetchone()


def test_upsert_keeps_existing_parent_when_none_is_confirmed(cursor):
    assert upsert(cursor, {SFID: (CNCF, "cncf", "CNCF")}, lfProjectId=SFID, lfProjectLogo="https://logo/a.svg") == (CNCF, "cncf", "CNCF", "https://logo/a.svg")
    assert upsert(cursor, {SFID: None}, lfProjectId=SFID) == (CNCF, "cncf", "CNCF", "https://logo/a.svg")


def test_upsert_keeps_logo_when_same_parent_resolves_through_another_field(cursor):
    upsert(cursor, {SFID: (CNCF, "cncf", "CNCF")}, lfProjectId=SFID, lfProjectLogo="https://logo/a.svg")
    assert upsert(cursor, {SFID: None, CNCF: (CNCF, "cncf", "CNCF")}, lfProjectId=SFID, lfProjectUid=CNCF) == (CNCF, "cncf", "CNCF", "https://logo/a.svg")


def test_upsert_replaces_metadata_when_parent_changes(cursor):
    upsert(cursor, {SFID: (CNCF, "cncf", "CNCF")}, lfProjectId=SFID, lfProjectLogo="https://logo/a.svg")
    assert upsert(cursor, {TPG: (TPG, "test-project-group", "TPG")}, lfProjectUid=TPG) == (TPG, "test-project-group", "TPG", None)


def test_duplicate_mentee_profiles_import_under_unique_index(cursor):
    m.migrate_user_profiles(cursor, DUPLICATE_PROFILES, set(), _Files())
    cursor.execute("SELECT id::text, profile_type FROM user_profiles WHERE user_id = %s ORDER BY profile_type", (USER,))
    assert cursor.fetchall() == [(NEW_MENTEE, "mentee"), (MENTOR, "mentor")]


def test_rerun_replaces_mentee_profile_when_keeper_changes(cursor):
    m.migrate_user_profiles(cursor, DUPLICATE_PROFILES[1:], set(), _Files())
    m.migrate_user_profiles(cursor, DUPLICATE_PROFILES, set(), _Files())
    cursor.execute("SELECT id::text, profile_type FROM user_profiles WHERE user_id = %s ORDER BY profile_type", (USER,))
    assert cursor.fetchall() == [(NEW_MENTEE, "mentee"), (MENTOR, "mentor")]


MEMBER_USERS = [
    {"id": "u-alice", "email": "Alice@Example.org", "lfid": "alice"},
    {"id": "u-shared-1", "email": "shared@example.org", "lfid": "owner"},
    {"id": "u-shared-2", "email": "shared@example.org", "lfid": "other"},
]
MEMBER_PROJECTS = [{"projectId": "p1", "lfid": "OWNER"}, {"projectId": "p2", "lfid": "nobody"}]


def _member(mid, email=None, project="p1", member_type="maintainer", **extra):
    return {"id": mid, "projectId": project, "memberType": member_type, "status": "accepted", "email": email, **extra}


def _user_ids(members, overrides=None):
    resolved = m.resolve_member_user_ids(members, MEMBER_USERS, MEMBER_PROJECTS, overrides or {})
    return {r["id"]: r.get("userId") for r in resolved}


def test_resolve_member_user_ids_matches_by_email_then_program_lfid():
    got = _user_ids([
        _member("m-email", "alice@example.ORG "),
        _member("m-shared", "shared@example.org"),
        _member("m-ambiguous", "shared@example.org", project="p2"),
        # The program-lfid tie-break identifies the creator, so it never applies to mentors.
        _member("m-mentor-shared", "shared@example.org", member_type="mentor"),
        _member("m-unknown", "ghost@example.org"),
        _member("m-noemail"),
        _member("m-has-user", "alice@example.org", userId="u-existing"),
    ])
    assert got == {
        "m-email": "u-alice",
        "m-shared": "u-shared-1",
        "m-ambiguous": None,
        "m-mentor-shared": None,
        "m-unknown": None,
        "m-noemail": None,
        "m-has-user": "u-existing",
    }


def test_resolve_member_user_ids_applies_overrides_only_when_matching_fails():
    got = _user_ids(
        [
            _member("m-a", "ghost@example.org"),
            _member("m-b", "ghost@example.org"),
            _member("m-ambiguous", "shared@example.org", project="p2"),
            # A stray override must not displace a unique email match.
            _member("m-auto", "alice@example.org"),
        ],
        {"m-a": "u-alice", "m-b": "u-missing", "m-ambiguous": "u-shared-2", "m-auto": "u-shared-1"},
    )
    assert got == {"m-a": "u-alice", "m-b": None, "m-ambiguous": "u-shared-2", "m-auto": "u-alice"}


def test_resolve_member_user_ids_does_not_mutate_input():
    members = [_member("m", "alice@example.org")]
    m.resolve_member_user_ids(members, MEMBER_USERS, MEMBER_PROJECTS, {})
    assert "userId" not in members[0]


def test_load_member_user_overrides(tmp_path):
    path = tmp_path / "overrides.csv"
    path.write_text("member_id,user_id\n m-a , u-alice \nm-b,\n")
    assert m.load_member_user_overrides(str(path)) == {"m-a": "u-alice"}
    assert m.load_member_user_overrides("") == {}


def test_user_fix_restores_an_lfid_before_the_tuples_are_seeded(cursor, tmp_path):
    from test_verify_migration import APPLICATION, MENTEE, import_sources

    cursor.execute("""TRUNCATE users, user_profiles, programs, program_terms, program_members, applications,
        tasks, quarantined_tasks, fga_outbox, index_outbox RESTART IDENTITY CASCADE""")
    import_sources(cursor)
    cursor.execute("UPDATE users SET lfid = NULL WHERE id = %s", (MENTEE,))
    cursor.execute("DELETE FROM fga_outbox")
    fixes = tmp_path / "user-fixes.sql"
    fixes.write_text(f"UPDATE users SET lfid = 'grace' WHERE id = '{MENTEE}';\n")

    m.apply_user_fixes(cursor, "")
    m.seed_derived_state(cursor)
    cursor.execute("SELECT object_uid FROM fga_outbox WHERE object_type = 'mentorship_application'")
    assert cursor.fetchall() == []

    m.apply_user_fixes(cursor, str(fixes))
    m.seed_derived_state(cursor)
    cursor.execute("SELECT object_uid FROM fga_outbox WHERE object_type = 'mentorship_application'")
    assert cursor.fetchall() == [(APPLICATION,)]
