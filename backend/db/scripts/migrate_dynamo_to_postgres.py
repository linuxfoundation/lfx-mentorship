#!/usr/bin/env python3
# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT


"""
DynamoDB → PostgreSQL Migration Script
=======================================
Migrates all jobspring-prod-* DynamoDB tables into the PostgreSQL schema
defined in backend/db/migrations/001_initial.up.sql.

Source → Target mapping
-----------------------
  jobspring-prod-users                → users
  jobspring-prod-user-profiles        → user_profiles
                                        (recordKind='github-profile-reservation'
                                        rows are skipped)
  jobspring-prod-projects             → programs
                                      → program_skills   (apprenticeNeeds.skills[])
                                      → program_funding_stats (amountRaised)
  jobspring-prod-program-terms        → program_terms
  jobspring-prod-project-members      → program_members
                                      → program_admins   (memberType='maintainer')
  jobspring-prod-program-term-mentees → applications     (all status values, mapped)
                                      → enrollments      (active|graduated|withdrawn|hold)
  jobspring-prod-tasks                → tasks
                                        (enrollment_id resolved via
                                        (program_term_id, assignee_id) lookup)

Key notes
---------
- All DynamoDB IDs are already valid UUIDs; they are used directly as Postgres PKs.
- startDateTime / endDateTime in program-terms are Unix epoch strings (seconds).
- user-profiles rows with recordKind='github-profile-reservation' are skipped.
- program-term-mentees status lifecycle:
    pending | accepted | declined | withdrawn → applications.status (mapped below)
    active | graduated | withdrawn | hold     → enrollments.status
  A mentee record where status ∈ {active, graduated, hold} also gets an
  applications row with status='accepted'.
- tasks.enrollment_id is resolved post-scan by matching (program_term_id, assignee_id)
  against the enrollments that were just inserted. Tasks whose enrollment cannot
  be resolved get enrollment_id=NULL (the column is nullable).
- All INSERTs use ON CONFLICT … DO UPDATE (idempotent; safe to re-run).

Usage
-----
  export AWS_ACCESS_KEY_ID=...
  export AWS_SECRET_ACCESS_KEY=...
  export AWS_SESSION_TOKEN=...          # for STS / temporary credentials
  export AWS_REGION=us-east-1

  export PG_DSN="host=localhost port=5432 dbname=mentorship user=postgres password=..."

  pip install boto3 psycopg2-binary
  python3 backend/db/scripts/migrate_dynamo_to_postgres.py
"""

import json
import logging
import os
import re
import sys
import uuid
from datetime import datetime, timezone
from decimal import Decimal

import boto3
import psycopg2
import psycopg2.extras
from boto3.dynamodb.types import TypeDeserializer as _TypeDeserializer

# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
)
log = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
REGION = os.environ.get("AWS_REGION", "us-east-1")
PG_DSN = os.environ.get(
    "PG_DSN",
    "host=localhost port=5432 dbname=mentorship user=postgres password=postgres",
)

TABLE_PREFIX = "jobspring-prod"

# Stable UUID namespace — must not change between runs to keep IDs deterministic.
_UUID_NS = uuid.UUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8")

# ---------------------------------------------------------------------------
# DynamoDB helpers
# ---------------------------------------------------------------------------
_deser = _TypeDeserializer()


def _deserialize(item: dict) -> dict:
    return {k: _deser.deserialize(v) for k, v in item.items()}


def scan_table(client, table_name: str) -> list:
    """Full table scan with automatic pagination."""
    log.info("Scanning %-55s", table_name + " ...")
    items: list = []
    kwargs: dict = {"TableName": table_name}
    while True:
        resp = client.scan(**kwargs)
        items.extend(_deserialize(raw) for raw in resp.get("Items", []))
        lek = resp.get("LastEvaluatedKey")
        if not lek:
            break
        kwargs["ExclusiveStartKey"] = lek
    log.info("  → %d items", len(items))
    return items


# ---------------------------------------------------------------------------
# General helpers
# ---------------------------------------------------------------------------


def _uuid5(scope: str, *parts) -> str:
    key = "|".join(str(p) for p in parts)
    return str(uuid.uuid5(_UUID_NS, f"{scope}:{key}"))


