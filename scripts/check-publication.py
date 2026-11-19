#!/usr/bin/env python3
"""Local publication privacy guard. Python is development tooling, not kit runtime.

Read-only: scans every outgoing commit tree, its commit email metadata and nested
ZIP contents. It can also scan a standalone ZIP before packaging. Diagnostics
contain relative filenames/categories, never matches.
No network requests, index writes, Git configuration changes or file extraction.
"""
from __future__ import annotations

import argparse
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import socket
import struct
import subprocess
import sys
import zipfile
import zlib

PRIVATE_DIRS = {".local-tests", ".cache", ".tools", ".arena", "models", "output", "logs"}
PUBLIC_EVALUATION = {"cases.json", "README.md"}
MAX_BLOB = 256 * 1024 * 1024
MAX_ARCHIVE_ENTRIES = 10000
MAX_ARCHIVE_DEPTH = 4
PRIVATE_JSON_KEYS = {
    "engine_pid", "app_pid", "pid", "hostname", "computer_name", "hardware",
    "os_version", "macos_version", "platform", "machine", "python",
    "start_rss_mib", "peak_rss_mib", "end_rss_mib", "engine_elapsed_seconds",
    "started_at", "finished_at", "loaded_at", "startup_seconds",
}
# These exact public upstream model-card copies contain an author's example
# home path. A changed card loses this exception; current HOME always blocks.
UPSTREAM_CARD_HASHES = {
    "1406be1b6b8fd8a6545870da516912804756593628a1d0fb0a7965211e82a7bb",
    "28546f5fc4f22ee1b3f17b908957f17fcfa66174032136265bdb27f404bc1e6c",
}
USER_PATH = re.compile(
    rb"(?i)(?:/(?:Users|home)/[a-z0-9_.@-]+(?:/[^\s\x00\"'<>]*)?"
    rb"|[a-z]:[\\/](?:Users|Documents and Settings)[\\/][a-z0-9_.@-]+(?:[\\/][^\s\x00\"'<>]*)?)"
)
# Emscripten uses this virtual filesystem HOME inside upstream browser code;
# it is not a host profile. A matching real current HOME still blocks separately.
VIRTUAL_USER_PATHS = {b"/home/web_user"}
ZIP_OWNER_EXTRAS = {0x5855, 0x7855, 0x7875, 0x000D}
ZIP_MAC_EXTRAS = {0x334D, 0x07C8, 0x2605, 0x2705, 0x2805}
CI_PATH = re.compile(rb"(?i)^/(?:Users|home)/runner/work(?:/|$)")
TEMP_PATH = re.compile(rb"/(?:private/)?var/folders/[a-zA-Z0-9_/-]+")
OBSERVED_MACHINE = re.compile(
    rb"(?i)(?:MacBook\s+Pro|Apple\s+M[0-9]+\s+(?:Pro|Max|Ultra)|\bM[0-9]+\s+Pro\b"
    rb"|macOS-[0-9]+\.[0-9]+|\bMac[0-9]+,[0-9]+\b"
    rb"|(?:engine[_ -]?pid|app[_ -]?pid|\bPID)\s*[:=]\s*[0-9]+"
    rb"|--engine-pid\s+[0-9]+)"
)
SAFE_EMAIL = re.compile(r"(?:[A-Za-z0-9._%+-]+@users\.noreply\.github\.com|noreply@example\.invalid)\Z")


class GuardError(Exception):
    """Message is always a fixed, non-sensitive category."""


