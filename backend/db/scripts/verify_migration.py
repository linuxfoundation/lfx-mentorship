# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

"""Go/no-go check of an import against legacy DynamoDB.

Samples rows from every legacy table and compares their fields with the rows the
importer wrote, then runs integrity queries the schema cannot enforce. Exits 1 on
any mismatch.

Usage (same environment as migrate_dynamo_to_postgres.py):
    python verify_migration.py
Optional: VERIFY_SAMPLE_SIZE (default 200 per table), VERIFY_SEED (repeatable sample),
MEMBER_USER_OVERRIDES (the same file the import used).
"""

import logging
import os
import random
import sys
from datetime import datetime, timezone

import psycopg2

import migrate_dynamo_to_postgres as m

log = logging.getLogger("verify_migration")

SAMPLE_SIZE = int(os.environ.get("VERIFY_SAMPLE_SIZE", "200"))
SEED = os.environ.get("VERIFY_SEED")

_APP_PRIORITY = {"graduated": 5, "accepted": 4, "hold": 3, "pending": 2, "declined": 1, "withdrawn": 0}
_TASK_STATUSES = {"incomplete", "in_progress", "complete", "submitted"}

# Each query lists rows that break an invariant; every one must return nothing.
INTEGRITY_CHECKS = {
    "task without its application": """
        SELECT t.id FROM tasks t LEFT JOIN applications a ON a.id = t.application_id WHERE a.id IS NULL""",
    "application without its term": """
        SELECT a.id FROM applications a LEFT JOIN program_terms pt ON pt.id = a.program_term_id WHERE pt.id IS NULL""",
    "term without its program": """
        SELECT pt.id FROM program_terms pt LEFT JOIN programs p ON p.id = pt.program_id WHERE p.id IS NULL""",
    "member without its program or user": """
        SELECT pm.id FROM program_members pm
        LEFT JOIN programs p ON p.id = pm.program_id LEFT JOIN users u ON u.id = pm.user_id
        WHERE p.id IS NULL OR u.id IS NULL""",
    "task term differs from its application's term": """
        SELECT t.id FROM tasks t JOIN applications a ON a.id = t.application_id
        WHERE t.program_term_id IS DISTINCT FROM a.program_term_id""",
    "task assignee differs from its applicant": """
        SELECT t.id FROM tasks t JOIN applications a ON a.id = t.application_id WHERE t.assignee_id <> a.user_id""",
    "application term status differs from its open or closed term": """
        SELECT a.id FROM applications a JOIN program_terms pt ON pt.id = a.program_term_id
        WHERE pt.status IN ('open', 'closed') AND a.program_term_status IS DISTINCT FROM pt.status""",
    "task term status differs from its open or closed term": """
        SELECT t.id FROM tasks t JOIN program_terms pt ON pt.id = t.program_term_id
        WHERE pt.status IN ('open', 'closed') AND t.program_term_status IS DISTINCT FROM pt.status""",
    "task both live and quarantined": """
        SELECT t.id FROM tasks t JOIN quarantined_tasks q ON q.id = t.id""",
    "more than one live mentee application per term and user": """
        SELECT program_term_id FROM applications WHERE role = 'mentee' AND status <> 'withdrawn'
        GROUP BY program_term_id, user_id HAVING count(*) > 1""",
}


def _text(value):
    return (value or "").strip() or None


def _ms(value):
    return None if value is None else round(value.timestamp() * 1000)


class Report:
    def __init__(self):
        self.sampled: dict = {}
        self.problems: list = []

    def compare(self, table, key, expected: dict, actual: dict) -> None:
        for column, want in expected.items():
            got = actual[column]
            # A row with no legacy timestamp is stamped with the import time.
            if column == "created_on" and want is None:
                continue
            if isinstance(want, datetime) or isinstance(got, datetime):
                want, got = _ms(want), _ms(got)
            if want != got:
                self.problems.append(f"{table} {key}: {column} is {got!r}, legacy gives {want!r}")

    def missing(self, table, key) -> None:
        self.problems.append(f"{table} {key}: no imported row")

    def unexpected(self, table, key) -> None:
        self.problems.append(f"{table} {key}: imported although the importer should have skipped it")


def _sample(items: list, rng: random.Random, size: int) -> list:
    return items if len(items) <= size else rng.sample(items, size)


