# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

"""
Legacy file migration contract shared by copy_legacy_objects.py and the ETL
(docs/rewrite/03-migration-plan.md §S3 objects, steps 2–3).

Legacy stored every file as an absolute URL into the world-readable uploads
bucket. The copy script moves each referenced object into the bucket its class
dictates and records the outcome in a copy manifest; the ETL rewrites the file
columns from that manifest. The upload allowlists here mirror
backend/internal/service/file_type.go and must stay in sync with it.
"""

import io
import json
import logging
import os
import struct
import zipfile
from urllib.parse import quote

log = logging.getLogger(__name__)

# File classes.
LOGO = "logo"
SUBMISSION = "submission"

# Dynamo table suffix, item field, Postgres column, class. quarantined_tasks.file
# comes from the same tasks field, so it shares the tasks.file mapping.
FILE_FIELDS = (
    ("users", "avatarUrl", "users.avatar_url", LOGO),
    ("user-profiles", "logoUrl", "user_profiles.logo_url", LOGO),
    ("projects", "logoUrl", "programs.logo_url", LOGO),
    ("tasks", "file", "tasks.file", SUBMISSION),
)

MAX_BYTES = {LOGO: 2 << 20, SUBMISSION: 20 << 20}
CACHE_CONTROL = {LOGO: "public, max-age=86400", SUBMISSION: "private, no-store"}
QUARANTINE_PREFIX = {LOGO: "quarantine/logos/", SUBMISSION: "quarantine/attachments/"}

# Manifest entry statuses.
COPIED = "copied"
QUARANTINED = "quarantined"
MISSING = "missing"


def legacy_url_prefix(bucket: str) -> str:
    return f"https://{bucket}.s3.amazonaws.com/"


# ---------------------------------------------------------------------------
# Upload allowlists (mirror of backend/internal/service/file_type.go)
# ---------------------------------------------------------------------------

_COMPOUND_SIGNATURE = b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"
_COMPOUND_DIR_ENTRY_SIZE = 128
_COMPOUND_STREAM = 2
_WORD_DOCUMENT_NAME = "WordDocument\x00".encode("utf-16-le")
_MAX_DOCX_ENTRIES = 1000


def _is_doc(data: bytes) -> bool:
    if not data.startswith(_COMPOUND_SIGNATURE):
        return False
    for off in range(512, len(data) - _COMPOUND_DIR_ENTRY_SIZE + 1, _COMPOUND_DIR_ENTRY_SIZE):
        entry = data[off : off + _COMPOUND_DIR_ENTRY_SIZE]
        (name_length,) = struct.unpack_from("<H", entry, 64)
        if name_length == len(_WORD_DOCUMENT_NAME) and entry[66] == _COMPOUND_STREAM and entry.startswith(_WORD_DOCUMENT_NAME):
            return True
    return False


def _is_docx(data: bytes) -> bool:
    if not data.startswith(b"PK\x03\x04") or data.count(b"PK\x01\x02") > _MAX_DOCX_ENTRIES:
        return False
    try:
        names = set(zipfile.ZipFile(io.BytesIO(data)).namelist())
    except zipfile.BadZipFile:
        return False
    return "[Content_Types].xml" in names and "word/document.xml" in names


def _is_plain_text(data: bytes) -> bool:
    if b"\x00" in data:
        return False
    try:
        data.decode("utf-8")
    except UnicodeDecodeError:
        return False
    return True


# (content type, matcher) per class, in match order; text last since it has no signature.
_ALLOWED = {
    LOGO: (
        ("image/png", lambda d: d.startswith(b"\x89PNG\r\n\x1a\n")),
        ("image/jpeg", lambda d: d.startswith(b"\xff\xd8\xff")),
    ),
    SUBMISSION: (
        ("application/pdf", lambda d: d.startswith(b"%PDF-")),
        ("application/msword", _is_doc),
        ("application/vnd.openxmlformats-officedocument.wordprocessingml.document", _is_docx),
        ("text/plain; charset=utf-8", _is_plain_text),
    ),
}


def identify(file_class: str, data: bytes) -> str | None:
    """Return the allowed Content-Type the bytes match for the class, or None."""
    for content_type, matches in _ALLOWED[file_class]:
        if matches(data):
            return content_type
    return None


# ---------------------------------------------------------------------------
# Copy manifest
# ---------------------------------------------------------------------------


def write_manifest(path: str, prefix: str, buckets: dict, entries: list) -> None:
    tmp = path + ".tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump({"legacy_url_prefix": prefix, "buckets": buckets, "entries": entries}, f, indent=1, sort_keys=True)
    os.replace(tmp, path)


class LegacyFileRewriter:
    """Rewrites legacy file column values from the copy manifest (03 §S3 objects, step 3)."""

    def __init__(self, manifest_path: str, cdn_url_prefix: str, legacy_prefix: str, buckets: dict):
        self._manifest_path = manifest_path
        self._cdn_url_prefix = cdn_url_prefix.rstrip("/")
        self._legacy_prefix = legacy_prefix
        self._buckets = buckets
        self._entries: dict | None = None
        self.counts: dict = {}

    def _lookup(self, file_class: str, value: str) -> dict:
        if self._entries is None:
            if not os.path.exists(self._manifest_path):
                raise RuntimeError(f"copy manifest {self._manifest_path} not found; run copy_legacy_objects.py first")
            with open(self._manifest_path, encoding="utf-8") as f:
                manifest = json.load(f)
            if manifest.get("legacy_url_prefix") != self._legacy_prefix:
                raise RuntimeError(f"copy manifest was written for {manifest.get('legacy_url_prefix')}, not {self._legacy_prefix}")
            # Keys are only valid in the buckets they were copied to.
            if not all(self._buckets.get(c) for c in (LOGO, SUBMISSION)):
                raise RuntimeError("LOGOS_S3_BUCKET and ATTACHMENTS_S3_BUCKET are required to rewrite legacy file values")
            if manifest.get("buckets") != self._buckets:
                raise RuntimeError(f"copy manifest was written for buckets {manifest.get('buckets')}, not {self._buckets}")
            self._entries = {(e["class"], e["value"]): e for e in manifest["entries"]}
        entry = self._entries.get((file_class, value))
        if entry is None:
            raise RuntimeError(f"{file_class} value {value!r} is not in the copy manifest; rerun copy_legacy_objects.py")
        return entry

    def _count(self, column: str, outcome: str) -> None:
        per_column = self.counts.setdefault(column, {})
        per_column[outcome] = per_column.get(outcome, 0) + 1

    def rewrite(self, column: str, file_class: str, value: str | None) -> str | None:
        value = (value or "").strip() or None
        if value is None:
            return None
        if not value.startswith(self._legacy_prefix):
            if file_class == SUBMISSION:
                # A foreign tasks.file value is not a key, and fetching it would be an SSRF path.
                self._count(column, "nulled_foreign")
                return None
            self._count(column, "carried_through")
            return value
        entry = self._lookup(file_class, value)
        if entry["status"] != COPIED:
            self._count(column, entry["status"])
            return None
        self._count(column, "rewritten")
        if file_class == SUBMISSION:
            return entry["key"]
        if not self._cdn_url_prefix:
            raise RuntimeError("LOGOS_CDN_URL_PREFIX is required to rewrite legacy logo URLs")
        return f"{self._cdn_url_prefix}/{quote(entry['key'], safe='')}"

    def report(self) -> None:
        for column, outcomes in sorted(self.counts.items()):
            log.info("  file rewrite %-24s %s", column, ", ".join(f"{k}={v}" for k, v in sorted(outcomes.items())))
