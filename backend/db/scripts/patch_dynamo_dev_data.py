#!/usr/bin/env python3
"""Repair synthetic Jobspring dev data for a complete Mentorship/FGA migration."""

# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

from __future__ import annotations

import argparse
import re
import uuid
from datetime import datetime, timezone
from typing import Any, Protocol, cast

import boto3


class DynamoResource(Protocol):
    def Table(self, name: str) -> Any: ...


NAMESPACE = uuid.UUID("4c7f6c44-3b54-5bb2-bb4b-9f3b5c6938a1")
UUID_RE = re.compile(
    r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$"
)


def scan(table):
    rows = []
    kwargs = {}
    while True:
        page = table.scan(**kwargs)
        rows.extend(page.get("Items", []))
        if "LastEvaluatedKey" not in page:
            return rows
        kwargs["ExclusiveStartKey"] = page["LastEvaluatedKey"]


def now() -> str:
    return datetime.now(timezone.utc).isoformat()


def synthetic_user(user_id: str, role: str) -> dict:
    short_id = user_id.replace("-", "")[:16]
    return {
        "id": user_id,
        "lfid": f"dev-synthetic-{short_id}",
        "email": f"dev-synthetic-{short_id}@example.invalid",
        "name": f"Synthetic Dev {role.title()} {short_id[:8]}",
        "givenName": "Synthetic",
        "familyName": f"Dev {short_id[:8]}",
        "createdAt": now(),
        "updatedAt": now(),
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--region", default="us-east-1")
    parser.add_argument("--table-prefix", default="jobspring-dev")
    parser.add_argument("--apply", action="store_true", help="write changes; default is dry-run")
    args = parser.parse_args()

    dynamo = cast(DynamoResource, boto3.resource("dynamodb", region_name=args.region))
    table = lambda suffix: dynamo.Table(f"{args.table_prefix}-{suffix}")
    members = table("project-members")
    terms = table("program-terms")
    mentees = table("program-term-mentees")
    tasks = table("tasks")
    profiles = table("user-profiles")
    users = table("users")

    member_rows = scan(members)
    term_rows = scan(terms)
    mentee_rows = scan(mentees)
    task_rows = scan(tasks)
    profile_rows = scan(profiles)
    existing_users = scan(users)
    user_ids = {row.get("id") for row in existing_users if row.get("id")}
    synthetic_users: dict[str, dict] = {}
    member_repairs: list[tuple[str, str]] = []
    task_repairs: list[tuple[str, str]] = []
    application_repairs: list[dict] = []
    profile_repairs: list[tuple[str, str]] = []

    def require_user(user_id: str | None, role: str) -> None:
        if user_id and user_id not in user_ids:
            synthetic_users.setdefault(user_id, synthetic_user(user_id, role))

    for row in member_rows:
        user_id = row.get("userId")
        if not user_id:
            user_id = str(uuid.uuid5(NAMESPACE, f"member-user:{row['id']}"))
            member_repairs.append((row["id"], user_id))
        require_user(user_id, "mentor")
    for row in mentee_rows:
        require_user(row.get("userId"), "mentee")
    for row in task_rows:
        require_user(row.get("assigneeId"), "mentee")
        owner_id = row.get("ownerId")
        if owner_id and UUID_RE.match(str(owner_id)):
            require_user(owner_id, "mentor")
        if row.get("assigneeId") is None:
            assignee_id = str(uuid.uuid5(NAMESPACE, f"task-assignee:{row['id']}"))
            task_repairs.append((row["id"], assignee_id))
            require_user(assignee_id, "mentee")
    for row in profile_rows:
        if not row.get("userId"):
            user_id = str(uuid.uuid5(NAMESPACE, f"profile-user:{row['id']}"))
            profile_repairs.append((row["id"], user_id))
            require_user(user_id, "mentee" if str(row.get("type", "")).lower() == "mentee" else "mentor")

    term_projects = {row.get("id"): row.get("projectId") for row in term_rows}
    existing_application_keys = {(row.get("programTermId"), row.get("userId")) for row in mentee_rows}
    for row in task_rows:
        term_id = row.get("programTermId")
        assignee_id = row.get("assigneeId") or str(uuid.uuid5(NAMESPACE, f"task-assignee:{row['id']}"))
        if term_id and (term_id, assignee_id) not in existing_application_keys:
            existing_application_keys.add((term_id, assignee_id))
            require_user(assignee_id, "mentee")
            application_repairs.append(
                {
                    "id": str(uuid.uuid5(NAMESPACE, f"task-application:{term_id}:{assignee_id}")),
                    "programTermId": term_id,
                    "projectId": term_projects.get(term_id),
                    "userId": assignee_id,
                    "status": "pending",
                    "programTermStatus": "open",
                    "tasksSubmitted": False,
                    "adminNotified": False,
                    "createdOn": now(),
                    "updatedOn": now(),
                }
            )

    # Program project links are not repaired here: lfProjectId, lfProjectName and
    # lfProjectLogo are the source fields, and the importer resolves lfProjectId.
    enum_repairs = [
        row["id"]
        for row in task_rows
        if row.get("status") in {"inProgress", "completed"}
        or row.get("category") == "nonPrerequisite"
    ]
    print(f"synthetic_users={len(synthetic_users)} member_repairs={len(member_repairs)} profile_repairs={len(profile_repairs)} application_repairs={len(application_repairs)} task_repairs={len(task_repairs)} enum_repairs={len(enum_repairs)} apply={args.apply}")
    if not args.apply:
        return

    for item in synthetic_users.values():
        users.put_item(Item=item)
    for member_id, user_id in member_repairs:
        members.update_item(Key={"id": member_id}, UpdateExpression="SET userId = :u", ExpressionAttributeValues={":u": user_id})
    for task_id, user_id in task_repairs:
        tasks.update_item(Key={"id": task_id}, UpdateExpression="SET assigneeId = :u", ExpressionAttributeValues={":u": user_id})
    for profile_id, user_id in profile_repairs:
        profiles.update_item(Key={"id": profile_id}, UpdateExpression="SET userId = :u", ExpressionAttributeValues={":u": user_id})
    for application in application_repairs:
        mentees.put_item(Item=application)
    for row in task_rows:
        values = {}
        if row.get("status") == "inProgress":
            values["status"] = "in_progress"
        elif row.get("status") == "pending":
            values["status"] = "incomplete"
        elif row.get("status") == "completed":
            values["status"] = "complete"
        if row.get("category") == "nonPrerequisite":
            values["category"] = "non_prerequisite"
        if values:
            names = {f"#{key}": key for key in values}
            expr = "SET " + ", ".join(f"#{key} = :{key}" for key in values)
            tasks.update_item(Key={"id": row["id"]}, UpdateExpression=expr, ExpressionAttributeNames=names, ExpressionAttributeValues={f":{key}": value for key, value in values.items()})
    print("status=applied")


if __name__ == "__main__":
    main()