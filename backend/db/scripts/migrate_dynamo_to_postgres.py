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
                                      → program_skills   (menteeNeeds.skills[])
                                      → program_funding_stats (amountRaised)
  jobspring-prod-program-terms        → program_terms
  jobspring-prod-project-members      → program_members
  jobspring-prod-program-term-mentees → applications     (full lifecycle;
                                        graduated|hold map directly)
  jobspring-prod-tasks                → tasks
                                        (application_id resolved via
                                        (program_term_id, assignee_id) lookup)

Key notes
---------
- All DynamoDB IDs are already valid UUIDs; they are used directly as Postgres PKs.
- startDateTime / endDateTime in program-terms are Unix epoch strings (seconds).
- user-profiles rows with recordKind='github-profile-reservation' are skipped.
- The enrollments table no longer exists; applications now covers the full mentee
  lifecycle (pending → accepted → graduated|withdrawn).
- attendance_type is not captured in DynamoDB; it is migrated as NULL.
- DynamoDB member status "approved" maps to program_members.status "active".
- DynamoDB mentee status "approved" (and "active") maps to applications.status
  "accepted" — applications.status has no "active" value.
- DynamoDB user-profile type for mentees maps to Postgres profile_type "mentee".
- applications.program_term_status and tasks.program_term_status are taken from
  the migrated term status (open|closed), not the stale legacy copy. Rows on
  deleted terms keep the legacy value.
- tasks.application_id is resolved post-scan by matching (program_term_id, assignee_id)
  against inserted applications. Tasks with no match are written to
  quarantined_tasks for repair; tasks.application_id is NOT NULL. A task that was
  live from an earlier run is removed with FGA delete_access and index deleted
  markers, so its tuples and search document are retracted.
- All INSERTs use ON CONFLICT … DO UPDATE (idempotent; safe to re-run).
- File columns (users.avatar_url, user_profiles.logo_url, programs.logo_url,
  tasks.file) are rewritten from the copy manifest copy_legacy_objects.py
  writes, so run that first: legacy-bucket URLs become CDN URLs (logos) or
  object keys (submissions), quarantined or missing objects become NULL,
  foreign logo URLs carry through and foreign tasks.file values are nulled.
- profile_links.resumeLink is dropped: resumes are not migrated.
- programs.lf_project_uid comes from the first project identifier that
  project-service confirms exists: lfProjectId first (a v1 Salesforce ID,
  translated over NATS lfx.lookup_v1_mapping), then legacy UUID fields.
  lf_project_slug and lf_project_name come from project-service, and
  lf_project_logo_url from lfProjectLogo when lfProjectId is the parent. A
  program with no confirmed parent is reported as UNMAPPED_PROGRAM; an existing
  one keeps its current parent. A project-service error other than not_found
  aborts the import.

Usage
-----
  export AWS_ACCESS_KEY_ID=...
  export AWS_SECRET_ACCESS_KEY=...
  export AWS_SESSION_TOKEN=...          # for STS / temporary credentials
  export AWS_REGION=us-east-1
    export DYNAMODB_TABLE_PREFIX=jobspring-dev  # defaults to jobspring-prod

  export PG_DSN="host=localhost port=5432 dbname=mentorship user=postgres password=..."
  export COPY_MANIFEST=legacy-object-manifest.json
  export LOGOS_CDN_URL_PREFIX=https://...
  export LOGOS_S3_BUCKET=... ATTACHMENTS_S3_BUCKET=...   # must match the manifest
  export NATS_URL=nats://...   # platform NATS, to verify project parents; unset leaves programs unmapped

  pip install boto3 psycopg2-binary nats-py
  python3 backend/db/scripts/copy_legacy_objects.py
  python3 backend/db/scripts/migrate_dynamo_to_postgres.py
"""

import json
import asyncio
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

import legacy_objects as lo

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

TABLE_PREFIX = os.environ.get("DYNAMODB_TABLE_PREFIX", "jobspring-prod")
LEGACY_BUCKET = os.environ.get("LEGACY_UPLOADS_BUCKET", f"{TABLE_PREFIX}-uploads")
COPY_MANIFEST = os.environ.get("COPY_MANIFEST", "legacy-object-manifest.json")
LOGOS_CDN_URL_PREFIX = os.environ.get("LOGOS_CDN_URL_PREFIX", "")
# Must name the buckets copy_legacy_objects.py copied into; the manifest records them.
LOGOS_S3_BUCKET = os.environ.get("LOGOS_S3_BUCKET", "").strip()
ATTACHMENTS_S3_BUCKET = os.environ.get("ATTACHMENTS_S3_BUCKET", "").strip()
NATS_URL = os.environ.get("NATS_URL", "").strip()

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
    page = 0
    while True:
        resp = client.scan(**kwargs)
        batch = [_deserialize(raw) for raw in resp.get("Items", [])]
        items.extend(batch)
        page += 1
        log.info("  page %d — %d items so far", page, len(items))
        lek = resp.get("LastEvaluatedKey")
        if not lek:
            break
        kwargs["ExclusiveStartKey"] = lek
    log.info("  → %d items total", len(items))
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


def _strict_uuid(value) -> str | None:
    """Return a canonical UUID string, or None; never coerce, so no parent is fabricated."""
    try:
        return str(uuid.UUID(str(value).strip())) if value else None
    except ValueError:
        return None


def _project_candidates(p: dict) -> list[tuple[str, str]]:
    """(field, raw value) pairs that may name the program's LF project, in priority order."""
    linked = p.get("project") if isinstance(p.get("project"), dict) else {}
    fields = [(f, p.get(f)) for f in ("lfProjectId", "projectUid", "lfProjectUid", "lfProjectUID")] + [("project.id", linked.get("id"))]
    return [(f, str(v).strip()) for f, v in fields if v and str(v).strip()]


