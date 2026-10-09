# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

import csv
import io

import pytest
from botocore.exceptions import ClientError

import backfill_member_user_ids as b

USERS = [
    {"id": "u-alice", "email": "Alice@Example.org", "lfid": "alice"},
    {"id": "u-shared-1", "email": "shared@example.org", "lfid": "owner"},
    {"id": "u-shared-2", "email": "shared@example.org", "lfid": "other"},
]
PROJECTS = [{"projectId": "p1", "lfid": "OWNER"}, {"projectId": "p2", "lfid": "nobody"}]
TARGET = "111111111111:us-east-1:jobspring-prod-project-members"


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
        value, run = ExpressionAttributeValues[":u"], ExpressionAttributeValues[":r"]
        if UpdateExpression.startswith("SET"):
            if item is None or item.get("userId"):
                raise ClientError({"Error": {"Code": "ConditionalCheckFailedException"}}, "UpdateItem")
            item["userId"], item[b.RUN_MARKER] = value, run
        else:
            if item is None or item.get("userId") != value or item.get(b.RUN_MARKER) != run:
                raise ClientError({"Error": {"Code": "ConditionalCheckFailedException"}}, "UpdateItem")
            del item["userId"], item[b.RUN_MARKER]


def test_apply_writes_only_resolved_rows_and_respects_concurrent_writes(tmp_path):
    members = [member("m-email", "alice@example.org"), member("m-race", "alice@example.org"), member("m-unknown", "ghost@example.org")]
    rows = b.plan(members, USERS, PROJECTS, {})
    table = FakeTable(members)
    table.items["m-race"]["userId"] = "u-set-meanwhile"
    report = tmp_path / "report.csv"

    b.apply(table, rows, str(report), TARGET)

    applied = {r["member_id"]: r["applied"] for r in rows}
    assert applied == {"m-email": "yes", "m-race": "skipped_changed", "m-unknown": ""}
    assert table.items["m-email"]["userId"] == "u-alice"
    assert table.items["m-race"]["userId"] == "u-set-meanwhile"
    assert "userId" not in table.items["m-unknown"]

    assert b.rollback(table, str(report), TARGET) == {"removed": 1}
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
        b.apply(Failing(), b.plan([member("m", "alice@example.org")], USERS, PROJECTS, {}), str(tmp_path / "r.csv"), TARGET)


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
        b.apply(table, b.plan(members, USERS, PROJECTS, {}), str(report), TARGET)

    # m-second's attempt is journaled but the write never happened; rollback's
    # conditional REMOVE skips it and still undoes m-first.
    assert journal_final(report) == {"m-first": "yes", "m-second": "attempted"}
    table.update_item = real_update
    assert b.rollback(table, str(report), TARGET) == {"removed": 1, "skipped_changed": 1}
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
        b.apply(table, b.plan(members, USERS, PROJECTS, {}), str(report), TARGET)

    assert journal_final(report) == {"m": "attempted"}
    assert table.items["m"]["userId"] == "u-alice"
    table.update_item = real_update
    assert b.rollback(table, str(report), TARGET) == {"removed": 1}
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
        b.apply(table, b.plan(members, USERS, PROJECTS, {}), str(report), TARGET)
    with open(report, "a", newline="") as f:
        f.write(torn_tail)  # the outcome line was cut off by the crash, no trailing newline

    assert b.journal_state(str(report))["m"]["applied"] == "attempted"
    table.update_item = real_update
    assert b.rollback(table, str(report), TARGET) == {"removed": 1}
    assert "userId" not in table.items["m"]


def test_reports_are_private_and_never_overwritten(tmp_path):
    rows = b.plan([member("m", "alice@example.org")], USERS, PROJECTS, {})
    table = FakeTable([member("m", "alice@example.org")])
    report = tmp_path / "apply.csv"
    b.apply(table, rows, str(report), TARGET)
    assert report.stat().st_mode & 0o777 == 0o600

    with pytest.raises(FileExistsError):
        b.apply(table, [], str(report), TARGET)
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

    b.apply(table, b.plan([member("m", "alice@example.org")], USERS, PROJECTS, {}), str(report), TARGET)

    assert events == [("fsync_dir", str(tmp_path)), ("update", "m")]