def _as_uuid(value) -> str | None:
    """Return a valid UUID string or None; coerce non-UUID strings via uuid5."""
    if value is None:
        return None
    s = str(value).strip()
    if not s:
        return None
    try:
        return str(uuid.UUID(s))
    except ValueError:
        return _uuid5("coerce", s)


def _as_int(value, default: int = 0) -> int:
    if value is None:
        return default
    if isinstance(value, Decimal):
        return int(value)
    try:
        return int(value)
    except (TypeError, ValueError):
        return default


def _as_float(value, default: float = 0.0) -> float:
    if value is None:
        return default
    if isinstance(value, Decimal):
        return float(value)
    try:
        return float(value)
    except (TypeError, ValueError):
        return default


def _as_bool(value, default: bool = False) -> bool:
    if value is None:
        return default
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        return value.lower() in ("true", "1", "yes")
    return bool(value)


def _to_jsonb(value) -> str | None:
    if value is None:
        return None

    def _default(obj):
        if isinstance(obj, Decimal):
            return float(obj)
        raise TypeError(f"Cannot serialize {type(obj)}")

    return json.dumps(value, default=_default)


def _parse_ts(s) -> datetime | None:
    """Parse a DynamoDB timestamp string → tz-aware datetime, or None."""
    if not s:
        return None
    # Unix epoch stored as string (e.g. "1757919600")
    if isinstance(s, (int, float, Decimal)):
        return datetime.fromtimestamp(float(s), tz=timezone.utc)
    cleaned = str(s).strip()
    if cleaned.isdigit():
        return datetime.fromtimestamp(int(cleaned), tz=timezone.utc)
    # Strip Go monotonic clock suffix
    cleaned = re.sub(r"\s+UTC\s+m=[+-][\d.]+$", "", cleaned)
    # Truncate sub-second precision to microseconds
    cleaned = re.sub(r"(\.\d{6})\d+", r"\1", cleaned)
    for fmt in (
        "%Y-%m-%d %H:%M:%S.%f %z",
        "%Y-%m-%d %H:%M:%S %z",
        "%Y-%m-%dT%H:%M:%S.%f%z",
        "%Y-%m-%dT%H:%M:%S%z",
        "%Y-%m-%d %H:%M:%S.%f",
        "%Y-%m-%d %H:%M:%S",
        "%Y-%m-%dT%H:%M:%S",
        "%Y-%m-%d",
    ):
        try:
            dt = datetime.strptime(cleaned, fmt)
            if dt.tzinfo is None:
                dt = dt.replace(tzinfo=timezone.utc)
            return dt
        except ValueError:
            continue
    log.warning("Could not parse timestamp: %r", s)
    return None


def _parse_epoch(s) -> datetime | None:
    """Parse a Unix epoch string (seconds) → tz-aware datetime, or None."""
    if s is None:
        return None
    try:
        return datetime.fromtimestamp(float(str(s)), tz=timezone.utc)
    except (ValueError, OSError, OverflowError):
        return None


def _redact_dsn(dsn: str) -> str:
    return re.sub(r"password=\S+", "password=***", dsn)


def _normalize_program_status(status: str | None) -> str:
    """Map DynamoDB project status to Postgres programs.status."""
    if not status:
        return "pending"
    m = {"published": "published", "pending": "pending", "archived": "archived"}
    return m.get(status.lower(), "pending")


def _map_application_status(dynamo_status: str | None) -> str:
    """Map full mentee lifecycle status → applications.status."""
    s = (dynamo_status or "pending").lower()
    # active/graduated/hold → were accepted before enrollment
    if s in ("active", "graduated", "hold"):
        return "accepted"
    if s in ("declined",):
        return "declined"
    if s in ("withdrawn",):
        return "withdrawn"
    if s in ("accepted",):
        return "accepted"
    return "pending"


def _is_enrollment_status(dynamo_status: str | None) -> bool:
    """Return True if the mentee row represents an active/graduated enrollment."""
    return (dynamo_status or "").lower() in ("active", "graduated", "withdrawn", "hold")


def _map_enrollment_status(dynamo_status: str | None) -> str:
    s = (dynamo_status or "active").lower()
    mapping = {
        "active": "active",
        "graduated": "graduated",
        "withdrawn": "withdrawn",
        "hold": "hold",
    }
    return mapping.get(s, "active")