def resolve_lf_projects(projects: list) -> dict:
    """Map each candidate project identifier to a verified v2 (uid, slug, name), or None.

    A v1 SFID is translated over lfx.lookup_v1_mapping; a UUID is taken as-is. Either
    way project-service must confirm the project exists, so an identifier fabricated
    by an older patch run, or naming a deleted project, resolves to None.
    """
    values = sorted({v for p in projects for _, v in _project_candidates(p)})
    if not values:
        return {}
    if not NATS_URL:
        log.warning("NATS_URL is not set: %d project identifiers cannot be verified; programs stay unmapped", len(values))
        return {}

    async def lookup() -> dict:
        import nats  # imported lazily so runs without project identifiers need no NATS client

        nc = await nats.connect(NATS_URL)
        try:
            async def ask(subject: str, body: str) -> str:
                return (await nc.request(subject, body.encode(), timeout=5)).data.decode().strip()

            async def project_field(subject: str, uid: str) -> str | None:
                reply = await ask(subject, uid)
                if not reply:
                    raise RuntimeError(f"{subject} {uid}: empty reply")
                # project-service answers failures with a JSON error body instead of a value.
                try:
                    body = json.loads(reply)
                except ValueError:
                    return reply
                if not isinstance(body, dict):
                    return reply
                if body.get("error") == "not_found":
                    return None
                raise RuntimeError(f"{subject} {uid}: {reply}")

            async def v1_project_uid(sfid: str) -> str | None:
                reply = await ask("lfx.lookup_v1_mapping", f"project.sfid.{sfid}")
                # An empty reply means no mapping; anything else that is not a UUID is a lookup failure.
                if reply and not _strict_uuid(reply):
                    raise RuntimeError(f"lfx.lookup_v1_mapping project.sfid.{sfid}: {reply}")
                return _strict_uuid(reply)

            resolved: dict = {}
            for value in values:
                uid = _strict_uuid(value) or await v1_project_uid(value)
                slug = await project_field("lfx.projects-api.get_slug", uid) if uid else None
                # A project deleted between the two lookups answers get_name with not_found.
                name = await project_field("lfx.projects-api.get_name", uid) if slug else None
                if not name:
                    log.warning("UNMAPPED_LF_PROJECT project_identifier=%s", value)
                    resolved[value] = None
                    continue
                resolved[value] = (uid, slug, name)
            return resolved
        finally:
            await nc.close()

    resolved = asyncio.run(lookup())
    log.info("  → %d of %d project identifiers verified in project-service", sum(1 for r in resolved.values() if r), len(values))
    return resolved


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


def _without_resume_link(links):
    """Drop resumeLink: resumes are not migrated (docs/rewrite/02 §file classes)."""
    if isinstance(links, dict):
        return {k: v for k, v in links.items() if k != "resumeLink"}
    return links


def _to_jsonb(value) -> str | None:
    if value is None:
        return None

    def _default(obj):
        if isinstance(obj, Decimal):
            return float(obj)
        raise TypeError(f"Cannot serialize {type(obj)}")

    # PostgreSQL rejects \u0000 inside JSON text columns
    return json.dumps(value, default=_default).replace("\\u0000", "")


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
    m = {
        "draft": "pending",
        "pending": "pending",
        "submitted": "submitted",
        "published": "published",
        "rejected": "rejected",
        "archived": "archived",
        "hidden": "hidden",
    }
    return m.get(status.lower(), "pending")


_VALID_APP_STATUSES = {"pending", "accepted", "declined", "withdrawn", "graduated", "hold"}
# DynamoDB used "approved" and "active" for an enrolled mentee; applications.status
# has no "active" value — "accepted" is the enrolled state until graduation.
_APP_STATUS_MAP = {"approved": "accepted", "active": "accepted"}


def _map_application_status(dynamo_status: str | None) -> str:
    """Map DynamoDB mentee status → applications.status."""
    s = (dynamo_status or "pending").lower()
    s = _APP_STATUS_MAP.get(s, s)
    return s if s in _VALID_APP_STATUSES else "pending"


def _map_profile_type(dynamo_type: str | None) -> str:
    """Map DynamoDB user-profile type → user_profiles.profile_type."""
    if (dynamo_type or "").strip().lower() == "mentor":
        return "mentor"
    return "mentee"


def _map_program_term_status(term_status: str | None, dynamo_value: str | None) -> str | None:
    """Derive the denormalised program_term_status from the migrated term status.

    The legacy copy on mentees and tasks is stale on many closed terms because
    the legacy cron's status push often did not finish. Deleted terms keep the
    legacy value.
    """
    if term_status in ("open", "closed"):
        return term_status
    return (dynamo_value or "").strip() or None


# ---------------------------------------------------------------------------
# Migration: users
# ---------------------------------------------------------------------------


