#!/usr/bin/env bash
# Sync the canonical Synapse Python modules into a downstream repo.
#
# Source of truth (canonical):
#   matrix-adapter/synapse-modules/alkemio_room_control.py
#   matrix-adapter/synapse-modules/alkemio_fileservice_provider.py
#
# Downstream layouts handled:
#
#   file mode   — server
#     <repo>/.build/synapse/modules/alkemio_room_control.py
#     <repo>/.build/synapse/modules/alkemio_fileservice_provider.py
#       → each overwritten verbatim with its canonical file (the provider file
#         is created if the target does not have it yet).
#
#   embed mode  — dev-orchestration, infrastructure-operations
#     <repo>/01-alkemio-platform/base/third-party/communication/synapse/01-synapse-setup-confmap.yml
#     <repo>/orchestration/base/third-party/communication/synapse/01-synapse-setup-confmap.yml
#       → the YAML literal block under each `<module>.py: |` key in `data:` is
#         replaced with the canonical content, indented by 4 spaces (so it nests
#         under `data:` properly). Each block is bounded by the NEXT data key
#         (or EOF if it is the last one), so multiple module blocks can coexist
#         in one ConfigMap in any order.
#
# Module requirements:
#   alkemio_room_control.py       REQUIRED in every target — it predates this
#                                 script and must be present.
#   alkemio_fileservice_provider  REQUIRED in every target. In embed mode a
#                                 missing `alkemio_fileservice_provider.py: |`
#                                 marker is a HARD FAILURE, not a quiet skip: the
#                                 provider is the media byte bridge, so a target
#                                 that silently stops receiving it drifts from
#                                 canonical while the sync job stays green. In
#                                 file mode the provider file is
#                                 created/overwritten alongside room-control.
#
#                                 A target that has genuinely not adopted the
#                                 provider block yet must opt out EXPLICITLY:
#                                 set ALLOW_MISSING_FILESERVICE_PROVIDER=1. That
#                                 downgrades the failure to a loud warning
#                                 (::warning:: on GitHub Actions) so the choice
#                                 is visible and deliberate, never accidental.
#
# Usage:
#   sync-synapse-module.sh <target-repo-root>
#
# Environment:
#   ALLOW_MISSING_FILESERVICE_PROVIDER=1  Explicit opt-out (see above).
#
# Exit codes:
#   0  — change(s) applied OR no change needed (idempotent)
#   2  — canonical source missing / bad usage
#   3  — target layout not recognised
#   4  — embed-mode marker not found for a required module

set -euo pipefail

# Script-level scratch dir with a single EXIT trap: every sync_embed temp lives
# inside it, so a failure in head/sed/tail/cmp under `set -e` (which exits the
# shell and would skip a per-call RETURN trap) never leaks a temp file.
_WORKDIR="$(mktemp -d)"
trap 'rm -rf "$_WORKDIR"' EXIT

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CANONICAL_ROOM="$REPO_ROOT/synapse-modules/alkemio_room_control.py"
CANONICAL_FILESERVICE="$REPO_ROOT/synapse-modules/alkemio_fileservice_provider.py"

for canonical in "$CANONICAL_ROOM" "$CANONICAL_FILESERVICE"; do
  if [[ ! -f "$canonical" ]]; then
    echo "ERROR: canonical source not found at $canonical" >&2
    exit 2
  fi
done

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <target-repo-root>" >&2
  exit 2
fi

TARGET="$(cd "$1" && pwd)"

DEV_ORCH_CONFMAP="$TARGET/01-alkemio-platform/base/third-party/communication/synapse/01-synapse-setup-confmap.yml"
INFRA_OPS_CONFMAP="$TARGET/orchestration/base/third-party/communication/synapse/01-synapse-setup-confmap.yml"
SERVER_MODULE_DIR="$TARGET/.build/synapse/modules"
SERVER_MODULE="$SERVER_MODULE_DIR/alkemio_room_control.py"

# --- helpers ---------------------------------------------------------------

# file mode: overwrite a downstream file verbatim with a canonical source.
sync_file() {
  local canonical="$1" dest="$2"
  if [[ -f "$dest" ]] && cmp -s "$canonical" "$dest"; then
    echo "no change: $dest"
    return 0
  fi
  cp "$canonical" "$dest"
  echo "updated: $dest"
}