# ---------------------------------------------------------------------------
# Migration: users
# ---------------------------------------------------------------------------


def migrate_users(cur, users: list) -> set:
    """Upsert users; return set of known user IDs."""
    log.info("Migrating users (%d rows) ...", len(users))
    rows = []
    ids: set = set()
    for u in users:
        uid = _as_uuid(u.get("id"))
        if not uid:
            continue
        ids.add(uid)
        rows.append(
            (
                uid,
                (u.get("email") or "").strip() or None,
                (u.get("lfid") or "").strip() or None,
                (u.get("name") or "").strip() or None,
                (u.get("givenName") or "").strip() or None,
                (u.get("familyName") or "").strip() or None,
                (u.get("avatarUrl") or "").strip() or None,
                _parse_ts(u.get("createdAt")),
                _parse_ts(u.get("updatedAt")),
            )
        )

    psycopg2.extras.execute_batch(
        cur,
        """
        INSERT INTO users
          (id, email, lfid, name, given_name, family_name, avatar_url, created_on, updated_on)
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
        ON CONFLICT (id) DO UPDATE SET
          email       = EXCLUDED.email,
          lfid        = EXCLUDED.lfid,
          name        = EXCLUDED.name,
          given_name  = EXCLUDED.given_name,
          family_name = EXCLUDED.family_name,
          avatar_url  = EXCLUDED.avatar_url,
          updated_on  = EXCLUDED.updated_on
        """,
        rows,
        page_size=500,
    )
    log.info("  → %d users upserted", len(rows))
    return ids


# ---------------------------------------------------------------------------
# Migration: user_profiles
# ---------------------------------------------------------------------------


def migrate_user_profiles(cur, profiles: list, known_user_ids: set) -> dict:
    """
    Upsert user_profiles; return {user_profile_id: user_id} for program_admins.
    Rows with recordKind='github-profile-reservation' are skipped.
    """
    log.info("Migrating user_profiles (%d raw rows) ...", len(profiles))
    rows = []
    profile_map: dict = {}  # profile_id → user_id
    skipped = 0

    for p in profiles:
        if p.get("recordKind") == "github-profile-reservation":
            skipped += 1
            continue
        pid = _as_uuid(p.get("id"))
        uid = _as_uuid(p.get("userId"))
        if not pid:
            skipped += 1
            continue
        # Insert placeholder user if user_id is referenced but not in users table
        if uid and uid not in known_user_ids:
            cur.execute(
                """
                INSERT INTO users (id, email, created_on, updated_on)
                VALUES (%s, %s, NOW(), NOW())
                ON CONFLICT (id) DO NOTHING
                """,
                (uid, f"placeholder-{uid}@placeholder.invalid"),
            )
            known_user_ids.add(uid)

        profile_map[pid] = uid
        rows.append(
            (
                pid,
                uid,
                (p.get("type") or "apprentice").strip(),
                (p.get("slug") or "").strip() or None,
                (p.get("firstName") or "").strip() or None,
                (p.get("lastName") or "").strip() or None,
                (p.get("email") or "").strip() or None,
                (p.get("phone") or "").strip() or None,
                (p.get("logoUrl") or "").strip() or None,
                (p.get("introduction") or "").strip() or None,
                _as_bool(p.get("termsAndConditions")),
                _as_int(p.get("numberOfProjects")),
                _to_jsonb(p.get("address")),
                _to_jsonb(p.get("demographics")),
                _to_jsonb(p.get("socioeconomics")),
                _to_jsonb(p.get("skillSet")),
                _to_jsonb(p.get("profileLinks")),
                _parse_ts(p.get("createdAt")),
                _parse_ts(p.get("updatedAt")),
            )
        )

    psycopg2.extras.execute_batch(
        cur,
        """
        INSERT INTO user_profiles
          (id, user_id, profile_type, slug, first_name, last_name, email, phone,
           logo_url, introduction, terms_and_conditions, number_of_projects,
           address, demographics, socioeconomics, skill_set, profile_links,
           created_on, updated_on)
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
        ON CONFLICT (id) DO UPDATE SET
          user_id              = EXCLUDED.user_id,
          profile_type         = EXCLUDED.profile_type,
          slug                 = EXCLUDED.slug,
          first_name           = EXCLUDED.first_name,
          last_name            = EXCLUDED.last_name,
          email                = EXCLUDED.email,
          phone                = EXCLUDED.phone,
          logo_url             = EXCLUDED.logo_url,
          introduction         = EXCLUDED.introduction,
          terms_and_conditions = EXCLUDED.terms_and_conditions,
          number_of_projects   = EXCLUDED.number_of_projects,
          address              = EXCLUDED.address,
          demographics         = EXCLUDED.demographics,
          socioeconomics       = EXCLUDED.socioeconomics,
          skill_set            = EXCLUDED.skill_set,
          profile_links        = EXCLUDED.profile_links,
          updated_on           = EXCLUDED.updated_on
        """,
        rows,
        page_size=500,
    )
    log.info("  → %d user_profiles upserted, %d skipped", len(rows), skipped)
    return profile_map