def migrate_users(cur, users: list, files: lo.LegacyFileRewriter) -> set:
    """Upsert users; return set of known user IDs."""
    log.info("Migrating users (%d rows) ...", len(users))
    rows = []
    ids: set = set()
    seen_emails: set = set()
    seen_lfids: set = set()
    for u in users:
        uid = _as_uuid(u.get("id"))
        if not uid:
            continue
        ids.add(uid)
        email = (u.get("email") or "").strip() or None
        lfid = (u.get("lfid") or "").strip() or None
        # null out duplicate emails/lfids — DynamoDB had no unique enforcement
        if email is not None:
            if email in seen_emails:
                email = None
            else:
                seen_emails.add(email)
        if lfid is not None:
            if lfid in seen_lfids:
                lfid = None
            else:
                seen_lfids.add(lfid)
        rows.append(
            (
                uid,
                email,
                lfid,
                (u.get("name") or "").strip() or None,
                (u.get("givenName") or "").strip() or None,
                (u.get("familyName") or "").strip() or None,
                files.rewrite("users.avatar_url", lo.LOGO, u.get("avatarUrl")),
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


def migrate_user_profiles(cur, profiles: list, known_user_ids: set, files: lo.LegacyFileRewriter) -> dict:
    """
    Upsert user_profiles; return {user_profile_id: user_id} for program_admins.
    Rows with recordKind='github-profile-reservation' are skipped, and a user
    with several mentee profiles keeps only the newest by (createdAt, id).
    """
    log.info("Migrating user_profiles (%d raw rows) ...", len(profiles))
    rows = []
    profile_map: dict = {}  # profile_id → user_id
    seen_slugs: set = set()
    skipped = 0
    duplicates = 0

    # uq_user_profiles_user_type allows one mentee profile per user.
    mentee_keeper: dict = {}  # user_id → (created_on, profile_id)
    for p in profiles:
        if p.get("recordKind") == "github-profile-reservation" or _map_profile_type(p.get("type")) != "mentee":
            continue
        pid, uid = _as_uuid(p.get("id")), _as_uuid(p.get("userId"))
        if pid and uid:
            key = (_parse_ts(p.get("createdAt")) or datetime.min.replace(tzinfo=timezone.utc), pid)
            if uid not in mentee_keeper or key > mentee_keeper[uid]:
                mentee_keeper[uid] = key

    for p in profiles:
        if p.get("recordKind") == "github-profile-reservation":
            skipped += 1
            continue
        pid = _as_uuid(p.get("id"))
        uid = _as_uuid(p.get("userId"))
        if not pid:
            skipped += 1
            continue
        if uid in mentee_keeper and _map_profile_type(p.get("type")) == "mentee" and mentee_keeper[uid][1] != pid:
            duplicates += 1
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

        slug = (p.get("slug") or "").strip() or None
        # null out duplicate slugs — DynamoDB had no unique enforcement
        if slug is not None:
            if slug in seen_slugs:
                slug = None
            else:
                seen_slugs.add(slug)

        profile_map[pid] = uid
        rows.append(
            (
                pid,
                uid,
                _map_profile_type(p.get("type")),
                slug,
                (p.get("firstName") or "").strip() or None,
                (p.get("lastName") or "").strip() or None,
                (p.get("email") or "").strip() or None,
                (p.get("phone") or "").strip() or None,
                files.rewrite("user_profiles.logo_url", lo.LOGO, p.get("logoUrl")),
                (p.get("introduction") or "").strip() or None,
                _as_bool(p.get("termsAndConditions")),
                _as_int(p.get("numberOfProjects")),
                _to_jsonb(p.get("address")),
                _to_jsonb(p.get("demographics")),
                _to_jsonb(p.get("socioeconomics")),
                _to_jsonb(p.get("skillSet")),
                _to_jsonb(_without_resume_link(p.get("profileLinks"))),
                _parse_ts(p.get("createdAt")),
                _parse_ts(p.get("updatedAt")),
            )
        )

    # A rerun can pick a different keeper than an earlier run stored.
    if mentee_keeper:
        cur.execute(
            """
            DELETE FROM user_profiles AS p
            USING unnest(%s::uuid[], %s::uuid[]) AS k(user_id, id)
            WHERE p.profile_type = 'mentee' AND p.user_id = k.user_id AND p.id <> k.id
            """,
            (list(mentee_keeper), [pid for _, pid in mentee_keeper.values()]),
        )
        duplicates += cur.rowcount

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
    log.info("  → %d user_profiles upserted, %d skipped, %d duplicate mentee profiles dropped", len(rows), skipped, duplicates)
    return profile_map


# ---------------------------------------------------------------------------
# Migration: programs + program_skills + program_funding_stats
# ---------------------------------------------------------------------------


def migrate_programs(cur, projects: list, known_user_ids: set, files: lo.LegacyFileRewriter, lf_projects: dict | None = None) -> set:
    """
    Upsert programs from jobspring-prod-projects.
    Also populates program_skills and program_funding_stats.
    lf_projects maps an lfProjectId (v1 SFID) to its (uid, slug, name) in v2.
    Returns set of known program IDs.
    """
    lf_projects = lf_projects or {}
    log.info("Migrating programs (%d rows) ...", len(projects))
    prog_rows = []
    skill_rows = []
    funding_rows = []
    program_ids: set = set()
    seen_slugs: set = set()
    unresolved_project_uids = []
    unresolved_project_mappings = []

    for p in projects:
        pid = _as_uuid(p.get("projectId"))
        if not pid:
            continue
        program_ids.add(pid)

        # project_uid is the LF project parent used by the authorization
        # inheritance chain. Do not substitute the program ID when the legacy
        # source does not provide an explicit project identifier.
        # Only a parent project-service confirms is imported; slug and name come from it too.
        chosen_field, lf_project = next(
            ((f, lf_projects[v]) for f, v in _project_candidates(p) if lf_projects.get(v) and lf_projects[v][0] != pid),
            (None, None),
        )
        project_uid, project_slug, project_name = lf_project or (None, None, None)
        if not project_uid:
            unresolved_project_uids.append(pid)
        # lfProjectLogo describes the lfProjectId project, so it only applies when that field won.
        project_logo_url = str(p.get("lfProjectLogo") or "").strip() or None if chosen_field == "lfProjectId" else None
        if not project_uid or not project_slug or not project_name:
            unresolved_project_mappings.append((pid, project_uid, project_slug, project_name))

        amount = _as_float(p.get("amountRaised")) / 100  # DynamoDB stores cents; convert to dollars

        # Generate slug from name if missing; ensure uniqueness
        slug = (p.get("slug") or "").strip() or None
        if not slug:
            name = (p.get("name") or "").strip()
            slug = re.sub(r"[^a-z0-9]+", "-", name.lower()).strip("-") or str(pid)
        base_slug = slug
        suffix = 1
        while slug in seen_slugs:
            slug = f"{base_slug}-{suffix}"
            suffix += 1
        seen_slugs.add(slug)

        prog_rows.append(
            (
                pid,
                project_uid,
                project_slug,
                project_name,
                project_logo_url,
                (p.get("name") or "").strip() or None,
                slug,
                _normalize_program_status(p.get("status")),
                False,  # is_paid — not captured in DynamoDB; assume false
                (p.get("description") or "").strip() or None,
                files.rewrite("programs.logo_url", lo.LOGO, p.get("logoUrl")),
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
                _to_jsonb(p.get("menteeNeeds")),  # Legacy apprenticeNeeds fallback is retained only for cutover imports.
                _to_jsonb(p.get("taskTemplates")),
                _parse_ts(p.get("createdOn")),
                _parse_ts(p.get("updatedOn")),
            )
        )

        # Keep the legacy field fallback until the final DynamoDB export is retired.
        needs = p.get("menteeNeeds") or p.get("apprenticeNeeds") or {}
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

    if unresolved_project_uids:
        log.warning(
            "%d programs are missing an explicit project UID; new ones are imported "
            "without an authorization parent and existing ones keep theirs",
            len(unresolved_project_uids),
        )
        for program_id in unresolved_project_uids:
            log.warning("UNMAPPED_PROGRAM program_id=%s", program_id)
    for program_id, project_uid, project_slug, project_name in unresolved_project_mappings:
        log.warning(
            "UNMAPPED_PROGRAM_PROJECT_MAPPING program_id=%s project_uid=%s project_slug=%s project_name_present=%s",
            program_id,
            project_uid or "",
            project_slug or "",
            bool(project_name),
        )

    psycopg2.extras.execute_batch(
        cur,
        """
        INSERT INTO programs
                    (id, lf_project_uid, lf_project_slug, lf_project_name, lf_project_logo_url, name, slug, status, is_paid, description, logo_url, website_url,
           repo_link, code_of_conduct, industry, color, lfid, cii_project_id,
           accept_applications, terms_and_conditions, program_term_status,
           discover_sort_rank, amount_raised, mentee_needs, task_templates,
           created_on, updated_on)
                    VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)
        ON CONFLICT (id) DO UPDATE SET
                    -- An unmapped re-import keeps the existing parent: OpenFGA already holds it, and the sync cannot clear a project reference.
                    lf_project_uid     = COALESCE(EXCLUDED.lf_project_uid, programs.lf_project_uid),
          lf_project_slug        = CASE WHEN EXCLUDED.lf_project_uid IS NULL THEN programs.lf_project_slug ELSE EXCLUDED.lf_project_slug END,
          lf_project_name        = CASE WHEN EXCLUDED.lf_project_uid IS NULL THEN programs.lf_project_name ELSE EXCLUDED.lf_project_name END,
          lf_project_logo_url   = CASE WHEN EXCLUDED.lf_project_uid IS NULL
                                         OR (EXCLUDED.lf_project_logo_url IS NULL AND EXCLUDED.lf_project_uid = programs.lf_project_uid)
                                       THEN programs.lf_project_logo_url ELSE EXCLUDED.lf_project_logo_url END,
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
          mentee_needs        = EXCLUDED.mentee_needs,
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


def migrate_program_terms(cur, terms: list, known_program_ids: set) -> dict:
    """Upsert program_terms; return {term_id: status} for known terms."""
    log.info("Migrating program_terms (%d rows) ...", len(terms))
    rows = []
    term_ids: dict = {}
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
        status = (t.get("Active") or "open").lower()
        if status not in ("open", "closed", "deleted"):
            status = "closed"
        term_ids[tid] = status
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


_MEMBER_TYPE_MAP = {
    "maintainer": "program_admin",
    "program_admin": "program_admin",
    "mentor": "mentor",
}
_VALID_MEMBER_STATUSES = {"invited", "requested", "pending", "active", "declined", "withdrawn"}
# DynamoDB used "approved" for accepted mentors; Postgres stores that as "active".
_MEMBER_STATUS_MAP = {"accepted": "active", "approved": "active"}


def migrate_program_members(
    cur,
    members: list,
    known_program_ids: set,
    known_user_ids: set,
) -> list:
    """Upsert program_members from jobspring-prod-project-members."""
    log.info("Migrating program_members (%d rows) ...", len(members))
    member_rows = []
    term_scoped_members = []
    skipped = 0

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

        raw_member_type = (m.get("memberType") or "").strip().lower()
        raw_status = (m.get("status") or "").strip().lower() or None
        if raw_member_type in {"apprentice", "mentee"}:
            term_scoped_members.append((mid, pid, uid, raw_status))
            skipped += 1
            continue
        member_type = _MEMBER_TYPE_MAP.get(raw_member_type)
        if member_type is None:
            log.warning(
                "UNMAPPED_MEMBER_TYPE member_id=%s program_id=%s user_id=%s member_type=%r",
                mid,
                pid,
                uid,
                raw_member_type,
            )
            skipped += 1
            continue
        if member_type == "mentor" and raw_status in {"pending", "declined", "rejected"}:
            log.warning(
                "UNMAPPED_MENTOR_MEMBER member_id=%s program_id=%s user_id=%s status=%s",
                mid,
                pid,
                uid,
                raw_status,
            )
            skipped += 1
            continue
        mapped_status = _MEMBER_STATUS_MAP.get(raw_status, raw_status) if raw_status else None
        if mapped_status not in _VALID_MEMBER_STATUSES:
            log.warning(
                "UNMAPPED_MEMBER_STATUS member_id=%s program_id=%s user_id=%s status=%r",
                mid,
                pid,
                uid,
                raw_status,
            )
            skipped += 1
            continue
        status = mapped_status
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
    return term_scoped_members


# ---------------------------------------------------------------------------
# Migration: applications + enrollments
# ---------------------------------------------------------------------------

# One mentee application id per (term, user), with the one that decides eligibility last so it
# wins when collected into a dict: a user can hold withdrawn applications beside a reapplication
# (uq_applications_active), so the live one wins, else the newest withdrawn.
_MENTEE_APPLICATIONS_BY_PRECEDENCE = (
    "SELECT program_term_id::text, user_id::text, id::text FROM applications WHERE role = 'mentee'"
    " ORDER BY status <> 'withdrawn', created_on, id"
)


def migrate_mentees(
    cur,
    mentees: list,
    known_term_ids: dict,
    known_user_ids: set,
) -> dict:
    """
    Upsert applications from jobspring-prod-program-term-mentees.
    Returns {(program_term_id, user_id): application_id} for task resolution.
    """
    log.info("Migrating applications (%d rows) ...", len(mentees))
    app_rows = []
    application_index: dict = {}
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
        app_rows.append(
            (
                mid,
                term_id,
                uid,
                "mentee",
                _map_application_status(dynamo_status),
                _map_program_term_status(known_term_ids[term_id], m.get("programTermStatus")),
                _parse_epoch(m.get("startDateTime")),
                _parse_epoch(m.get("endDateTime")),
                _as_bool(m.get("tasksSubmitted")),
                _as_bool(m.get("adminNotified")),
                None,  # attendance_type — not captured in DynamoDB
                _parse_ts(m.get("createdOn")),
                _parse_ts(m.get("updatedOn")),
            )
        )
        application_index[(term_id, uid)] = mid

    # Deterministic dedup: for duplicate (term, user) rows keep the highest-status row;
    # break ties by newest updated_on so scan-order cannot change the outcome.
    _STATUS_PRIORITY = {
        "graduated": 5, "accepted": 4,
        "hold": 3, "pending": 2, "declined": 1, "withdrawn": 0,
    }
    best: dict = {}
    for row in app_rows:
        key = (row[1], row[2])  # (program_term_id, user_id)
        prev = best.get(key)
        if prev is None:
            best[key] = row
        else:
            prev_sort = (
                _STATUS_PRIORITY.get(prev[4], -1),
                prev[12].timestamp() if prev[12] is not None else float("-inf"),
                prev[0],
            )
            row_sort = (
                _STATUS_PRIORITY.get(row[4], -1),
                row[12].timestamp() if row[12] is not None else float("-inf"),
                row[0],
            )
            if row_sort > prev_sort:
                best[key] = row

    # Each (term, user) keeps the row a previous run wrote for it: the live one, else the newest
    # withdrawn. Upserting by that id rather than the winner's own lets a rerun whose winner
    # changed update the row in place instead of adding a second live row (uq_applications_active).
    cur.execute(_MENTEE_APPLICATIONS_BY_PRECEDENCE)
    existing = {(row[0], row[1]): row[2] for row in cur.fetchall()}
    # created_on and updated_on are NOT NULL; legacy records can lack either, so fall back on
    # the other one, then on the time of this run.
    migrated_on = datetime.now(timezone.utc)
    app_rows = [
        (existing.get(key, row[0]),) + row[1:11]
        + (row[11] or row[12] or migrated_on, row[12] or row[11] or migrated_on)
        for key, row in best.items()
    ]

    psycopg2.extras.execute_batch(
        cur,
        """
        INSERT INTO applications
          (id, program_term_id, user_id, role, status, program_term_status,
           start_date_time, end_date_time, tasks_submitted, admin_notified,
           attendance_type, created_on, updated_on)
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
        ON CONFLICT (id) DO UPDATE SET
          status              = EXCLUDED.status,
          program_term_status = EXCLUDED.program_term_status,
          start_date_time     = EXCLUDED.start_date_time,
          end_date_time       = EXCLUDED.end_date_time,
          tasks_submitted     = EXCLUDED.tasks_submitted,
          admin_notified      = EXCLUDED.admin_notified,
          updated_on          = EXCLUDED.updated_on
        """,
        app_rows,
        page_size=500,
    )
    log.info("  → %d applications upserted, %d skipped", len(app_rows), skipped)

    # Rebuild index from DB so ON CONFLICT winners are used for task resolution.
    cur.execute(_MENTEE_APPLICATIONS_BY_PRECEDENCE)
    application_index = {(row[0], row[1]): row[2] for row in cur.fetchall()}

    return application_index


def reconcile_term_scoped_members(cur, term_scoped_members: list) -> None:
    """Report legacy mentee aliases that have no canonical term application."""
    cur.execute(
        """
        SELECT program_terms.program_id::text, applications.user_id::text
        FROM applications
        JOIN program_terms ON program_terms.id = applications.program_term_id
        WHERE applications.role = 'mentee'
        """
    )
    canonical_members = {(program_id, user_id) for program_id, user_id in cur.fetchall()}
    unmatched = []
    for member_id, program_id, user_id, status in term_scoped_members:
        if (program_id, user_id) not in canonical_members:
            unmatched.append((member_id, program_id, user_id, status))
    for member_id, program_id, user_id, status in unmatched:
        log.warning(
            "UNMAPPED_TERM_SCOPED_MEMBER member_id=%s program_id=%s user_id=%s status=%s",
            member_id,
            program_id,
            user_id,
            status,
        )
    log.info(
        "  → %d term-scoped member rows reconciled, %d unmatched",
        len(term_scoped_members) - len(unmatched),
        len(unmatched),
    )


# ---------------------------------------------------------------------------
# Migration: tasks
# ---------------------------------------------------------------------------


_VALID_TASK_CATEGORIES = {"prerequisite", "non_prerequisite"}


def migrate_tasks(
    cur,
    tasks: list,
    application_index: dict,
    known_term_ids: dict,
    known_user_ids: set,
    files: lo.LegacyFileRewriter,
) -> None:
    """Upsert tasks; resolve application_id via (program_term_id, assignee_id)."""
    log.info("Migrating tasks (%d rows) ...", len(tasks))
    rows = []
    quarantined = []
    skipped = 0
    unresolved_tasks = []

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

        application_id = application_index.get((term_id, assignee_id)) if term_id else None
        if not application_id:
            unresolved_tasks.append((tid, term_id, assignee_id))

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

        raw_category = (t.get("category") or "").strip() or None
        category = raw_category if raw_category in _VALID_TASK_CATEGORIES else None

        row = (
            tid,
            application_id,
            resolved_term_id,
            assignee_id,
            owner_id,
            (t.get("name") or "").strip() or None,
            (t.get("description") or "").strip() or None,
            category,
            status,
            (t.get("applicationStatus") or "").strip() or None,
            _map_program_term_status(known_term_ids.get(resolved_term_id), t.get("programTermStatus")),
            _as_bool(t.get("custom")),
            (t.get("submitFile") or "").strip() or None,
            files.rewrite("tasks.file", lo.SUBMISSION, t.get("file")),
            due_date,
            (t.get("createdBy") or "").strip() or None,
            _parse_ts(t.get("createdOn")),
            _parse_ts(t.get("updatedOn")),
        )
        (rows if application_id else quarantined).append(row)

    if unresolved_tasks:
        log.warning(
            "%d tasks could not be linked to an application and are "
            "quarantined in quarantined_tasks for repair",
            len(unresolved_tasks),
        )
        for task_id, program_term_id, assignee_id in unresolved_tasks:
            log.warning(
                "UNMAPPED_TASK task_id=%s program_term_id=%s assignee_id=%s",
                task_id,
                program_term_id,
                assignee_id,
            )

    columns = (
        "id, application_id, program_term_id, assignee_id, owner_id, name, "
        "description, category, status, application_status, program_term_status, "
        "custom, submit_file, file, due_date, created_by, created_on, updated_on"
    )
    updates = ",\n          ".join(
        f"{column} = EXCLUDED.{column}"
        for column in (name.strip() for name in columns.split(","))
        if column not in ("id", "created_on")
    )
    placeholders = ",".join(["%s"] * len(columns.split(",")))
    if rows:
        psycopg2.extras.execute_batch(
            cur,
            f"""
            INSERT INTO tasks ({columns})
            VALUES ({placeholders})
            ON CONFLICT (id) DO UPDATE SET
              {updates}
            """,
            rows,
            page_size=500,
        )
    if quarantined:
        # A task imported live earlier was already published, so retract it through both relays.
        cur.execute(
            """
            WITH removed AS (
                DELETE FROM tasks WHERE id = ANY(%s::uuid[]) RETURNING id
            ), fga AS (
                INSERT INTO fga_outbox (marker_kind, object_type, object_uid, desired_operation)
                SELECT 'object', 'mentorship_task', id::text, 'delete_access' FROM removed
                ON CONFLICT (object_type, object_uid) WHERE marker_kind = 'object'
                DO UPDATE SET desired_operation = EXCLUDED.desired_operation,
                              generation = fga_outbox.generation + 1,
                              state = CASE WHEN fga_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END,
                              claimed_generation = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_generation ELSE NULL END,
                              claimed_at = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_at ELSE NULL END,
                              attempts = 0, next_attempt_at = NOW(), last_error = NULL,
                              updated_on = NOW()
            )
            INSERT INTO index_outbox (object_type, object_uid, action, headers)
            SELECT 'mentorship_task', id, 'deleted', '{}'::jsonb FROM removed
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
                sent_on = NULL
            """,
            ([row[0] for row in quarantined],),
        )
        psycopg2.extras.execute_batch(
            cur,
            f"""
            INSERT INTO quarantined_tasks ({columns}, quarantine_reason)
            VALUES ({placeholders}, 'missing application_id')
            ON CONFLICT (id) DO UPDATE SET
              {updates},
              quarantined_on = NOW()
            """,
            quarantined,
            page_size=500,
        )
    # A task that now has a parent in tasks is never also held in quarantine.
    cur.execute(
        "DELETE FROM quarantined_tasks USING tasks WHERE quarantined_tasks.id = tasks.id"
    )
    log.info(
        "  → %d tasks upserted, %d skipped, %d quarantined without an application",
        len(rows),
        skipped,
        len(quarantined),
    )


def seed_derived_state(cur) -> None:
    """Queue current-state FGA and index snapshots through the normal relays."""
    log.info("Queuing derived-state seeds ...")
    cur.execute(
        """
        SELECT programs.id, program_members.id, program_members.user_id
        FROM programs
        JOIN program_members ON program_members.program_id = programs.id
        JOIN users ON users.id = program_members.user_id
        WHERE program_members.status = 'active'
          AND program_members.member_type IN ('program_admin', 'mentor')
          AND NULLIF(users.lfid, '') IS NULL
        ORDER BY programs.id, program_members.id
        """
    )
    unresolved_members = cur.fetchall()
    for program_id, member_id, user_id in unresolved_members:
        log.warning(
            "UNMAPPED_PROGRAM_MEMBER program_id=%s member_id=%s user_id=%s",
            program_id,
            member_id,
            user_id,
        )

    cur.execute(
        """
        SELECT applications.id, applications.user_id
        FROM applications
        JOIN users ON users.id = applications.user_id
        WHERE NULLIF(users.lfid, '') IS NULL
        ORDER BY applications.id
        """
    )
    unresolved_applications = cur.fetchall()
    for application_id, user_id in unresolved_applications:
        log.warning(
            "UNMAPPED_APPLICATION_USER application_id=%s user_id=%s",
            application_id,
            user_id,
        )

    cur.execute(
        """
        SELECT tasks.id, tasks.assignee_id
        FROM tasks
        JOIN users ON users.id = tasks.assignee_id
        WHERE tasks.application_id IS NOT NULL
          AND NULLIF(users.lfid, '') IS NULL
        ORDER BY tasks.id
        """
    )
    unresolved_task_users = cur.fetchall()
    for task_id, assignee_id in unresolved_task_users:
        log.warning(
            "UNMAPPED_TASK_ASSIGNEE task_id=%s assignee_id=%s",
            task_id,
            assignee_id,
        )

    cur.execute(
        """
        SELECT mentorship_approver_team_members.user_id
        FROM mentorship_approver_team_members
        JOIN users ON users.id = mentorship_approver_team_members.user_id
        WHERE NULLIF(users.lfid, '') IS NULL
        ORDER BY mentorship_approver_team_members.user_id
        """
    )
    for (user_id,) in cur.fetchall():
        log.warning("UNMAPPED_APPROVER_USER user_id=%s", user_id)

    cur.execute(
            """
            WITH seed AS (
                SELECT 'mentorship_program'::text AS object_type, id::text AS object_uid
      FROM programs AS program
    WHERE lf_project_uid IS NOT NULL
        AND NOT EXISTS (
          SELECT 1
          FROM program_members
          JOIN users ON users.id = program_members.user_id
          WHERE program_members.program_id = program.id
            AND program_members.status = 'active'
            AND program_members.member_type IN ('program_admin', 'mentor')
            AND NULLIF(users.lfid, '') IS NULL
        )
                UNION ALL
                SELECT 'mentorship_application', applications.id::text
                FROM applications
                JOIN users ON users.id = applications.user_id
                WHERE NULLIF(users.lfid, '') IS NOT NULL
                UNION ALL
                SELECT 'mentorship_task', tasks.id::text
                FROM tasks
                JOIN users ON users.id = tasks.assignee_id
                WHERE tasks.application_id IS NOT NULL
                    AND NULLIF(users.lfid, '') IS NOT NULL
            )
            INSERT INTO fga_outbox
                (marker_kind, object_type, object_uid, desired_operation)
            SELECT 'object', object_type, object_uid, 'update_access'
            FROM seed
            ON CONFLICT (object_type, object_uid) WHERE marker_kind = 'object'
            DO UPDATE SET
                desired_operation = CASE WHEN fga_outbox.state = 'dead_letter' THEN fga_outbox.desired_operation ELSE 'update_access' END,
                generation = CASE WHEN fga_outbox.state = 'dead_letter' THEN fga_outbox.generation ELSE fga_outbox.generation + 1 END,
                state = CASE WHEN fga_outbox.state IN ('in_flight', 'dead_letter') THEN fga_outbox.state ELSE 'pending' END,
                claimed_generation = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_generation ELSE NULL END,
                claimed_at = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_at ELSE NULL END,
                attempts = CASE WHEN fga_outbox.state = 'dead_letter' THEN fga_outbox.attempts ELSE 0 END,
                next_attempt_at = CASE WHEN fga_outbox.state = 'dead_letter' THEN fga_outbox.next_attempt_at ELSE NOW() END,
                last_error = CASE WHEN fga_outbox.state = 'dead_letter' THEN fga_outbox.last_error ELSE NULL END,
                updated_on = NOW()
            RETURNING object_type
            """
    )
    fga_counts = {}
    for (object_type,) in cur.fetchall():
            fga_counts[object_type] = fga_counts.get(object_type, 0) + 1
    log.info(
            "  → FGA seeds queued: programs=%d applications=%d tasks=%d",
            fga_counts.get("mentorship_program", 0),
            fga_counts.get("mentorship_application", 0),
            fga_counts.get("mentorship_task", 0),
    )

    cur.execute(
            """
            INSERT INTO fga_outbox
                (marker_kind, object_type, object_uid, relation, username, desired_operation)
            SELECT
                'membership',
                'mentorship_approver_team',
                'global',
                'member',
                users.lfid,
                'sync'
            FROM mentorship_approver_team_members
            JOIN users ON users.id = mentorship_approver_team_members.user_id
            WHERE NULLIF(users.lfid, '') IS NOT NULL
            ON CONFLICT (object_type, object_uid, relation, username)
                WHERE marker_kind = 'membership'
            DO UPDATE SET
                desired_operation = 'sync',
                generation = CASE WHEN fga_outbox.state = 'dead_letter' THEN fga_outbox.generation ELSE fga_outbox.generation + 1 END,
                state = CASE WHEN fga_outbox.state IN ('in_flight', 'dead_letter') THEN fga_outbox.state ELSE 'pending' END,
                claimed_generation = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_generation ELSE NULL END,
                claimed_at = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_at ELSE NULL END,
                attempts = CASE WHEN fga_outbox.state = 'dead_letter' THEN fga_outbox.attempts ELSE 0 END,
                next_attempt_at = CASE WHEN fga_outbox.state = 'dead_letter' THEN fga_outbox.next_attempt_at ELSE NOW() END,
                last_error = CASE WHEN fga_outbox.state = 'dead_letter' THEN fga_outbox.last_error ELSE NULL END,
                updated_on = NOW()
            RETURNING id
            """
    )
    log.info("  → %d approver membership seeds queued", len(cur.fetchall()))

    cur.execute(
            """
            INSERT INTO index_outbox
                (object_type, object_uid, action, headers, data, indexing_config)
            SELECT
                'mentorship_program',
                id,
                'updated',
                '{}'::jsonb,
                jsonb_strip_nulls(jsonb_build_object(
                    'id', id,
                    'project_uid', lf_project_uid,
                    'project_slug', lf_project_slug,
                    'project_name', lf_project_name,
                    'project_logo_url', lf_project_logo_url,
                    'name', name,
                    'slug', slug,
                    'status', status,
                    'logo_url', logo_url,
                    'stats', jsonb_build_object(
                        'mentors', (SELECT COUNT(*) FROM program_members pm WHERE pm.program_id = programs.id AND pm.member_type = 'mentor' AND pm.status = 'active'),
                        'mentees', (SELECT COUNT(*) FROM applications a JOIN program_terms pt ON pt.id = a.program_term_id WHERE pt.program_id = programs.id AND a.role = 'mentee' AND a.status = 'accepted'),
                        'graduated', (SELECT COUNT(*) FROM applications a JOIN program_terms pt ON pt.id = a.program_term_id WHERE pt.program_id = programs.id AND a.role = 'mentee' AND a.status = 'graduated')
                    ),
                    'created_on', created_on,
                    'updated_on', updated_on
                )),
                jsonb_build_object(
                    'object_id', id,
                    'access_check_object', 'mentorship_program:' || id::text,
                    'access_check_relation', 'viewer',
                    'history_check_object', 'mentorship_program:' || id::text,
                    'history_check_relation', 'auditor',
                    'sort_name', name,
                    'name_and_aliases', jsonb_build_array(name, slug),
                    'public', status = 'published',
                    'tags', CASE
                        WHEN lf_project_uid IS NULL THEN jsonb_build_array('status:' || status)
                        ELSE jsonb_build_array('status:' || status, 'project_uid:' || lf_project_uid)
                    END
                ) || CASE
                        WHEN lf_project_uid IS NULL THEN '{}'::jsonb
                        ELSE jsonb_build_object('parent_refs', jsonb_build_array('project:' || lf_project_uid))
                END
            FROM programs
            ON CONFLICT (object_type, object_uid) DO UPDATE SET
                action = EXCLUDED.action,
                headers = EXCLUDED.headers,
                data = EXCLUDED.data,
                indexing_config = EXCLUDED.indexing_config,
                generation = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.generation ELSE index_outbox.generation + 1 END,
                state = CASE WHEN index_outbox.state IN ('in_flight', 'dead_letter') THEN index_outbox.state ELSE 'pending' END,
                claimed_generation = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_generation ELSE NULL END,
                claimed_at = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_at ELSE NULL END,
                attempts = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.attempts ELSE 0 END,
                next_attempt_at = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.next_attempt_at ELSE NOW() END,
                sent_on = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.sent_on ELSE NULL END
            RETURNING object_uid
            """
    )
    log.info("  → %d program index seeds queued", len(cur.fetchall()))

    cur.execute(
        """
        INSERT INTO index_outbox
            (object_type, object_uid, action, headers, data, indexing_config)
        SELECT
            'mentorship_application',
            a.id,
            'updated',
            '{}'::jsonb,
            jsonb_strip_nulls(jsonb_build_object(
                'id', a.id,
                'program_term_id', a.program_term_id,
                'user_id', a.user_id,
                'role', a.role,
                'status', a.status,
                'program_term_status', a.program_term_status,
                'start_date_time', a.start_date_time,
                'end_date_time', a.end_date_time,
                'attendance_type', a.attendance_type,
                'tasks_submitted', a.tasks_submitted,
                'created_on', a.created_on,
                'updated_on', a.updated_on
            )),
            jsonb_build_object(
                'object_id', a.id,
                'access_check_object', 'mentorship_application:' || a.id::text,
                'access_check_relation', 'auditor',
                'history_check_object', 'mentorship_program:' || pt.program_id::text,
                'history_check_relation', 'auditor',
                'sort_name', a.user_id::text,
                'name_and_aliases', jsonb_build_array(a.user_id::text, a.role),
                'public', false,
                'tags', jsonb_build_array('role:' || a.role, 'status:' || a.status),
                'parent_refs', jsonb_build_array('mentorship_program:' || pt.program_id::text)
            )
        FROM applications a
        JOIN program_terms pt ON pt.id = a.program_term_id
        JOIN users u ON u.id = a.user_id
        WHERE NULLIF(u.lfid, '') IS NOT NULL
        ON CONFLICT (object_type, object_uid) DO UPDATE SET
            action = EXCLUDED.action,
            headers = EXCLUDED.headers,
            data = EXCLUDED.data,
            indexing_config = EXCLUDED.indexing_config,
            generation = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.generation ELSE index_outbox.generation + 1 END,
            state = CASE WHEN index_outbox.state IN ('in_flight', 'dead_letter') THEN index_outbox.state ELSE 'pending' END,
            claimed_generation = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_generation ELSE NULL END,
            claimed_at = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_at ELSE NULL END,
            attempts = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.attempts ELSE 0 END,
            next_attempt_at = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.next_attempt_at ELSE NOW() END,
            sent_on = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.sent_on ELSE NULL END
        RETURNING object_uid
        """
    )
    log.info("  → %d application index seeds queued", len(cur.fetchall()))

    cur.execute(
        """
        INSERT INTO index_outbox
            (object_type, object_uid, action, headers, data, indexing_config)
        SELECT
            'mentorship_task',
            t.id,
            'updated',
            '{}'::jsonb,
            jsonb_strip_nulls(jsonb_build_object(
                'id', t.id,
                'application_id', t.application_id,
                'assignee_id', t.assignee_id,
                'name', t.name,
                'description', t.description,
                'category', t.category,
                'prerequisite', t.category = 'prerequisite',
                'status', t.status,
                'application_status', t.application_status,
                'program_term_status', t.program_term_status,
                'custom', t.custom,
                'submit_file', t.submit_file,
                'has_file', t.file IS NOT NULL,
                'due_date', t.due_date,
                'created_on', t.created_on,
                'updated_on', t.updated_on
            )),
            jsonb_build_object(
                'object_id', t.id,
                'access_check_object', 'mentorship_task:' || t.id::text,
                'access_check_relation', 'auditor',
                'history_check_object', 'mentorship_application:' || t.application_id::text,
                'history_check_relation', 'auditor',
                'sort_name', COALESCE(t.name, ''),
                'name_and_aliases', jsonb_build_array(COALESCE(t.name, ''), COALESCE(t.category, '')),
                'public', false,
                'tags', jsonb_build_array(
                    'status:' || t.status,
                    'category:' || COALESCE(t.category, ''),
                    'assignee_id:' || t.assignee_id::text
                ),
                'parent_refs', jsonb_build_array('mentorship_application:' || t.application_id::text)
            )
        FROM tasks t
        JOIN applications a ON a.id = t.application_id
        JOIN users u ON u.id = t.assignee_id
        WHERE t.application_id IS NOT NULL
          AND NULLIF(u.lfid, '') IS NOT NULL
        ON CONFLICT (object_type, object_uid) DO UPDATE SET
            action = EXCLUDED.action,
            headers = EXCLUDED.headers,
            data = EXCLUDED.data,
            indexing_config = EXCLUDED.indexing_config,
            generation = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.generation ELSE index_outbox.generation + 1 END,
            state = CASE WHEN index_outbox.state IN ('in_flight', 'dead_letter') THEN index_outbox.state ELSE 'pending' END,
            claimed_generation = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_generation ELSE NULL END,
            claimed_at = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_at ELSE NULL END,
            attempts = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.attempts ELSE 0 END,
            next_attempt_at = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.next_attempt_at ELSE NOW() END,
            sent_on = CASE WHEN index_outbox.state = 'dead_letter' THEN index_outbox.sent_on ELSE NULL END
        RETURNING object_uid
        """
    )
    log.info("  → %d task index seeds queued", len(cur.fetchall()))


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------


def main() -> None:
    log.info("Connecting to DynamoDB (region=%s) ...", REGION)
    dynamo = boto3.client("dynamodb", region_name=REGION)

    log.info("Connecting to PostgreSQL: %s", _redact_dsn(PG_DSN))
    conn = psycopg2.connect(PG_DSN)
    psycopg2.extras.register_uuid()
    files = lo.LegacyFileRewriter(
        COPY_MANIFEST,
        LOGOS_CDN_URL_PREFIX,
        lo.legacy_url_prefix(LEGACY_BUCKET),
        {lo.LOGO: LOGOS_S3_BUCKET, lo.SUBMISSION: ATTACHMENTS_S3_BUCKET},
    )

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
        with conn:  # single transaction: commits on clean exit, rolls back on exception
            with conn.cursor() as cur:
                known_user_ids    = migrate_users(cur, users_raw, files)
                profile_map       = migrate_user_profiles(cur, profiles_raw, known_user_ids, files)
                known_program_ids = migrate_programs(cur, projects_raw, known_user_ids, files, resolve_lf_projects(projects_raw))
                known_term_ids    = migrate_program_terms(cur, terms_raw, known_program_ids)
                term_scoped_members = migrate_program_members(cur, members_raw, known_program_ids, known_user_ids)
                application_index = migrate_mentees(cur, mentees_raw, known_term_ids, known_user_ids)
                reconcile_term_scoped_members(cur, term_scoped_members)
                migrate_tasks(cur, tasks_raw, application_index, known_term_ids, known_user_ids, files)
                seed_derived_state(cur)

        files.report()
        log.info("Migration complete.")

    except Exception:
        conn.rollback()
        log.exception("Migration failed — rolled back.")
        sys.exit(1)
    finally:
        conn.close()


if __name__ == "__main__":
    main()
