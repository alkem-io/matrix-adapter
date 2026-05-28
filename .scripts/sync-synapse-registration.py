#!/usr/bin/env python3
"""
Sync the canonical Synapse AppService registration shape into a downstream repo.

Source of truth:
  matrix-adapter-go/registration.yaml

Downstream layouts:

  file mode — server
    <target>/.build/synapse/matrix-adapter.yaml
      Overwritten with canonical content, except url / as_token / hs_token
      which are preserved from the current file (these are dev secrets,
      not part of the schema).

  printf-block mode — dev-orchestration, infrastructure-operations
    <target>/01-alkemio-platform/base/third-party/communication/synapse/01-synapse-setup-confmap.yml
    <target>/orchestration/base/third-party/communication/synapse/01-synapse-setup-confmap.yml
      The shell heredoc that emits /data/matrix-adapter.yaml at pod startup
      is regenerated from canonical, preserving the existing url /
      ${MATRIX_AS_TOKEN} / ${MATRIX_HS_TOKEN} lines.

Schema fields are taken from canonical; url / as_token / hs_token are taken
from the target. Comments and blank lines in canonical are dropped (the
output doesn't have a natural place for them).

Usage:
  sync-synapse-registration.py <target-repo-root>

Exit codes:
  0 — change applied OR no change needed
  2 — usage error / canonical missing
  3 — target layout not recognised
  4 — printf-block markers not found in target
"""

import re
import sys
from pathlib import Path

SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parent
CANONICAL = REPO_ROOT / "registration.yaml"

PRESERVED_FIELDS = ("url", "as_token", "hs_token")


def detect_target(root: Path):
    server = root / ".build" / "synapse" / "matrix-adapter.yaml"
    dev_orch = (
        root
        / "01-alkemio-platform/base/third-party/communication/synapse/01-synapse-setup-confmap.yml"
    )
    infra_ops = (
        root
        / "orchestration/base/third-party/communication/synapse/01-synapse-setup-confmap.yml"
    )
    if server.exists():
        return "file", server
    if dev_orch.exists():
        return "printf", dev_orch
    if infra_ops.exists():
        return "printf", infra_ops
    print(f"ERROR: {root} does not match any known downstream layout", file=sys.stderr)
    for p in (server, dev_orch, infra_ops):
        print(f"  expected one of: {p}", file=sys.stderr)
    sys.exit(3)


def get_field_value(yaml_text: str, field: str) -> str | None:
    """Return the verbatim VALUE portion of a top-level `field: VALUE` line."""
    pattern = re.compile(rf"^{re.escape(field)}:\s+(.+?)\s*$", re.MULTILINE)
    m = pattern.search(yaml_text)
    return m.group(1) if m else None


def strip_leading_comment_block(text: str) -> str:
    """Drop the contiguous comment/blank lines at the top of a YAML file.

    Comments inside the body are preserved — only the front-matter block,
    which is canonical-only metadata ("this file is auto-synced…"), is removed.
    """
    lines = text.splitlines(keepends=True)
    idx = 0
    while idx < len(lines):
        stripped = lines[idx].lstrip()
        if stripped.startswith("#") or stripped == "" or stripped == "\n":
            idx += 1
            continue
        break
    return "".join(lines[idx:])


def sync_file_mode(canonical_text: str, target: Path) -> bool:
    target_text = target.read_text()
    preserved = {f: get_field_value(target_text, f) for f in PRESERVED_FIELDS}
    new_text = strip_leading_comment_block(canonical_text)
    for field, value in preserved.items():
        if value is None:
            continue
        new_text = re.sub(
            rf"^{re.escape(field)}:.*$",
            f"{field}: {value}",
            new_text,
            count=1,
            flags=re.MULTILINE,
        )
    if new_text == target_text:
        print(f"no change: {target}")
        return False
    target.write_text(new_text)
    print(f"updated: {target}")
    return True