# ---------------------------------------------------------------------------
# Migration: programs + program_skills + program_funding_stats
# ---------------------------------------------------------------------------


def migrate_programs(cur, projects: list, known_user_ids: set) -> set:
    """
    Upsert programs from jobspring-prod-projects.
    Also populates program_skills and program_funding_stats.
    Returns set of known program IDs.
    """
    log.info("Migrating programs (%d rows) ...", len(projects))
    prog_rows = []
    skill_rows = []
    funding_rows = []
    program_ids: set = set()

    for p in projects:
        pid = _as_uuid(p.get("projectId"))
        if not pid:
            continue
        program_ids.add(pid)

        amount = _as_float(p.get("amountRaised"))
        prog_rows.append(
            (
                pid,
                (p.get("name") or "").strip() or None,
                (p.get("slug") or "").strip() or None,
                _normalize_program_status(p.get("status")),
                False,  # is_paid — not captured in DynamoDB; assume false
                (p.get("description") or "").strip() or None,
                (p.get("logoUrl") or "").strip() or None,
                (p.get("websiteUrl") or "").strip() or None,
                (p.get("repoLink") or "").strip() or None,
                (p.get("codeOfConduct") or "").strip() or None,
                (p.get("industry") or "").strip() or None,
                (p.get("color") or "").strip() or None,
                (p.get("lfid") or "").strip() or None,
                (p.get("projectCIIProjectId") or "").strip() or None,
                _as_bool(p.get("acceptApplications")),
                _as_bool(p.get("termsAndConditions")),
                (p.get("programTermStatus") or "").strip() or None,
                _as_int(p.get("discoverSortRank")),
                amount,
                _to_jsonb(p.get("apprenticeNeeds")),
                _to_jsonb(p.get("taskTemplates")),
                _parse_ts(p.get("createdOn")),
                _parse_ts(p.get("updatedOn")),
            )
        )

        # Normalise skills from apprenticeNeeds.skills[]
        needs = p.get("apprenticeNeeds") or {}
        for skill in needs.get("skills") or []:
            if skill and str(skill).strip():
                skill_rows.append(
                    (
                        str(uuid.uuid5(_UUID_NS, f"skill:{pid}|{skill}")),
                        pid,
                        str(skill).strip(),
                    )
                )

        # Funding stats — one row per program
        funding_rows.append(
            (
                str(uuid.uuid5(_UUID_NS, f"funding:{pid}")),
                pid,
                amount,
            )
        )

    psycopg2.extras.execute_batch(
        cur,
        """
        INSERT INTO programs
          (id, name, slug, status, is_paid, description, logo_url, website_url,
           repo_link, code_of_conduct, industry, color, lfid, cii_project_id,
           accept_applications, terms_and_conditions, program_term_status,
           discover_sort_rank, amount_raised, apprentice_needs, task_templates,
           created_on, updated_on)
        VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)
        ON CONFLICT (id) DO UPDATE SET
          name                = EXCLUDED.name,
          slug                = EXCLUDED.slug,
          status              = EXCLUDED.status,
          is_paid             = EXCLUDED.is_paid,
          description         = EXCLUDED.description,
          logo_url            = EXCLUDED.logo_url,
          website_url         = EXCLUDED.website_url,
          repo_link           = EXCLUDED.repo_link,
          code_of_conduct     = EXCLUDED.code_of_conduct,
          industry            = EXCLUDED.industry,
          color               = EXCLUDED.color,
          lfid                = EXCLUDED.lfid,
          cii_project_id      = EXCLUDED.cii_project_id,
          accept_applications = EXCLUDED.accept_applications,
          terms_and_conditions = EXCLUDED.terms_and_conditions,
          program_term_status = EXCLUDED.program_term_status,
          discover_sort_rank  = EXCLUDED.discover_sort_rank,
          amount_raised       = EXCLUDED.amount_raised,
          apprentice_needs    = EXCLUDED.apprentice_needs,
          task_templates      = EXCLUDED.task_templates,
          updated_on          = EXCLUDED.updated_on
        """,
        prog_rows,
        page_size=500,
    )
    log.info("  → %d programs upserted", len(prog_rows))

    psycopg2.extras.execute_batch(
        cur,
        """
        INSERT INTO program_skills (id, program_id, skill)
        VALUES (%s, %s, %s)
        ON CONFLICT (program_id, skill) DO NOTHING
        """,
        skill_rows,
        page_size=500,
    )
    log.info("  → %d program_skills upserted", len(skill_rows))

    psycopg2.extras.execute_batch(
        cur,
        """
        INSERT INTO program_funding_stats (id, program_id, amount_raised)
        VALUES (%s, %s, %s)
        ON CONFLICT (program_id) DO UPDATE SET
          amount_raised = EXCLUDED.amount_raised,
          updated_on    = NOW()
        """,
        funding_rows,
        page_size=500,
    )
    log.info("  → %d program_funding_stats upserted", len(funding_rows))
    return program_ids