def git(*args: str) -> bytes:
    result = subprocess.run(["git", *args], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if result.returncode:
        raise GuardError("git_query_failed")
    return result.stdout


def commit_oid(ref: str) -> str:
    return git("rev-parse", "--verify", "--end-of-options", ref + "^{commit}").decode().strip()


def commits_between(head: str, bases: list[str]) -> list[str]:
    return git("rev-list", "--reverse", head, "--not", *bases).decode().splitlines()


def path_category(name: str) -> str | None:
    parts = PurePosixPath(name.replace("\\", "/")).parts
    if any(p in PRIVATE_DIRS for p in parts):
        return "private_generated_directory"
    if "evaluation" in parts:
        i = parts.index("evaluation")
        if len(parts[i + 1:]) != 1 or parts[-1] not in PUBLIC_EVALUATION:
            return "private_evaluation_artifact"
    if name.endswith(".verification.json"):
        return "private_package_verification"
    if name.lower().endswith((".gguf", ".safetensors", ".part", ".log")):
        return "private_model_or_operational_artifact"
    if ".DS_Store" in parts or "__MACOSX" in parts:
        return "local_filesystem_metadata"
    return None


def metadata_present(value: object) -> bool:
    if isinstance(value, dict):
        if PRIVATE_JSON_KEYS.intersection(value):
            return True
        return any(metadata_present(v) for v in value.values())
    if isinstance(value, list):
        return any(metadata_present(v) for v in value)
    return False


class Scanner:
    def __init__(self) -> None:
        self.findings: set[tuple[str, str]] = set()
        self.scanned: set[tuple[str, str]] = set()
        home = os.path.expanduser("~")
        self.home = home.encode().lower() if home not in ("/", "~", "") else b""
        hostname = socket.gethostname().lower()
        self.hostnames = {h.encode() for h in (hostname, hostname.split(".")[0])
                          if len(h) >= 8 and h not in {"localhost", "localhost.localdomain"}}

    def add(self, label: str, category: str) -> None:
        self.findings.add((label, category))

    def content(self, data: bytes, label: str) -> None:
        if self.home and self.home in data.lower():
            self.add(label, "current_home_path")
        if any(host in data.lower() for host in self.hostnames):
            self.add(label, "current_machine_hostname")
        if TEMP_PATH.search(data):
            self.add(label, "machine_temporary_path")
        public_card = hashlib.sha256(data).hexdigest() in UPSTREAM_CARD_HASHES
        if not public_card and any(not CI_PATH.match(m[0]) and m[0] not in VIRTUAL_USER_PATHS for m in USER_PATH.finditer(data)):
            self.add(label, "absolute_user_home_path")
        try:
            parsed = json.loads(data)
        except (ValueError, UnicodeDecodeError, RecursionError):
            parsed = None
        if metadata_present(parsed):
            self.add(label, "local_machine_or_benchmark_metadata")
        # Target platform requirements are public; observed host identifiers and
        # concrete process IDs in prose are private. Source code is not prose.
        if label.lower().endswith((".md", ".txt", ".json", ".rst")) and not public_card:
            if OBSERVED_MACHINE.search(data):
                self.add(label, "observed_machine_or_process_details")

    def blob(self, data: bytes, label: str, depth: int = 0) -> None:
        self.content(data, label)
        if label.lower().endswith(".zip") or data.startswith(b"PK\x03\x04") or data.startswith(b"PK\x05\x06"):
            if depth >= MAX_ARCHIVE_DEPTH:
                self.add(label, "archive_recursion_limit")
                return
            try:
                with zipfile.ZipFile(io.BytesIO(data)) as archive:
                    entries = archive.infolist()
                    if len(entries) > MAX_ARCHIVE_ENTRIES or sum(x.file_size for x in entries) > MAX_BLOB:
                        self.add(label, "archive_scan_limit")
                        return
                    for entry in entries:
                        child = label + "!/" + entry.filename
                        extras = [entry.extra]
                        local = entry.header_offset
                        if local < 0 or local + 30 > len(data) or data[local:local + 4] != b"PK\x03\x04":
                            self.add(child, "archive_local_header_not_readable")
                        else:
                            filename_length, extra_length = struct.unpack_from("<HH", data, local + 26)
                            extra_start = local + 30 + filename_length
                            if extra_start + extra_length > len(data):
                                self.add(child, "archive_local_header_not_readable")
                            else:
                                extras.append(data[extra_start:extra_start + extra_length])
                        for extra in extras:
                            offset = 0
                            while offset < len(extra):
                                if offset + 4 > len(extra):
                                    self.add(child, "archive_extra_not_readable")
                                    break
                                field_id, length = struct.unpack_from("<HH", extra, offset)
                                offset += 4 + length
                                if offset > len(extra):
                                    self.add(child, "archive_extra_not_readable")
                                    break
                                if field_id in ZIP_OWNER_EXTRAS:
                                    self.add(child, "archive_unix_owner_metadata")
                                elif field_id in ZIP_MAC_EXTRAS:
                                    self.add(child, "archive_macos_metadata")
                        if entry.is_dir():
                            continue
                        category = path_category(entry.filename)
                        if category:
                            self.add(child, category)
                        self.blob(archive.read(entry), child, depth + 1)
            except (zipfile.BadZipFile, RuntimeError, NotImplementedError, OSError):
                self.add(label, "archive_not_readable")
        elif data.startswith((b"\xcf\xfa\xed\xfe", b"\xce\xfa\xed\xfe", b"\xca\xfe\xba\xbe", b"\x7fELF", b"MZ")):
            # The official engine includes gzip-compressed UI text. Inspect it
            # in memory too; never run or extract embedded code.
            for match in re.finditer(b"\x1f\x8b\x08", data):
                try:
                    decoder = zlib.decompressobj(31)
                    payload = decoder.decompress(data[match.start():], 20 * 1024 * 1024)
                    if decoder.eof:
                        self.content(payload, label + "!/<embedded-asset>")
                    else:
                        self.add(label, "embedded_asset_scan_limit")
                except zlib.error:
                    continue

    def entry(self, oid: str, path: str) -> None:
        category = path_category(path)
        if category:
            self.add(path, category)
        key = (oid, path)
        if key in self.scanned:
            return
        self.scanned.add(key)
        if int(git("cat-file", "-s", oid)) > MAX_BLOB:
            self.add(path, "blob_scan_limit")
            return
        self.blob(git("cat-file", "blob", oid), path)

    def tree(self, ref: str) -> None:
        for line in git("ls-tree", "-r", "-z", ref).split(b"\0"):
            if not line:
                continue
            header, path = line.split(b"\t", 1)
            _, kind, oid = header.split()
            name = path.decode("utf8", errors="replace")
            if kind != b"blob":
                self.add(name, "non_blob_entry_not_scanned")
                continue
            self.entry(oid.decode(), name)

    def index(self) -> None:
        for line in git("ls-files", "--stage", "-z").split(b"\0"):
            if not line:
                continue
            header, path = line.split(b"\t", 1)
            _, oid, stage = header.split()
            name = path.decode("utf8", errors="replace")
            if stage != b"0":
                self.add(name, "unmerged_index")
            else:
                self.entry(oid.decode(), name)

    def commits(self, commits: list[str]) -> None:
        for oid in commits:
            emails = git("show", "-s", "--format=%ae%x00%ce", oid).decode().strip().split("\0")
            if len(emails) != 2 or any(not SAFE_EMAIL.fullmatch(email) for email in emails):
                self.add("<commit " + oid[:12] + ">", "non_public_commit_email")
            # A clean file tree does not protect a commit message copied from local logs.
            message = git("show", "-s", "--format=%B", oid)
            self.content(message, "<commit " + oid[:12] + " message>.txt")
            self.tree(oid)


def pre_push_commits(remote: str, lines: list[str]) -> list[str]:
    # Remote-tracking refs and advertised remote OIDs are local information.
    # This hook never fetches. An unknown base makes scanning more conservative.
    tips = []
    for line in git("for-each-ref", "--format=%(refname) %(objectname)", "refs/remotes/").decode().splitlines():
        ref, oid = line.split()
        if ref.startswith("refs/remotes/" + remote + "/"):
            tips.append(oid)
    outgoing: set[str] = set()
    for line in lines:
        fields = line.split()
        if len(fields) != 4:
            raise GuardError("invalid_pre_push_input")
        _, local_oid, _, remote_oid = fields
        if not re.fullmatch(r"[0-9a-f]{40}|[0-9a-f]{64}", local_oid + ""):
            raise GuardError("invalid_pre_push_object")
        if set(local_oid) == {"0"}:  # deletion has no outgoing commit tree
            continue
        head = commit_oid(local_oid)
        bases = list(tips)
        if set(remote_oid) != {"0"}:
            try:
                bases.append(commit_oid(remote_oid))
            except GuardError:
                pass
        outgoing.update(commits_between(head, bases))
    return sorted(outgoing)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", help="Already-public commit/ref; excluded together with its ancestors")
    parser.add_argument("--head", default="HEAD", help="Outgoing commit/ref (default: HEAD)")
    parser.add_argument("--tree", nargs="?", const=":index", help="Scan one tree, or the index if no ref is supplied; no commit-email checks")
    parser.add_argument("--pre-push", metavar="REMOTE", help="Read Git pre-push ref updates from stdin; no network")
    parser.add_argument("--archive", type=Path, help="Scan a standalone ZIP, without checking commits")
    args = parser.parse_args()
    if sum([bool(args.base), args.tree is not None, bool(args.pre_push), args.archive is not None]) != 1:
        parser.error("choose --base, --tree, --pre-push or --archive")
    if args.archive is not None and args.archive.suffix.lower() != ".zip":
        parser.error("--archive requires a ZIP file")
    scanner = Scanner()
    count = 0
    try:
        if args.archive is not None:
            if args.archive.stat().st_size > MAX_BLOB:
                scanner.add(args.archive.name, "blob_scan_limit")
            else:
                scanner.blob(args.archive.read_bytes(), args.archive.name)
        elif args.tree == ":index":
            scanner.index()
        elif args.tree is not None:
            scanner.tree(git("rev-parse", "--verify", "--end-of-options", args.tree + "^{tree}").decode().strip())
        else:
            commits = (pre_push_commits(args.pre_push, sys.stdin.read().splitlines()) if args.pre_push
                       else commits_between(commit_oid(args.head), [commit_oid(args.base)]))
            count = len(commits)
            scanner.commits(commits)
    except (GuardError, OSError, ValueError, RecursionError):
        print("Publication blocked: <scan> [scan_failed]", file=sys.stderr)
        return 2
    if scanner.findings:
        print("Publication blocked: private data or unverified content found.", file=sys.stderr)
        for path, category in sorted(scanner.findings):
            # Escape control characters in filenames; never print matched values.
            safe_path = USER_PATH.sub(b"<user-path>", path.encode()).decode(errors="replace")
            safe_path = TEMP_PATH.sub(b"<temporary-path>", safe_path.encode()).decode(errors="replace")
            if scanner.home:
                safe_path = re.sub(re.escape(scanner.home.decode()), "<home>", safe_path, flags=re.I)
            for host in scanner.hostnames:
                safe_path = re.sub(re.escape(host.decode()), "<hostname>", safe_path, flags=re.I)
            print(json.dumps(safe_path, ensure_ascii=True) + " [" + category + "]", file=sys.stderr)
        return 1
    if args.archive is not None:
        print("Archive privacy check passed.")
    else:
        print("Publication privacy check passed (" + str(count) + " outgoing commits).")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
