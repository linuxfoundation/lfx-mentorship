#!/usr/bin/env python3
# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

"""Backfill userId on Jobspring project-members rows that never got one.

Jobspring creates a program creator's maintainer row (and some mentor rows) with
only email and name. This script links each such row to its real user:

1. the one user whose email matches the row's email (case-insensitive);
2. for maintainer rows only, if several users share that email, the one whose
   lfid is the program's lfid (the program's lfid names its creator);
3. otherwise an explicit --overrides entry; anything else is left untouched.

Dry-run by default. --apply writes only `userId`, guarded by
attribute_not_exists(userId), so re-runs and concurrent app writes are safe.
--rollback removes exactly the userIds a previous --apply report recorded.
Reports contain emails: they are created 0600 and never overwritten, so each
run needs a new --report path (reports/ is gitignored).
"""

from __future__ import annotations

import argparse
import csv
import os
import sys
from collections import Counter, defaultdict
from typing import Any, Iterable

import boto3
from botocore.exceptions import ClientError

REPORT_FIELDS = ["member_id", "project_id", "member_type", "status", "email", "result", "user_id", "applied", "candidates"]

RESOLVED = {"email_unique", "email_and_program_lfid", "override"}


def _norm(value: Any) -> str:
    return str(value or "").strip().lower()


def scan(table) -> list[dict]:
    rows: list[dict] = []
    # Strongly consistent, so rows committed just before a cutover run are seen.
    kwargs: dict = {"ConsistentRead": True}
    while True:
        page = table.scan(**kwargs)
        rows.extend(page.get("Items", []))
        if "LastEvaluatedKey" not in page:
            return rows
        kwargs["ExclusiveStartKey"] = page["LastEvaluatedKey"]


def load_overrides(path: str | None) -> dict[str, str]:
    if not path:
        return {}
    with open(path, newline="") as f:
        return {row["member_id"].strip(): row["user_id"].strip() for row in csv.DictReader(f) if row.get("user_id", "").strip()}


def resolve(
    member: dict,
    users_by_email: dict[str, list[dict]],
    program_lfids: dict[str, str],
    user_ids: set[str],
    overrides: dict[str, str],
) -> tuple[str, str, list[dict]]:
    """Return (result, user_id, candidates) for a member row without userId."""
    result, user_id, candidates = _resolve_automatically(member, users_by_email, program_lfids)
    if user_id:
        return result, user_id, candidates
    override = overrides.get(member["id"])
    if override:
        return ("override", override, candidates) if override in user_ids else ("override_unknown_user", "", candidates)
    return result, "", candidates


def _resolve_automatically(
    member: dict,
    users_by_email: dict[str, list[dict]],
    program_lfids: dict[str, str],
) -> tuple[str, str, list[dict]]:
    email = _norm(member.get("email"))
    if not email:
        return "no_email", "", []
    candidates = users_by_email.get(email, [])
    if len(candidates) == 1:
        return "email_unique", candidates[0]["id"], candidates
    if not candidates:
        return "no_user_for_email", "", []
    # The program's lfid names its creator, and only the creator gets a legacy
    # maintainer row — so the tie-break is meaningless for mentor rows.
    if _norm(member.get("memberType")) == "maintainer":
        lfid = program_lfids.get(member.get("projectId", ""), "")
        by_lfid = [c for c in candidates if lfid and _norm(c.get("lfid")) == lfid]
        if len(by_lfid) == 1:
            return "email_and_program_lfid", by_lfid[0]["id"], candidates
    return "ambiguous", "", candidates


def plan(members: Iterable[dict], users: Iterable[dict], projects: Iterable[dict], overrides: dict[str, str]) -> list[dict]:
    users = list(users)
    user_ids = {u["id"] for u in users if u.get("id")}
    users_by_email: dict[str, list[dict]] = defaultdict(list)
    for u in users:
        if u.get("id") and _norm(u.get("email")):
            users_by_email[_norm(u["email"])].append(u)
    program_lfids = {p["projectId"]: _norm(p.get("lfid")) for p in projects if p.get("projectId")}

    rows = []
    for m in members:
        if m.get("userId") or not m.get("id"):
            continue
        result, user_id, candidates = resolve(m, users_by_email, program_lfids, user_ids, overrides)
        rows.append(
            {
                "member_id": m["id"],
                "project_id": m.get("projectId", ""),
                "member_type": m.get("memberType", ""),
                "status": m.get("status", ""),
                "email": m.get("email", ""),
                "result": result,
                "user_id": user_id,
                "applied": "",
                "candidates": "|".join(f"{c['id']}:{c.get('lfid', '')}" for c in candidates),
            }
        )
    return rows