# ---------------------------------------------------------------------------
# Migration: program_terms
# ---------------------------------------------------------------------------


def migrate_program_terms(cur, terms: list, known_program_ids: set) -> set:
    """Upsert program_terms; return set of known term IDs."""
    log.info("Migrating program_terms (%d rows) ...", len(terms))
    rows = []
    term_ids: set = set()
    skipped = 0

    for t in terms:
        tid = _as_uuid(t.get("id"))
        pid = _as_uuid(t.get("projectId"))
        if not tid or not pid:
            skipped += 1
            continue
        if pid not in known_program_ids:
            log.warning("  program_term %s references unknown program %s — skipping", tid, pid)
            skipped += 1
            continue
        term_ids.add(tid)
        status = (t.get("Active") or "open").lower()
        if status not in ("open", "closed"):
            status = "closed"
        rows.append(
            (
                tid,
                pid,
                (t.get("name") or "").strip() or None,
                status,
                _as_int(t.get("activeUsers")),
                _parse_epoch(t.get("startDateTime")),
                _parse_epoch(t.get("endDateTime")),
                _parse_epoch(t.get("applicationStartDate")),
                _parse_epoch(t.get("applicationEndDate")),
                _parse_ts(t.get("createdOn")),
                _parse_ts(t.get("updatedOn")),
            )
        )

    psycopg2.extras.execute_batch(
        cur,
        """
        INSERT INTO program_terms
          (id, program_id, name, status, active_users, start_date_time,
           end_date_time, application_start_date, application_end_date,
           created_on, updated_on)
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
        ON CONFLICT (id) DO UPDATE SET
          program_id            = EXCLUDED.program_id,
          name                  = EXCLUDED.name,
          status                = EXCLUDED.status,
          active_users          = EXCLUDED.active_users,
          start_date_time       = EXCLUDED.start_date_time,
          end_date_time         = EXCLUDED.end_date_time,
          application_start_date = EXCLUDED.application_start_date,
          application_end_date  = EXCLUDED.application_end_date,
          updated_on            = EXCLUDED.updated_on
        """,
        rows,
        page_size=500,
    )
    log.info("  → %d program_terms upserted, %d skipped", len(rows), skipped)
    return term_ids


# ---------------------------------------------------------------------------
# Migration: program_members + program_admins
# ---------------------------------------------------------------------------


