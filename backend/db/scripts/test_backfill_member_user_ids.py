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


def member(mid, email=None, project="p1", member_type="maintainer", **extra):
    return {"id": mid, "projectId": project, "memberType": member_type, "status": "accepted", "email": email, **extra}


def by_id(rows):
    return {r["member_id"]: r for r in rows}


def journal_final(path):
    with open(path, newline="") as f:
        return {r["member_id"]: r["applied"] for r in csv.DictReader(f)}


def test_plan_resolves_and_classifies():
    members = [
        member("m-email", "alice@example.ORG "),
        member("m-shared", "shared@example.org"),
        member("m-ambiguous", "shared@example.org", project="p2"),
        # The program-lfid tie-break identifies the creator, so it never applies to mentors.
        member("m-mentor-shared", "shared@example.org", member_type="mentor"),
        member("m-unknown", "ghost@example.org"),
        member("m-noemail"),
        member("m-has-user", "alice@example.org", userId="u-existing"),
    ]
    rows = by_id(b.plan(members, USERS, PROJECTS, {}))

    assert "m-has-user" not in rows
    assert (rows["m-email"]["result"], rows["m-email"]["user_id"]) == ("email_unique", "u-alice")
    assert (rows["m-shared"]["result"], rows["m-shared"]["user_id"]) == ("email_and_program_lfid", "u-shared-1")
    assert (rows["m-ambiguous"]["result"], rows["m-ambiguous"]["user_id"]) == ("ambiguous", "")
    assert (rows["m-mentor-shared"]["result"], rows["m-mentor-shared"]["user_id"]) == ("ambiguous", "")
    assert rows["m-unknown"]["result"] == "no_user_for_email"
    assert rows["m-noemail"]["result"] == "no_email"


def test_overrides_apply_only_when_automatic_matching_fails():
    members = [
        member("m-a", "ghost@example.org"),
        member("m-b", "ghost@example.org"),
        member("m-noemail"),
        member("m-ambiguous", "shared@example.org", project="p2"),
        # An accidental override must not displace a unique email match.
        member("m-auto", "alice@example.org"),
    ]
    overrides = {"m-a": "u-alice", "m-b": "u-missing", "m-noemail": "u-alice", "m-ambiguous": "u-shared-2", "m-auto": "u-shared-1"}
    rows = by_id(b.plan(members, USERS, PROJECTS, overrides))
    assert (rows["m-a"]["result"], rows["m-a"]["user_id"]) == ("override", "u-alice")
    assert (rows["m-b"]["result"], rows["m-b"]["user_id"]) == ("override_unknown_user", "")
    assert (rows["m-noemail"]["result"], rows["m-noemail"]["user_id"]) == ("override", "u-alice")
    assert (rows["m-ambiguous"]["result"], rows["m-ambiguous"]["user_id"]) == ("override", "u-shared-2")
    assert (rows["m-auto"]["result"], rows["m-auto"]["user_id"]) == ("email_unique", "u-alice")


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
    report = tmp_path / "report.csv"

    b.apply(table, rows, str(report))

    applied = {r["member_id"]: r["applied"] for r in rows}
    assert applied == {"m-email": "yes", "m-race": "skipped_changed", "m-unknown": ""}
    assert table.items["m-email"]["userId"] == "u-alice"
    assert table.items["m-race"]["userId"] == "u-set-meanwhile"
    assert "userId" not in table.items["m-unknown"]

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
def test_apply_surfaces_unexpected_errors(code, tmp_path):
    class Failing:
        def update_item(self, **_):
            raise ClientError({"Error": {"Code": code}}, "UpdateItem")

    with pytest.raises(ClientError):
        b.apply(Failing(), b.plan([member("m", "alice@example.org")], USERS, PROJECTS, {}), str(tmp_path / "r.csv"))