def apply(table, rows: list[dict], report_path: str) -> None:
    write_report(report_path, rows, create=True)
    try:
        for row in rows:
            if row["result"] not in RESOLVED:
                continue
            try:
                table.update_item(
                    Key={"id": row["member_id"]},
                    UpdateExpression="SET userId = :u",
                    ConditionExpression="attribute_exists(id) AND attribute_not_exists(userId)",
                    ExpressionAttributeValues={":u": row["user_id"]},
                )
                row["applied"] = "yes"
            except ClientError as e:
                if e.response["Error"]["Code"] != "ConditionalCheckFailedException":
                    raise
                row["applied"] = "skipped_changed"
    finally:
        # A partial failure must still record every committed write so --rollback can undo it.
        write_report(report_path, rows)


def rollback(table, report_path: str) -> Counter:
    counts: Counter = Counter()
    with open(report_path, newline="") as f:
        for row in csv.DictReader(f):
            if row.get("applied") != "yes":
                continue
            try:
                table.update_item(
                    Key={"id": row["member_id"]},
                    UpdateExpression="REMOVE userId",
                    ConditionExpression="userId = :u",
                    ExpressionAttributeValues={":u": row["user_id"]},
                )
                counts["removed"] += 1
            except ClientError as e:
                if e.response["Error"]["Code"] != "ConditionalCheckFailedException":
                    raise
                counts["skipped_changed"] += 1
    return counts


def write_report(path: str, rows: list[dict], create: bool = False) -> None:
    # create=True refuses an existing path so an earlier run's rollback manifest is never lost.
    flags = os.O_WRONLY | os.O_CREAT | (os.O_EXCL if create else os.O_TRUNC)
    with os.fdopen(os.open(path, flags, 0o600), "w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=REPORT_FIELDS)
        writer.writeheader()
        writer.writerows(rows)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--table-prefix", required=True, help="e.g. jobspring-prod or jobspring-dev")
    parser.add_argument("--region", default="us-east-1")
    parser.add_argument("--report", required=True, help="new CSV to write (dry-run/apply) or existing one to read (--rollback); contains emails")
    parser.add_argument("--overrides", help="CSV with member_id,user_id for rows that automatic matching cannot settle")
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--apply", action="store_true", help="write userId; default is dry-run")
    mode.add_argument("--rollback", action="store_true", help="remove userIds recorded as applied in --report")
    args = parser.parse_args()

    dynamo = boto3.resource("dynamodb", region_name=args.region)
    members_table = dynamo.Table(f"{args.table_prefix}-project-members")

    if args.rollback:
        print(f"rollback: {dict(rollback(members_table, args.report))}")
        return 0
    if os.path.exists(args.report):
        parser.error(f"--report {args.report} already exists; use a new path so earlier reports stay intact")
    os.makedirs(os.path.dirname(args.report) or ".", mode=0o700, exist_ok=True)

    members = scan(members_table)
    users = scan(dynamo.Table(f"{args.table_prefix}-users"))
    projects = scan(dynamo.Table(f"{args.table_prefix}-projects"))
    rows = plan(members, users, projects, load_overrides(args.overrides))
    if args.apply:
        apply(members_table, rows, args.report)
    else:
        write_report(args.report, rows, create=True)

    print(f"scanned members={len(members)} users={len(users)} projects={len(projects)}")
    print(f"members without userId: {len(rows)}")
    for (member_type, result), n in sorted(Counter((r["member_type"], r["result"]) for r in rows).items()):
        print(f"  {member_type:<12} {result:<24} {n}")
    if args.apply:
        print(f"applied: {dict(Counter(r['applied'] or 'not_resolved' for r in rows))}")
    else:
        print(f"dry-run: {sum(r['result'] in RESOLVED for r in rows)} rows would be updated; re-run with --apply")
    print(f"report: {args.report}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