def migrate_program_members(
    cur,
    members: list,
    known_program_ids: set,
    known_user_ids: set,
    profile_map: dict,
) -> None:
    """
    Upsert program_members from jobspring-prod-project-members.
    Also creates program_admins rows for maintainer-type members whose
    user_id has a matching user_profile.
    """
    log.info("Migrating program_members (%d rows) ...", len(members))
    member_rows = []
    admin_rows = []
    skipped = 0

    # Build a reverse map: user_id → profile_id (first profile found)
    user_to_profile: dict = {uid: pid for pid, uid in profile_map.items() if uid}

    for m in members:
        mid = _as_uuid(m.get("id"))
        pid = _as_uuid(m.get("projectId"))
        uid = _as_uuid(m.get("userId"))
        if not mid or not pid or not uid:
            skipped += 1
            continue
        if pid not in known_program_ids:
            skipped += 1
            continue
        if uid not in known_user_ids:
            cur.execute(
                """
                INSERT INTO users (id, email, created_on, updated_on)
                VALUES (%s, %s, NOW(), NOW())
                ON CONFLICT (id) DO NOTHING
                """,
                (uid, f"placeholder-{uid}@placeholder.invalid"),
            )
            known_user_ids.add(uid)

        member_type = (m.get("memberType") or "mentor").strip()
        status = (m.get("status") or "").strip() or None
        member_rows.append(
            (
                mid,
                pid,
                uid,
                member_type,
                status,
                (m.get("email") or "").strip() or None,
                _parse_ts(m.get("createdOn")),
                _parse_ts(m.get("updatedOn")),
            )
        )

        # Maintainers → program_admins (if they have a profile)
        if member_type == "maintainer":
            profile_id = user_to_profile.get(uid)
            if profile_id:
                admin_rows.append(
                    (
                        str(uuid.uuid5(_UUID_NS, f"admin:{pid}|{profile_id}")),
                        pid,
                        profile_id,
                        "maintainer",
                    )
                )

    psycopg2.extras.execute_batch(
        cur,
        """
        INSERT INTO program_members
          (id, program_id, user_id, member_type, status, email, created_on, updated_on)
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s)
        ON CONFLICT (program_id, user_id, member_type) DO UPDATE SET
          status     = EXCLUDED.status,
          email      = EXCLUDED.email,
          updated_on = EXCLUDED.updated_on
        """,
        member_rows,
        page_size=500,
    )
    log.info("  → %d program_members upserted, %d skipped", len(member_rows), skipped)

    if admin_rows:
        psycopg2.extras.execute_batch(
            cur,
            """
            INSERT INTO program_admins (id, program_id, user_profile_id, role)
            VALUES (%s, %s, %s, %s)
            ON CONFLICT (program_id, user_profile_id) DO NOTHING
            """,
            admin_rows,
            page_size=500,
        )
        log.info("  → %d program_admins upserted", len(admin_rows))


# ---------------------------------------------------------------------------
# Migration: applications + enrollments
# ---------------------------------------------------------------------------


