# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

import importlib
import io

import pytest
from botocore.exceptions import ClientError

import copy_legacy_objects as c
import legacy_objects as lo
import migrate_dynamo_to_postgres as m

PNG = b"\x89PNG\r\n\x1a\n" + b"\x00" * 16
BUCKETS = {lo.LOGO: "logos", lo.SUBMISSION: "attachments"}
CHUNK = 8 << 10
OVERSIZED = PNG + b"\x00" * (lo.MAX_BYTES[lo.LOGO] + 1)


class TrackingBody(io.BytesIO):
    """Records the size of every read, so a test can prove nothing was read whole."""

    def __init__(self, data, reads):
        super().__init__(data)
        self.reads = reads

    def read(self, size=-1):
        chunk = super().read(size)
        self.reads.append(len(chunk))
        return chunk


class FakeS3:
    """Minimal S3 client: head/get/put/upload over an in-memory {(bucket, key): bytes} store."""

    def __init__(self, objects=None):
        self.objects = dict(objects or {})
        self.puts = []
        self.reads = []
        self.bodies = []

    def head_object(self, Bucket, Key):
        if (Bucket, Key) not in self.objects:
            raise ClientError({"Error": {"Code": "404"}}, "HeadObject")
        return {"ContentLength": len(self.objects[(Bucket, Key)]), "ContentType": "binary/octet-stream"}

    def get_object(self, Bucket, Key):
        body = TrackingBody(self.objects[(Bucket, Key)], self.reads)
        self.bodies.append(body)
        return {"Body": body}

    def put_object(self, Bucket, Key, Body, ContentType, CacheControl):
        self.objects[(Bucket, Key)] = Body
        self.puts.append((Bucket, Key, ContentType, CacheControl))

    def upload_fileobj(self, Fileobj, Bucket, Key, ExtraArgs):
        # Like the managed transfer: reads the source in bounded chunks.
        parts = iter(lambda: Fileobj.read(CHUNK), b"")
        self.objects[(Bucket, Key)] = b"".join(parts)
        self.puts.append((Bucket, Key, ExtraArgs["ContentType"], ExtraArgs["CacheControl"]))

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
    [b"<svg xmlns='http://www.w3.org/2000/svg'/>", OVERSIZED],
    ids=["disallowed type", "over the cap"],
)
def test_failing_object_is_quarantined_in_the_private_bucket(data):
    cp, _, dest = copier({("legacy", "a.png"): data})
    got = cp.resolve(lo.LOGO, "a.png")
    assert (got["status"], got["key"]) == (lo.QUARANTINED, "quarantine/logos/a.png")
    assert dest.objects[("attachments", "quarantine/logos/a.png")] == data
    assert dest.puts == [("attachments", "quarantine/logos/a.png", "application/octet-stream", lo.CACHE_CONTROL[lo.SUBMISSION])]
    assert ("logos", "a.png") not in dest.objects


def test_oversized_object_is_streamed_never_read_whole():
    cp, source, dest = copier({("legacy", "a.png"): OVERSIZED})
    assert cp.resolve(lo.LOGO, "a.png")["status"] == lo.QUARANTINED
    assert source.reads and max(source.reads) <= CHUNK
    assert dest.objects[("attachments", "quarantine/logos/a.png")] == OVERSIZED
    assert all(body.closed for body in source.bodies)


def test_object_that_grew_after_the_head_is_read_at_most_one_byte_past_the_cap():
    cp, source, dest = copier({("legacy", "a.png"): OVERSIZED})
    source.head_object = lambda Bucket, Key: {"ContentLength": len(PNG), "ContentType": "image/png"}
    assert cp.resolve(lo.LOGO, "a.png")["status"] == lo.QUARANTINED
    assert max(source.reads) == lo.MAX_BYTES[lo.LOGO] + 1
    assert dest.objects[("attachments", "quarantine/logos/a.png")] == OVERSIZED
    assert len(source.bodies) == 2 and all(body.closed for body in source.bodies)


def test_already_copied_object_is_not_read_or_written_again():
    cp, source, dest = copier({}, {("logos", "a.png"): PNG})
    assert cp.resolve(lo.LOGO, "a.png") == {"status": lo.COPIED, "key": "a.png"}
    assert dest.puts == []


def test_decoded_key_is_a_fallback_and_missing_objects_are_reported():
    cp, _, dest = copier({("legacy", "a b.png"): PNG})
    assert cp.resolve(lo.LOGO, "a%20b.png") == {"status": lo.COPIED, "key": "a b.png"}
    assert cp.resolve(lo.LOGO, "gone.png")["status"] == lo.MISSING


@pytest.fixture
def legacy_env(monkeypatch):
    """Pod-like environment; the importer is reloaded because LEGACY_REGION is read at import."""
    for name in ("LEGACY_AWS_REGION", "LEGACY_AWS_PROFILE", "LEGACY_AWS_ACCESS_KEY_ID", "LEGACY_AWS_SECRET_ACCESS_KEY", "LEGACY_AWS_SESSION_TOKEN"):
        monkeypatch.delenv(name, raising=False)
    # An EKS pod has the cluster's region injected; legacy reads must not follow it.
    monkeypatch.setenv("AWS_REGION", "us-west-2")
    monkeypatch.setenv("AWS_DEFAULT_REGION", "us-west-2")

    def reload():
        return importlib.reload(m)

    yield monkeypatch, reload
    monkeypatch.undo()
    importlib.reload(m)


def test_legacy_session_ignores_the_injected_region(legacy_env):
    _, reload = legacy_env
    module = reload()
    assert module.LEGACY_REGION == "us-east-1"
    assert module.legacy_session().region_name == "us-east-1"


def test_legacy_region_can_be_overridden(legacy_env):
    env, reload = legacy_env
    env.setenv("LEGACY_AWS_REGION", "eu-west-1")
    assert reload().legacy_session().region_name == "eu-west-1"


def test_legacy_session_uses_legacy_keys_over_the_default_chain(legacy_env):
    env, reload = legacy_env
    env.setenv("AWS_ACCESS_KEY_ID", "AKIADEFAULT")
    env.setenv("LEGACY_AWS_ACCESS_KEY_ID", "AKIALEGACY")
    env.setenv("LEGACY_AWS_SECRET_ACCESS_KEY", "secret")
    env.setenv("LEGACY_AWS_SESSION_TOKEN", "token")
    creds = reload().legacy_session().get_credentials()
    assert (creds.access_key, creds.token) == ("AKIALEGACY", "token")


def test_default_chain_fallback_is_logged(legacy_env, caplog):
    _, reload = legacy_env
    reload().legacy_session()
    assert "legacy reads use the default credential chain" in caplog.text