def test_partial_apply_failure_keeps_committed_writes_rollbackable(tmp_path):
    members = [member("m-first", "alice@example.org"), member("m-second", "shared@example.org")]
    table = FakeTable(members)
    real_update = table.update_item

    def throttle_after_first(**kwargs):
        if kwargs["Key"]["id"] == "m-second":
            raise ClientError({"Error": {"Code": "ProvisionedThroughputExceededException"}}, "UpdateItem")
        real_update(**kwargs)

    table.update_item = throttle_after_first
    report = tmp_path / "report.csv"

    with pytest.raises(ClientError):
        b.apply(table, b.plan(members, USERS, PROJECTS, {}), str(report))

    # m-second's attempt is journaled but the write never happened; rollback's
    # conditional REMOVE skips it and still undoes m-first.
    assert journal_final(report) == {"m-first": "yes", "m-second": "attempted"}
    table.update_item = real_update
    assert b.rollback(table, str(report)) == {"removed": 1, "skipped_changed": 1}
    assert "userId" not in table.items["m-first"]


def test_crash_before_outcome_line_still_rolls_back_the_committed_write(tmp_path):
    members = [member("m", "alice@example.org")]
    table = FakeTable(members)
    real_update = table.update_item

    def write_then_die(**kwargs):
        real_update(**kwargs)
        raise KeyboardInterrupt  # simulates the process dying after DynamoDB committed

    table.update_item = write_then_die
    report = tmp_path / "report.csv"
    with pytest.raises(KeyboardInterrupt):
        b.apply(table, b.plan(members, USERS, PROJECTS, {}), str(report))

    assert journal_final(report) == {"m": "attempted"}
    assert table.items["m"]["userId"] == "u-alice"
    table.update_item = real_update
    assert b.rollback(table, str(report)) == {"removed": 1}
    assert "userId" not in table.items["m"]


@pytest.mark.parametrize(
    "torn_tail",
    [
        "m,p1,maintainer,accepted,alice@example.org,email_unique,u-alice,ye",  # torn inside `applied`
        "m,p1,maintainer,accepted,alice@example.org,email_unique,u-alice,skipped_changed",  # `candidates` missing
        "m,p1,maintainer,accepted,alice@example.org,email_unique,u-ali",  # torn inside `user_id`
        'm,p1,maintainer,accepted,"alice@exa',  # torn inside a quoted field
    ],
)
def test_torn_final_line_never_masks_a_durable_attempt(tmp_path, torn_tail):
    members = [member("m", "alice@example.org")]
    table = FakeTable(members)
    real_update = table.update_item

    def write_then_die(**kwargs):
        real_update(**kwargs)
        raise KeyboardInterrupt

    table.update_item = write_then_die
    report = tmp_path / "report.csv"
    with pytest.raises(KeyboardInterrupt):
        b.apply(table, b.plan(members, USERS, PROJECTS, {}), str(report))
    with open(report, "a", newline="") as f:
        f.write(torn_tail)  # the outcome line was cut off by the crash, no trailing newline

    assert b.journal_state(str(report))["m"]["applied"] == "attempted"
    table.update_item = real_update
    assert b.rollback(table, str(report)) == {"removed": 1}
    assert "userId" not in table.items["m"]


def test_reports_are_private_and_never_overwritten(tmp_path):
    rows = b.plan([member("m", "alice@example.org")], USERS, PROJECTS, {})
    table = FakeTable([member("m", "alice@example.org")])
    report = tmp_path / "apply.csv"
    b.apply(table, rows, str(report))
    assert report.stat().st_mode & 0o777 == 0o600

    with pytest.raises(FileExistsError):
        b.apply(table, [], str(report))
    with pytest.raises(FileExistsError):
        b.write_report(str(report), [])
    assert journal_final(report) == {"m": "yes"}


def test_report_directory_is_synced_before_the_first_write(tmp_path, monkeypatch):
    events = []
    monkeypatch.setattr(b, "fsync_dir", lambda path: events.append(("fsync_dir", path)))
    table = FakeTable([member("m", "alice@example.org")])
    real_update = table.update_item
    table.update_item = lambda **kw: (events.append(("update", kw["Key"]["id"])), real_update(**kw))
    report = tmp_path / "report.csv"

    b.apply(table, b.plan([member("m", "alice@example.org")], USERS, PROJECTS, {}), str(report))

    assert events == [("fsync_dir", str(tmp_path)), ("update", "m")]
