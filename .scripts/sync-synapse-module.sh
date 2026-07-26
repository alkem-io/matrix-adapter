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
#   alkemio_fileservice_provider  BEST-EFFORT — in embed mode it is skipped (not
#                                 an error) when its `alkemio_fileservice_provider.py: |`
#                                 marker is absent, so syncing an unrelated
#                                 room-control change never fails against a
#                                 target that has not yet adopted the provider
#                                 block. In file mode the provider file is
#                                 created/overwritten alongside room-control.
#
# Usage:
#   sync-synapse-module.sh <target-repo-root>
#
# Exit codes:
#   0  — change(s) applied OR no change needed (idempotent)
#   2  — canonical source missing / bad usage
#   3  — target layout not recognised
#   4  — embed-mode marker not found for a REQUIRED module (room-control)

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
#   $4 required=1 → abort (exit 4) if the marker is missing
#      required=0 → skip quietly if the marker is missing
sync_embed() {
  local dest="$1" canonical="$2" key="$3" required="$4"

  local key_line
  key_line="$(grep -n "^  ${key}: |\$" "$dest" | head -1 | cut -d: -f1 || true)"
  if [[ -z "${key_line:-}" ]]; then
    if [[ "$required" == 1 ]]; then
      echo "ERROR: marker '  ${key}: |' not found in $dest" >&2
      exit 4
    fi
    echo "skip: marker '  ${key}: |' not present in $dest (optional module)"
    return 0
  fi

  # The literal block scalar body is any blank line OR any line indented deeper than
  # the 2-space key (i.e. >=3 spaces — YAML requires block-scalar content indented
  # more than its key, whatever the exact body indent). It ENDS at the first non-blank
  # line NOT so indented (a 2-space `  nextkey:` or `  #comment`, a `---`, or EOF).
  # Resume the tail there — NOT at the next data key — so YAML comments and blank lines
  # BETWEEN this block's content and the next key are preserved (the `^  [A-Za-z_]...:`
  # next-key scan skips them, which would otherwise silently swallow them each sync).
  # Empty means the block runs to EOF (it is the last key in the ConfigMap).
  local block_end
  block_end="$(awk -v start="$key_line" 'NR > start && $0 != "" && !/^   / { print NR; exit }' "$dest" || true)"

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
# room-control is required; the fileservice provider is synced only where its
# block already exists (skip-if-absent keeps room-control-only syncs green).
sync_embed "$DEST" "$CANONICAL_ROOM" "alkemio_room_control.py" 1
sync_embed "$DEST" "$CANONICAL_FILESERVICE" "alkemio_fileservice_provider.py" 0