def _fetch(cur, sql: str, params) -> dict | None:
    cur.execute(sql, params)
    row = cur.fetchone()
    if row is None:
        return None
    return dict(zip([c.name for c in cur.description], row))


def _duplicated(values) -> set:
    seen, dupes = set(), set()
    for v in values:
        if v is not None:
            (dupes if v in seen else seen).add(v)
    return dupes


def verify_users(cur, users, report, rng, size):
    dup_emails = _duplicated(_text(u.get("email")) for u in users)
    dup_lfids = _duplicated(_text(u.get("lfid")) for u in users)
    candidates = [u for u in users if m._as_uuid(u.get("id"))]
    for u in _sample(candidates, rng, size):
        uid = m._as_uuid(u.get("id"))
        row = _fetch(cur, "SELECT email, lfid, name, given_name, family_name, created_on FROM users WHERE id = %s", (uid,))
        if row is None:
            report.missing("users", uid)
            continue
        expected = {
            "name": _text(u.get("name")),
            "given_name": _text(u.get("givenName")),
            "family_name": _text(u.get("familyName")),
            "created_on": m._parse_ts(u.get("createdAt")),
        }
        # Duplicates keep the first holder's value and null the rest.
        email, lfid = _text(u.get("email")), _text(u.get("lfid"))
        if not (email in dup_emails and row["email"] is None):
            expected["email"] = email
        if not (lfid in dup_lfids and row["lfid"] is None):
            expected["lfid"] = lfid
        report.compare("users", uid, expected, row)
    report.sampled["users"] = min(len(candidates), size)


def verify_profiles(cur, profiles, report, rng, size):
    keeper: dict = {}
    for p in profiles:
        pid, uid = m._as_uuid(p.get("id")), m._as_uuid(p.get("userId"))
        if p.get("recordKind") == "github-profile-reservation" or not pid or m._map_profile_type(p.get("type")) != "mentee":
            continue
        key = (m._parse_ts(p.get("createdAt")) or datetime.min.replace(tzinfo=timezone.utc), pid)
        if uid not in keeper or key > keeper[uid]:
            keeper[uid] = key
    candidates = [p for p in profiles if p.get("recordKind") != "github-profile-reservation" and m._as_uuid(p.get("id"))]
    for p in _sample(candidates, rng, size):
        pid, uid = m._as_uuid(p.get("id")), m._as_uuid(p.get("userId"))
        profile_type = m._map_profile_type(p.get("type"))
        row = _fetch(cur, "SELECT user_id::text, profile_type, first_name, last_name, email, introduction FROM user_profiles WHERE id = %s", (pid,))
        if profile_type == "mentee" and keeper.get(uid, (None, pid))[1] != pid:
            if row is not None:
                report.unexpected("user_profiles", pid)
            continue
        if row is None:
            report.missing("user_profiles", pid)
            continue
        report.compare("user_profiles", pid, {
            "user_id": uid,
            "profile_type": profile_type,
            "first_name": _text(p.get("firstName")),
            "last_name": _text(p.get("lastName")),
            "email": _text(p.get("email")),
            "introduction": _text(p.get("introduction")),
        }, row)
    report.sampled["user_profiles"] = min(len(candidates), size)


def verify_programs(cur, projects, report, rng, size):
    candidates = [p for p in projects if m._as_uuid(p.get("projectId"))]
    for p in _sample(candidates, rng, size):
        pid = m._as_uuid(p.get("projectId"))
        row = _fetch(cur, "SELECT name, status, description, website_url, repo_link, lfid, created_on FROM programs WHERE id = %s", (pid,))
        if row is None:
            report.missing("programs", pid)
            continue
        report.compare("programs", pid, {
            "name": _text(p.get("name")),
            "status": m._normalize_program_status(p.get("status")),
            "description": _text(p.get("description")),
            "website_url": _text(p.get("websiteUrl")),
            "repo_link": _text(p.get("repoLink")),
            "lfid": _text(p.get("lfid")),
            "created_on": m._parse_ts(p.get("createdOn")),
        }, row)
    report.sampled["programs"] = min(len(candidates), size)


