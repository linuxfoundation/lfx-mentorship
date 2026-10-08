# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

import csv

import pytest
from botocore.exceptions import ClientError

import backfill_member_user_ids as b

USERS = [
    {"id": "u-alice", "email": "Alice@Example.org", "lfid": "alice"},
    {"id": "u-shared-1", "email": "shared@example.org", "lfid": "owner"},
    {"id": "u-shared-2", "email": "shared@example.org", "lfid": "other"},
]
PROJECTS = [{"projectId": "p1", "lfid": "OWNER"}, {"projectId": "p2", "lfid": "nobody"}]


def member(mid, email=None, project="p1", **extra):
    return {"id": mid, "projectId": project, "memberType": "maintainer", "status": "accepted", "email": email, **extra}


def by_id(rows):
    return {r["member_id"]: r for r in rows}


def test_plan_resolves_and_classifies():
    members = [
        member("m-email", "alice@example.ORG "),
        member("m-shared", "shared@example.org"),
        member("m-ambiguous", "shared@example.org", project="p2"),
        member("m-unknown", "ghost@example.org"),
        member("m-noemail"),
        member("m-has-user", "alice@example.org", userId="u-existing"),
    ]
    rows = by_id(b.plan(members, USERS, PROJECTS, {}))

    assert "m-has-user" not in rows
    assert (rows["m-email"]["result"], rows["m-email"]["user_id"]) == ("email_unique", "u-alice")
    assert (rows["m-shared"]["result"], rows["m-shared"]["user_id"]) == ("email_and_program_lfid", "u-shared-1")
    assert (rows["m-ambiguous"]["result"], rows["m-ambiguous"]["user_id"]) == ("ambiguous", "")
    assert rows["m-unknown"]["result"] == "no_user_for_email"
    assert rows["m-noemail"]["result"] == "no_email"


def test_overrides_win_and_must_name_a_real_user():
    members = [member("m-a", "ghost@example.org"), member("m-b", "ghost@example.org")]
    rows = by_id(b.plan(members, USERS, PROJECTS, {"m-a": "u-alice", "m-b": "u-missing"}))
    assert (rows["m-a"]["result"], rows["m-a"]["user_id"]) == ("override", "u-alice")
    assert (rows["m-b"]["result"], rows["m-b"]["user_id"]) == ("override_unknown_user", "")


class FakeTable:
    def __init__(self, items):
        self.items = {i["id"]: dict(i) for i in items}

    def update_item(self, Key, UpdateExpression, ConditionExpression, ExpressionAttributeValues):
        item = self.items.get(Key["id"])
        value = ExpressionAttributeValues[":u"]
        if UpdateExpression.startswith("SET"):
            if item is None or item.get("userId"):
                raise ClientError({"Error": {"Code": "ConditionalCheckFailedException"}}, "UpdateItem")
            item["userId"] = value
        else:
            if item is None or item.get("userId") != value:
                raise ClientError({"Error": {"Code": "ConditionalCheckFailedException"}}, "UpdateItem")
            del item["userId"]


def test_apply_writes_only_resolved_rows_and_respects_concurrent_writes(tmp_path):
    members = [member("m-email", "alice@example.org"), member("m-race", "alice@example.org"), member("m-unknown", "ghost@example.org")]
    rows = b.plan(members, USERS, PROJECTS, {})
    table = FakeTable(members)
    table.items["m-race"]["userId"] = "u-set-meanwhile"

    b.apply(table, rows)

    applied = {r["member_id"]: r["applied"] for r in rows}
    assert applied == {"m-email": "yes", "m-race": "skipped_changed", "m-unknown": ""}
    assert table.items["m-email"]["userId"] == "u-alice"
    assert table.items["m-race"]["userId"] == "u-set-meanwhile"
    assert "userId" not in table.items["m-unknown"]

    report = tmp_path / "report.csv"
    b.write_report(str(report), rows)
    assert b.rollback(table, str(report)) == {"removed": 1}
    assert "userId" not in table.items["m-email"]
    assert table.items["m-race"]["userId"] == "u-set-meanwhile"


def test_load_overrides_skips_blank_user_ids(tmp_path):
    path = tmp_path / "overrides.csv"
    with open(path, "w", newline="") as f:
        csv.writer(f).writerows([["member_id", "user_id"], [" m-a ", " u-alice "], ["m-b", ""]])
    assert b.load_overrides(str(path)) == {"m-a": "u-alice"}
    assert b.load_overrides(None) == {}


@pytest.mark.parametrize("code", ["ProvisionedThroughputExceededException", "AccessDeniedException"])
def test_apply_surfaces_unexpected_errors(code):
    class Failing:
        def update_item(self, **_):
            raise ClientError({"Error": {"Code": code}}, "UpdateItem")

    with pytest.raises(ClientError):
        b.apply(Failing(), b.plan([member("m", "alice@example.org")], USERS, PROJECTS, {}))