def shell_quote(line: str) -> str:
    """Wrap a YAML line in shell quoting for use as a printf argument.

    Three cases:
      - no `"` in the line          → double quotes (preserves `${VAR}` interp)
      - `"` but no `'`              → single quotes (verbatim)
      - both `"` and `'` in the line → single quotes with `'` → `'\\''` escape
        (canonical YAML rarely contains both; this branch only exists so a
        future canonical change doesn't hard-fail the workflow)
    """
    if '"' not in line:
        return f'"{line}"'
    if "'" not in line:
        return f"'{line}'"
    escaped = line.replace("'", "'\\''")
    return f"'{escaped}'"


def extract_printf_block(text: str) -> tuple[str, int, int, str]:
    """Return (block_indent, body_start, body_end, body_text).

    body_text spans from just after the `printf '%s\\n' \\` line through
    just before the `> /data/matrix-adapter.yaml` line.
    """
    start_match = re.search(
        r"^(\s+)printf\s+'%s\\n'\s*\\\s*$", text, re.MULTILINE
    )
    if not start_match:
        print("ERROR: printf '%s\\n' \\ start marker not found", file=sys.stderr)
        sys.exit(4)
    block_indent = start_match.group(1)
    body_start = start_match.end() + 1  # skip the newline after the marker

    end_marker = "> /data/matrix-adapter.yaml"
    end_idx = text.find(end_marker, body_start)
    if end_idx == -1:
        print(
            f"ERROR: '{end_marker}' end marker not found", file=sys.stderr
        )
        sys.exit(4)
    body_end = text.rfind("\n", 0, end_idx) + 1
    return block_indent, body_start, body_end, text[body_start:body_end]


def extract_preserved_lines(body: str) -> dict[str, str]:
    """Pull the existing url / as_token / hs_token VALUE portions out of the
    printf block so we can re-emit them unchanged.
    """
    preserved: dict[str, str] = {}
    for field in PRESERVED_FIELDS:
        # Each shell line looks like:   "field: VALUE" \   or   'field: VALUE' \
        pattern = re.compile(
            rf'^\s+["\']{re.escape(field)}:\s+(.+?)["\']\s*\\\s*$',
            re.MULTILINE,
        )
        m = pattern.search(body)
        if m:
            preserved[field] = m.group(1)
    return preserved


def sync_printf_mode(canonical_text: str, target: Path) -> bool:
    target_text = target.read_text()
    block_indent, body_start, body_end, existing_body = extract_printf_block(
        target_text
    )
    preserved = extract_preserved_lines(existing_body)

    string_indent = block_indent + "  "
    new_lines: list[str] = []
    for raw in canonical_text.splitlines():
        line = raw.rstrip()
        if not line or line.lstrip().startswith("#"):
            continue
        top_field = re.match(r"^([A-Za-z_][A-Za-z0-9_.-]*):", line)
        if top_field and top_field.group(1) in PRESERVED_FIELDS:
            field = top_field.group(1)
            if field in preserved:
                line = f"{field}: {preserved[field]}"
        new_lines.append(f"{string_indent}{shell_quote(line)} \\")

    new_body = "\n".join(new_lines) + "\n"
    new_text = target_text[:body_start] + new_body + target_text[body_end:]

    if new_text == target_text:
        print(f"no change: {target}")
        return False
    target.write_text(new_text)
    print(f"updated: {target}")
    return True


def main() -> int:
    if len(sys.argv) != 2:
        print(f"usage: {sys.argv[0]} <target-repo-root>", file=sys.stderr)
        return 2
    if not CANONICAL.exists():
        print(f"ERROR: canonical not found at {CANONICAL}", file=sys.stderr)
        return 2

    target_root = Path(sys.argv[1]).resolve()
    canonical_text = CANONICAL.read_text()
    mode, dest = detect_target(target_root)
    if mode == "file":
        sync_file_mode(canonical_text, dest)
    else:
        sync_printf_mode(canonical_text, dest)
    return 0


if __name__ == "__main__":
    sys.exit(main())