def verify_terms(cur, terms, program_ids, report, rng, size):
    candidates = [t for t in terms if m._as_uuid(t.get("id")) and m._as_uuid(t.get("projectId"))]
    for t in _sample(candidates, rng, size):
        tid, pid = m._as_uuid(t.get("id")), m._as_uuid(t.get("projectId"))
        row = _fetch(cur, """SELECT program_id::text, name, status, start_date_time, end_date_time,
            application_start_date, application_end_date FROM program_terms WHERE id = %s""", (tid,))
        if pid not in program_ids:
            if row is not None:
                report.unexpected("program_terms", tid)
            continue
        if row is None:
            report.missing("program_terms", tid)
            continue
        status = (t.get("Active") or "open").lower()
        report.compare("program_terms", tid, {
            "program_id": pid,
            "name": _text(t.get("name")),
            "status": status if status in ("open", "closed", "deleted") else "closed",
            "start_date_time": m._parse_epoch(t.get("startDateTime")),
            "end_date_time": m._parse_epoch(t.get("endDateTime")),
            "application_start_date": m._parse_epoch(t.get("applicationStartDate")),
            "application_end_date": m._parse_epoch(t.get("applicationEndDate")),
        }, row)
    report.sampled["program_terms"] = min(len(candidates), size)


def _member_target(member) -> tuple | None:
    """The (program, user, type, status) the importer writes, or None when it skips the row."""
    raw_type = (member.get("memberType") or "").strip().lower()
    raw_status = (member.get("status") or "").strip().lower() or None
    member_type = m._MEMBER_TYPE_MAP.get(raw_type)
    if member_type is None or (member_type == "mentor" and raw_status in {"pending", "declined", "rejected"}):
        return None
    status = m._MEMBER_STATUS_MAP.get(raw_status, raw_status) if raw_status else None
    if status not in m._VALID_MEMBER_STATUSES:
        return None
    return m._as_uuid(member.get("projectId")), m._as_uuid(member.get("userId")), member_type, status


def verify_members(cur, members, program_ids, report, rng, size):
    # The importer upserts on (program, user, type), so the last row for a key wins.
    final: dict = {}
    for member in members:
        if not (m._as_uuid(member.get("id")) and m._as_uuid(member.get("projectId")) and m._as_uuid(member.get("userId"))):
            continue
        target = _member_target(member)
        if target and target[0] in program_ids:
            final[target[:3]] = (target[3], _text(member.get("email")))
    for key in _sample(sorted(final), rng, size):
        row = _fetch(cur, """SELECT status, email FROM program_members
            WHERE program_id = %s AND user_id = %s AND member_type = %s""", key)
        if row is None:
            report.missing("program_members", key)
            continue
        status, email = final[key]
        report.compare("program_members", key, {"status": status, "email": email}, row)
    report.sampled["program_members"] = min(len(final), size)


def verify_applications(cur, mentees, term_ids, report, rng, size) -> set:
    """Returns the (term, user) pairs the importer gave an application, for the task check."""
    best: dict = {}
    for item in mentees:
        mid, term_id, uid = (m._as_uuid(item.get(k)) for k in ("id", "programTermId", "userId"))
        if not (mid and term_id and uid) or term_id not in term_ids:
            continue
        status = m._map_application_status(item.get("status") or "pending")
        updated = m._parse_ts(item.get("updatedOn"))
        rank = (_APP_PRIORITY.get(status, -1), updated.timestamp() if updated else float("-inf"), mid)
        if (term_id, uid) not in best or rank > best[(term_id, uid)][0]:
            best[(term_id, uid)] = (rank, status, item)
    for key in _sample(sorted(best), rng, size):
        _, status, item = best[key]
        cur.execute("""SELECT status, start_date_time, end_date_time, tasks_submitted, admin_notified
            FROM applications WHERE program_term_id = %s AND user_id = %s AND role = 'mentee'""", key)
        rows = [dict(zip([c.name for c in cur.description], r)) for r in cur.fetchall()]
        if not rows:
            report.missing("applications", key)
            continue
        if len(rows) > 1:
            report.problems.append(f"applications {key}: {len(rows)} mentee rows; the importer writes one per term and user")
            continue
        report.compare("applications", key, {
            "status": status,
            "start_date_time": m._parse_epoch(item.get("startDateTime")),
            "end_date_time": m._parse_epoch(item.get("endDateTime")),
            "tasks_submitted": m._as_bool(item.get("tasksSubmitted")),
            "admin_notified": m._as_bool(item.get("adminNotified")),
        }, rows[0])
    report.sampled["applications"] = min(len(best), size)
    return set(best)


