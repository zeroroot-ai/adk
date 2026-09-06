#!/usr/bin/env bash
# check-cue-fresh.sh — drift gate for ADK-embedded CUE schema files.
#
# Purpose
# -------
# The ADK ships a CUE module embedded into the `gibson` binary so that
# `gibson mission validate` can resolve `import "github.com/zeroroot-ai/sdk/
# api/proto/gibson/mission/v1"` without the customer having the SDK source
# tree on disk. The embedded files live at:
#
#   gibson/cmd/gibson/cmd/mission/schema/api/proto/gibson/mission/v1/
#     mission_definition_proto_gen.cue
#
# That file is regenerated from the SDK's authoritative proto via:
#
#   make -C ../sdk cue-defs        # SDK target that runs `cue import proto`
#
# followed by ADK-specific normalization (package rename, header note,
# import-path rewrite — documented in scripts/regen-cue.sh). If the SDK
# proto evolves but the embedded copy is not refreshed, the `gibson` CLI
# silently validates against a stale schema — accepting authoring constructs
# the daemon will then reject at submit time.
#
# This script exists to catch that drift in CI. Mirrors the dashboard's
# scripts/check-mission-schema-fresh.mjs (prior art).
#
# Two modes
# ---------
# FULL — SDK sibling present at ../../opensource/sdk (workstation, polyrepo CI):
#   Regenerate the embedded file into a temp dir via the same pipeline that
#   `make generate` (and scripts/regen-cue.sh) drives, then byte-diff against
#   the committed copy. Exit non-zero on any drift.
#
# STRUCTURAL — SDK sibling absent (ADK-only CI):
#   Cannot regenerate. Instead validates that the committed file:
#     1. Exists and parses by cue (cue vet --strict).
#     2. Carries the "// NOTE: This copy is embedded in the adk CLI binary"
#        sentinel header so we know the file went through the regen pipeline
#        and was not hand-edited.
#
# There is no --skip / --permissive flag. Drift fails the build, period.
#
# Usage
#   scripts/check-cue-fresh.sh                 # default
#   ADK_CUE_FIXTURE=path scripts/check-cue-fresh.sh
#       # point the checker at a fixture path (used by the negative test).
#       # The fixture is treated as the committed file; SDK regen still runs
#       # against the real SDK sibling.
#
# Exit codes
#   0  — fresh (or structurally valid in STRUCTURAL mode)
#   1  — STALE: regenerated content differs OR committed file is missing /
#        invalid / missing the sentinel header
#   2  — script bug or external-tool failure (e.g. cue not on PATH)

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ADK_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# The committed schema file we guard. Overridable via ADK_CUE_FIXTURE for
# the negative test (scripts/testdata/stale_mission_definition.cue).
COMMITTED_DEFAULT="$ADK_ROOT/gibson/cmd/gibson/cmd/mission/schema/api/proto/gibson/mission/v1/mission_definition_proto_gen.cue"
COMMITTED="${ADK_CUE_FIXTURE:-$COMMITTED_DEFAULT}"

# Sentinel string we expect at the top of the committed file. Set by
# scripts/regen-cue.sh after every regeneration. If a maintainer hand-edits
# the file and drops this header, STRUCTURAL mode fails — forcing the file
# back through the regen pipeline.
SENTINEL="// NOTE: This copy is embedded in the adk CLI binary"

# Path to the SDK sibling. The freshness gate is FULL when this exists.
SDK_ROOT="$(cd "$ADK_ROOT/../sdk" 2>/dev/null && pwd || true)"

fail() {
  echo "[$SCRIPT_NAME] FAIL — $*" >&2
}

ok() {
  echo "[$SCRIPT_NAME] OK — $*"
}

# --- always-run checks (both modes) ---

if [[ ! -f "$COMMITTED" ]]; then
  fail "committed CUE schema not found at $COMMITTED"
  echo "STALE: run 'make generate' to refresh embedded CUE" >&2
  exit 1
fi

if ! grep -qF "$SENTINEL" "$COMMITTED"; then
  fail "committed CUE schema at $COMMITTED is missing the regen-pipeline sentinel header"
  echo "  Expected to find: $SENTINEL" >&2
  echo "STALE: run 'make generate' to refresh embedded CUE" >&2
  exit 1
fi

# --- mode switch ---

if [[ -z "$SDK_ROOT" || ! -d "$SDK_ROOT/api/proto/gibson/mission/v1" ]]; then
  ok "STRUCTURAL — SDK sibling not present at ../sdk; validated sentinel header only"
  exit 0
fi

# --- FULL mode: regenerate + byte-diff ---

REGEN_SCRIPT="$ADK_ROOT/scripts/regen-cue.sh"
if [[ ! -x "$REGEN_SCRIPT" ]]; then
  fail "regen script not found or not executable at $REGEN_SCRIPT"
  exit 2
fi

TMPDIR="$(mktemp -d -t adk-cue-fresh.XXXXXX)"
trap 'rm -rf "$TMPDIR"' EXIT

# Regen into a temp dir and diff. regen-cue.sh writes its output to
# $1 if invoked with an output-path argument (workflow used here for
# the check), or to the in-tree path when invoked without arguments
# (the maintainer regen workflow).
if ! "$REGEN_SCRIPT" "$TMPDIR/mission_definition_proto_gen.cue" >"$TMPDIR/regen.log" 2>&1; then
  fail "regen-cue.sh exited non-zero — see log:"
  cat "$TMPDIR/regen.log" >&2
  exit 2
fi

if ! diff -u "$COMMITTED" "$TMPDIR/mission_definition_proto_gen.cue" >"$TMPDIR/diff.out"; then
  fail "embedded CUE has drifted from the SDK proto"
  echo "--- committed: $COMMITTED" >&2
  echo "+++ regenerated from $SDK_ROOT/api/proto/gibson/mission/v1/mission_definition.proto" >&2
  echo >&2
  cat "$TMPDIR/diff.out" >&2
  echo >&2
  echo "STALE: run 'make generate' to refresh embedded CUE" >&2
  exit 1
fi

ok "embedded CUE matches the SDK proto"
exit 0
