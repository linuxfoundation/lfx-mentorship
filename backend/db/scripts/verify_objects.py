# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

"""Go/no-go check of the copied legacy objects (03-migration-plan.md §S3 objects, step 5 (b) and (c)).

- Every non-NULL file column resolves with HeadObject in its bucket. Private columns
  hold the key; public columns hold the CDN URL, stripped of the prefix and decoded.
  Public values outside the CDN prefix (foreign avatar or logo URLs) are skipped.
- Each referenced object has its legacy source's ETag, a true MD5 for these
  single-part objects. Each quarantined object has its source's size instead, since
  one streamed in parts has a multipart ETag.

Exits 1 on any problem. Usage (same environment as copy_legacy_objects.py and the import):
    python verify_objects.py
Optional: VERIFY_OBJECTS_WORKERS (concurrent HEAD requests, default 32).
"""

import json
import logging
import os
import sys
from concurrent.futures import ThreadPoolExecutor
from urllib.parse import unquote

import boto3
import psycopg2

import legacy_objects as lo
import migrate_dynamo_to_postgres as m
from copy_legacy_objects import DEST_REGION, SOURCE_REGION, _head

log = logging.getLogger("verify_objects")

WORKERS = int(os.environ.get("VERIFY_OBJECTS_WORKERS", "32"))

PUBLIC_COLUMNS = (("users", "avatar_url"), ("user_profiles", "logo_url"), ("programs", "logo_url"))
PRIVATE_COLUMNS = (("tasks", "file"), ("quarantined_tasks", "file"))


def referenced_keys(cur, cdn_prefix: str) -> dict:
    """Maps (class, key) to the first column that references it."""
    refs: dict = {}
    prefix = cdn_prefix.rstrip("/") + "/"
    for table, column in PUBLIC_COLUMNS:
        cur.execute(f"SELECT DISTINCT {column} FROM {table} WHERE starts_with({column}, %s)", (prefix,))
        for (value,) in cur.fetchall():
            refs.setdefault((lo.LOGO, unquote(value[len(prefix):])), f"{table}.{column}")
    for table, column in PRIVATE_COLUMNS:
        cur.execute(f"SELECT DISTINCT {column} FROM {table} WHERE {column} IS NOT NULL")
        for (value,) in cur.fetchall():
            refs.setdefault((lo.SUBMISSION, value), f"{table}.{column}")
    return refs


def check(refs: dict, manifest_entries: list, source, dest, legacy_bucket: str, buckets: dict) -> list:
    """Returns one problem string per object that is absent or differs from its legacy source."""

    def referenced(item):
        (file_class, key), column = item
        got = _head(dest, buckets[file_class], key)
        if got is None:
            return f"{column} {key}: not in {buckets[file_class]}"
        want = _head(source, legacy_bucket, key)
        if want is None:
            return f"{column} {key}: no legacy object under this key"
        if got["ETag"] != want["ETag"]:
            return f"{column} {key}: ETag {got['ETag']} differs from the legacy {want['ETag']}"
        return None

    def quarantined(entry):
        key = entry["key"]
        got = _head(dest, buckets[lo.SUBMISSION], key)
        if got is None:
            return f"quarantine {key}: not in {buckets[lo.SUBMISSION]}"
        want = _head(source, legacy_bucket, key[len(lo.QUARANTINE_PREFIX[entry["class"]]):])
        if want is None or got["ContentLength"] != want["ContentLength"]:
            return f"quarantine {key}: {got['ContentLength']} bytes, legacy has {want and want['ContentLength']}"
        return None

    held = [e for e in manifest_entries if e["status"] == lo.QUARANTINED]
    with ThreadPoolExecutor(WORKERS) as pool:
        results = list(pool.map(referenced, sorted(refs.items()))) + list(pool.map(quarantined, held))
    return [r for r in results if r]


def main() -> None:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    if not m.LOGOS_CDN_URL_PREFIX:
        sys.exit("LOGOS_CDN_URL_PREFIX is required")
    with open(m.COPY_MANIFEST, encoding="utf-8") as f:
        manifest = json.load(f)
    if manifest["legacy_url_prefix"] != lo.legacy_url_prefix(m.LEGACY_BUCKET):
        sys.exit(f"copy manifest was written for {manifest['legacy_url_prefix']}, not bucket {m.LEGACY_BUCKET}")

    conn = psycopg2.connect(m.PG_DSN)
    try:
        with conn.cursor() as cur:
            refs = referenced_keys(cur, m.LOGOS_CDN_URL_PREFIX)
    finally:
        conn.rollback()
        conn.close()

    source = m.legacy_session().client("s3", region_name=SOURCE_REGION)
    dest = boto3.client("s3", region_name=DEST_REGION)
    log.info("Checking %d referenced objects and the quarantine", len(refs))
    problems = check(refs, manifest["entries"], source, dest, m.LEGACY_BUCKET, manifest["buckets"])

    for problem in problems:
        log.error("MISMATCH %s", problem)
    if problems:
        log.error("Object verification failed: %d mismatches", len(problems))
        sys.exit(1)
    log.info("Object verification passed: no mismatches")


if __name__ == "__main__":
    main()