def verify_tasks(cur, tasks, term_ids, applied: set, report, rng, size):
    candidates = [t for t in tasks if m._as_uuid(t.get("id")) and m._as_uuid(t.get("assigneeId"))]
    columns = "assignee_id::text, owner_id::text, program_term_id::text, name, description, category, status, custom, created_on"
    for t in _sample(candidates, rng, size):
        tid = m._as_uuid(t.get("id"))
        term_id, assignee = m._as_uuid(t.get("programTermId")), m._as_uuid(t.get("assigneeId"))
        live = term_id is not None and (term_id, assignee) in applied
        table = "tasks" if live else "quarantined_tasks"
        row = _fetch(cur, f"SELECT {columns} FROM {table} WHERE id = %s", (tid,))
        if row is None:
            report.missing(table, tid)
            continue
        status = (t.get("status") or "incomplete").lower()
        category = _text(t.get("category"))
        report.compare(table, tid, {
            "assignee_id": assignee,
            "owner_id": m._as_uuid(t.get("ownerId")),
            "program_term_id": term_id if term_id in term_ids else None,
            "name": _text(t.get("name")),
            "description": _text(t.get("description")),
            "category": category if category in m._VALID_TASK_CATEGORIES else None,
            "status": status if status in _TASK_STATUSES else "incomplete",
            "custom": m._as_bool(t.get("custom")),
            "created_on": m._parse_ts(t.get("createdOn")),
        }, row)
    report.sampled["tasks"] = min(len(candidates), size)


def check_integrity(cur, report):
    for name, sql in INTEGRITY_CHECKS.items():
        cur.execute(f"SELECT count(*) FROM ({sql}) AS broken")
        count = cur.fetchone()[0]
        if count:
            report.problems.append(f"integrity: {count} {name}")


def verify(cur, sources: dict, sample_size: int, rng: random.Random) -> Report:
    """Compare sampled legacy items with the imported rows; sources maps table name to scanned items."""
    report = Report()
    program_ids = {m._as_uuid(p.get("projectId")) for p in sources["projects"]} - {None}
    term_ids = {
        m._as_uuid(t.get("id"))
        for t in sources["program-terms"]
        if m._as_uuid(t.get("id")) and m._as_uuid(t.get("projectId")) in program_ids
    }
    verify_users(cur, sources["users"], report, rng, sample_size)
    verify_profiles(cur, sources["user-profiles"], report, rng, sample_size)
    verify_programs(cur, sources["projects"], report, rng, sample_size)
    verify_terms(cur, sources["program-terms"], program_ids, report, rng, sample_size)
    verify_members(cur, sources["project-members"], program_ids, report, rng, sample_size)
    applied = verify_applications(cur, sources["program-term-mentees"], term_ids, report, rng, sample_size)
    verify_tasks(cur, sources["tasks"], term_ids, applied, report, rng, sample_size)
    check_integrity(cur, report)
    return report


def main() -> None:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    dynamo = m.legacy_session().client("dynamodb")
    sources = {name: m.scan_table(dynamo, f"{m.TABLE_PREFIX}-{name}") for name in (
        "users", "user-profiles", "projects", "program-terms", "project-members", "program-term-mentees", "tasks")}
    # The import linked members through the same resolution, so compare against its result.
    sources["project-members"] = m.resolve_member_user_ids(
        sources["project-members"], sources["users"], sources["projects"], m.load_member_user_overrides(m.MEMBER_USER_OVERRIDES))

    seed = SEED if SEED is not None else str(random.randrange(1 << 32))
    log.info("Sampling up to %d rows per table (VERIFY_SEED=%s)", SAMPLE_SIZE, seed)
    conn = psycopg2.connect(m.PG_DSN)
    try:
        with conn.cursor() as cur:
            report = verify(cur, sources, SAMPLE_SIZE, random.Random(seed))
    finally:
        conn.rollback()
        conn.close()

    for table, count in report.sampled.items():
        log.info("checked %-16s %d rows", table, count)
    for problem in report.problems:
        log.error("MISMATCH %s", problem)
    if report.problems:
        log.error("Verification failed: %d mismatches", len(report.problems))
        sys.exit(1)
    log.info("Verification passed: no mismatches")


if __name__ == "__main__":
    main()