# embed mode: replace the YAML literal block under `  <key>: |` with the
# canonical content, indented 4 spaces, bounded by the next data key (or EOF).
# A missing marker ALWAYS aborts (exit 4) — a silent skip lets a downstream stop
# receiving a canonical module while the sync job reports success. The only way
# past it is the explicit, logged opt-out in sync_embed_optional below.
sync_embed() {
  local dest="$1" canonical="$2" key="$3"

  local key_line
  key_line="$(grep -n "^  ${key}: |\$" "$dest" | head -1 | cut -d: -f1 || true)"
  if [[ -z "${key_line:-}" ]]; then
    echo "::error::marker '  ${key}: |' not found in $dest" >&2
    echo "ERROR: marker '  ${key}: |' not found in $dest" >&2
    echo "  The canonical module cannot be synced into this target. Add the" >&2
    echo "  '  ${key}: |' key to the ConfigMap's data: block, or (only if the" >&2
    echo "  target deliberately does not run this module) re-run with" >&2
    echo "  ALLOW_MISSING_FILESERVICE_PROVIDER=1." >&2
    exit 4
  fi

  # The literal block scalar body is any blank line OR any line indented deeper than
  # the 2-space key (i.e. >=3 spaces — YAML requires block-scalar content indented
  # more than its key, whatever the exact body indent). It ENDS at the first non-blank
  # line NOT so indented (a 2-space `  nextkey:` or `  #comment`, a `---`, or EOF).
  # Resume the tail there — NOT at the next data key — so YAML comments and blank lines
  # BETWEEN this block's content and the next key are preserved (the `^  [A-Za-z_]...:`
  # next-key scan skips them, which would otherwise silently swallow them each sync).
  # Empty means the block runs to EOF (it is the last key in the ConfigMap).
  #
  # Blank lines are ambiguous: one INSIDE the Python source is block content, but the
  # separator blank line(s) immediately BEFORE the next key belong to the file, not to
  # the block. Treating every blank as content (the naive `$0 != "" && !/^   /`) ends
  # the block AT the next key and so eats that separator on every sync — the exact
  # swallowing the comment above promises not to do. So: remember where the current
  # run of blank lines started, and when a non-blank, non-indented line finally ends
  # the block, resume from the START of that trailing run instead. A blank run that
  # is followed by more indented content is not trailing, so the marker is cleared.
  local block_end
  block_end="$(awk -v start="$key_line" '
    NR <= start        { next }
    $0 == ""           { if (!blank_run) blank_run = NR; next }
    /^   /             { blank_run = 0; next }
                       { print (blank_run ? blank_run : NR); found = 1; exit }
    END                { if (!found && blank_run) print blank_run }
  ' "$dest" || true)"

  local tmp
  tmp="$(mktemp -p "$_WORKDIR")"
  head -n "$key_line" "$dest" > "$tmp"
  # Indent by 4 spaces; on otherwise-blank lines, leave them truly blank
  # (matches the existing infra-ops style and produces deterministic output).
  sed -e 's/^/    /' -e 's/^    $//' "$canonical" >> "$tmp"
  if [[ -n "${block_end:-}" ]]; then
    tail -n +"$block_end" "$dest" >> "$tmp"
  fi

  if cmp -s "$tmp" "$dest"; then
    rm -f "$tmp"
    echo "no change: $dest ($key)"
  else
    mv "$tmp" "$dest"
    echo "updated: $dest ($key)"
  fi
}

# --- dispatch --------------------------------------------------------------

if [[ -f "$SERVER_MODULE" ]]; then
  # file mode (server): both modules are plain files, overwritten verbatim.
  sync_file "$CANONICAL_ROOM" "$SERVER_MODULE"
  sync_file "$CANONICAL_FILESERVICE" "$SERVER_MODULE_DIR/alkemio_fileservice_provider.py"
  exit 0
fi

if [[ -f "$DEV_ORCH_CONFMAP" ]]; then
  DEST="$DEV_ORCH_CONFMAP"
elif [[ -f "$INFRA_OPS_CONFMAP" ]]; then
  DEST="$INFRA_OPS_CONFMAP"
else
  echo "ERROR: $TARGET does not match any known downstream layout" >&2
  echo "  expected one of:" >&2
  echo "    $SERVER_MODULE" >&2
  echo "    $DEV_ORCH_CONFMAP" >&2
  echo "    $INFRA_OPS_CONFMAP" >&2
  exit 3
fi

# embed mode (dev-orchestration / infrastructure-operations).
# Both modules are required. A missing marker fails the sync loudly so drift
# cannot go unnoticed; the provider has an explicit, logged opt-out for a target
# that genuinely does not run it yet.
sync_embed "$DEST" "$CANONICAL_ROOM" "alkemio_room_control.py"

if [[ "${ALLOW_MISSING_FILESERVICE_PROVIDER:-0}" == 1 ]] &&
  ! grep -q "^  alkemio_fileservice_provider.py: |\$" "$DEST"; then
  # Explicit opt-out ONLY. Loud on purpose: this target is knowingly running
  # without the canonical media storage provider.
  echo "::warning::alkemio_fileservice_provider.py block absent in $DEST and" \
    "ALLOW_MISSING_FILESERVICE_PROVIDER=1 — skipping the media storage provider sync." >&2
  echo "WARNING: skipping alkemio_fileservice_provider.py (explicit opt-out); $DEST" \
    "will NOT receive canonical media-provider changes." >&2
else
  sync_embed "$DEST" "$CANONICAL_FILESERVICE" "alkemio_fileservice_provider.py"
fi
