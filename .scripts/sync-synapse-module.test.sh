#!/usr/bin/env bash
# Fixture tests for .scripts/sync-synapse-module.sh (embed mode).
#
# The embed path rewrites a YAML literal block inside a ConfigMap in place. That
# is the kind of text surgery that silently corrodes a file one sync at a time,
# so the structural promises it makes are asserted here against real fixtures:
#
#   - the blank separator line before the NEXT data key survives;
#   - YAML comments between the block and the next key survive;
#   - a second run is a no-op (idempotent) — the real proof nothing is eaten;
#   - a block that is the LAST key still works;
#   - a missing marker fails loudly (exit 4), and ALLOW_MISSING_FILESERVICE_PROVIDER=1
#     is the documented, working opt-out.
#
# Run: make test-scripts

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SYNC="$SCRIPT_DIR/sync-synapse-module.sh"
CONFMAP_REL="01-alkemio-platform/base/third-party/communication/synapse/01-synapse-setup-confmap.yml"

failures=0
pass() { echo "  ok   — $1"; }
fail() { echo "  FAIL — $1" >&2; failures=$((failures + 1)); }

check() { # check <description> <expected> <actual>
  if [[ "$2" == "$3" ]]; then pass "$1"; else
    fail "$1"
    echo "        expected: $2" >&2
    echo "        actual:   $3" >&2
  fi
}

make_target() { # make_target <confmap-body-file> -> echoes the target root
  local body="$1" root
  root="$(mktemp -d)"
  mkdir -p "$root/$(dirname "$CONFMAP_REL")"
  cp "$body" "$root/$CONFMAP_REL"
  echo "$root"
}

# --- fixture: both module blocks, blank separator, trailing comment ----------

FIXTURE_BOTH="$(mktemp)"
cat > "$FIXTURE_BOTH" <<'YAML'
apiVersion: v1
kind: ConfigMap
metadata:
  name: synapse-setup
data:
  alkemio_room_control.py: |
    # placeholder room control

    print("stale")

  alkemio_fileservice_provider.py: |
    # placeholder provider
    print("stale")

  # A comment between the last module block and the next key.
  homeserver.yaml: |
    server_name: alkemio.test
YAML

echo "sync-synapse-module.sh — embed mode"

# --- 1. the blank separator before the next data key survives ---------------

TARGET="$(make_target "$FIXTURE_BOTH")"
"$SYNC" "$TARGET" > /dev/null
OUT="$TARGET/$CONFMAP_REL"

# The line immediately before each following key must still be blank.
prev_line_of() { # prev_line_of <key>
  local n
  n="$(grep -n "^  $1:" "$OUT" | head -1 | cut -d: -f1)"
  [[ -n "$n" && "$n" -gt 1 ]] && sed -n "$((n - 1))p" "$OUT"
}
check "blank separator before the next module key survives" "" "$(prev_line_of 'alkemio_fileservice_provider.py')"
check "comment between the last block and the next key survives" \
  "  # A comment between the last module block and the next key." \
  "$(grep -c '^  # A comment between the last module block' "$OUT" > /dev/null && grep '^  # A comment between' "$OUT")"
check "the following data key is still present" "1" "$(grep -c '^  homeserver.yaml: |$' "$OUT")"
check "canonical content replaced the placeholder" "0" "$(grep -c 'print("stale")' "$OUT")"
check "provider content landed, indented 4 spaces" "1" \
  "$(grep -c '^    """$' "$OUT" > /dev/null && echo 1 || echo 0)"

if command -v python3 > /dev/null; then
  if python3 -c "import sys,yaml;yaml.safe_load(open(sys.argv[1]))" "$OUT" 2> /dev/null; then
    pass "output still parses as YAML"
  else
    # PyYAML may be absent; only fail when it is present and rejects the file.
    if python3 -c "import yaml" 2> /dev/null; then
      fail "output still parses as YAML"
    else
      echo "  skip — PyYAML not installed"
    fi
  fi
fi

# --- 2. a second run is a no-op (nothing is eaten a line at a time) ---------

BEFORE="$(mktemp)"
cp "$OUT" "$BEFORE"
"$SYNC" "$TARGET" > /dev/null
if cmp -s "$BEFORE" "$OUT"; then
  pass "second run is idempotent (no line is consumed per sync)"
else
  fail "second run changed the file"
  diff -u "$BEFORE" "$OUT" | head -20 >&2
fi
rm -rf "$TARGET" "$BEFORE"

# --- 3. a block that is the LAST key in the ConfigMap -----------------------

FIXTURE_LAST="$(mktemp)"
cat > "$FIXTURE_LAST" <<'YAML'
apiVersion: v1
kind: ConfigMap
data:
  alkemio_room_control.py: |
    print("stale")

  alkemio_fileservice_provider.py: |
    print("stale")
YAML
TARGET="$(make_target "$FIXTURE_LAST")"
"$SYNC" "$TARGET" > /dev/null
OUT="$TARGET/$CONFMAP_REL"
check "trailing block: placeholder replaced" "0" "$(grep -c 'print("stale")' "$OUT")"
BEFORE="$(mktemp)"
cp "$OUT" "$BEFORE"
"$SYNC" "$TARGET" > /dev/null
if cmp -s "$BEFORE" "$OUT"; then
  pass "trailing block: second run is idempotent"
else
  fail "trailing block: second run changed the file"
fi
rm -rf "$TARGET" "$BEFORE"

# --- 4. a missing provider marker fails loudly, and the opt-out works -------

FIXTURE_NO_PROVIDER="$(mktemp)"
cat > "$FIXTURE_NO_PROVIDER" <<'YAML'
apiVersion: v1
kind: ConfigMap
data:
  alkemio_room_control.py: |
    print("stale")

  homeserver.yaml: |
    server_name: alkemio.test
YAML

TARGET="$(make_target "$FIXTURE_NO_PROVIDER")"
"$SYNC" "$TARGET" > /dev/null 2>&1
check "missing provider marker exits 4" "4" "$?"
rm -rf "$TARGET"

TARGET="$(make_target "$FIXTURE_NO_PROVIDER")"
ALLOW_MISSING_FILESERVICE_PROVIDER=1 "$SYNC" "$TARGET" > /dev/null 2>&1
check "ALLOW_MISSING_FILESERVICE_PROVIDER=1 downgrades it to a warning" "0" "$?"
rm -rf "$TARGET"

rm -f "$FIXTURE_BOTH" "$FIXTURE_LAST" "$FIXTURE_NO_PROVIDER"

if [[ "$failures" -gt 0 ]]; then
  echo "$failures check(s) failed" >&2
  exit 1
fi
echo "all checks passed"
