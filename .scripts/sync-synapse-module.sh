#!/usr/bin/env bash
# Sync the canonical Synapse AlkemioRoomControl module into a downstream repo.
#
# Source of truth:
#   matrix-adapter/synapse-modules/alkemio_room_control.py
#
# Downstream layouts handled:
#
#   file mode   — server
#     <repo>/.build/synapse/modules/alkemio_room_control.py
#       → overwritten verbatim with the canonical file.
#
#   embed mode  — dev-orchestration, infrastructure-operations
#     <repo>/01-alkemio-platform/base/third-party/communication/synapse/01-synapse-setup-confmap.yml
#     <repo>/orchestration/base/third-party/communication/synapse/01-synapse-setup-confmap.yml
#       → the YAML literal block under the `alkemio_room_control.py: |`
#         key in `data:` is replaced with the canonical content, indented
#         by 4 spaces (so it nests under `data:` properly). The literal
#         block is assumed to be the LAST data key in the ConfigMap; the
#         script aborts if another data key appears after it.
#
# Usage:
#   sync-synapse-module.sh <target-repo-root>
#
# Exit codes:
#   0  — change applied OR no change needed (idempotent)
#   2  — canonical source missing
#   3  — target layout not recognised
#   4  — embed-mode marker not found
#   5  — another data key follows the module block (script needs updating)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CANONICAL="$REPO_ROOT/synapse-modules/alkemio_room_control.py"

if [[ ! -f "$CANONICAL" ]]; then
  echo "ERROR: canonical source not found at $CANONICAL" >&2
  exit 2
fi

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <target-repo-root>" >&2
  exit 2
fi

TARGET="$(cd "$1" && pwd)"

DEV_ORCH_CONFMAP="$TARGET/01-alkemio-platform/base/third-party/communication/synapse/01-synapse-setup-confmap.yml"
INFRA_OPS_CONFMAP="$TARGET/orchestration/base/third-party/communication/synapse/01-synapse-setup-confmap.yml"
SERVER_MODULE="$TARGET/.build/synapse/modules/alkemio_room_control.py"

if [[ -f "$SERVER_MODULE" ]]; then
  MODE=file
  DEST="$SERVER_MODULE"
elif [[ -f "$DEV_ORCH_CONFMAP" ]]; then
  MODE=embed
  DEST="$DEV_ORCH_CONFMAP"
elif [[ -f "$INFRA_OPS_CONFMAP" ]]; then
  MODE=embed
  DEST="$INFRA_OPS_CONFMAP"
else
  echo "ERROR: $TARGET does not match any known downstream layout" >&2
  echo "  expected one of:" >&2
  echo "    $SERVER_MODULE" >&2
  echo "    $DEV_ORCH_CONFMAP" >&2
  echo "    $INFRA_OPS_CONFMAP" >&2
  exit 3
fi

if [[ "$MODE" == file ]]; then
  if cmp -s "$CANONICAL" "$DEST"; then
    echo "no change: $DEST"
    exit 0
  fi
  cp "$CANONICAL" "$DEST"
  echo "updated: $DEST"
  exit 0
fi

# embed mode
KEY_LINE="$(grep -n '^  alkemio_room_control.py: |$' "$DEST" | head -1 | cut -d: -f1 || true)"
if [[ -z "${KEY_LINE:-}" ]]; then
  echo "ERROR: marker '  alkemio_room_control.py: |' not found in $DEST" >&2
  exit 4
fi

NEXT_KEY="$(awk -v start="$KEY_LINE" 'NR > start && /^  [A-Za-z_][A-Za-z0-9_.-]*:/ { print NR; exit }' "$DEST" || true)"
if [[ -n "${NEXT_KEY:-}" ]]; then
  echo "ERROR: another ConfigMap data key appears at line $NEXT_KEY (after the module block)." >&2
  echo "       This script assumes alkemio_room_control.py is the last key in data; please update it." >&2
  exit 5
fi

TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT

head -n "$KEY_LINE" "$DEST" > "$TMP"
# Indent by 4 spaces; on otherwise-blank lines, leave them truly blank
# (matches the existing infra-ops style and produces deterministic output).
sed -e 's/^/    /' -e 's/^    $//' "$CANONICAL" >> "$TMP"

if cmp -s "$TMP" "$DEST"; then
  echo "no change: $DEST"
  exit 0
fi

mv "$TMP" "$DEST"
trap - EXIT
echo "updated: $DEST"