def migrate_mentees(
    cur,
    mentees: list,
    known_term_ids: set,
    known_user_ids: set,
) -> dict:
    """
    Upsert applications and enrollments from jobspring-prod-program-term-mentees.
    Returns {(program_term_id, mentee_user_id): enrollment_id} for task resolution.
    """
    log.info("Migrating applications + enrollments (%d rows) ...", len(mentees))
    app_rows = []
    enroll_rows = []
    enrollment_index: dict = {}
    skipped = 0

    for m in mentees:
        mid = _as_uuid(m.get("id"))
        term_id = _as_uuid(m.get("programTermId"))
        uid = _as_uuid(m.get("userId"))
        if not mid or not term_id or not uid:
            skipped += 1
            continue
        if term_id not in known_term_ids:
            skipped += 1
            continue
        if uid not in known_user_ids:
            cur.execute(
                """
                INSERT INTO users (id, email, created_on, updated_on)
                VALUES (%s, %s, NOW(), NOW())
                ON CONFLICT (id) DO NOTHING
                """,
                (uid, f"placeholder-{uid}@placeholder.invalid"),
            )
            known_user_ids.add(uid)

        dynamo_status = m.get("status") or "pending"
        term_status = (m.get("programTermStatus") or "").strip() or None
        tasks_submitted = _as_bool(m.get("tasksSubmitted"))
        admin_notified = _as_bool(m.get("adminNotified"))
        created_on = _parse_ts(m.get("createdOn"))
        updated_on = _parse_ts(m.get("updatedOn"))

        # --- applications row ---
        app_status = _map_application_status(dynamo_status)
        app_rows.append(
            (
                mid,
                term_id,
                uid,
                "mentee",
                app_status,
                term_status,
                tasks_submitted,
                admin_notified,
                created_on,
                updated_on,
            )
        )

        # --- enrollments row (only post-acceptance statuses) ---
        if _is_enrollment_status(dynamo_status):
            enroll_status = _map_enrollment_status(dynamo_status)
            enroll_rows.append(
                (
                    mid,  # reuse same UUID as application for simplicity
                    term_id,
                    uid,
                    enroll_status,
                    term_status,
                    _parse_epoch(m.get("startDateTime")),
                    _parse_epoch(m.get("endDateTime")),
                    tasks_submitted,
                    admin_notified,
                    created_on,
                    updated_on,
                )
            )
            enrollment_index[(term_id, uid)] = mid

    psycopg2.extras.execute_batch(
        cur,
        """
        INSERT INTO applications
          (id, program_term_id, user_id, role, status, program_term_status,
           tasks_submitted, admin_notified, created_on, updated_on)
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
        ON CONFLICT (program_term_id, user_id, role) DO UPDATE SET
          status              = EXCLUDED.status,
          program_term_status = EXCLUDED.program_term_status,
          tasks_submitted     = EXCLUDED.tasks_submitted,
          admin_notified      = EXCLUDED.admin_notified,
          updated_on          = EXCLUDED.updated_on
        """,
        app_rows,
        page_size=500,
    )
    log.info("  → %d applications upserted, %d skipped", len(app_rows), skipped)

    psycopg2.extras.execute_batch(
        cur,
        """
        INSERT INTO enrollments
          (id, program_term_id, mentee_user_id, status, program_term_status,
           start_date_time, end_date_time, tasks_submitted, admin_notified,
           created_on, updated_on)
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
        ON CONFLICT (program_term_id, mentee_user_id) DO UPDATE SET
          status              = EXCLUDED.status,
          program_term_status = EXCLUDED.program_term_status,
          start_date_time     = EXCLUDED.start_date_time,
          end_date_time       = EXCLUDED.end_date_time,
          tasks_submitted     = EXCLUDED.tasks_submitted,
          admin_notified      = EXCLUDED.admin_notified,
          updated_on          = EXCLUDED.updated_on
        """,
        enroll_rows,
        page_size=500,
    )
    log.info("  → %d enrollments upserted", len(enroll_rows))
    return enrollment_index


# ---------------------------------------------------------------------------
# Migration: tasks
# ---------------------------------------------------------------------------


