# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

import io

import pytest
from botocore.exceptions import ClientError

import copy_legacy_objects as c
import legacy_objects as lo
import migrate_dynamo_to_postgres as m

PNG = b"\x89PNG\r\n\x1a\n" + b"\x00" * 16
BUCKETS = {lo.LOGO: "logos", lo.SUBMISSION: "attachments"}


class FakeS3:
    """Minimal S3 client: head/get/put over an in-memory {(bucket, key): bytes} store."""

    def __init__(self, objects=None):
        self.objects = dict(objects or {})
        self.puts = []

    def head_object(self, Bucket, Key):
        if (Bucket, Key) not in self.objects:
            raise ClientError({"Error": {"Code": "404"}}, "HeadObject")
        return {"ContentLength": len(self.objects[(Bucket, Key)]), "ContentType": "binary/octet-stream"}

    def get_object(self, Bucket, Key):
        return {"Body": io.BytesIO(self.objects[(Bucket, Key)])}

    def put_object(self, Bucket, Key, Body, ContentType, CacheControl):
        self.objects[(Bucket, Key)] = Body
        self.puts.append((Bucket, Key, ContentType, CacheControl))

    def copy_object(self, **_):
        raise AssertionError("a cross-account CopyObject must not be used")


def copier(source_objects, dest_objects=None):
    source, dest = FakeS3(source_objects), FakeS3(dest_objects)
    return c.Copier(source, dest, "legacy", BUCKETS), source, dest


def test_allowed_object_is_written_by_the_destination_identity():
    cp, _, dest = copier({("legacy", "a.png"): PNG})
    assert cp.resolve(lo.LOGO, "a.png") == {"status": lo.COPIED, "key": "a.png"}
    assert dest.objects[("logos", "a.png")] == PNG
    assert dest.puts == [("logos", "a.png", "image/png", lo.CACHE_CONTROL[lo.LOGO])]


@pytest.mark.parametrize(
    "data",
    [b"<svg xmlns='http://www.w3.org/2000/svg'/>", PNG + b"\x00" * (lo.MAX_BYTES[lo.LOGO] + 1)],
    ids=["disallowed type", "over the cap"],
)
def test_failing_object_is_quarantined_in_the_private_bucket(data):
    cp, _, dest = copier({("legacy", "a.png"): data})
    got = cp.resolve(lo.LOGO, "a.png")
    assert (got["status"], got["key"]) == (lo.QUARANTINED, "quarantine/logos/a.png")
    assert dest.objects[("attachments", "quarantine/logos/a.png")] == data
    assert ("logos", "a.png") not in dest.objects


def test_already_copied_object_is_not_read_or_written_again():
    cp, source, dest = copier({}, {("logos", "a.png"): PNG})
    assert cp.resolve(lo.LOGO, "a.png") == {"status": lo.COPIED, "key": "a.png"}
    assert dest.puts == []


def test_decoded_key_is_a_fallback_and_missing_objects_are_reported():
    cp, _, dest = copier({("legacy", "a b.png"): PNG})
    assert cp.resolve(lo.LOGO, "a%20b.png") == {"status": lo.COPIED, "key": "a b.png"}
    assert cp.resolve(lo.LOGO, "gone.png")["status"] == lo.MISSING


@pytest.fixture
def clean_legacy_env(monkeypatch):
    for name in ("LEGACY_AWS_PROFILE", "LEGACY_AWS_ACCESS_KEY_ID", "LEGACY_AWS_SECRET_ACCESS_KEY", "LEGACY_AWS_SESSION_TOKEN"):
        monkeypatch.delenv(name, raising=False)
    # An EKS pod has the cluster's region injected; legacy reads must not follow it.
    monkeypatch.setenv("AWS_REGION", "us-west-2")
    monkeypatch.setenv("AWS_DEFAULT_REGION", "us-west-2")
    return monkeypatch


def test_legacy_session_ignores_the_injected_region(clean_legacy_env):
    assert m.legacy_session().region_name == m.LEGACY_REGION == "us-east-1"


def test_legacy_session_uses_legacy_keys_over_the_default_chain(clean_legacy_env):
    clean_legacy_env.setenv("AWS_ACCESS_KEY_ID", "AKIADEFAULT")
    clean_legacy_env.setenv("LEGACY_AWS_ACCESS_KEY_ID", "AKIALEGACY")
    clean_legacy_env.setenv("LEGACY_AWS_SECRET_ACCESS_KEY", "secret")
    clean_legacy_env.setenv("LEGACY_AWS_SESSION_TOKEN", "token")
    creds = m.legacy_session().get_credentials()
    assert (creds.access_key, creds.token) == ("AKIALEGACY", "token")
