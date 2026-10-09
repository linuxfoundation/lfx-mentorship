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
  routes' allowlist and size caps are applied to the bytes; an object that
  fails is copied to a quarantine/ prefix in the private bucket instead.
- The legacy tables and bucket sit in another AWS account than the new buckets,
  so the bytes are read with the legacy session and written with the default
  (v2) identity; no cross-account grant is needed.
- Idempotent: a value whose destination (or quarantine) object already exists
  is not copied again. The manifest is rewritten on every run.

Usage
-----
  export AWS_REGION=us-west-2                   # destination buckets; v2 credentials from the default chain
  export LEGACY_AWS_PROFILE=...                 # legacy reads: see migrate_dynamo_to_postgres.py
  export LEGACY_UPLOADS_REGION=us-east-1        # defaults to LEGACY_AWS_REGION
  export DYNAMODB_TABLE_PREFIX=jobspring-prod   # also names the legacy bucket
  export LEGACY_UPLOADS_BUCKET=jobspring-prod-uploads   # optional override
  export LOGOS_S3_BUCKET=... ATTACHMENTS_S3_BUCKET=...
  export COPY_MANIFEST=legacy-object-manifest.json
  python3 backend/db/scripts/copy_legacy_objects.py

The legacy credentials need read access to the legacy bucket and the DynamoDB
tables; the default credentials need write access to both destination buckets.
"""

import logging
import os
import sys
from urllib.parse import unquote

import boto3
from botocore.exceptions import ClientError

import legacy_objects as lo
from migrate_dynamo_to_postgres import COPY_MANIFEST, LEGACY_BUCKET, LEGACY_REGION, TABLE_PREFIX, legacy_session, scan_table

log = logging.getLogger(__name__)

DEST_REGION = os.environ.get("AWS_REGION", "us-west-2")
SOURCE_REGION = os.environ.get("LEGACY_UPLOADS_REGION", LEGACY_REGION)


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

    def _already_done(self, file_class: str, key: str) -> dict | None:
        if _head(self.dest, self.buckets[file_class], key):
            return {"status": lo.COPIED, "key": key}
        quarantine_key = lo.QUARANTINE_PREFIX[file_class] + key
        if _head(self.dest, self.attachments, quarantine_key):
            return {"status": lo.QUARANTINED, "key": quarantine_key}
        return None

    def _put(self, bucket: str, key: str, data: bytes, content_type: str, cache_control: str) -> None:
        self.dest.put_object(Bucket=bucket, Key=key, Body=data, ContentType=content_type, CacheControl=cache_control)

    def resolve(self, file_class: str, raw: str) -> dict:
        # The decoded key is a fallback only for a raw key absent from both sides.
        for key in self._candidates(raw):
            done = self._already_done(file_class, key)
            if done:
                return done
            head = _head(self.source, self.legacy_bucket, key)
            if head:
                break
        else:
            return {"status": lo.MISSING, "reason": "not in the legacy bucket"}

        reason = None
        content_type = None
        data = self.source.get_object(Bucket=self.legacy_bucket, Key=key)["Body"].read()
        if len(data) > lo.MAX_BYTES[file_class]:
            reason = f"{len(data)} bytes exceeds the {file_class} cap"
        else:
            content_type = lo.identify(file_class, data)
            if content_type is None:
                reason = f"bytes are not an allowed {file_class} type (stored as {head.get('ContentType')})"

        if reason is None:
            self._put(self.buckets[file_class], key, data, content_type, lo.CACHE_CONTROL[file_class])
            return {"status": lo.COPIED, "key": key}
        quarantine_key = lo.QUARANTINE_PREFIX[file_class] + key
        self._put(self.attachments, quarantine_key, data, "application/octet-stream", lo.CACHE_CONTROL[lo.SUBMISSION])
        return {"status": lo.QUARANTINED, "key": quarantine_key, "reason": reason}


def main() -> None:
    buckets = {lo.LOGO: _required_env("LOGOS_S3_BUCKET"), lo.SUBMISSION: _required_env("ATTACHMENTS_S3_BUCKET")}
    prefix = lo.legacy_url_prefix(LEGACY_BUCKET)
    legacy = legacy_session()
    dynamo = legacy.client("dynamodb")
    copier = Copier(legacy.client("s3", region_name=SOURCE_REGION), boto3.client("s3", region_name=DEST_REGION), LEGACY_BUCKET, buckets)

    entries: dict = {}
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
            if (file_class, value) not in entries:
                result = copier.resolve(file_class, value[len(prefix):]) if value != prefix else {"status": lo.MISSING, "reason": "empty key"}
                entries[(file_class, value)] = {"class": file_class, "value": value, **result}
                if result["status"] != lo.COPIED:
                    log.warning("%s %s: %s %s", column, result["status"].upper(), value, result.get("reason", ""))
            status = entries[(file_class, value)]["status"]
            per_column[status] = per_column.get(status, 0) + 1

    lo.write_manifest(COPY_MANIFEST, prefix, buckets, sorted(entries.values(), key=lambda e: (e["class"], e["value"])))
    for column, outcomes in counts.items():
        log.info("%-24s %s", column, ", ".join(f"{k}={v}" for k, v in sorted(outcomes.items())) or "no values")
    log.info("Wrote %d manifest entries to %s", len(entries), COPY_MANIFEST)


if __name__ == "__main__":
    main()