def migrate_tasks(
    cur,
    tasks: list,
    enrollment_index: dict,
    known_term_ids: set,
    known_user_ids: set,
) -> None:
    """Upsert tasks; resolve enrollment_id via (program_term_id, assignee_id)."""
    log.info("Migrating tasks (%d rows) ...", len(tasks))
    rows = []
    skipped = 0
    unresolved = 0

    for t in tasks:
        tid = _as_uuid(t.get("id"))
        term_id = _as_uuid(t.get("programTermId"))
        assignee_id = _as_uuid(t.get("assigneeId"))
        owner_id = _as_uuid(t.get("ownerId"))
        if not tid or not assignee_id:
            skipped += 1
            continue

        # Ensure referenced users exist
        for uid in [assignee_id, owner_id]:
            if uid and uid not in known_user_ids:
                cur.execute(
                    """
                    INSERT INTO users (id, email, created_on, updated_on)
                    VALUES (%s, %s, NOW(), NOW())
                    ON CONFLICT (id) DO NOTHING
                    """,
                    (uid, f"placeholder-{uid}@placeholder.invalid"),
                )
                known_user_ids.add(uid)

        enrollment_id = enrollment_index.get((term_id, assignee_id)) if term_id else None
        if not enrollment_id:
            unresolved += 1

        # term_id FK must exist
        resolved_term_id = term_id if term_id in known_term_ids else None

        dynamo_status = (t.get("status") or "incomplete").lower()
        valid_statuses = {"incomplete", "in_progress", "complete", "submitted"}
        status = dynamo_status if dynamo_status in valid_statuses else "incomplete"

        due_date_raw = (t.get("dueDate") or "").strip() or None
        due_date = None
        if due_date_raw:
            try:
                due_date = datetime.strptime(due_date_raw, "%Y-%m-%d").date()
            except ValueError:
                pass

        rows.append(
            (
                tid,
                enrollment_id,
                resolved_term_id,
                assignee_id,
                owner_id,
                (t.get("name") or "").strip() or None,
                (t.get("description") or "").strip() or None,
                (t.get("category") or "").strip() or None,
                status,
                (t.get("applicationStatus") or "").strip() or None,
                (t.get("programTermStatus") or "").strip() or None,
                _as_bool(t.get("custom")),
                (t.get("submitFile") or "").strip() or None,
                (t.get("file") or "").strip() or None,
                due_date,
                (t.get("createdBy") or "").strip() or None,
                _parse_ts(t.get("createdOn")),
                _parse_ts(t.get("updatedOn")),
            )
        )

    psycopg2.extras.execute_batch(
        cur,
        """
        INSERT INTO tasks
          (id, enrollment_id, program_term_id, assignee_id, owner_id, name,
           description, category, status, application_status, program_term_status,
           custom, submit_file, file, due_date, created_by, created_on, updated_on)
        VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)
        ON CONFLICT (id) DO UPDATE SET
          enrollment_id       = EXCLUDED.enrollment_id,
          program_term_id     = EXCLUDED.program_term_id,
          assignee_id         = EXCLUDED.assignee_id,
          owner_id            = EXCLUDED.owner_id,
          name                = EXCLUDED.name,
          description         = EXCLUDED.description,
          category            = EXCLUDED.category,
          status              = EXCLUDED.status,
          application_status  = EXCLUDED.application_status,
          program_term_status = EXCLUDED.program_term_status,
          custom              = EXCLUDED.custom,
          submit_file         = EXCLUDED.submit_file,
          file                = EXCLUDED.file,
          due_date            = EXCLUDED.due_date,
          created_by          = EXCLUDED.created_by,
          updated_on          = EXCLUDED.updated_on
        """,
        rows,
        page_size=500,
    )
    log.info(
        "  → %d tasks upserted, %d skipped, %d with unresolved enrollment_id",
        len(rows),
        skipped,
        unresolved,
    )


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------


def main() -> None:
    log.info("Connecting to DynamoDB (region=%s) ...", REGION)
    dynamo = boto3.client("dynamodb", region_name=REGION)

    log.info("Connecting to PostgreSQL: %s", _redact_dsn(PG_DSN))
    conn = psycopg2.connect(PG_DSN)
    psycopg2.extras.register_uuid()

    try:
        # ── 1. Scan all DynamoDB tables ──────────────────────────────────────
        users_raw        = scan_table(dynamo, f"{TABLE_PREFIX}-users")
        profiles_raw     = scan_table(dynamo, f"{TABLE_PREFIX}-user-profiles")
        projects_raw     = scan_table(dynamo, f"{TABLE_PREFIX}-projects")
        terms_raw        = scan_table(dynamo, f"{TABLE_PREFIX}-program-terms")
        members_raw      = scan_table(dynamo, f"{TABLE_PREFIX}-project-members")
        mentees_raw      = scan_table(dynamo, f"{TABLE_PREFIX}-program-term-mentees")
        tasks_raw        = scan_table(dynamo, f"{TABLE_PREFIX}-tasks")

        # ── 2. Migrate in FK dependency order ───────────────────────────────
        with conn:
            with conn.cursor() as cur:
                known_user_ids   = migrate_users(cur, users_raw)
                conn.commit()

                profile_map      = migrate_user_profiles(cur, profiles_raw, known_user_ids)
                conn.commit()

                known_program_ids = migrate_programs(cur, projects_raw, known_user_ids)
                conn.commit()

                known_term_ids   = migrate_program_terms(cur, terms_raw, known_program_ids)
                conn.commit()

                migrate_program_members(cur, members_raw, known_program_ids, known_user_ids, profile_map)
                conn.commit()

                enrollment_index = migrate_mentees(cur, mentees_raw, known_term_ids, known_user_ids)
                conn.commit()

                migrate_tasks(cur, tasks_raw, enrollment_index, known_term_ids, known_user_ids)
                conn.commit()

        log.info("Migration complete.")

    except Exception:
        conn.rollback()
        log.exception("Migration failed — rolled back.")
        sys.exit(1)
    finally:
        conn.close()


if __name__ == "__main__":
    main()