def test_apply_stamps_the_run_marker_with_the_value(tmp_path):
    table = FakeTable([member("m", "alice@example.org")])
    report = tmp_path / "report.csv"
    b.apply(table, b.plan([member("m", "alice@example.org")], USERS, PROJECTS, {}), str(report), TARGET)

    with open(report, newline="") as f:
        lines = list(csv.DictReader(f))
    assert {line["target"] for line in lines} == {TARGET}
    assert table.items["m"][b.RUN_MARKER] == lines[0]["run_id"]


def test_rollback_never_removes_another_writers_identical_value(tmp_path):
    members = [member("m", "alice@example.org")]
    table = FakeTable(members)

    def die_before_writing(**_):
        raise KeyboardInterrupt  # journaled `attempted`, but this run's write never happened

    table.update_item, real_update = die_before_writing, table.update_item
    report = tmp_path / "report.csv"
    with pytest.raises(KeyboardInterrupt):
        b.apply(table, b.plan(members, USERS, PROJECTS, {}), str(report), TARGET)
    # Another writer then sets the very value this run had planned, without our marker.
    table.items["m"]["userId"] = "u-alice"
    table.update_item = real_update

    assert b.rollback(table, str(report), TARGET) == {"skipped_changed": 1}
    assert table.items["m"]["userId"] == "u-alice"


@pytest.mark.parametrize(
    "other_target",
    [
        "222222222222:us-east-1:jobspring-prod-project-members",  # wrong account/profile
        "111111111111:us-west-2:jobspring-prod-project-members",  # wrong region
        "111111111111:us-east-1:jobspring-dev-project-members",  # wrong --table-prefix
    ],
)
def test_rollback_refuses_a_journal_for_another_target(tmp_path, other_target):
    members = [member("m", "alice@example.org")]
    table = FakeTable(members)
    report = tmp_path / "report.csv"
    b.apply(table, b.plan(members, USERS, PROJECTS, {}), str(report), TARGET)

    calls = []
    table.update_item = lambda **kw: calls.append(kw)
    with pytest.raises(b.TargetMismatchError):
        b.rollback(table, str(report), other_target)
    assert calls == []


def _crash_after_commit(tmp_path, members):
    table = FakeTable(members)
    real_update = table.update_item

    def write_then_die(**kwargs):
        real_update(**kwargs)
        raise KeyboardInterrupt

    table.update_item = write_then_die
    report = tmp_path / "report.csv"
    with pytest.raises(KeyboardInterrupt):
        b.apply(table, b.plan(members, USERS, PROJECTS, {}), str(report), TARGET)
    table.update_item = real_update
    with open(report, newline="") as f:
        attempt = [r for r in csv.DictReader(f) if r["applied"] == "attempted"][-1]
    return table, report, attempt


def _csv_line(row, terminated=True):
    buf = io.StringIO()
    csv.DictWriter(buf, fieldnames=b.REPORT_FIELDS).writerow(row)
    line = buf.getvalue()
    return line if terminated else line.rstrip("\r\n")


def test_tear_inside_the_final_target_field_neither_masks_nor_blocks_rollback(tmp_path):
    table, report, attempt = _crash_after_commit(tmp_path, [member("m", "alice@example.org")])
    torn = _csv_line({**attempt, "applied": "yes"}, terminated=False)[:-5]  # cut inside `target`
    with open(report, "a", newline="") as f:
        f.write(torn)

    assert b.journal_state(str(report))["m"]["target"] == TARGET
    assert b.rollback(table, str(report), TARGET) == {"removed": 1}
    assert "userId" not in table.items["m"]


def test_torn_line_for_an_unresolved_row_does_not_poison_the_target_check(tmp_path):
    members = [member("m", "alice@example.org"), member("m-ghost", "ghost@example.org")]
    table, report, attempt = _crash_after_commit(tmp_path, members)
    torn = _csv_line({**attempt, "member_id": "m-ghost", "result": "no_user_for_email", "user_id": "", "applied": ""}, terminated=False)[:-5]
    with open(report, "a", newline="") as f:
        f.write(torn)

    assert b.rollback(table, str(report), TARGET) == {"removed": 1}


@pytest.mark.parametrize("field", ["user_id", "run_id", "target"])
def test_later_line_must_match_the_attempts_full_identity(tmp_path, field):
    table, report, attempt = _crash_after_commit(tmp_path, [member("m", "alice@example.org")])
    with open(report, "a", newline="") as f:
        f.write(_csv_line({**attempt, "applied": "skipped_changed", field: attempt[field] + "-x"}))

    assert b.journal_state(str(report))["m"]["applied"] == "attempted"
    assert b.rollback(table, str(report), TARGET) == {"removed": 1}
