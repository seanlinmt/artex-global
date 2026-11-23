#!/usr/bin/env python3
"""Check a git diff (or revision range) for new/changed CJK string literals not yet in i18n catalogs.

Designed for post-`git pull` / rebase tracking on artex-global:
  python3 scripts/check-i18n-delta.py origin/main@{1}..origin/main
  python3 scripts/check-i18n-delta.py 06b36d7..HEAD --check-orphans
  python3 scripts/check-i18n-delta.py              # scans uncommitted working-tree diff

Exit codes:
  0  No untranslated CJK string literals (and no missing en.json keys) found.
  1  Untranslated CJK string literals or missing web/messages/en.json keys found.
  2  Invalid git arguments or repository error.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from collections import defaultdict
from pathlib import Path

# Han (CJK Unified + Extension A + Compatibility + Extension B+) and Hangul (Syllables + Jamo).
CJK_RE = re.compile(
    r"[\u3400-\u9fff\uf900-\ufaff\uac00-\ud7af\u1100-\u11ff\u3130-\u318f\U00020000-\U0002ffff]"
)

# Known wire/brain sentinels that must remain untranslated (see intercept/msg_i18n.go).
IGNORED_LITERALS = {
    "[模型]",
}

GO_CATALOG_PATHS = [
    "config/i18n.go",
    "server/errmsg_catalog.go",
    "server/activity_msg_i18n.go",
    "selfupdate/msg_i18n.go",
    "intercept/label_i18n.go",
    "intercept/msg_i18n.go",
    "agent/displaymsg_i18n.go",
]

TS_CATALOG_KEY_PATHS = [
    "web/src/lib/client-msg.ts",
    "web/src/lib/builtin-i18n.ts",
]

TS_CATALOG_SEED_PATHS = [
    "web/src/lib/agent-i18n.ts",
]

JSON_CATALOG_PATHS = [
    "web/src/lib/builtin-rule-messages.json",
    "web/src/lib/builtin-tool-sources.json",
]

UNQUOTED_TS_KEY_RE = re.compile(r"([^\s:{},\"'`]+)\s*:")
HUNK_RE = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@")


def has_cjk(text: str) -> bool:
    return bool(CJK_RE.search(text))


def git_root() -> Path:
    try:
        out = subprocess.check_output(
            ["git", "rev-parse", "--show-toplevel"],
            text=True,
            stderr=subprocess.PIPE,
        ).strip()
    except subprocess.CalledProcessError as exc:
        print(f"error: failed to locate git repository root: {exc.stderr.strip()}", file=sys.stderr)
        sys.exit(2)
    return Path(out)


def run_git(root: Path, args: list[str]) -> str:
    proc = subprocess.run(
        ["git", *args],
        cwd=root,
        text=True,
        capture_output=True,
    )
    if proc.returncode != 0:
        msg = proc.stderr.strip() or proc.stdout.strip() or f"git {' '.join(args)} failed"
        print(f"error: {msg}", file=sys.stderr)
        sys.exit(2)
    return proc.stdout


def unescape_string(raw: str, quote: str) -> str:
    if quote == "`":
        return raw
    out: list[str] = []
    i = 0
    n = len(raw)
    simple = {
        "n": "\n",
        "r": "\r",
        "t": "\t",
        "a": "\a",
        "b": "\b",
        "f": "\f",
        "v": "\v",
        "\\": "\\",
        '"': '"',
        "'": "'",
    }
    while i < n:
        ch = raw[i]
        if ch != "\\" or i + 1 >= n:
            out.append(ch)
            i += 1
            continue
        nxt = raw[i + 1]
        if nxt in simple:
            out.append(simple[nxt])
            i += 2
        elif nxt == "x" and i + 3 < n:
            hex_part = raw[i + 2 : i + 4]
            try:
                out.append(chr(int(hex_part, 16)))
                i += 4
                continue
            except ValueError:
                pass
            out.append("\\")
            i += 1
        elif nxt == "u" and i + 5 < n:
            hex_part = raw[i + 2 : i + 6]
            try:
                out.append(chr(int(hex_part, 16)))
                i += 6
                continue
            except ValueError:
                pass
            out.append("\\")
            i += 1
        elif nxt == "U" and i + 9 < n:
            hex_part = raw[i + 2 : i + 10]
            try:
                out.append(chr(int(hex_part, 16)))
                i += 10
                continue
            except ValueError:
                pass
            out.append("\\")
            i += 1
        else:
            out.append(nxt)
            i += 2
    return "".join(out)


def extract_literals_from_line(
    line: str,
    *,
    allow_single_quote: bool,
    hash_comments: bool,
    slash_comments: bool,
    bat_comments: bool = False,
) -> list[tuple[str, bool]]:
    """Return [(literal_value, followed_by_colon), ...] from a single source line."""
    stripped = line.lstrip()
    if slash_comments and (stripped.startswith("//") or stripped.startswith("/*") or stripped.startswith("*")):
        return []
    if hash_comments and stripped.startswith("#"):
        return []
    if bat_comments and (
        stripped.lower().startswith("rem ")
        or stripped.lower() == "rem"
        or stripped.startswith("::")
    ):
        return []

    quotes = {'"', "`"}
    if allow_single_quote:
        quotes.add("'")

    results: list[tuple[str, bool]] = []
    i = 0
    n = len(line)
    while i < n:
        ch = line[i]
        if slash_comments and ch == "/" and i + 1 < n:
            nxt = line[i + 1]
            if nxt == "/":
                break
            if nxt == "*":
                end = line.find("*/", i + 2)
                if end == -1:
                    break
                i = end + 2
                continue
        if hash_comments and ch == "#" and (i == 0 or line[i - 1].isspace()):
            break
        if ch in quotes:
            quote = ch
            j = i + 1
            while j < n:
                c = line[j]
                if c == "\\" and quote != "`":
                    j += 2
                    continue
                if c == "\\" and quote == "`" and allow_single_quote:
                    # JS/TS template literal escape
                    j += 2
                    continue
                if c == quote:
                    break
                j += 1
            if j < n and line[j] == quote:
                raw_inner = line[i + 1 : j]
                k = j + 1
                while k < n and line[k] in (" ", "\t"):
                    k += 1
                followed_by_colon = k < n and line[k] == ":"
                if quote == "`" and allow_single_quote and "${" in raw_inner:
                    results.extend(_split_template_literal(raw_inner))
                else:
                    val = unescape_string(raw_inner, quote)
                    results.append((val, followed_by_colon))
                i = j + 1
                continue
        i += 1
    return results


def _split_template_literal(raw: str) -> list[tuple[str, bool]]:
    """Split a JS/TS template literal body around ${...} expressions."""
    items: list[tuple[str, bool]] = []
    i = 0
    n = len(raw)
    static_start = 0
    while i < n:
        if raw[i] == "\\" and i + 1 < n:
            i += 2
            continue
        if raw[i] == "$" and i + 1 < n and raw[i + 1] == "{":
            static_part = raw[static_start:i]
            if static_part:
                items.append((static_part, False))
            # Find matching '}' accounting for nested braces and quotes.
            depth = 1
            j = i + 2
            in_q: str | None = None
            while j < n and depth > 0:
                c = raw[j]
                if in_q is not None:
                    if c == "\\":
                        j += 2
                        continue
                    if c == in_q:
                        in_q = None
                else:
                    if c in ('"', "'", "`"):
                        in_q = c
                    elif c == "{":
                        depth += 1
                    elif c == "}":
                        depth -= 1
                j += 1
            expr = raw[i + 2 : j - 1] if depth == 0 else raw[i + 2 :]
            items.extend(
                extract_literals_from_line(
                    expr, allow_single_quote=True, hash_comments=False, slash_comments=True
                )
            )
            static_start = j
            i = j
            continue
        i += 1
    tail = raw[static_start:]
    if tail:
        items.append((tail, False))
    return items


def discover_go_catalogs(root: Path) -> list[Path]:
    paths = {root / rel for rel in GO_CATALOG_PATHS if (root / rel).is_file()}
    for pattern in ("*_i18n.go", "*_catalog.go"):
        for p in root.rglob(pattern):
            if p.name.endswith("_test.go"):
                continue
            if any(part in {".git", "node_modules", "vendor", "data"} for part in p.parts):
                continue
            paths.add(p)
    return sorted(paths)


def load_catalog_keys(root: Path) -> set[str]:
    keys: set[str] = set()

    for path in discover_go_catalogs(root):
        in_block_comment = False
        for line in path.read_text(encoding="utf-8", errors="ignore").splitlines():
            s = line.strip()
            if in_block_comment:
                if "*/" in s:
                    in_block_comment = False
                continue
            if s.startswith("/*") and "*/" not in s:
                in_block_comment = True
                continue
            for val, followed_by_colon in extract_literals_from_line(
                line, allow_single_quote=False, hash_comments=False, slash_comments=True
            ):
                if followed_by_colon and has_cjk(val):
                    keys.add(val)
                    # server/errmsg_catalog.go double-escapes \n and \" inside generated keys.
                    if "\\n" in val or '\\"' in val:
                        keys.add(val.replace("\\n", "\n").replace('\\"', '"'))

    for rel in TS_CATALOG_KEY_PATHS:
        path = root / rel
        if not path.is_file():
            continue
        for line in path.read_text(encoding="utf-8", errors="ignore").splitlines():
            for val, followed_by_colon in extract_literals_from_line(
                line, allow_single_quote=True, hash_comments=False, slash_comments=True
            ):
                if followed_by_colon and has_cjk(val):
                    keys.add(val)
            # Handle unquoted object keys (e.g. `概述: "overview"` in builtin-i18n.ts).
            code_part = line.split("//", 1)[0]
            for m in UNQUOTED_TS_KEY_RE.finditer(code_part):
                tok = m.group(1)
                if has_cjk(tok):
                    keys.add(tok)

    for rel in TS_CATALOG_SEED_PATHS:
        path = root / rel
        if not path.is_file():
            continue
        for line in path.read_text(encoding="utf-8", errors="ignore").splitlines():
            for val, _ in extract_literals_from_line(
                line, allow_single_quote=True, hash_comments=False, slash_comments=True
            ):
                if has_cjk(val):
                    keys.add(val)

    for rel in JSON_CATALOG_PATHS:
        path = root / rel
        if not path.is_file():
            continue
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
        except json.JSONDecodeError:
            continue
        if isinstance(data, dict):
            for k, v in data.items():
                if isinstance(k, str) and has_cjk(k):
                    keys.add(k)
                if isinstance(v, str) and has_cjk(v):
                    keys.add(v)

    return keys


def flatten_json(obj: object, prefix: str = "") -> dict[str, str]:
    flat: dict[str, str] = {}
    if isinstance(obj, dict):
        for k, v in obj.items():
            full = f"{prefix}.{k}" if prefix else str(k)
            if isinstance(v, dict):
                flat.update(flatten_json(v, full))
            elif isinstance(v, str):
                flat[full] = v
    return flat


def check_web_messages(root: Path) -> list[str]:
    msg_dir = root / "web" / "messages"
    en_path = msg_dir / "en.json"
    if not en_path.is_file():
        return ["web/messages/en.json does not exist"]

    en_flat = flatten_json(json.loads(en_path.read_text(encoding="utf-8")))
    ref_keys: set[str] = set()
    for ref_name in ("ko.json", "zh.json"):
        ref_path = msg_dir / ref_name
        if ref_path.is_file():
            ref_keys.update(flatten_json(json.loads(ref_path.read_text(encoding="utf-8"))).keys())

    missing: list[str] = []
    for k in sorted(ref_keys):
        if k not in en_flat or not en_flat[k].strip():
            missing.append(k)
    return missing


def classify_file(rel: str) -> str | None:
    """Return 'go', 'ts', 'sh', 'bat', or None if the path is out of scope."""
    norm = rel.replace("\\", "/")
    name = os.path.basename(norm)
    if norm.endswith(".go"):
        if (
            name == "i18n.go"
            or name.endswith("_test.go")
            or name.endswith("_i18n.go")
            or name.endswith("_catalog.go")
            or norm in GO_CATALOG_PATHS
        ):
            return None
        return "go"
    if norm.startswith("web/src/") and (norm.endswith(".ts") or norm.endswith(".tsx")):
        if name.endswith(".test.ts") or name.endswith(".test.tsx"):
            return None
        if norm in set(TS_CATALOG_KEY_PATHS + TS_CATALOG_SEED_PATHS):
            return None
        return "ts"
    if norm.endswith(".sh"):
        return "sh"
    if norm.endswith(".bat"):
        return "bat"
    return None


def extract_for_kind(line: str, kind: str) -> list[str]:
    if kind == "go":
        pairs = extract_literals_from_line(
            line, allow_single_quote=False, hash_comments=False, slash_comments=True
        )
    elif kind == "ts":
        pairs = extract_literals_from_line(
            line, allow_single_quote=True, hash_comments=False, slash_comments=True
        )
    elif kind == "sh":
        pairs = extract_literals_from_line(
            line, allow_single_quote=True, hash_comments=True, slash_comments=False
        )
    else:  # bat
        pairs = extract_literals_from_line(
            line, allow_single_quote=False, hash_comments=False, slash_comments=False, bat_comments=True
        )
    return [val for val, _ in pairs]


def collect_current_source_literals(root: Path) -> set[str]:
    literals: set[str] = set()
    tracked = run_git(root, ["ls-files", "--cached", "--others", "--exclude-standard"]).splitlines()
    for rel in tracked:
        kind = classify_file(rel)
        if kind not in {"go", "ts"}:
            continue
        p = root / rel
        if not p.is_file():
            continue
        for line in p.read_text(encoding="utf-8", errors="ignore").splitlines():
            for val in extract_for_kind(line, kind):
                if has_cjk(val):
                    literals.add(val)
    return literals


def parse_diff(
    diff_text: str,
    catalog_keys: set[str],
    *,
    check_orphans: bool,
) -> tuple[dict[str, list[tuple[int, str]]], set[str], dict[str, set[str]]]:
    """Parse `git diff -U0` output.

    Returns:
      untranslated: {rel_path: [(line_no, literal), ...]}
      modified_scripts: set of .sh/.bat paths touched in the diff
      removed_catalog_hits: {literal: {rel_path, ...}} from '-' lines (for orphan detection)
    """
    untranslated: dict[str, list[tuple[int, str]]] = defaultdict(list)
    seen_per_file: dict[str, set[tuple[int, str]]] = defaultdict(set)
    modified_scripts: set[str] = set()
    removed_catalog_hits: dict[str, set[str]] = defaultdict(set)

    cur_file: str | None = None
    cur_kind: str | None = None
    cur_line = 0

    for raw_line in diff_text.splitlines():
        if raw_line.startswith("diff --git "):
            cur_file = None
            cur_kind = None
            continue
        if raw_line.startswith("+++ "):
            target = raw_line[4:].strip()
            if target == "/dev/null":
                cur_file = None
                cur_kind = None
                continue
            if target.startswith("b/"):
                target = target[2:]
            cur_file = target
            cur_kind = classify_file(cur_file)
            if cur_kind in {"sh", "bat"}:
                modified_scripts.add(cur_file)
            continue
        if raw_line.startswith("--- "):
            continue
        if raw_line.startswith("@@ "):
            m = HUNK_RE.match(raw_line)
            if m:
                cur_line = int(m.group(1))
            continue

        if cur_file is None or cur_kind is None:
            continue

        if raw_line.startswith("+"):
            code = raw_line[1:]
            for val in extract_for_kind(code, cur_kind):
                if not has_cjk(val):
                    continue
                if val in IGNORED_LITERALS or val in catalog_keys:
                    continue
                key = (cur_line, val)
                if key not in seen_per_file[cur_file]:
                    seen_per_file[cur_file].add(key)
                    untranslated[cur_file].append(key)
            cur_line += 1
        elif raw_line.startswith("-"):
            if check_orphans and cur_kind in {"go", "ts"}:
                code = raw_line[1:]
                for val in extract_for_kind(code, cur_kind):
                    if has_cjk(val) and val in catalog_keys:
                        removed_catalog_hits[val].add(cur_file)
        elif raw_line.startswith(" "):
            cur_line += 1

    return untranslated, modified_scripts, removed_catalog_hits


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Report new/changed CJK string literals in a git diff that are missing from i18n catalogs.",
    )
    parser.add_argument(
        "rev_range",
        nargs="?",
        help="Git revision or range (e.g. origin/main@{1}..origin/main or 06b36d7..HEAD). Omit for working-tree diff.",
    )
    parser.add_argument(
        "--check-orphans",
        action="store_true",
        help="Also flag catalog keys removed/changed in the diff that no longer appear in non-catalog source files.",
    )
    args = parser.parse_args()

    root = git_root()
    catalog_keys = load_catalog_keys(root)

    diff_args = ["diff", "-U0", "--no-color"]
    name_args = ["diff", "--name-only"]
    if args.rev_range:
        diff_args.append(args.rev_range)
        name_args.append(args.rev_range)

    diff_text = run_git(root, diff_args)
    changed_files = set(run_git(root, name_args).splitlines())

    untranslated, modified_scripts, removed_hits = parse_diff(
        diff_text, catalog_keys, check_orphans=args.check_orphans
    )

    # Include renamed/touched shell scripts even if diff text had no content lines (e.g. mode/empty change).
    for f in changed_files:
        if classify_file(f) in {"sh", "bat"}:
            modified_scripts.add(f)

    missing_en_keys: list[str] = []
    if any(f.startswith("web/messages/") and f.endswith(".json") for f in changed_files):
        missing_en_keys = check_web_messages(root)

    orphans: list[tuple[str, list[str]]] = []
    if args.check_orphans and removed_hits:
        current_literals = collect_current_source_literals(root)
        for key in sorted(removed_hits):
            if key not in current_literals:
                orphans.append((key, sorted(removed_hits[key])))

    # Print report
    scope_label = args.rev_range if args.rev_range else "working tree"
    print(f"i18n delta check ({scope_label}) — loaded {len(catalog_keys)} catalog keys")

    if untranslated:
        total_hits = sum(len(items) for items in untranslated.values())
        print(f"\nUntranslated CJK literals ({total_hits} in {len(untranslated)} files):")
        for path in sorted(untranslated):
            for line_no, literal in untranslated[path]:
                display = literal.replace("\n", "\\n")
                if len(display) > 120:
                    display = display[:117] + "..."
                print(f"  {path}:{line_no}  {display}")
    else:
        print("\nUntranslated CJK literals: none")

    if missing_en_keys:
        print(f"\nMissing/empty keys in web/messages/en.json ({len(missing_en_keys)}):")
        for k in missing_en_keys[:50]:
            print(f"  {k}")
        if len(missing_en_keys) > 50:
            print(f"  ... and {len(missing_en_keys) - 50} more")

    if modified_scripts:
        print(f"\nNote: {len(modified_scripts)} shell/batch script(s) modified (cannot use Go/TS catalogs):")
        for s in sorted(modified_scripts):
            has_hits = " [has CJK literals above]" if s in untranslated else ""
            print(f"  {s}{has_hits}")

    if args.check_orphans:
        if orphans:
            print(f"\nPotentially orphaned catalog keys ({len(orphans)}):")
            for key, files in orphans:
                display = key.replace("\n", "\\n")
                if len(display) > 100:
                    display = display[:97] + "..."
                print(f"  {display}  (removed from {', '.join(files)})")
        else:
            print("\nPotentially orphaned catalog keys: none")

    if untranslated or missing_en_keys:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
