#!/usr/bin/env python3
# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

"""
Legacy object copy (docs/rewrite/03-migration-plan.md §S3 objects, step 2)
==========================================================================
Copies every object the legacy file fields reference out of the world-readable
legacy uploads bucket into the new buckets, and writes the copy manifest the
ETL (migrate_dynamo_to_postgres.py) rewrites the file columns from.

- Walks users.avatarUrl, user-profiles.logoUrl, projects.logoUrl and tasks.file
  in DynamoDB (quarantined tasks come from the same tasks table). Values not
  under the legacy bucket URL are left to the ETL.
- Each object keeps its legacy key. The key is the rest of the stored string,
  never a parsed URL path; the percent-decoded form is tried only when the raw
  key is not found.
- Logos go to the public bucket, submissions to the private one. The upload
  routes' allowlist and size caps are applied to the bytes — plus, for logos
  only, legacy SVGs that carry no active content (legacy_objects.py); an
  object that fails is copied to a quarantine/ prefix in the private bucket
  instead.
- The legacy tables and bucket sit in another AWS account than the new buckets,
  so the bytes are read with the legacy session and written with the default
  (v2) identity; no cross-account grant is needed.
- Idempotent: a value whose destination object already exists is not copied
  again, though a held copy an interrupted promotion left behind is removed.
  A quarantined object is re-evaluated on every run — promoted, and its
  held copy removed, if it now passes; otherwise left as held — so that
  widening the allowlist needs only a rerun. The manifest is rewritten on
  every run.
- Each distinct value is resolved once, COPY_WORKERS at a time: a copy is a few
  cross-region round trips, so one at a time runs at about three objects a
  second and the prod bucket takes most of a day.

Usage
-----
  export AWS_REGION=us-west-2                   # destination buckets; v2 credentials from the default chain
  export LEGACY_AWS_PROFILE=...                 # or LEGACY_AWS_* keys; required in a v2 pod, see migrate_dynamo_to_postgres.py
  export LEGACY_UPLOADS_REGION=us-east-1        # defaults to LEGACY_AWS_REGION
  export DYNAMODB_TABLE_PREFIX=jobspring-prod   # also names the legacy bucket
  export LEGACY_UPLOADS_BUCKET=jobspring-prod-uploads   # optional override
  export LOGOS_S3_BUCKET=... ATTACHMENTS_S3_BUCKET=...
  export COPY_MANIFEST=legacy-object-manifest.json
  export COPY_WORKERS=16                        # concurrent copies; optional
  python3 backend/db/scripts/copy_legacy_objects.py

The legacy credentials need read access to the legacy bucket and the DynamoDB
tables; the default credentials need write access to both destination buckets.
"""

import io
import logging
import os
import sys
from concurrent.futures import ThreadPoolExecutor
from contextlib import closing
from urllib.parse import unquote

import boto3
from botocore.config import Config
from botocore.exceptions import ClientError

import legacy_objects as lo
from migrate_dynamo_to_postgres import COPY_MANIFEST, LEGACY_BUCKET, LEGACY_REGION, TABLE_PREFIX, legacy_session, scan_table

log = logging.getLogger(__name__)

DEST_REGION = os.environ.get("AWS_REGION", "us-west-2")
SOURCE_REGION = os.environ.get("LEGACY_UPLOADS_REGION", LEGACY_REGION)
COPY_WORKERS = int(os.environ.get("COPY_WORKERS", "16"))
PROGRESS_EVERY = 5000


def _required_env(name: str) -> str:
    value = os.environ.get(name, "").strip()
    if not value:
        sys.exit(f"{name} is required")
    return value


def _head(s3, bucket: str, key: str) -> dict | None:
    try:
        return s3.head_object(Bucket=bucket, Key=key)
    except ClientError as e:
        if e.response.get("Error", {}).get("Code") in ("404", "NoSuchKey", "NotFound"):
            return None
        raise


