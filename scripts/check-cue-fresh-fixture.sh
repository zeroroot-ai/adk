#!/usr/bin/env bash
# check-cue-fresh-fixture.sh — the failing fixture of check-cue-fresh.sh.
#
# It runs the gate against scripts/testdata/stale_mission_definition.cue in
# place of the committed embedded CUE. The fixture is a copy of the embedded
# CUE without MissionDefinition.workspace, so it differs from the mission
# proto of the pinned SDK. The gate must:
#   - exit with status 1 (STALE, not a script bug),
#   - print "STALE: run 'make generate' to refresh embedded CUE",
#   - print a diff that names the dropped `workspace` field,
#   - name the pinned SDK version that it compared against.
#
# A gate that passes this fixture cannot detect drift (adk#27, adk#206).

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ADK_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
FIXTURE="$SCRIPT_DIR/testdata/stale_mission_definition.cue"
SDK_VERSION="$(cd "$ADK_ROOT/gibson" && go list -m -f '{{.Version}}' github.com/zeroroot-ai/sdk)"

set +e
OUT="$(ADK_CUE_FIXTURE="$FIXTURE" "$SCRIPT_DIR/check-cue-fresh.sh" 2>&1)"
CODE=$?
set -e

fail() {
  echo "[$SCRIPT_NAME] FAIL — $*" >&2
  echo "--- output of check-cue-fresh.sh:" >&2
  echo "$OUT" >&2
  exit 1
}

[[ $CODE -eq 1 ]] || fail "check-cue-fresh.sh exited $CODE on the stale fixture, want 1"
for want in "STALE: run 'make generate' to refresh embedded CUE" \
  "workspace?: #WorkspaceConfig" \
  "github.com/zeroroot-ai/sdk@$SDK_VERSION"; do
  grep -qF -- "$want" <<<"$OUT" || fail "the output does not contain: $want"
done

echo "[$SCRIPT_NAME] OK — the gate fails on the stale fixture against sdk $SDK_VERSION"
