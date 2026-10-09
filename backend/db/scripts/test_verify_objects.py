# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

import os

import psycopg2
import pytest

import legacy_objects as lo
import verify_objects as v
from test_copy_legacy_objects import BUCKETS, PNG, FakeS3

CDN = "https://cdn.example.org"
PDF = b"%PDF-1.7 submission"
QUARANTINED = {"class": lo.LOGO, "value": "https://legacy/big.png", "status": lo.QUARANTINED, "key": "quarantine/logos/big.png"}
REFS = {(lo.LOGO, "a b.png"): "programs.logo_url", (lo.SUBMISSION, "t.pdf"): "tasks.file"}


def stores():
    source = FakeS3({("legacy", "a b.png"): PNG, ("legacy", "t.pdf"): PDF, ("legacy", "big.png"): PNG * 3})
    dest = FakeS3({("logos", "a b.png"): PNG, ("attachments", "t.pdf"): PDF, ("attachments", "quarantine/logos/big.png"): PNG * 3})
    return source, dest


def check(source, dest):
    return v.check(REFS, [QUARANTINED], source, dest, "legacy", BUCKETS)


def test_matching_objects_pass():
    assert check(*stores()) == []


def test_missing_destination_object_is_reported():
    source, dest = stores()
    del dest.objects[("attachments", "t.pdf")]
    assert check(source, dest) == ["tasks.file t.pdf: not in attachments"]


def test_different_bytes_are_reported_by_etag():
    source, dest = stores()
    dest.objects[("logos", "a b.png")] = PNG + b"x"
    problems = check(source, dest)
    assert len(problems) == 1 and problems[0].startswith("programs.logo_url a b.png: ETag")


def test_quarantined_object_is_compared_by_size():
    source, dest = stores()
    dest.objects[("attachments", "quarantine/logos/big.png")] = PNG
    assert check(source, dest) == [f"quarantine quarantine/logos/big.png: {len(PNG)} bytes, legacy has {len(PNG) * 3}"]


DSN = os.environ.get("MIGRATION_TEST_PG_DSN")


def test_referenced_keys_strip_and_decode_public_urls_and_skip_foreign_ones():
    if not DSN:
        pytest.skip("MIGRATION_TEST_PG_DSN is not set")
    conn = psycopg2.connect(DSN, options="-c search_path=mentorship,public")
    try:
        with conn.cursor() as cur:
            cur.execute("TRUNCATE users, user_profiles, programs, tasks, quarantined_tasks RESTART IDENTITY CASCADE")
            cur.execute("""INSERT INTO users (id, email, avatar_url, created_on, updated_on) VALUES
                (gen_random_uuid(), 'a@example.org', %s, now(), now()),
                (gen_random_uuid(), 'b@example.org', 'https://avatars.example.org/b.png', now(), now())""",
                (f"{CDN}/a%20b.png",))
            assert v.referenced_keys(cur, CDN + "/") == {(lo.LOGO, "a b.png"): "users.avatar_url"}
    finally:
        conn.rollback()
        conn.close()