class Copier:
    def __init__(self, source, dest, legacy_bucket: str, buckets: dict):
        self.source = source
        self.dest = dest
        self.legacy_bucket = legacy_bucket
        self.buckets = buckets  # class → destination bucket
        self.attachments = buckets[lo.SUBMISSION]

    def _candidates(self, raw: str) -> list:
        decoded = unquote(raw)
        return [raw] if decoded == raw else [raw, decoded]

    def _copied(self, file_class: str, key: str) -> bool:
        return _head(self.dest, self.buckets[file_class], key) is not None

    def _held(self, file_class: str, key: str) -> bool:
        return _head(self.dest, self.attachments, lo.QUARANTINE_PREFIX[file_class] + key) is not None

    def _put(self, bucket: str, key: str, data: bytes, content_type: str, cache_control: str) -> None:
        self.dest.put_object(Bucket=bucket, Key=key, Body=data, ContentType=content_type, CacheControl=cache_control)

    def resolve(self, file_class: str, raw: str) -> dict:
        # The decoded key is a fallback only for a raw key absent from both sides.
        for key in self._candidates(raw):
            if self._copied(file_class, key):
                # A promotion interrupted between its put and its release left the held copy behind.
                self._release(file_class, key)
                return {"status": lo.COPIED, "key": key}
            head = _head(self.source, self.legacy_bucket, key)
            if head:
                break
        else:
            return {"status": lo.MISSING, "reason": "not in the legacy bucket"}

        # A held object is re-evaluated on every run rather than short-circuited, so a
        # widened allowlist (the legacy SVG allowance) promotes what an earlier run quarantined.
        held = self._held(file_class, key)
        cap = lo.MAX_BYTES[file_class]
        if head["ContentLength"] > cap:
            return self._quarantine(file_class, key, held, lambda: self._body(key), f"{head['ContentLength']} bytes exceeds the {file_class} cap")
        with closing(self._body(key)) as body:
            data = body.read(cap + 1)
        if len(data) > cap:  # replaced by a larger object since the HEAD
            return self._quarantine(file_class, key, held, lambda: self._body(key), f"more than {cap} bytes exceeds the {file_class} cap")
        content_type = lo.identify(file_class, data)
        if content_type is None:
            reason = f"bytes are not an allowed {file_class} type (stored as {head.get('ContentType')})"
            return self._quarantine(file_class, key, held, lambda: io.BytesIO(data), reason)
        self._put(self.buckets[file_class], key, data, content_type, lo.CACHE_CONTROL[file_class])
        if held:
            self._release(file_class, key)
        return {"status": lo.COPIED, "key": key}

    def _release(self, file_class: str, key: str) -> None:
        # HEAD first: a DeleteObject on a missing key still writes a delete marker in a versioned bucket.
        if self._held(file_class, key):
            self.dest.delete_object(Bucket=self.attachments, Key=lo.QUARANTINE_PREFIX[file_class] + key)

    def _body(self, key: str):
        return self.source.get_object(Bucket=self.legacy_bucket, Key=key)["Body"]

    def _quarantine(self, file_class: str, key: str, held: bool, open_body, reason: str) -> dict:
        quarantine_key = lo.QUARANTINE_PREFIX[file_class] + key
        if not held:
            # A managed upload streams in parts, so an object of any size never sits in memory.
            with closing(open_body()) as body:
                self.dest.upload_fileobj(
                    body,
                    self.attachments,
                    quarantine_key,
                    ExtraArgs={"ContentType": "application/octet-stream", "CacheControl": lo.CACHE_CONTROL[lo.SUBMISSION]},
                )
        return {"status": lo.QUARANTINED, "key": quarantine_key, "reason": reason}


def s3_clients(legacy, workers: int) -> tuple:
    """Source and destination S3 clients with a connection per worker.

    botocore pools 10 connections by default; past that, each request opens a
    new connection and urllib3 logs a warning when returning it. The x2 covers
    the transfer threads upload_fileobj runs on the destination client.
    """
    config = Config(max_pool_connections=workers * 2)
    return legacy.client("s3", region_name=SOURCE_REGION, config=config), boto3.client("s3", region_name=DEST_REGION, config=config)


def resolve_all(copier, prefix: str, references: list, workers: int) -> dict:
    """Resolve each distinct (class, value) once, `workers` at a time; a failure stops the run."""
    first_column: dict = {}
    for column, file_class, value in references:
        first_column.setdefault((file_class, value), column)

    def resolve(key):
        file_class, value = key
        if value == prefix:
            return {"status": lo.MISSING, "reason": "empty key"}
        return copier.resolve(file_class, value[len(prefix):])

    entries: dict = {}
    log.info("Resolving %d distinct objects from %d references with %d workers ...", len(first_column), len(references), workers)
    with ThreadPoolExecutor(max_workers=workers) as pool:
        try:
            for key, result in zip(first_column, pool.map(resolve, first_column)):
                entries[key] = {"class": key[0], "value": key[1], **result}
                if result["status"] != lo.COPIED:
                    log.warning("%s %s: %s %s", first_column[key], result["status"].upper(), key[1], result.get("reason", ""))
                if len(entries) % PROGRESS_EVERY == 0:
                    log.info("  %d/%d resolved", len(entries), len(first_column))
        except BaseException:
            pool.shutdown(cancel_futures=True)  # don't drain the queue before reporting the failure
            raise
    return entries


def main() -> None:
    buckets = {lo.LOGO: _required_env("LOGOS_S3_BUCKET"), lo.SUBMISSION: _required_env("ATTACHMENTS_S3_BUCKET")}
    prefix = lo.legacy_url_prefix(LEGACY_BUCKET)
    legacy = legacy_session()
    dynamo = legacy.client("dynamodb")
    copier = Copier(*s3_clients(legacy, COPY_WORKERS), LEGACY_BUCKET, buckets)

    references: list = []  # (column, file_class, value), in scan order
    counts: dict = {}
    for table, field, column, file_class in lo.FILE_FIELDS:
        per_column = counts.setdefault(column, {})
        for item in scan_table(dynamo, f"{TABLE_PREFIX}-{table}"):
            value = (item.get(field) or "").strip()
            if not value:
                continue
            if not value.startswith(prefix):
                per_column["foreign"] = per_column.get("foreign", 0) + 1
                continue
            references.append((column, file_class, value))

    entries = resolve_all(copier, prefix, references, COPY_WORKERS)
    for column, file_class, value in references:
        status = entries[(file_class, value)]["status"]
        counts[column][status] = counts[column].get(status, 0) + 1

    lo.write_manifest(COPY_MANIFEST, prefix, buckets, sorted(entries.values(), key=lambda e: (e["class"], e["value"])))
    for column, outcomes in counts.items():
        log.info("%-24s %s", column, ", ".join(f"{k}={v}" for k, v in sorted(outcomes.items())) or "no values")
    log.info("Wrote %d manifest entries to %s", len(entries), COPY_MANIFEST)


if __name__ == "__main__":
    main()
