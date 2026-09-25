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


def slugify(value: object) -> str:
    slug = re.sub(r"[^a-z0-9]+", "-", str(value or "").lower()).strip("-")
    return slug or "project"


def synthetic_user(user_id: str, role: str) -> dict:
    short_id = user_id.replace("-", "")[:16]
    now = datetime.now(timezone.utc).isoformat()
    return {
        "id": user_id,
        "lfid": f"dev-synthetic-{short_id}",
        "email": f"dev-synthetic-{short_id}@example.invalid",
        "name": f"Synthetic Dev {role.title()} {short_id[:8]}",
        "givenName": "Synthetic",
        "familyName": f"Dev {short_id[:8]}",
        "createdAt": now,
        "updatedAt": now,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--region", default="us-east-1")
    parser.add_argument("--table-prefix", default="jobspring-dev")
    parser.add_argument("--apply", action="store_true", help="write changes; default is dry-run")
    args = parser.parse_args()

    dynamo = cast(DynamoResource, boto3.resource("dynamodb", region_name=args.region))
    table = lambda suffix: dynamo.Table(f"{args.table_prefix}-{suffix}")
    projects = table("projects")
    members = table("project-members")
    mentees = table("program-term-mentees")
    tasks = table("tasks")
    profiles = table("user-profiles")
    users = table("users")

    project_rows = scan(projects)
    member_rows = scan(members)
    mentee_rows = scan(mentees)
    task_rows = scan(tasks)
    profile_rows = scan(profiles)
    existing_users = scan(users)
    user_ids = {row.get("id") for row in existing_users if row.get("id")}
    synthetic_users: dict[str, dict] = {}
    member_repairs: list[tuple[str, str]] = []
    task_repairs: list[tuple[str, str]] = []
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

    used_slugs: set[str] = set()
    project_repairs = []
    for row in sorted(project_rows, key=lambda item: str(item.get("projectId", ""))):
        project_id = row["projectId"]
        base_slug = slugify(row.get("slug") or row.get("name") or project_id)
        project_slug = base_slug
        if project_slug in used_slugs:
            project_slug = f"{base_slug}-{str(project_id).replace('-', '')[:8]}"
        used_slugs.add(project_slug)
        project_name = str(row.get("name") or f"Synthetic LF Project {str(project_id)[:8]}").strip()
        project_logo_url = str(row.get("logoUrl") or "https://example.invalid/lf-projects/default.svg").strip()
        existing_uid = row.get("projectUid") or row.get("lfProjectId") or row.get("lfProjectUID") or row.get("fundspringProjectId")
        project_uid = existing_uid if existing_uid and UUID_RE.match(str(existing_uid)) else str(uuid.uuid5(NAMESPACE, f"lf-project:{project_slug}"))
        project_repairs.append((project_id, project_uid, project_slug, project_name, project_logo_url))

    enum_repairs = [
        row["id"]
        for row in task_rows
        if row.get("status") in {"inProgress", "completed"}
        or row.get("category") == "nonPrerequisite"
    ]
    print(f"projects={len(project_repairs)} synthetic_users={len(synthetic_users)} member_repairs={len(member_repairs)} profile_repairs={len(profile_repairs)} task_repairs={len(task_repairs)} enum_repairs={len(enum_repairs)} apply={args.apply}")
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
    for project_id, project_uid, project_slug, project_name, project_logo_url in project_repairs:
        projects.update_item(
            Key={"projectId": project_id},
            UpdateExpression="SET lfProjectUid = :uid, lfProjectSlug = :slug, lfProjectName = :name, lfProjectLogoUrl = :logo",
            ExpressionAttributeValues={":uid": project_uid, ":slug": project_slug, ":name": project_name, ":logo": project_logo_url},
        )
    print("status=applied")


if __name__ == "__main__":
    main()